package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Session struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
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
	now := time.Now().UTC()
	session := Session{
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return Session{}, fmt.Errorf("create sessions directory: %w", err)
	}

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

func (s *Store) Get(id string) (Session, error) {
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
	index, err := s.loadIndex()
	if err != nil {
		return nil, err
	}

	sessions := make([]Session, 0, len(index))
	for _, session := range index {
		sessions = append(sessions, session)
	}

	return sessions, nil
}

func (s *Store) Delete(id string) error {
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
