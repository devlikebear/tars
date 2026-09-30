package sessionworktree

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/pkg/session"
)

// Copying gitignored files and folders into a new worktree.
//
// A worktree starts without the checkout's gitignored files, so a session
// could not run the project's tools until it reinstalled its dependencies.
// worktree_include names what to bring along. Each entry is copied, never
// linked: a linked node_modules let an install in the worktree empty the
// checkout's. Where the file system supports it (APFS, Btrfs, XFS) each file
// is a copy-on-write clone, so a dependency folder costs almost no time or
// space. Elsewhere files are copied up to copyLimit bytes per worktree.
//
// The list comes from the repository's own .tars settings, so it can only
// reach inside the repository: entries are relative to its root, may not
// climb out with "..", may not pass through a symlink, and symlinks inside a
// copied folder are kept only when they point inside the repository too.
//
// Copies are made in a staging folder next to the worktree and moved into
// place when complete, so a session never sees half a folder. Create waits
// includeWait for them; what is not done by then keeps going in the
// background and is reported through OnIncludeDone.

const (
	defaultIncludeWait = 3 * time.Second
	defaultCopyLimit   = int64(1) << 30
)

// errCloneUnsupported means the platform or file system cannot clone.
var errCloneUnsupported = errors.New("copy-on-write clones are not supported here")

var errCopyLimit = errors.New("copy limit reached")

// IncludeDone reports the include entries that finished after Create had
// returned them as pending.
type IncludeDone struct {
	SessionID string
	Worktree  session.SessionWorktree
	Copied    []string
	Skipped   []string
}

// OnIncludeDone registers fn to hear about background copies as they
// finish. It runs on the copying goroutine.
func (m *Manager) OnIncludeDone(fn func(IncludeDone)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onInclude = fn
}

// includeItem is one repository path to copy.
type includeItem struct {
	rel string // slash-separated, relative to the repository root
}

type includeOutcome struct {
	rel     string
	copied  bool
	skipped []string
}

type includeJob struct {
	cancel context.CancelFunc
	done   chan struct{}

	mu       sync.Mutex
	outcomes []includeOutcome
	detached bool // Create returned before the job finished
}

// planIncludes turns the configured entries into repository paths, and the
// entries it refuses into skip reasons.
func planIncludes(repo string, include []string) (items []includeItem, skipped []string) {
	seen := map[string]bool{}
	for _, entry := range include {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if !includeEntryInside(entry) {
			skipped = append(skipped, entry+": must be a path inside the repository")
			continue
		}
		clean := filepath.Clean(filepath.FromSlash(entry))
		matches, err := filepath.Glob(filepath.Join(repo, clean))
		if err != nil || len(matches) == 0 {
			skipped = append(skipped, entry+": not found")
			continue
		}
		sort.Strings(matches)
		for _, match := range matches {
			rel, err := filepath.Rel(repo, match)
			if err != nil || !includeEntryInside(filepath.ToSlash(rel)) {
				skipped = append(skipped, entry+": must be a path inside the repository")
				continue
			}
			if linked, err := symlinkOnPath(repo, rel); err != nil || linked {
				skipped = append(skipped, filepath.ToSlash(rel)+": symlinks are not followed")
				continue
			}
			key := filepath.ToSlash(rel)
			if !seen[key] {
				seen[key] = true
				items = append(items, includeItem{rel: key})
			}
		}
	}
	return items, skipped
}

// includeEntryInside reports whether a slash- or OS-separated relative path
// stays inside the repository and out of its .git folder.
func includeEntryInside(entry string) bool {
	slashed := filepath.ToSlash(entry)
	if strings.HasPrefix(slashed, "/") || filepath.IsAbs(entry) || filepath.VolumeName(entry) != "" {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(slashed)))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return false
	}
	first := strings.SplitN(clean, "/", 2)[0]
	return first != ".git"
}

// symlinkOnPath reports whether any existing component of rel under root is
// a symlink. Components that do not exist yet are fine.
func symlinkOnPath(root, rel string) (bool, error) {
	current := root
	for _, part := range strings.Split(filepath.Clean(rel), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return true, nil
		}
	}
	return false, nil
}

// stagingDir is where a worktree's includes are assembled before they move
// into place: beside the worktree, on the same file system.
func (m *Manager) stagingDir(wt session.SessionWorktree) string {
	return filepath.Join(m.root, repoKey(wt.RepoRoot), filepath.Base(wt.Path)+".include")
}

// startIncludes copies items into wt, waits up to includeWait, and returns
// what finished and what is still running.
func (m *Manager) startIncludes(sessionID string, wt session.SessionWorktree, items []includeItem) (copied, skipped, pending []string) {
	if len(items) == 0 {
		return nil, nil, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	job := &includeJob{cancel: cancel, done: make(chan struct{})}
	m.mu.Lock()
	m.jobs[wt.Path] = job
	m.mu.Unlock()
	go m.runIncludes(ctx, sessionID, wt, items, job)

	timer := time.NewTimer(m.includeWait)
	defer timer.Stop()
	select {
	case <-job.done:
	case <-timer.C:
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	for _, outcome := range job.outcomes {
		if outcome.copied {
			copied = append(copied, outcome.rel)
		}
		skipped = append(skipped, outcome.skipped...)
	}
	for _, item := range items[len(job.outcomes):] {
		pending = append(pending, item.rel)
	}
	job.detached = len(pending) > 0
	return copied, skipped, pending
}

func (m *Manager) runIncludes(ctx context.Context, sessionID string, wt session.SessionWorktree, items []includeItem, job *includeJob) {
	staging := m.stagingDir(wt)
	budget := &copyBudget{left: m.copyLimit, clone: m.clone}
	var late []includeOutcome
	for i, item := range items {
		outcome := m.includeOne(ctx, wt, filepath.Join(staging, fmt.Sprint(i)), item, budget)
		job.mu.Lock()
		job.outcomes = append(job.outcomes, outcome)
		if job.detached {
			late = append(late, outcome)
		}
		job.mu.Unlock()
	}
	_ = os.RemoveAll(staging)

	m.mu.Lock()
	if m.jobs[wt.Path] == job {
		delete(m.jobs, wt.Path)
	}
	notify := m.onInclude
	m.mu.Unlock()
	if len(late) > 0 && notify != nil {
		done := IncludeDone{SessionID: sessionID, Worktree: wt}
		for _, outcome := range late {
			if outcome.copied {
				done.Copied = append(done.Copied, outcome.rel)
			}
			done.Skipped = append(done.Skipped, outcome.skipped...)
		}
		notify(done)
	}
	close(job.done)
}

// includeOne copies one item into staging and moves it into the worktree.
func (m *Manager) includeOne(ctx context.Context, wt session.SessionWorktree, staging string, item includeItem, budget *copyBudget) includeOutcome {
	outcome := includeOutcome{rel: item.rel}
	fail := func(reason string) includeOutcome {
		_ = os.RemoveAll(staging)
		outcome.skipped = append(outcome.skipped, item.rel+": "+reason)
		return outcome
	}
	rel := filepath.FromSlash(item.rel)
	if m.copyHook != nil {
		m.copyHook(ctx)
	}
	if ctx.Err() != nil {
		return fail("canceled")
	}
	if err := os.MkdirAll(filepath.Dir(staging), 0o700); err != nil {
		return fail(err.Error())
	}
	dropped, err := copyEntry(ctx, filepath.Join(wt.RepoRoot, rel), staging, item.rel, budget)
	switch {
	case ctx.Err() != nil:
		return fail("canceled")
	case errors.Is(err, errCopyLimit):
		return fail(fmt.Sprintf("over the %d MiB copy limit for file systems without copy-on-write clones", m.copyLimit>>20))
	case err != nil:
		return fail(err.Error())
	}
	target := filepath.Join(wt.Path, rel)
	if linked, err := symlinkOnPath(wt.Path, rel); err != nil || linked {
		return fail("symlinks are not followed")
	}
	if _, err := os.Lstat(target); err == nil {
		return fail("already exists in the worktree; left as it is")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fail(err.Error())
	}
	if err := os.Rename(staging, target); err != nil {
		return fail(err.Error())
	}
	outcome.copied = true
	if dropped > 0 {
		noun := "symlinks"
		if dropped == 1 {
			noun = "symlink"
		}
		outcome.skipped = append(outcome.skipped, fmt.Sprintf("%s: %d %s pointing outside the repository not copied", item.rel, dropped, noun))
	}
	return outcome
}

// stopIncludes cancels the worktree's background copy and waits for it,
// so nothing lands in a worktree that is being removed.
func (m *Manager) stopIncludes(wt session.SessionWorktree) {
	m.mu.Lock()
	job := m.jobs[wt.Path]
	m.mu.Unlock()
	if job != nil {
		job.cancel()
		<-job.done
	}
	_ = os.RemoveAll(m.stagingDir(wt))
}

// copyBudget limits how many bytes are copied without a clone.
type copyBudget struct {
	left  int64
	clone func(src, dst string) error
	// noClone is set after a clone fails, so a folder on a file system
	// without clones does not try once per file.
	noClone bool
}

func (b *copyBudget) file(src, dst string, size int64) error {
	if !b.noClone && b.clone != nil {
		err := b.clone(src, dst)
		if err == nil {
			return nil
		}
		_ = os.Remove(dst)
		b.noClone = true
	}
	if size > b.left {
		return errCopyLimit
	}
	b.left -= size
	return plainCopy(src, dst)
}

// copyEntry copies src, a file or folder at relative path rel in the
// repository, to dst. Symlinks are recreated only when they are relative and
// resolve inside the repository; the others are dropped and counted.
func copyEntry(ctx context.Context, src, dst, rel string, budget *copyBudget) (dropped int, err error) {
	info, err := os.Lstat(src)
	if err != nil {
		return 0, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return 0, errors.New("symlinks are not followed")
	}
	if !info.IsDir() {
		if !info.Mode().IsRegular() {
			return 0, errors.New("not a regular file")
		}
		return 0, budget.file(src, dst, info.Size())
	}
	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		sub, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, sub)
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o755)
		case d.Type()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil || !linkStaysInside(filepath.Join(filepath.FromSlash(rel), sub), link) || os.Symlink(link, target) != nil {
				dropped++
			}
			return nil
		case d.Type().IsRegular():
			fi, err := d.Info()
			if err != nil {
				return err
			}
			return budget.file(path, target, fi.Size())
		default:
			// Sockets, pipes and devices have no place in a worktree.
			return nil
		}
	})
	return dropped, err
}

// linkStaysInside reports whether a symlink at repository path linkPath with
// the given target resolves inside the repository, outside .git.
func linkStaysInside(linkPath, target string) bool {
	if target == "" || filepath.IsAbs(target) || filepath.VolumeName(target) != "" || strings.HasPrefix(filepath.ToSlash(target), "/") {
		return false
	}
	resolved := filepath.Join(filepath.Dir(linkPath), target)
	return includeEntryInside(filepath.ToSlash(resolved))
}

// plainCopy copies a regular file's bytes and permission bits.
func plainCopy(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
