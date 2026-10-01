package focuspipeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/devlikebear/tars/internal/atomicwrite"
)

// fileSuffix names a pipeline file next to its session's transcript.
const fileSuffix = ".pipeline.json"

// ErrInvalidSessionID is a session id that could name a path outside the
// sessions folder.
var ErrInvalidSessionID = errors.New("invalid session id")

// Store keeps one pipeline per session as <sessions>/<id>.pipeline.json.
type Store struct {
	dir string
}

// dirLocks serializes read-modify-write cycles per sessions folder, so two
// Store values over the same folder (one per request) still agree.
var dirLocks sync.Map // dir → *sync.Mutex

// NewStore keeps pipelines in sessionsDir (<workspace>/sessions).
func NewStore(sessionsDir string) *Store {
	return &Store{dir: sessionsDir}
}

func (s *Store) lock() func() {
	mu, _ := dirLocks.LoadOrStore(filepath.Clean(s.dir), &sync.Mutex{})
	m := mu.(*sync.Mutex)
	m.Lock()
	return m.Unlock
}

func (s *Store) path(sessionID string) (string, error) {
	id := strings.TrimSpace(sessionID)
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, `/\`) || strings.ContainsRune(id, 0) {
		return "", ErrInvalidSessionID
	}
	return filepath.Join(s.dir, id+fileSuffix), nil
}

// Get reads a session's pipeline; false when it has none.
func (s *Store) Get(sessionID string) (Pipeline, bool, error) {
	path, err := s.path(sessionID)
	if err != nil {
		return Pipeline{}, false, err
	}
	return readPipeline(path)
}

func readPipeline(path string) (Pipeline, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Pipeline{}, false, nil
		}
		return Pipeline{}, false, err
	}
	var p Pipeline
	if err := json.Unmarshal(raw, &p); err != nil {
		return Pipeline{}, false, fmt.Errorf("read pipeline %s: %w", filepath.Base(path), err)
	}
	if p.Cards == nil {
		p.Cards = []Card{}
	}
	return p, true, nil
}

// Save writes a pipeline atomically.
func (s *Store) Save(p Pipeline) error {
	unlock := s.lock()
	defer unlock()
	return s.save(p)
}

func (s *Store) save(p Pipeline) error {
	path, err := s.path(p.SessionID)
	if err != nil {
		return err
	}
	if p.Cards == nil {
		p.Cards = []Card{}
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return atomicwrite.Write(path, raw)
}

// Update reads a session's pipeline, applies fn and saves the result, all
// under the folder's lock, so a turn's hook and a gate action cannot lose
// each other's change. fn must not call into the session store (or anything
// else that takes a lock): the session store's delete path calls into this
// package with its index lock held. fn's error is returned as is and nothing is saved;
// the pipeline fn returned is returned either way. ok is false when the
// session has no pipeline (fn is not called).
func (s *Store) Update(sessionID string, fn func(Pipeline) (Pipeline, error)) (Pipeline, bool, error) {
	unlock := s.lock()
	defer unlock()
	current, ok, err := s.Get(sessionID)
	if err != nil || !ok {
		return current, ok, err
	}
	next, err := fn(current)
	if err != nil {
		return next, true, err
	}
	if err := s.save(next); err != nil {
		return current, true, err
	}
	return next, true, nil
}

// Delete removes a session's pipeline; a missing one is not an error.
//
// It deliberately does not take the folder lock: it runs from the session
// store's delete hook with the session index lock held, and waiting for an
// Update there could deadlock. Removing the file is atomic on its own; an
// Update racing a delete can at worst rewrite the file of a session that is
// gone, which List callers skip and the startup sweep removes.
func (s *Store) Delete(sessionID string) error {
	path, err := s.path(sessionID)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// List returns every pipeline, most recently updated first. Unreadable
// files are skipped.
func (s *Store) List() ([]Pipeline, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Pipeline{}, nil
		}
		return nil, err
	}
	out := []Pipeline{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), fileSuffix) {
			continue
		}
		p, ok, err := readPipeline(filepath.Join(s.dir, e.Name()))
		if err != nil || !ok {
			continue
		}
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
