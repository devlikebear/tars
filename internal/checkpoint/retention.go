package checkpoint

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// dropTurnRefs deletes the refs of turns that fell out of a session's
// window. Their objects go at the next sweep's gc.
func (s *Store) dropTurnRefs(ctx context.Context, sessionID string, dropped []Entry) {
	byShadow := map[string][]string{}
	for _, e := range dropped {
		if e.Shadow == "" {
			continue
		}
		byShadow[e.Shadow] = append(byShadow[e.Shadow], turnRef(sessionID, e.TurnID, "start"), turnRef(sessionID, e.TurnID, "end"))
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

// existingRefs keeps the refs that exist, since update-ref --stdin refuses to
// delete a missing one.
func (s *Store) existingRefs(ctx context.Context, sh *shadowRepo, refs []string) []string {
	all, err := s.refsUnder(ctx, sh, refNamespace)
	if err != nil {
		return nil
	}
	have := map[string]bool{}
	for _, r := range all {
		have[r] = true
	}
	var out []string
	for _, r := range refs {
		if have[r] {
			out = append(out, r)
		}
	}
	return out
}

func (s *Store) refsUnder(ctx context.Context, sh *shadowRepo, prefix string) ([]string, error) {
	out, _, err := s.git.run(ctx, sh.call("for-each-ref", "--format=%(refname)", prefix))
	if err != nil {
		return nil, err
	}
	var refs []string
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			refs = append(refs, line)
		}
	}
	return refs, nil
}

// DropSession removes every checkpoint of a session: its refs in each shadow
// it used and its index. Call it when the session is deleted.
func (s *Store) DropSession(ctx context.Context, sessionID string) error {
	if err := validID("session", sessionID); err != nil {
		return err
	}
	unlock := s.lockSession(sessionID)
	defer unlock()
	idx, err := s.readIndex(sessionID)
	if err != nil {
		return err
	}
	shadows := map[string]bool{}
	for _, e := range idx.Turns {
		if e.Shadow != "" {
			shadows[e.Shadow] = true
		}
	}
	for key := range shadows {
		if err := s.dropSessionRefs(ctx, key, sessionID); err != nil {
			return err
		}
	}
	if err := os.Remove(s.indexPath(sessionID)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (s *Store) dropSessionRefs(ctx context.Context, key, sessionID string) error {
	sh, err := s.openShadow(key)
	if err != nil {
		return nil // already gone
	}
	unlock := s.lockRoot(key)
	defer unlock()
	refs, err := s.refsUnder(ctx, sh, sessionRefPrefix(sessionID))
	if err != nil {
		return err
	}
	return s.deleteRefs(ctx, sh, refs)
}

// SweepOrphans drops checkpoints of sessions that no longer exist, removes
// shadows no session refers to, and compacts the rest. alive reports whether
// a session still exists. Run it in the background at startup.
func (s *Store) SweepOrphans(ctx context.Context, alive func(sessionID string) bool) error {
	indexes, err := os.ReadDir(filepath.Join(s.dir, "sessions"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, f := range indexes {
		sessionID, ok := strings.CutSuffix(f.Name(), ".json")
		if !ok || f.IsDir() || validID("session", sessionID) != nil {
			continue
		}
		if !alive(sessionID) {
			if err := s.DropSession(ctx, sessionID); err != nil {
				return err
			}
		}
	}
	shadows, err := os.ReadDir(filepath.Join(s.dir, "shadow"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, d := range shadows {
		key, ok := strings.CutSuffix(d.Name(), ".git")
		if !ok || !d.IsDir() {
			continue
		}
		if err := s.sweepShadow(ctx, key, alive); err != nil {
			return err
		}
	}
	return nil
}

// looseObjectsBeforeGC is how many loose objects a shadow may collect before
// a sweep packs it and prunes what dropped refs left behind.
const looseObjectsBeforeGC = 2000

func (s *Store) sweepShadow(ctx context.Context, key string, alive func(string) bool) error {
	gitDir := filepath.Join(s.dir, "shadow", key+".git")
	sh, err := s.openShadow(key)
	if err != nil {
		// No marker: an interrupted initialization. Nothing refers to it.
		return removeAllWritable(gitDir)
	}
	unlock := s.lockRoot(key)
	defer unlock()
	refs, err := s.refsUnder(ctx, sh, refNamespace)
	if err != nil {
		return err
	}
	var stale []string
	live := 0
	for _, ref := range refs {
		rest := strings.TrimPrefix(ref, refNamespace)
		sessionID, _, _ := strings.Cut(rest, "/")
		if s.isActive(sessionID) {
			live++
			continue
		}
		if _, err := os.Stat(s.indexPath(sessionID)); err != nil || !alive(sessionID) {
			stale = append(stale, ref)
			continue
		}
		live++
	}
	if err := s.deleteRefs(ctx, sh, stale); err != nil {
		return err
	}
	if live == 0 {
		return removeAllWritable(gitDir)
	}
	out, _, err := s.git.run(ctx, sh.call("count-objects", "-v"))
	if err != nil {
		return err
	}
	if looseObjects(string(out)) > looseObjectsBeforeGC {
		if _, _, err := s.git.run(ctx, sh.call("gc", "--prune=now", "--quiet")); err != nil {
			return err
		}
	}
	return nil
}

func looseObjects(countObjects string) int {
	for _, line := range strings.Split(countObjects, "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), "count:"); ok {
			n, _ := strconv.Atoi(strings.TrimSpace(value))
			return n
		}
	}
	return 0
}
