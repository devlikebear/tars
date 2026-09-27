package checkpoint

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Scope picks the two snapshots a diff compares.
type Scope string

const (
	// ScopeTurn is what the turn changed: its start against its end.
	ScopeTurn Scope = "turn"
	// ScopeSession is everything the session's turns changed in this root:
	// the first start against the latest end.
	ScopeSession Scope = "session"
	// ScopeSince is the turn's start against the work tree now, including
	// later turns and edits made outside any turn.
	ScopeSince Scope = "since"
)

// FileDiff is one file's change between two snapshots.
type FileDiff struct {
	Path      string `json:"path"`
	OldPath   string `json:"old_path,omitempty"`
	Status    string `json:"status"` // added, modified, deleted, renamed
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Binary    bool   `json:"binary,omitempty"`
	Patch     string `json:"patch,omitempty"`
	// Truncated reports a patch cut at MaxPatchBytes; its hunks are omitted,
	// so only the whole file can be reverted.
	Truncated bool   `json:"truncated,omitempty"`
	Hunks     []Hunk `json:"hunks,omitempty"`
}

// Hunk locates one @@ block of a file's patch. IDs are stable for a given
// pair of snapshots.
type Hunk struct {
	ID       string `json:"id"`
	OldStart int    `json:"old_start"`
	OldLines int    `json:"old_lines"`
	NewStart int    `json:"new_start"`
	NewLines int    `json:"new_lines"`
	Header   string `json:"header"`
}

// DiffResult is a scoped diff of one turn.
type DiffResult struct {
	TurnID  string     `json:"turn_id"`
	Scope   Scope      `json:"scope"`
	Root    string     `json:"root"`
	From    string     `json:"from"`
	To      string     `json:"to"`
	Files   []FileDiff `json:"files"`
	Unknown []string   `json:"unknown,omitempty"`
}

// MaxPatchBytes caps one file's patch in a diff result.
const MaxPatchBytes = 512 << 10

// ErrSkipped reports a turn without a checkpoint to diff.
var ErrSkipped = errors.New("checkpoint: the turn has no checkpoint")

// Diff compares a turn's snapshots in the given scope. path, when set,
// limits the result to that root-relative file.
func (s *Store) Diff(ctx context.Context, sessionID, turnID string, scope Scope, path string) (DiffResult, error) {
	if err := validID("session", sessionID); err != nil {
		return DiffResult{}, err
	}
	idx, err := s.readIndex(sessionID)
	if err != nil {
		return DiffResult{}, err
	}
	pos := slices.IndexFunc(idx.Turns, func(e Entry) bool { return e.TurnID == turnID })
	if pos < 0 {
		return DiffResult{}, ErrNotFound
	}
	entry := idx.Turns[pos]
	if entry.Start == "" || entry.End == "" {
		return DiffResult{}, fmt.Errorf("%w (%s)", ErrSkipped, entry.Skipped)
	}
	sh, err := s.openShadow(entry.Shadow)
	if err != nil {
		return DiffResult{}, err
	}
	result := DiffResult{TurnID: turnID, Scope: scope, Root: entry.Root, From: entry.Start, To: entry.End}
	unknown := entry.Unknown
	switch scope {
	case ScopeTurn, "":
		result.Scope = ScopeTurn
	case ScopeSession:
		// Turns are stored in the order they ended.
		result.From = firstStart(idx.Turns, entry.Shadow)
		for _, e := range idx.Turns {
			if e.Shadow != entry.Shadow || e.Start == "" || e.End == "" {
				continue
			}
			result.To = e.End
			unknown = mergeSorted(unknown, e.Unknown)
		}
	case ScopeSince:
		unlock := s.lockRoot(sh.key)
		snap, err := s.takeSnapshot(ctx, sh, snapshotMessage(sessionID, turnID, "now"))
		unlock()
		if err != nil {
			return DiffResult{}, err
		}
		result.To = snap.commit
		unknown = mergeSorted(unknown, snap.unknown)
	default:
		return DiffResult{}, fmt.Errorf("checkpoint: unknown scope %q", scope)
	}
	result.Unknown = unknown
	files, err := s.diffFiles(ctx, sh, result.From, result.To, path, unknown)
	if err != nil {
		return DiffResult{}, err
	}
	result.Files = files
	return result, nil
}

func firstStart(turns []Entry, shadow string) string {
	for _, e := range turns {
		if e.Shadow == shadow && e.Start != "" && e.End != "" {
			return e.Start
		}
	}
	return ""
}

// openShadow finds an existing shadow by key without creating one.
func (s *Store) openShadow(key string) (*shadowRepo, error) {
	if !safeID.MatchString(key) {
		return nil, fmt.Errorf("checkpoint: invalid shadow %q", key)
	}
	gitDir := filepath.Join(s.dir, "shadow", key+".git")
	root, err := readMarker(gitDir)
	if err != nil {
		return nil, err
	}
	return &shadowRepo{root: root, key: key, gitDir: gitDir}, nil
}

type diffStats struct {
	files, additions, deletions int
}

func (s *Store) stats(ctx context.Context, sh *shadowRepo, from, to string, unknown []string) (diffStats, error) {
	entries, err := s.nameStatus(ctx, sh, from, to, nil)
	if err != nil {
		return diffStats{}, err
	}
	counts, err := s.numstat(ctx, sh, from, to, nil)
	if err != nil {
		return diffStats{}, err
	}
	var st diffStats
	for i, e := range entries {
		if hidden(unknown, e.path, e.oldPath) {
			continue
		}
		st.files++
		if i < len(counts) {
			st.additions += counts[i].additions
			st.deletions += counts[i].deletions
		}
	}
	return st, nil
}

type statusEntry struct {
	status  string
	path    string
	oldPath string
}

// diffBaseArgs keep the user's global config (external diff tools, colors,
// textconv) out of what the store parses.
var diffBaseArgs = []string{"diff", "--no-ext-diff", "--no-color", "--no-textconv", "-M"}

func diffArgs(from, to string, paths []string, extra ...string) []string {
	args := append(slices.Clone(diffBaseArgs), extra...)
	args = append(args, from, to)
	if len(paths) > 0 {
		args = append(append(args, "--"), paths...)
	}
	return args
}

func onePath(path string) []string {
	if path == "" {
		return nil
	}
	return []string{path}
}

func (s *Store) nameStatus(ctx context.Context, sh *shadowRepo, from, to string, paths []string) ([]statusEntry, error) {
	out, _, err := s.git.run(ctx, sh.call(diffArgs(from, to, paths, "--name-status", "-z")...))
	if err != nil {
		return nil, err
	}
	fields := splitNUL(out)
	var entries []statusEntry
	for i := 0; i < len(fields); {
		code := fields[i]
		switch {
		case strings.HasPrefix(code, "R") || strings.HasPrefix(code, "C"):
			if i+2 >= len(fields) {
				return nil, errors.New("checkpoint: short rename record")
			}
			status := "renamed"
			if strings.HasPrefix(code, "C") {
				status = "added"
			}
			entries = append(entries, statusEntry{status: status, oldPath: fields[i+1], path: fields[i+2]})
			i += 3
		default:
			if i+1 >= len(fields) {
				return nil, errors.New("checkpoint: short status record")
			}
			entries = append(entries, statusEntry{status: statusName(code), path: fields[i+1]})
			i += 2
		}
	}
	return entries, nil
}

func statusName(code string) string {
	switch code {
	case "A":
		return "added"
	case "D":
		return "deleted"
	default:
		return "modified"
	}
}

type numstatEntry struct {
	additions, deletions int
	binary               bool
}

func (s *Store) numstat(ctx context.Context, sh *shadowRepo, from, to string, paths []string) ([]numstatEntry, error) {
	out, _, err := s.git.run(ctx, sh.call(diffArgs(from, to, paths, "--numstat", "-z")...))
	if err != nil {
		return nil, err
	}
	fields := splitNUL(out)
	var entries []numstatEntry
	for i := 0; i < len(fields); {
		parts := strings.SplitN(fields[i], "\t", 3)
		if len(parts) < 3 {
			return nil, fmt.Errorf("checkpoint: bad numstat record %q", fields[i])
		}
		e := numstatEntry{binary: parts[0] == "-"}
		e.additions, _ = strconv.Atoi(parts[0])
		e.deletions, _ = strconv.Atoi(parts[1])
		entries = append(entries, e)
		if parts[2] == "" {
			i += 3 // rename: old and new paths follow as their own fields
		} else {
			i++
		}
	}
	return entries, nil
}

func (s *Store) diffFiles(ctx context.Context, sh *shadowRepo, from, to, path string, unknown []string) ([]FileDiff, error) {
	entries, err := s.nameStatus(ctx, sh, from, to, onePath(path))
	if err != nil {
		return nil, err
	}
	counts, err := s.numstat(ctx, sh, from, to, onePath(path))
	if err != nil {
		return nil, err
	}
	out, _, err := s.git.run(ctx, sh.call(diffArgs(from, to, onePath(path), "-U3")...))
	if err != nil {
		return nil, err
	}
	chunks := splitPatch(string(out))
	if len(chunks) != len(entries) || len(counts) != len(entries) {
		return s.diffFilesOneByOne(ctx, sh, from, to, entries, unknown)
	}
	files := make([]FileDiff, 0, len(entries))
	for i, e := range entries {
		if hidden(unknown, e.path, e.oldPath) {
			continue
		}
		files = append(files, buildFileDiff(e, counts[i], chunks[i]))
	}
	return files, nil
}

// diffFilesOneByOne is the fallback when the combined outputs cannot be
// lined up: one diff per path.
func (s *Store) diffFilesOneByOne(ctx context.Context, sh *shadowRepo, from, to string, entries []statusEntry, unknown []string) ([]FileDiff, error) {
	var files []FileDiff
	for _, e := range entries {
		if hidden(unknown, e.path, e.oldPath) {
			continue
		}
		paths := []string{e.path}
		if e.oldPath != "" {
			paths = append(paths, e.oldPath)
		}
		out, _, err := s.git.run(ctx, sh.call(diffArgs(from, to, paths, "-U3")...))
		if err != nil {
			return nil, err
		}
		counts, err := s.numstat(ctx, sh, from, to, paths)
		if err != nil {
			return nil, err
		}
		var count numstatEntry
		if len(counts) > 0 {
			count = counts[0]
		}
		patch := ""
		if chunks := splitPatch(string(out)); len(chunks) > 0 {
			patch = chunks[0]
		}
		files = append(files, buildFileDiff(e, count, patch))
	}
	return files, nil
}

func buildFileDiff(e statusEntry, count numstatEntry, patch string) FileDiff {
	f := FileDiff{Path: e.path, OldPath: e.oldPath, Status: e.status, Additions: count.additions, Deletions: count.deletions, Binary: count.binary}
	if f.Binary {
		return f
	}
	if len(patch) > MaxPatchBytes {
		cut := strings.LastIndexByte(patch[:MaxPatchBytes], '\n')
		if cut < 0 {
			cut = MaxPatchBytes
		}
		f.Patch = patch[:cut+1]
		f.Truncated = true
		return f
	}
	f.Patch = patch
	f.Hunks = parseHunks(patch)
	return f
}

// splitPatch cuts a multi-file patch at each "diff --git" header.
func splitPatch(patch string) []string {
	if patch == "" {
		return nil
	}
	var chunks []string
	start := -1
	offset := 0
	for _, line := range strings.SplitAfter(patch, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			if start >= 0 {
				chunks = append(chunks, patch[start:offset])
			}
			start = offset
		}
		offset += len(line)
	}
	if start >= 0 {
		chunks = append(chunks, patch[start:])
	}
	return chunks
}

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

func parseHunks(patch string) []Hunk {
	var hunks []Hunk
	for _, line := range strings.Split(patch, "\n") {
		m := hunkHeader.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		h := Hunk{ID: "h" + strconv.Itoa(len(hunks)), Header: strings.TrimRight(line, "\r")}
		h.OldStart, _ = strconv.Atoi(m[1])
		h.OldLines = countOrOne(m[2])
		h.NewStart, _ = strconv.Atoi(m[3])
		h.NewLines = countOrOne(m[4])
		hunks = append(hunks, h)
	}
	return hunks
}

func countOrOne(s string) int {
	if s == "" {
		return 1
	}
	n, _ := strconv.Atoi(s)
	return n
}

func hidden(unknown []string, paths ...string) bool {
	for _, p := range paths {
		if p == "" {
			continue
		}
		if _, found := slices.BinarySearch(unknown, p); found {
			return true
		}
		for _, u := range unknown {
			if strings.HasSuffix(u, "/") && strings.HasPrefix(p, u) {
				return true
			}
		}
	}
	return false
}
