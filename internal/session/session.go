package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Session struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Kind      string    `json:"kind,omitempty"`
	Hidden    bool      `json:"hidden,omitempty"`
	ProjectID string    `json:"project_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Store struct {
	dir string
}

func NewStore(dir string) *Store {
	return &Store{
		dir: filepath.Join(dir, "sessions"),
	}
}

func (s *Store) Create(title string) (Session, error) {
	return s.CreateWithOptions(title, "", false)
}

func (s *Store) CreateWithOptions(title string, kind string, hidden bool) (Session, error) {
	now := time.Now().UTC()
	session := Session{
		Title:     title,
		Kind:      strings.TrimSpace(kind),
		Hidden:    hidden,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return Session{}, fmt.Errorf("create sessions directory: %w", err)
	}

	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return Session{}, err
	}

	for {
		id, err := generateID()
		if err != nil {
			return Session{}, err
		}
		if _, exists := index[id]; exists {
			continue
		}
		session.ID = id
		break
	}

	index[session.ID] = session

	if err := s.saveIndex(index); err != nil {
		return Session{}, err
	}

	return session, nil
}

func (s *Store) EnsureMain() (Session, error) {
	return s.ensureNamedSession("main", "main", false)
}

func (s *Store) EnsureWorker(projectID string) (Session, error) {
	id := strings.TrimSpace(projectID)
	if id == "" {
		return Session{}, fmt.Errorf("project id is required")
	}
	return s.ensureNamedSession("worker:"+id, "worker", true)
}

func (s *Store) ensureNamedSession(title string, kind string, hidden bool) (Session, error) {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return Session{}, fmt.Errorf("create sessions directory: %w", err)
	}
	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return Session{}, err
	}
	trimmedTitle := strings.TrimSpace(title)
	trimmedKind := strings.TrimSpace(kind)
	for id, sess := range index {
		if strings.TrimSpace(sess.Kind) == trimmedKind && strings.TrimSpace(sess.Title) == trimmedTitle {
			sess.Hidden = hidden
			if sess.CreatedAt.IsZero() {
				sess.CreatedAt = time.Now().UTC()
			}
			if sess.UpdatedAt.IsZero() {
				sess.UpdatedAt = sess.CreatedAt
			}
			index[id] = sess
			if err := s.saveIndex(index); err != nil {
				return Session{}, err
			}
			return sess, nil
		}
	}
	now := time.Now().UTC()
	created := Session{
		Title:     trimmedTitle,
		Kind:      trimmedKind,
		Hidden:    hidden,
		CreatedAt: now,
		UpdatedAt: now,
	}
	for {
		id, err := generateID()
		if err != nil {
			return Session{}, err
		}
		if _, exists := index[id]; exists {
			continue
		}
		created.ID = id
		break
	}
	index[created.ID] = created
	if err := s.saveIndex(index); err != nil {
		return Session{}, err
	}
	return created, nil
}

func (s *Store) Get(id string) (Session, error) {
	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return Session{}, err
	}

	session, ok := index[id]
	if !ok {
		return Session{}, fmt.Errorf("session not found")
	}

	return session, nil
}

func (s *Store) List() ([]Session, error) {
	return s.list(false)
}

func (s *Store) ListAll() ([]Session, error) {
	return s.list(true)
}

func (s *Store) list(includeHidden bool) ([]Session, error) {
	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return nil, err
	}

	sessions := make([]Session, 0, len(index))
	for _, session := range index {
		if session.Hidden && !includeHidden {
			continue
		}
		sessions = append(sessions, session)
	}

	return sessions, nil
}

func (s *Store) Touch(id string, updatedAt time.Time) error {
	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return err
	}
	sess, ok := index[id]
	if !ok {
		return fmt.Errorf("session not found")
	}
	sess.UpdatedAt = updatedAt.UTC()
	index[id] = sess
	return s.saveIndex(index)
}

// SetTitle renames a session.
func (s *Store) SetTitle(id string, title string) error {
	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return err
	}
	sess, ok := index[id]
	if !ok {
		return fmt.Errorf("session not found")
	}
	sess.Title = strings.TrimSpace(title)
	sess.UpdatedAt = time.Now().UTC()
	index[id] = sess
	return s.saveIndex(index)
}

func (s *Store) SetProjectID(id string, projectID string) error {
	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return err
	}
	sess, ok := index[id]
	if !ok {
		return fmt.Errorf("session not found")
	}
	sess.ProjectID = strings.TrimSpace(projectID)
	sess.UpdatedAt = time.Now().UTC()
	index[id] = sess
	return s.saveIndex(index)
}

func (s *Store) Latest() (Session, error) {
	return s.latest(false)
}

func (s *Store) LatestAll() (Session, error) {
	return s.latest(true)
}

func (s *Store) latest(includeHidden bool) (Session, error) {
	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return Session{}, err
	}
	var latest Session
	hasLatest := false
	for _, sess := range index {
		if sess.Hidden && !includeHidden {
			continue
		}
		if !hasLatest {
			latest = sess
			hasLatest = true
			continue
		}
		switch {
		case sess.UpdatedAt.After(latest.UpdatedAt):
			latest = sess
		case sess.UpdatedAt.Equal(latest.UpdatedAt) && sess.CreatedAt.After(latest.CreatedAt):
			latest = sess
		}
	}
	if !hasLatest {
		return Session{}, fmt.Errorf("session not found")
	}
	return latest, nil
}

func (s *Store) Delete(id string) error {
	unlock := lockPath(s.indexPath())
	defer unlock()
	index, err := s.loadIndex()
	if err != nil {
		return err
	}

	if _, ok := index[id]; !ok {
		return nil
	}

	delete(index, id)
	if err := s.saveIndex(index); err != nil {
		return err
	}

	_ = os.Remove(s.TranscriptPath(id))

	return nil
}

func (s *Store) TranscriptPath(id string) string {
	return filepath.Join(s.dir, id+".jsonl")
}

func (s *Store) indexPath() string {
	return filepath.Join(s.dir, "sessions.json")
}

func (s *Store) WorkspaceDir() string {
	if s == nil {
		return ""
	}
	return filepath.Dir(s.dir)
}

func (s *Store) loadIndex() (map[string]Session, error) {
	raw, err := os.ReadFile(s.indexPath())
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]Session{}, nil
		}
		return nil, err
	}

	index := make(map[string]Session)
	if len(raw) == 0 {
		return index, nil
	}

	if err := json.Unmarshal(raw, &index); err != nil {
		return nil, fmt.Errorf("load sessions index: %w", err)
	}

	return index, nil
}

func (s *Store) saveIndex(index map[string]Session) error {
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.indexPath(), data, 0o644)
}

func generateID() (string, error) {
	raw := make([]byte, 8)
	n, err := rand.Read(raw)
	if err != nil {
		return "", err
	}
	if n != len(raw) {
		return "", fmt.Errorf("failed to generate random id")
	}
	return hex.EncodeToString(raw), nil
}
