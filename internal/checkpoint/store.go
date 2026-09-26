// Package checkpoint records what a chat turn changed on disk. Before and
// after each turn it snapshots the session's work tree into a shadow git
// repository that TARS owns, so every provider's edits can be diffed —
// including a CLI provider's, made inside its own process where TARS never
// sees the tool calls — without writing to the user's repository.
package checkpoint

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/devlikebear/tars/internal/git"
)

// Options configure a Store. Zero values take the defaults.
type Options struct {
	Limits Limits
	// KeepTurns is how many recent turns a session keeps; older checkpoints
	// are dropped as new ones arrive. Default 50.
	KeepTurns int
	// CommandTimeout bounds each git call. Default one minute.
	CommandTimeout time.Duration
	Now            func() time.Time
}

// Store keeps checkpoints under dir: shadow repositories in shadow/, one per
// work-tree root, and a per-session index in sessions/.
type Store struct {
	dir    string
	git    gitRunner
	limits Limits
	keep   int
	now    func() time.Time

	mu        sync.Mutex
	rootLocks map[string]*sync.Mutex
	sessLocks map[string]*sync.Mutex
	// active counts turns per session that have a start ref but no stored
	// entry yet, so a sweep does not mistake them for orphans.
	active map[string]int
}

// ErrNotFound reports an unknown turn.
var ErrNotFound = errors.New("checkpoint: turn not found")

// Open prepares a store rooted at dir. It fails only when git is missing or
// dir cannot be created.
func Open(dir string, opts Options) (*Store, error) {
	exe, err := git.Executable()
	if err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("checkpoint: resolve dir: %w", err)
	}
	for _, sub := range []string{"sessions", "shadow"} {
		if err := os.MkdirAll(filepath.Join(abs, sub), 0o755); err != nil {
			return nil, fmt.Errorf("checkpoint: create %s: %w", sub, err)
		}
	}
	limits := opts.Limits
	if limits == (Limits{}) {
		limits = DefaultLimits
	}
	keep := opts.KeepTurns
	if keep <= 0 {
		keep = 50
	}
	timeout := opts.CommandTimeout
	if timeout <= 0 {
		timeout = time.Minute
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Store{
		dir:       abs,
		git:       gitRunner{exe: exe, timeout: timeout},
		limits:    limits,
		keep:      keep,
		now:       now,
		rootLocks: map[string]*sync.Mutex{},
		sessLocks: map[string]*sync.Mutex{},
		active:    map[string]int{},
	}, nil
}

func (s *Store) lock(m map[string]*sync.Mutex, key string) func() {
	s.mu.Lock()
	l, ok := m[key]
	if !ok {
		l = &sync.Mutex{}
		m[key] = l
	}
	s.mu.Unlock()
	l.Lock()
	return l.Unlock
}

func (s *Store) lockRoot(key string) func()          { return s.lock(s.rootLocks, key) }
func (s *Store) lockSession(sessionID string) func() { return s.lock(s.sessLocks, sessionID) }

func (s *Store) markActive(sessionID string, delta int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active[sessionID] += delta
	if s.active[sessionID] <= 0 {
		delete(s.active, sessionID)
	}
}

func (s *Store) isActive(sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active[sessionID] > 0
}

// ResolveRoot picks the work tree a turn in cwd is recorded against: the top
// level of the git repository cwd is in, or cwd itself when it is not in a
// repository or that repository ignores it (a gitignored workspace folder,
// for one).
func (s *Store) ResolveRoot(ctx context.Context, cwd string) (string, error) {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("checkpoint: resolve cwd: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("checkpoint: cwd: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("checkpoint: cwd %q is not a directory", abs)
	}
	out, code, err := s.git.run(ctx, gitCall{dir: abs, args: []string{"rev-parse", "--show-toplevel", "--show-prefix"}, okExit: []int{128}})
	if err != nil {
		return "", err
	}
	if code != 0 {
		return abs, nil
	}
	lines := strings.Split(strings.TrimRight(string(out), "\r\n"), "\n")
	top := filepath.Clean(filepath.FromSlash(strings.TrimSpace(lines[0])))
	prefix := ""
	if len(lines) > 1 {
		prefix = strings.TrimSuffix(strings.TrimSpace(lines[1]), "/")
	}
	if prefix == "" {
		return top, nil
	}
	_, code, err = s.git.run(ctx, gitCall{dir: top, args: []string{"check-ignore", "-q", "--", prefix}, okExit: []int{1}, patternPaths: true})
	if err != nil {
		return "", err
	}
	if code == 0 {
		return abs, nil
	}
	return top, nil
}

// Turn is a checkpoint in progress: the start snapshot is taken, the end is
// not.
type Turn struct {
	store     *Store
	sessionID string
	sh        *shadowRepo
	entry     Entry
	active    bool
}

// Entry returns what is known so far, including a skip reason.
func (t *Turn) Entry() Entry { return t.entry }

// BeginTurn snapshots cwd's root before a turn runs. A folder over the
// limits, or a git failure, does not fail the turn: the returned Turn
// carries the skip reason and End records it.
func (s *Store) BeginTurn(ctx context.Context, sessionID, turnID, cwd, preview string) (*Turn, error) {
	if err := validID("session", sessionID); err != nil {
		return nil, err
	}
	if err := validID("turn", turnID); err != nil {
		return nil, err
	}
	t := &Turn{store: s, sessionID: sessionID, entry: Entry{TurnID: turnID, StartedAt: s.now().UTC(), Preview: clipPreview(preview)}}
	root, err := s.ResolveRoot(ctx, cwd)
	if err != nil {
		t.skip(err)
		return t, nil
	}
	t.entry.Root = root
	sh, err := s.shadow(ctx, root)
	if err != nil {
		t.skip(err)
		return t, nil
	}
	t.sh = sh
	t.entry.Shadow = sh.key
	unlock := s.lockRoot(sh.key)
	snap, err := s.takeSnapshot(ctx, sh, snapshotMessage(sessionID, turnID, "start"))
	if err == nil {
		err = s.updateRef(ctx, sh, turnRef(sessionID, turnID, "start"), snap.commit)
	}
	if err == nil {
		s.markActive(sessionID, 1)
		t.active = true
	}
	unlock()
	if err != nil {
		t.skip(err)
		return t, nil
	}
	t.entry.Start = snap.commit
	t.entry.Unknown = snap.unknown
	return t, nil
}

// End snapshots the root again, records the turn's stats, and stores the
// entry. Call it on every path out of the turn — success, error, and
// cancellation all leave edits on disk.
func (t *Turn) End(ctx context.Context) (Entry, error) {
	s := t.store
	e := t.entry
	e.EndedAt = s.now().UTC()
	if e.Skipped == "" {
		unlock := s.lockRoot(t.sh.key)
		err := t.finish(ctx, &e)
		unlock()
		if err != nil {
			// Without an end there is nothing to diff; drop the start ref.
			_ = s.deleteRefs(ctx, t.sh, []string{turnRef(t.sessionID, e.TurnID, "start")})
			e.Start = ""
			e.Skipped, e.SkipDetail = skipFields(err)
		}
	}
	t.entry = e
	err := s.appendEntry(ctx, t.sessionID, e)
	if t.active {
		t.active = false
		s.markActive(t.sessionID, -1)
	}
	return e, err
}

func (t *Turn) finish(ctx context.Context, e *Entry) error {
	s := t.store
	snap, err := s.takeSnapshot(ctx, t.sh, snapshotMessage(t.sessionID, e.TurnID, "end"))
	if err != nil {
		return err
	}
	if err := s.updateRef(ctx, t.sh, turnRef(t.sessionID, e.TurnID, "end"), snap.commit); err != nil {
		return err
	}
	e.End = snap.commit
	e.Unknown = mergeSorted(e.Unknown, snap.unknown)
	stats, err := s.stats(ctx, t.sh, e.Start, e.End, e.Unknown)
	if err != nil {
		return err
	}
	e.Files, e.Additions, e.Deletions = stats.files, stats.additions, stats.deletions
	return nil
}

func (t *Turn) skip(err error) {
	t.entry.Skipped, t.entry.SkipDetail = skipFields(err)
}

func skipFields(err error) (string, string) {
	var skip *skipError
	if errors.As(err, &skip) {
		return skip.reason, skip.detail
	}
	return SkipFailed, err.Error()
}

// Prime creates cwd's shadow and takes a first snapshot without recording a
// turn, so the next real turn only hashes what changed since.
func (s *Store) Prime(ctx context.Context, cwd string) error {
	root, err := s.ResolveRoot(ctx, cwd)
	if err != nil {
		return err
	}
	sh, err := s.shadow(ctx, root)
	if err != nil {
		return err
	}
	unlock := s.lockRoot(sh.key)
	defer unlock()
	_, err = s.takeSnapshot(ctx, sh, "tars checkpoint prime")
	return err
}

// List returns a session's recorded turns, oldest first.
func (s *Store) List(sessionID string) ([]Entry, error) {
	if err := validID("session", sessionID); err != nil {
		return nil, err
	}
	idx, err := s.readIndex(sessionID)
	if err != nil {
		return nil, err
	}
	return idx.Turns, nil
}

func (s *Store) appendEntry(ctx context.Context, sessionID string, e Entry) error {
	unlock := s.lockSession(sessionID)
	defer unlock()
	idx, err := s.readIndex(sessionID)
	if err != nil {
		return err
	}
	idx.Turns = slices.DeleteFunc(idx.Turns, func(old Entry) bool { return old.TurnID == e.TurnID })
	idx.Turns = append(idx.Turns, e)
	if extra := len(idx.Turns) - s.keep; extra > 0 {
		dropped := idx.Turns[:extra]
		idx.Turns = slices.Clone(idx.Turns[extra:])
		s.dropTurnRefs(ctx, sessionID, dropped)
	}
	return s.writeIndex(idx)
}

func (s *Store) updateRef(ctx context.Context, sh *shadowRepo, ref, commit string) error {
	_, _, err := s.git.run(ctx, sh.call("update-ref", ref, commit))
	return err
}

func (s *Store) deleteRefs(ctx context.Context, sh *shadowRepo, refs []string) error {
	if sh == nil || len(refs) == 0 {
		return nil
	}
	var b strings.Builder
	for _, ref := range refs {
		b.WriteString("delete " + ref + "\n")
	}
	call := sh.call("update-ref", "--stdin")
	call.stdin = []byte(b.String())
	_, _, err := s.git.run(ctx, call)
	return err
}

func turnRef(sessionID, turnID, phase string) string {
	return "refs/tars/checkpoints/" + sessionID + "/" + turnID + "/" + phase
}

func sessionRefPrefix(sessionID string) string {
	return "refs/tars/checkpoints/" + sessionID + "/"
}

func snapshotMessage(sessionID, turnID, phase string) string {
	return fmt.Sprintf("tars checkpoint\n\nsession: %s\nturn: %s\nphase: %s", sessionID, turnID, phase)
}

func clipPreview(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	const max = 120
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max-1]) + "…"
}

func mergeSorted(a, b []string) []string {
	out := append(slices.Clone(a), b...)
	slices.Sort(out)
	return slices.Compact(out)
}
