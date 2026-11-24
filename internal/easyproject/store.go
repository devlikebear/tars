package easyproject

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// StoreFile is where the projects are recorded, under the workspace.
const StoreFile = "_shared/easy/projects.json"

// ErrNotFound is a project id the store does not know.
var ErrNotFound = errors.New("project not found")

// Store keeps the projects in one JSON file. A workspace has one store;
// the mutex makes each read-modify-write one step.
type Store struct {
	path string
	mu   sync.Mutex
}

// NewStore is the store of a workspace.
func NewStore(workspaceDir string) *Store {
	return &Store{path: filepath.Join(workspaceDir, filepath.FromSlash(StoreFile))}
}

type storeFile struct {
	Projects []Project `json:"projects"`
}

func (s *Store) read() ([]Project, error) {
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f storeFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	return f.Projects, nil
}

func (s *Store) write(list []Project) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(storeFile{Projects: list}, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// List returns the projects, newest first.
func (s *Store) List() ([]Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.read()
	if err != nil {
		return nil, err
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
	return list, nil
}

// Get returns one project.
func (s *Store) Get(id string) (Project, error) {
	if !validID(id) {
		return Project{}, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.read()
	if err != nil {
		return Project{}, err
	}
	for _, p := range list {
		if p.ID == id {
			return p, nil
		}
	}
	return Project{}, ErrNotFound
}

// Put adds a project or replaces the one with its id.
func (s *Store) Put(p Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.read()
	if err != nil {
		return err
	}
	for i := range list {
		if list[i].ID == p.ID {
			list[i] = p
			return s.write(list)
		}
	}
	return s.write(append(list, p))
}

// MarkInterrupted fails every project still creating: called at start-up,
// when no creation from before can still be running. It returns how many
// it changed.
func (s *Store) MarkInterrupted(now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.read()
	if err != nil {
		return 0, err
	}
	n := 0
	for i := range list {
		if list[i].Status == StatusCreating {
			list[i].Status, list[i].ErrorCode, list[i].UpdatedAt = StatusFailed, ErrorInterrupted, now.UTC()
			n++
		}
	}
	if n == 0 {
		return 0, nil
	}
	return n, s.write(list)
}

// Update replaces the project with id by what change answers, in one step.
func (s *Store) Update(id string, change func(Project) (Project, error)) (Project, error) {
	if !validID(id) {
		return Project{}, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.read()
	if err != nil {
		return Project{}, err
	}
	for i := range list {
		if list[i].ID != id {
			continue
		}
		next, err := change(list[i])
		if err != nil {
			return Project{}, err
		}
		next.ID = id
		list[i] = next
		return next, s.write(list)
	}
	return Project{}, ErrNotFound
}

// Remove drops the project with id from the record. Its folder is not the
// store's: DeleteFolder moves it away first.
func (s *Store) Remove(id string) error {
	if !validID(id) {
		return ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.read()
	if err != nil {
		return err
	}
	for i := range list {
		if list[i].ID == id {
			return s.write(append(list[:i:i], list[i+1:]...))
		}
	}
	return ErrNotFound
}
