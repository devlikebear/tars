package checkpoint

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/atomicwrite"
)

// ErrBusy reports a revert while one of the session's turns is running: its
// end snapshot would record the revert as the agent's own edit.
var ErrBusy = errors.New("checkpoint: a turn is in progress")

// ErrConflict reports an apply refused because files changed after the turn
// in ways the revert cannot merge. The result lists them; Force overwrites.
var ErrConflict = errors.New("checkpoint: the revert conflicts with later edits")

// RevertFile names a file to revert, and optionally some of its hunks (IDs
// from the turn's diff). No hunks means the whole file.
type RevertFile struct {
	Path    string   `json:"path"`
	HunkIDs []string `json:"hunk_ids,omitempty"`
}

// RevertRequest asks to undo what a turn changed. Scope is ScopeTurn (the
// default: the turn's own edits, merged into whatever came after) or
// ScopeSince (every file back to how it was before the turn). No Files means
// every file in scope. Without Apply nothing is written: the result is a
// preview. Force writes the reverted content over conflicts.
type RevertRequest struct {
	Scope Scope        `json:"scope,omitempty"`
	Files []RevertFile `json:"files,omitempty"`
	Apply bool         `json:"apply,omitempty"`
	Force bool         `json:"force,omitempty"`
}

// Per-file outcomes of a revert or undo.
const (
	// RevertWrite: the file was as the turn left it; the target is written.
	RevertWrite = "write"
	// RevertMerge: the file changed after the turn; the revert merges in.
	RevertMerge = "merge"
	// RevertUnchanged: the file already has the target content.
	RevertUnchanged = "unchanged"
	// RevertConflict: later edits overlap the revert.
	RevertConflict = "conflict"
	// RevertFailed: the file cannot be reverted (not a regular file, not
	// recorded, or the write failed).
	RevertFailed = "failed"
)

// RevertFileResult says what happens, or happened, to one file.
type RevertFileResult struct {
	Path    string `json:"path"`
	Outcome string `json:"outcome"`
	// Delete: the target is no file at all.
	Delete bool `json:"delete,omitempty"`
	// Forced: a conflict overwritten with the target.
	Forced bool `json:"forced,omitempty"`
	// Merged is a conflicting merge with its markers, for review.
	Merged string `json:"merged,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// RevertResult reports a revert or undo. RevertID is set once one applied.
type RevertResult struct {
	RevertID  string             `json:"revert_id,omitempty"`
	TurnID    string             `json:"turn_id"`
	Scope     Scope              `json:"scope"`
	Applied   bool               `json:"applied"`
	Conflicts int                `json:"conflicts"`
	Failed    int                `json:"failed"`
	Files     []RevertFileResult `json:"files"`
}

// RevertEntry records an applied revert so it can be shown and undone. Pre
// and Post are snapshots of the root around it.
type RevertEntry struct {
	ID     string    `json:"id"`
	TurnID string    `json:"turn_id"`
	Scope  Scope     `json:"scope"`
	Shadow string    `json:"shadow"`
	Pre    string    `json:"pre"`
	Post   string    `json:"post"`
	At     time.Time `json:"at"`
	// Targets is what was asked for, per file; Files what was written.
	Targets  []RevertFile `json:"targets"`
	Files    []string     `json:"files"`
	UndoneAt time.Time    `json:"undone_at,omitzero"`
}

// keepReverts caps a session's revert records; older ones lose their undo.
var keepReverts = 20

// fileState is a file's content, or its absence.
type fileState struct {
	ok   bool
	data []byte
}

func (f fileState) equal(o fileState) bool {
	return f.ok == o.ok && bytes.Equal(f.data, o.data)
}

// move takes one file from base (how the revert expects to find it) to
// target. Turn reverts, since reverts, and undo all reduce to moves.
type move struct {
	path   string
	base   fileState
	target fileState
	// unsupported names why the file cannot be moved at all.
	unsupported string
}

type plannedFile struct {
	move
	result RevertFileResult
	write  fileState
	apply  bool
}

// Labels for the three sides of a conflict's markers.
var revertLabels = [3]string{"current", "after the turn", "reverted"}
var undoLabels = [3]string{"current", "after the revert", "before the revert"}

// Revert undoes what a turn changed, or previews it. The user's repository
// metadata is never touched: files are written in the work tree only.
func (s *Store) Revert(ctx context.Context, sessionID, turnID string, req RevertRequest) (RevertResult, error) {
	if err := validID("session", sessionID); err != nil {
		return RevertResult{}, err
	}
	scope := req.Scope
	if scope == "" {
		scope = ScopeTurn
	}
	if scope != ScopeTurn && scope != ScopeSince {
		return RevertResult{}, fmt.Errorf("%w: a revert's scope is turn or since, not %q", ErrInvalid, scope)
	}
	entry, err := s.turnEntry(sessionID, turnID)
	if err != nil {
		return RevertResult{}, err
	}
	sh, err := s.openShadow(entry.Shadow)
	if err != nil {
		return RevertResult{}, err
	}
	result := RevertResult{TurnID: turnID, Scope: scope}

	unlock := s.lockRoot(sh.key)
	defer func() { unlock() }()
	if req.Apply && s.isActive(sessionID) {
		return result, ErrBusy
	}
	// A snapshot of now: the base of a since revert, and the undo point of
	// an applied one.
	var pre snapshot
	if req.Apply || scope == ScopeSince {
		if pre, err = s.takeSnapshot(ctx, sh, snapshotMessage(sessionID, turnID, "revert")); err != nil {
			return result, err
		}
	}
	var moves []move
	var targets []RevertFile
	if scope == ScopeSince {
		moves, targets, err = s.sinceMoves(ctx, sh, entry, pre, req.Files)
	} else {
		moves, targets, err = s.turnMoves(ctx, sh, entry, req.Files)
	}
	if err != nil {
		return result, err
	}
	planned, err := s.planMoves(ctx, sh, moves, req.Force, revertLabels)
	if err != nil {
		return result, err
	}
	if req.Apply {
		refuseUnrecorded(planned, pre.unknown)
	}
	result.Files, result.Conflicts, result.Failed = summarize(planned)
	if !req.Apply {
		return result, nil
	}
	if result.Conflicts > 0 && !req.Force {
		return result, ErrConflict
	}
	rec := RevertEntry{
		ID: newRevertID(s.now()), TurnID: turnID, Scope: scope,
		Shadow: sh.key, Pre: pre.commit, At: s.now().UTC(), Targets: targets,
	}
	// Pin the undo point before anything is written: a snapshot no ref
	// holds is gone at the next sweep's gc.
	preRef := revertRef(sessionID, rec.ID, "pre")
	if err := s.updateRef(ctx, sh, preRef, rec.Pre); err != nil {
		return result, err
	}
	written := s.applyMoves(sh.root, planned)
	result.Files, result.Conflicts, result.Failed = summarize(planned)
	if len(written) == 0 {
		_ = s.deleteRefs(ctx, sh, []string{preRef})
		return result, nil
	}
	result.Applied = true
	rec.Files = written
	post, postErr := s.takeSnapshot(ctx, sh, snapshotMessage(sessionID, turnID, "reverted"))
	if postErr == nil {
		rec.Post = post.commit
		postErr = s.updateRef(ctx, sh, revertRef(sessionID, rec.ID, "post"), rec.Post)
	}
	// The index is updated under the session lock, which is always taken
	// before a root lock.
	unlock()
	unlock = func() {}
	if err := s.appendRevert(ctx, sessionID, rec); err != nil {
		return result, err
	}
	result.RevertID = rec.ID
	if postErr != nil {
		// The files are written and recorded; only the undo is missing.
		return result, fmt.Errorf("checkpoint: revert applied, but it cannot be undone: %w", postErr)
	}
	return result, nil
}

// Undo puts back the files an applied revert wrote, as they were just before
// it. Files edited since are merged; Force overwrites conflicts.
func (s *Store) Undo(ctx context.Context, sessionID, revertID string, force bool) (RevertResult, error) {
	if err := validID("session", sessionID); err != nil {
		return RevertResult{}, err
	}
	idx, err := s.readIndex(sessionID)
	if err != nil {
		return RevertResult{}, err
	}
	pos := slices.IndexFunc(idx.Reverts, func(r RevertEntry) bool { return r.ID == revertID })
	if pos < 0 {
		return RevertResult{}, ErrNotFound
	}
	rec := idx.Reverts[pos]
	result := RevertResult{RevertID: rec.ID, TurnID: rec.TurnID, Scope: rec.Scope}
	if !rec.UndoneAt.IsZero() {
		return result, fmt.Errorf("%w: revert %s is already undone", ErrInvalid, rec.ID)
	}
	if rec.Post == "" {
		return result, fmt.Errorf("%w: revert %s has no snapshot to undo from", ErrInvalid, rec.ID)
	}
	sh, err := s.openShadow(rec.Shadow)
	if err != nil {
		return result, err
	}
	unlock := s.lockRoot(sh.key)
	defer func() { unlock() }()
	if s.isActive(sessionID) {
		return result, ErrBusy
	}
	moves := make([]move, 0, len(rec.Files))
	for _, path := range rec.Files {
		m := move{path: path}
		if m.base, m.unsupported, err = s.blobAt(ctx, sh, rec.Post, path); err != nil {
			return result, err
		}
		var why string
		if m.target, why, err = s.blobAt(ctx, sh, rec.Pre, path); err != nil {
			return result, err
		}
		if m.unsupported == "" {
			m.unsupported = why
		}
		moves = append(moves, m)
	}
	planned, err := s.planMoves(ctx, sh, moves, force, undoLabels)
	if err != nil {
		return result, err
	}
	result.Files, result.Conflicts, result.Failed = summarize(planned)
	if result.Conflicts > 0 && !force {
		return result, ErrConflict
	}
	s.applyMoves(sh.root, planned)
	result.Files, result.Conflicts, result.Failed = summarize(planned)
	result.Applied = true
	unlock()
	unlock = func() {}
	return result, s.markUndone(sessionID, rec.ID)
}

// Reverts lists a session's applied reverts, oldest first.
func (s *Store) Reverts(sessionID string) ([]RevertEntry, error) {
	if err := validID("session", sessionID); err != nil {
		return nil, err
	}
	idx, err := s.readIndex(sessionID)
	if err != nil {
		return nil, err
	}
	return idx.Reverts, nil
}

func (s *Store) turnEntry(sessionID, turnID string) (Entry, error) {
	idx, err := s.readIndex(sessionID)
	if err != nil {
		return Entry{}, err
	}
	pos := slices.IndexFunc(idx.Turns, func(e Entry) bool { return e.TurnID == turnID })
	if pos < 0 {
		return Entry{}, ErrNotFound
	}
	entry := idx.Turns[pos]
	if entry.Start == "" || entry.End == "" {
		return Entry{}, fmt.Errorf("%w (%s)", ErrSkipped, entry.Skipped)
	}
	return entry, nil
}

// turnMoves turns the turn's own edits back: from its end to its start, or
// for chosen hunks, to its end with those hunks spliced back.
func (s *Store) turnMoves(ctx context.Context, sh *shadowRepo, e Entry, files []RevertFile) ([]move, []RevertFile, error) {
	entries, err := s.nameStatus(ctx, sh, e.Start, e.End, nil)
	if err != nil {
		return nil, nil, err
	}
	entries = slices.DeleteFunc(entries, func(se statusEntry) bool { return hidden(e.Unknown, se.path, se.oldPath) })
	wanted, err := pickFiles(entries, files)
	if err != nil {
		return nil, nil, err
	}
	var moves []move
	var targets []RevertFile
	for _, w := range wanted {
		if len(w.hunks) > 0 {
			m, all, err := s.hunkMove(ctx, sh, e, w.entry, w.hunks)
			if err != nil {
				return nil, nil, err
			}
			if !all {
				moves = append(moves, m)
				targets = append(targets, RevertFile{Path: w.entry.path, HunkIDs: w.hunks})
				continue
			}
		}
		ms, err := s.wholeMoves(ctx, sh, w.entry, e.End, e.Start)
		if err != nil {
			return nil, nil, err
		}
		moves = append(moves, ms...)
		targets = append(targets, RevertFile{Path: w.entry.path})
	}
	return moves, targets, nil
}

// sinceMoves puts every file changed since the turn began back to its start,
// taking now (the pre snapshot) as the base.
func (s *Store) sinceMoves(ctx context.Context, sh *shadowRepo, e Entry, now snapshot, files []RevertFile) ([]move, []RevertFile, error) {
	for _, f := range files {
		if len(f.HunkIDs) > 0 {
			return nil, nil, fmt.Errorf("%w: a since revert takes whole files", ErrInvalid)
		}
	}
	entries, err := s.nameStatus(ctx, sh, e.Start, now.commit, nil)
	if err != nil {
		return nil, nil, err
	}
	unknown := mergeSorted(e.Unknown, now.unknown)
	entries = slices.DeleteFunc(entries, func(se statusEntry) bool { return hidden(unknown, se.path, se.oldPath) })
	wanted, err := pickFiles(entries, files)
	if err != nil {
		return nil, nil, err
	}
	var moves []move
	var targets []RevertFile
	for _, w := range wanted {
		ms, err := s.wholeMoves(ctx, sh, w.entry, now.commit, e.Start)
		if err != nil {
			return nil, nil, err
		}
		moves = append(moves, ms...)
		targets = append(targets, RevertFile{Path: w.entry.path})
	}
	return moves, targets, nil
}

type wantedFile struct {
	entry statusEntry
	hunks []string
}

// pickFiles matches the requested files to the diff; none means all.
func pickFiles(entries []statusEntry, files []RevertFile) ([]wantedFile, error) {
	if len(files) == 0 {
		out := make([]wantedFile, 0, len(entries))
		for _, e := range entries {
			out = append(out, wantedFile{entry: e})
		}
		return out, nil
	}
	var out []wantedFile
	seen := map[string]bool{}
	for _, f := range files {
		if err := validDiffPath(f.Path); err != nil || f.Path == "" {
			return nil, fmt.Errorf("%w: path %q", ErrInvalid, f.Path)
		}
		if seen[f.Path] {
			continue
		}
		seen[f.Path] = true
		pos := slices.IndexFunc(entries, func(e statusEntry) bool { return e.path == f.Path || e.oldPath == f.Path })
		if pos < 0 {
			return nil, fmt.Errorf("%w: %s did not change in scope", ErrInvalid, f.Path)
		}
		out = append(out, wantedFile{entry: entries[pos], hunks: f.HunkIDs})
	}
	return out, nil
}

// wholeMoves moves a file from its content at base to its content at
// target. A rename moves both of its paths.
func (s *Store) wholeMoves(ctx context.Context, sh *shadowRepo, e statusEntry, base, target string) ([]move, error) {
	paths := []string{e.path}
	if e.oldPath != "" {
		paths = append(paths, e.oldPath)
	}
	moves := make([]move, 0, len(paths))
	for _, p := range paths {
		m := move{path: p}
		var err error
		var why string
		if m.base, m.unsupported, err = s.blobAt(ctx, sh, base, p); err != nil {
			return nil, err
		}
		if m.target, why, err = s.blobAt(ctx, sh, target, p); err != nil {
			return nil, err
		}
		if m.unsupported == "" {
			m.unsupported = why
		}
		moves = append(moves, m)
	}
	return moves, nil
}

// hunkMove reverts chosen hunks of a modified file. all reports that every
// hunk was chosen, which is a whole-file revert.
func (s *Store) hunkMove(ctx context.Context, sh *shadowRepo, e Entry, se statusEntry, ids []string) (move, bool, error) {
	if se.status != "modified" {
		return move{}, false, fmt.Errorf("%w: hunks can be reverted only in a modified file, and %s is %s", ErrInvalid, se.path, se.status)
	}
	out, _, err := s.git.run(ctx, sh.objectCall(diffArgs(e.Start, e.End, []string{se.path}, "-U3")...))
	if err != nil {
		return move{}, false, err
	}
	if len(out) > MaxPatchBytes {
		return move{}, false, fmt.Errorf("%w: %s's diff is too long to revert by hunk", ErrInvalid, se.path)
	}
	hunks := parseHunks(string(out))
	chosen := make([]int, 0, len(ids))
	for _, id := range ids {
		pos := slices.IndexFunc(hunks, func(h Hunk) bool { return h.ID == id })
		if pos < 0 {
			return move{}, false, fmt.Errorf("%w: %s has no hunk %q", ErrInvalid, se.path, id)
		}
		chosen = append(chosen, pos)
	}
	slices.Sort(chosen)
	chosen = slices.Compact(chosen)
	if len(chosen) == len(hunks) {
		return move{}, true, nil
	}
	m := move{path: se.path}
	start, why, err := s.blobAt(ctx, sh, e.Start, se.path)
	if err != nil {
		return move{}, false, err
	}
	if m.base, m.unsupported, err = s.blobAt(ctx, sh, e.End, se.path); err != nil {
		return move{}, false, err
	}
	if m.unsupported == "" {
		m.unsupported = why
	}
	if m.unsupported != "" {
		return m, false, nil
	}
	if binaryContent(start.data) || binaryContent(m.base.data) {
		return move{}, false, fmt.Errorf("%w: %s is binary; revert it whole", ErrInvalid, se.path)
	}
	data, err := spliceHunks(start.data, m.base.data, hunks, chosen)
	if err != nil {
		return move{}, false, err
	}
	m.target = fileState{ok: true, data: data}
	return m, false, nil
}

// blobAt reads path from a snapshot. Links and nested repositories are
// recorded by name only, so they cannot be moved.
func (s *Store) blobAt(ctx context.Context, sh *shadowRepo, commit, path string) (fileState, string, error) {
	out, _, err := s.git.run(ctx, sh.objectCall("ls-tree", "-z", commit, "--", path))
	if err != nil {
		return fileState{}, "", err
	}
	record := strings.TrimSuffix(string(out), "\x00")
	if record == "" {
		return fileState{}, "", nil
	}
	meta, _, _ := strings.Cut(record, "\t")
	fields := strings.Fields(meta)
	if len(fields) != 3 {
		return fileState{}, "", fmt.Errorf("checkpoint: bad ls-tree record %q", record)
	}
	switch {
	case fields[1] == "tree":
		return fileState{}, "a directory", nil
	case fields[0] == "120000":
		return fileState{}, "a symbolic link", nil
	case fields[0] == "160000":
		return fileState{}, "a nested repository", nil
	}
	data, _, err := s.git.run(ctx, sh.objectCall("cat-file", "blob", fields[2]))
	if err != nil {
		return fileState{}, "", err
	}
	return fileState{ok: true, data: data}, "", nil
}

// planMoves decides each move against the file as it is on disk now.
func (s *Store) planMoves(ctx context.Context, sh *shadowRepo, moves []move, force bool, labels [3]string) ([]plannedFile, error) {
	planned := make([]plannedFile, 0, len(moves))
	for _, m := range moves {
		p := plannedFile{move: m, result: RevertFileResult{Path: m.path, Delete: !m.target.ok}}
		cur, why, err := readWorkFile(sh.root, m.path)
		switch {
		case err != nil:
			p.result.Outcome, p.result.Detail = RevertFailed, err.Error()
		case m.unsupported != "" || why != "":
			p.result.Outcome, p.result.Detail = RevertFailed, "not a regular file: "+firstNonEmpty(m.unsupported, why)
		case cur.equal(m.target):
			p.result.Outcome = RevertUnchanged
		case cur.equal(m.base):
			p.result.Outcome, p.write, p.apply = RevertWrite, m.target, true
		default:
			if err := s.mergeMove(ctx, sh, &p, cur, labels); err != nil {
				return nil, err
			}
			if p.result.Outcome == RevertConflict && force {
				p.result.Outcome, p.result.Forced, p.write, p.apply = RevertWrite, true, m.target, true
			}
		}
		planned = append(planned, p)
	}
	return planned, nil
}

// mergeMove applies base→target to the current file with git merge-file.
func (s *Store) mergeMove(ctx context.Context, sh *shadowRepo, p *plannedFile, cur fileState, labels [3]string) error {
	if !cur.ok || !p.base.ok || !p.target.ok || binaryContent(cur.data) || binaryContent(p.base.data) || binaryContent(p.target.data) {
		p.result.Outcome, p.result.Detail = RevertConflict, "the file changed after the turn and cannot be merged"
		return nil
	}
	dir, err := os.MkdirTemp("", "tars-revert-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	names := [3]string{"current", "base", "target"}
	for i, data := range [3][]byte{cur.data, p.base.data, p.target.data} {
		if err := os.WriteFile(filepath.Join(dir, names[i]), data, 0o600); err != nil {
			return err
		}
	}
	call := gitCall{gitDir: sh.gitDir, dir: dir, args: []string{
		"-c", "merge.conflictStyle=merge", "merge-file", "-p",
		"-L", labels[0], "-L", labels[1], "-L", labels[2], names[0], names[1], names[2],
	}}
	out, code, err := s.git.run(ctx, call)
	var gerr *gitError
	switch {
	case err == nil:
		p.result.Outcome, p.write, p.apply = RevertMerge, fileState{ok: true, data: out}, true
	case errors.As(err, &gerr) && code > 0 && code < 128:
		p.result.Outcome, p.result.Detail = RevertConflict, "later edits overlap the revert"
		p.result.Merged = clipText(string(out))
	default:
		return err
	}
	return nil
}

// refuseUnrecorded keeps a revert away from files the undo snapshot could
// not hold: undoing would otherwise delete them.
func refuseUnrecorded(planned []plannedFile, unknown []string) {
	for i := range planned {
		p := &planned[i]
		if p.apply && hidden(unknown, p.path) {
			p.apply = false
			p.result.Outcome, p.result.Detail = RevertFailed, "too large or unreadable to back up for undo"
		}
	}
}

// applyMoves writes the planned files and returns the paths written. A file
// that cannot be written is reported and the rest go ahead: the pre
// snapshot makes the whole revert undoable either way.
func (s *Store) applyMoves(root string, planned []plannedFile) []string {
	var written []string
	for i := range planned {
		p := &planned[i]
		if !p.apply {
			continue
		}
		if err := writeWorkFile(root, p.path, p.write); err != nil {
			p.result.Outcome, p.result.Detail = RevertFailed, err.Error()
			continue
		}
		written = append(written, p.path)
	}
	return written
}

func summarize(planned []plannedFile) ([]RevertFileResult, int, int) {
	files := make([]RevertFileResult, 0, len(planned))
	conflicts, failed := 0, 0
	for _, p := range planned {
		switch p.result.Outcome {
		case RevertConflict:
			conflicts++
		case RevertFailed:
			failed++
		}
		files = append(files, p.result)
	}
	return files, conflicts, failed
}

// readWorkFile reads a root-relative file. A directory or link in its place
// is reported, not followed.
func readWorkFile(root, rel string) (fileState, string, error) {
	abs := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Lstat(abs)
	if errors.Is(err, fs.ErrNotExist) {
		return fileState{}, "", nil
	}
	if err != nil {
		return fileState{}, "", err
	}
	if !info.Mode().IsRegular() {
		return fileState{}, "the path is " + info.Mode().Type().String(), nil
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return fileState{}, "", err
	}
	return fileState{ok: true, data: data}, "", nil
}

// writeWorkFile puts content at a root-relative path, or removes the file.
// An existing file keeps its permissions.
func writeWorkFile(root, rel string, content fileState) error {
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if !content.ok {
		if err := os.Remove(abs); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	mode := fs.FileMode(0o644)
	if info, err := os.Lstat(abs); err == nil {
		mode = info.Mode().Perm()
	}
	if err := atomicwrite.Write(abs, content.data); err != nil {
		return err
	}
	return os.Chmod(abs, mode)
}

func (s *Store) appendRevert(ctx context.Context, sessionID string, rec RevertEntry) error {
	unlock := s.lockSession(sessionID)
	defer unlock()
	idx, err := s.readIndex(sessionID)
	if err != nil {
		return err
	}
	idx.Reverts = append(idx.Reverts, rec)
	if extra := len(idx.Reverts) - keepReverts; extra > 0 {
		dropped := idx.Reverts[:extra]
		idx.Reverts = slices.Clone(idx.Reverts[extra:])
		s.dropRevertRefs(ctx, sessionID, dropped)
	}
	return s.writeIndex(idx)
}

func (s *Store) markUndone(sessionID, revertID string) error {
	unlock := s.lockSession(sessionID)
	defer unlock()
	idx, err := s.readIndex(sessionID)
	if err != nil {
		return err
	}
	for i := range idx.Reverts {
		if idx.Reverts[i].ID == revertID {
			idx.Reverts[i].UndoneAt = s.now().UTC()
		}
	}
	return s.writeIndex(idx)
}

func (s *Store) dropRevertRefs(ctx context.Context, sessionID string, dropped []RevertEntry) {
	byShadow := map[string][]string{}
	for _, r := range dropped {
		byShadow[r.Shadow] = append(byShadow[r.Shadow], revertRef(sessionID, r.ID, "pre"), revertRef(sessionID, r.ID, "post"))
	}
	for key, refs := range byShadow {
		sh, err := s.openShadow(key)
		if err != nil {
			continue
		}
		unlock := s.lockRoot(key)
		_ = s.deleteRefs(ctx, sh, s.existingRefs(ctx, sh, refs))
		unlock()
	}
}

func revertRef(sessionID, revertID, phase string) string {
	return refNamespace + sessionID + "/reverts/" + revertID + "/" + phase
}

// newRevertID is time-ordered, with a random tail against equal clocks.
func newRevertID(now time.Time) string {
	var tail [4]byte
	_, _ = rand.Read(tail[:])
	return "r" + now.UTC().Format("20060102T150405") + "-" + hex.EncodeToString(tail[:])
}

func clipText(s string) string {
	if len(s) <= MaxPatchBytes {
		return s
	}
	cut := strings.LastIndexByte(s[:MaxPatchBytes], '\n')
	if cut < 0 {
		cut = MaxPatchBytes
	}
	return s[:cut+1]
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
