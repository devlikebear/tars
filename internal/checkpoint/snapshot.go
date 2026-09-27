package checkpoint

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Limits bound the work of one snapshot. The first snapshot of a root hashes
// every file git does not ignore; later ones hash only what changed.
type Limits struct {
	// MaxFiles caps the files hashed by one snapshot.
	MaxFiles int
	// MaxBytes caps the bytes hashed by one snapshot.
	MaxBytes int64
	// MaxFileBytes leaves larger files out; they are reported as unknown and
	// never touched by a revert.
	MaxFileBytes int64
}

// DefaultLimits suit a source repository; a folder over them is skipped
// with a reason instead of stalling the turn.
var DefaultLimits = Limits{MaxFiles: 20000, MaxBytes: 512 << 20, MaxFileBytes: 16 << 20}

// Reasons a checkpoint was skipped.
const (
	SkipTooManyFiles = "too_many_files"
	SkipTooLarge     = "too_large"
	SkipFailed       = "failed"
)

// skipError reports a snapshot that was deliberately not taken.
type skipError struct {
	reason string
	detail string
}

func (e *skipError) Error() string { return "checkpoint skipped: " + e.reason + ": " + e.detail }

type snapshot struct {
	commit  string
	unknown []string
}

// takeSnapshot records the root's current files in its shadow and returns
// the commit. The caller holds the root's lock.
func (s *Store) takeSnapshot(ctx context.Context, sh *shadowRepo, message string) (snapshot, error) {
	sh.removeStaleLock()
	if err := s.writeExcludes(sh, nil); err != nil {
		return snapshot{}, err
	}
	forced, err := s.trackedButIgnored(ctx, sh)
	if err != nil {
		return snapshot{}, err
	}
	scan, err := s.scanChanges(ctx, sh, forced)
	if err != nil {
		return snapshot{}, err
	}
	unknown := append(slices.Clone(scan.oversize), scan.nested...)
	if len(unknown) > 0 {
		if err := s.writeExcludes(sh, unknown); err != nil {
			return snapshot{}, err
		}
	}
	// A file that grew past the limit leaves the snapshot instead of keeping
	// a stale copy.
	if len(scan.oversizeTracked) > 0 {
		call := sh.call("rm", "--cached", "--quiet", "--ignore-unmatch", "--pathspec-from-file=-", "--pathspec-file-nul")
		call.stdin = nulList(scan.oversizeTracked)
		if _, _, err := s.git.run(ctx, call); err != nil {
			return snapshot{}, err
		}
	}
	failed, err := s.addAll(ctx, sh, sh.call("add", "-A", "--ignore-errors"))
	if err != nil {
		return snapshot{}, err
	}
	unknown = append(unknown, failed...)
	if len(scan.forced) > 0 {
		call := sh.call("add", "-f", "--ignore-errors", "--pathspec-from-file=-", "--pathspec-file-nul")
		call.stdin = nulList(scan.forced)
		failed, err := s.addAll(ctx, sh, call)
		if err != nil {
			return snapshot{}, err
		}
		unknown = append(unknown, failed...)
	}
	tree, _, err := s.git.run(ctx, sh.call("write-tree"))
	if err != nil {
		return snapshot{}, err
	}
	commit, _, err := s.git.run(ctx, sh.call("commit-tree", strings.TrimSpace(string(tree)), "-m", message))
	if err != nil {
		return snapshot{}, err
	}
	slices.Sort(unknown)
	return snapshot{commit: strings.TrimSpace(string(commit)), unknown: slices.Compact(unknown)}, nil
}

type changeScan struct {
	oversize        []string
	oversizeTracked []string
	nested          []string
	forced          []string
}

// candidateSet is every path the next add may hash, in first-seen order.
type candidateSet struct {
	paths []string
	// tracked marks files already in the shadow's index, which a snapshot
	// must drop rather than keep stale when they grow past the limit.
	tracked map[string]bool
	forced  map[string]bool
}

// scanChanges lists what the next add would hash and enforces the limits
// before any object is written.
func (s *Store) scanChanges(ctx context.Context, sh *shadowRepo, forced []string) (changeScan, error) {
	set, err := s.changeCandidates(ctx, sh, forced)
	if err != nil {
		return changeScan{}, err
	}
	scan, files, total := s.classify(sh.root, set)
	if s.limits.MaxFiles > 0 && files > s.limits.MaxFiles {
		return changeScan{}, &skipError{reason: SkipTooManyFiles, detail: fmt.Sprintf("%d files to record, limit %d", files, s.limits.MaxFiles)}
	}
	if s.limits.MaxBytes > 0 && total > s.limits.MaxBytes {
		return changeScan{}, &skipError{reason: SkipTooLarge, detail: fmt.Sprintf("%d bytes to record, limit %d", total, s.limits.MaxBytes)}
	}
	return scan, nil
}

func (s *Store) changeCandidates(ctx context.Context, sh *shadowRepo, forced []string) (candidateSet, error) {
	others, _, err := s.git.run(ctx, sh.call(gitLsFiles, "-z", "--others", "--exclude-standard"))
	if err != nil {
		return candidateSet{}, err
	}
	modified, _, err := s.git.run(ctx, sh.call(gitLsFiles, "-z", "--modified"))
	if err != nil {
		return candidateSet{}, err
	}
	set := candidateSet{tracked: map[string]bool{}, forced: map[string]bool{}}
	seen := map[string]bool{}
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			set.paths = append(set.paths, p)
		}
	}
	for _, p := range splitNUL(others) {
		add(p)
	}
	for _, p := range splitNUL(modified) {
		set.tracked[p] = true
		add(p)
	}
	for _, p := range forced {
		set.forced[p] = true
		add(p)
	}
	return set, nil
}

// classify sorts candidates into what the snapshot hashes, what it leaves
// out, and the file count and bytes that the limits apply to.
func (s *Store) classify(root string, set candidateSet) (changeScan, int, int64) {
	var scan changeScan
	files, total := 0, int64(0)
	for _, p := range set.paths {
		if strings.HasSuffix(p, "/") {
			// ls-files reports a repository nested in the root as its
			// directory; its contents are not the root's to snapshot.
			scan.nested = append(scan.nested, p)
			continue
		}
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			continue // deleted; nothing to hash
		}
		if info.Mode().IsRegular() && info.Size() > s.limits.MaxFileBytes {
			scan.oversize = append(scan.oversize, p)
			if set.tracked[p] {
				scan.oversizeTracked = append(scan.oversizeTracked, p)
			}
			continue
		}
		if set.forced[p] {
			scan.forced = append(scan.forced, p)
		}
		files++
		total += info.Size()
	}
	return scan, files, total
}

// trackedButIgnored returns files the root's own repository tracks even
// though its ignore rules match them, and that the shadow has not taken in
// yet. Once in the shadow's index they follow later adds like any file.
func (s *Store) trackedButIgnored(ctx context.Context, sh *shadowRepo) ([]string, error) {
	if _, err := os.Lstat(filepath.Join(sh.root, ".git")); err != nil {
		return nil, nil
	}
	// A read-only listing of the user's index (optional locks are off).
	out, _, err := s.git.run(ctx, gitCall{dir: sh.root, args: []string{gitLsFiles, "-z"}})
	if err != nil {
		return nil, nil // not a usable repository; the snapshot does not need it
	}
	tracked := splitNUL(out)
	if len(tracked) == 0 {
		return nil, nil
	}
	call := sh.call("check-ignore", "--no-index", "-z", "--stdin")
	call.stdin = nulList(tracked)
	call.okExit = []int{1}
	call.patternPaths = true
	ignoredOut, _, err := s.git.run(ctx, call)
	if err != nil {
		return nil, err
	}
	ignored := splitNUL(ignoredOut)
	if len(ignored) == 0 {
		return nil, nil
	}
	have, _, err := s.git.run(ctx, sh.call(gitLsFiles, "-z"))
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, p := range splitNUL(have) {
		known[p] = true
	}
	var missing []string
	for _, p := range ignored {
		if !known[p] {
			missing = append(missing, p)
		}
	}
	return missing, nil
}

var addFailure = regexp.MustCompile(`(?m)^error: (?:open\("(.+)"\)|unable to index file '(.+)'|short read while indexing (.+))`)

// addAll runs an add with --ignore-errors. Files git could not read — held
// open with no sharing on Windows, or unreadable — come back as unknown; the
// rest of the snapshot is still recorded.
func (s *Store) addAll(ctx context.Context, sh *shadowRepo, call gitCall) ([]string, error) {
	_, _, err := s.git.run(ctx, call)
	if err == nil {
		return nil, nil
	}
	var gerr *gitError
	if !errors.As(err, &gerr) {
		return nil, err
	}
	var failed []string
	for _, m := range addFailure.FindAllStringSubmatch(gerr.stderr, -1) {
		for _, p := range m[1:] {
			if p != "" {
				failed = append(failed, relativeToRoot(sh.root, p))
			}
		}
	}
	// Some git versions end with "fatal: adding files failed", others only
	// list the files; either way the named files are the whole failure.
	if len(failed) == 0 {
		return nil, err
	}
	return failed, nil
}

// relativeToRoot turns a path from git's error text into a root-relative
// slash path; git reports these relative to the command's directory.
func relativeToRoot(root, p string) string {
	if filepath.IsAbs(p) {
		if rel, ok := relativeInside(root, p); ok {
			return rel
		}
	}
	return filepath.ToSlash(p)
}

func nulList(paths []string) []byte {
	return []byte(strings.Join(paths, "\x00") + "\x00")
}
