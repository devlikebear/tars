package cron

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Job struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Prompt    string    `json:"prompt"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Store struct {
	mu   sync.Mutex
	dir  string
	path string
}

func NewStore(workspaceDir string) *Store {
	dir := filepath.Join(workspaceDir, "cron")
	return &Store{
		dir:  dir,
		path: filepath.Join(dir, "jobs.json"),
	}
}

func (s *Store) List() ([]Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

func (s *Store) Create(name, prompt string) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	name = strings.TrimSpace(name)
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return Job{}, fmt.Errorf("prompt is required")
	}
	if name == "" {
		name = defaultJobName(prompt)
	}

	jobs, err := s.load()
	if err != nil {
		return Job{}, err
	}
	now := time.Now().UTC()
	job := Job{
		ID:        newJobID(),
		Name:      name,
		Prompt:    prompt,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	jobs = append(jobs, job)
	if err := s.save(jobs); err != nil {
		return Job{}, err
	}
	return job, nil
}

func (s *Store) Get(id string) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobs, err := s.load()
	if err != nil {
		return Job{}, err
	}
	for _, job := range jobs {
		if job.ID == id {
			return job, nil
		}
	}
	return Job{}, fmt.Errorf("job not found: %s", id)
}

func (s *Store) load() ([]Job, error) {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return nil, fmt.Errorf("create cron directory: %w", err)
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Job{}, nil
		}
		return nil, fmt.Errorf("read cron jobs: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return []Job{}, nil
	}
	var jobs []Job
	if err := json.Unmarshal(data, &jobs); err != nil {
		return nil, fmt.Errorf("decode cron jobs: %w", err)
	}
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].CreatedAt.Before(jobs[j].CreatedAt)
	})
	return jobs, nil
}

func (s *Store) save(jobs []Job) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create cron directory: %w", err)
	}
	payload, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return fmt.Errorf("encode cron jobs: %w", err)
	}
	if err := os.WriteFile(s.path, append(payload, '\n'), 0o644); err != nil {
		return fmt.Errorf("write cron jobs: %w", err)
	}
	return nil
}

func defaultJobName(prompt string) string {
	if prompt == "" {
		return "cron job"
	}
	line := strings.TrimSpace(strings.Split(prompt, "\n")[0])
	if line == "" {
		return "cron job"
	}
	if len(line) > 48 {
		return line[:48] + "..."
	}
	return line
}

func newJobID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("job_%d", time.Now().UTC().UnixNano())
	}
	return "job_" + hex.EncodeToString(b[:])
}
