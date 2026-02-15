package cron

import (
	"bufio"
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

	cronv3 "github.com/robfig/cron/v3"
)

type Job struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Prompt         string     `json:"prompt"`
	Schedule       string     `json:"schedule"`
	Enabled        bool       `json:"enabled"`
	DeleteAfterRun bool       `json:"delete_after_run,omitempty"`
	LastRunAt      *time.Time `json:"last_run_at,omitempty"`
	LastRunError   string     `json:"last_run_error,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type RunRecord struct {
	JobID   string    `json:"job_id"`
	RanAt   time.Time `json:"ran_at"`
	Error   string    `json:"error,omitempty"`
	Created time.Time `json:"created_at"`
}

type CreateInput struct {
	Name           string
	Prompt         string
	Schedule       string
	Enabled        bool
	HasEnable      bool
	DeleteAfterRun bool
}

type UpdateInput struct {
	Name           *string
	Prompt         *string
	Schedule       *string
	Enabled        *bool
	DeleteAfterRun *bool
}

type Store struct {
	mu       sync.Mutex
	dir      string
	path     string
	runsPath string
}

func NewStore(workspaceDir string) *Store {
	dir := filepath.Join(workspaceDir, "cron")
	return &Store{
		dir:      dir,
		path:     filepath.Join(dir, "jobs.json"),
		runsPath: filepath.Join(dir, "runs.jsonl"),
	}
}

func (s *Store) List() ([]Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

func (s *Store) Create(name, prompt string) (Job, error) {
	return s.CreateWithOptions(CreateInput{
		Name:      name,
		Prompt:    prompt,
		Schedule:  "",
		Enabled:   true,
		HasEnable: true,
	})
}

func (s *Store) CreateWithOptions(input CreateInput) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	name := strings.TrimSpace(input.Name)
	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" {
		return Job{}, fmt.Errorf("prompt is required")
	}
	if name == "" {
		name = defaultJobName(prompt)
	}
	schedule, err := normalizeSchedule(input.Schedule)
	if err != nil {
		return Job{}, err
	}
	enabled := true
	if input.HasEnable {
		enabled = input.Enabled
	}

	jobs, err := s.load()
	if err != nil {
		return Job{}, err
	}
	now := time.Now().UTC()
	job := Job{
		ID:             newJobID(),
		Name:           name,
		Prompt:         prompt,
		Schedule:       schedule,
		Enabled:        enabled,
		DeleteAfterRun: input.DeleteAfterRun,
		CreatedAt:      now,
		UpdatedAt:      now,
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

func (s *Store) Update(id string, input UpdateInput) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobs, err := s.load()
	if err != nil {
		return Job{}, err
	}
	for i := range jobs {
		if jobs[i].ID != id {
			continue
		}
		if input.Name != nil {
			name := strings.TrimSpace(*input.Name)
			if name == "" {
				return Job{}, fmt.Errorf("name is required")
			}
			jobs[i].Name = name
		}
		if input.Prompt != nil {
			prompt := strings.TrimSpace(*input.Prompt)
			if prompt == "" {
				return Job{}, fmt.Errorf("prompt is required")
			}
			jobs[i].Prompt = prompt
		}
		if input.Schedule != nil {
			schedule, err := normalizeSchedule(*input.Schedule)
			if err != nil {
				return Job{}, err
			}
			jobs[i].Schedule = schedule
		}
		if input.Enabled != nil {
			jobs[i].Enabled = *input.Enabled
		}
		if input.DeleteAfterRun != nil {
			jobs[i].DeleteAfterRun = *input.DeleteAfterRun
		}
		jobs[i].UpdatedAt = time.Now().UTC()
		if err := s.save(jobs); err != nil {
			return Job{}, err
		}
		return jobs[i], nil
	}
	return Job{}, fmt.Errorf("job not found: %s", id)
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobs, err := s.load()
	if err != nil {
		return err
	}
	filtered := make([]Job, 0, len(jobs))
	found := false
	for _, job := range jobs {
		if job.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, job)
	}
	if !found {
		return fmt.Errorf("job not found: %s", id)
	}
	if err := s.save(filtered); err != nil {
		return err
	}
	return s.pruneRunsForJob(id)
}

func (s *Store) MarkRunResult(id string, ranAt time.Time, runErr error) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobs, err := s.load()
	if err != nil {
		return Job{}, err
	}
	for i := range jobs {
		if jobs[i].ID != id {
			continue
		}
		ran := ranAt.UTC()
		jobs[i].LastRunAt = &ran
		if runErr != nil {
			jobs[i].LastRunError = strings.TrimSpace(runErr.Error())
		} else {
			jobs[i].LastRunError = ""
		}
		jobs[i].UpdatedAt = ran
		record := RunRecord{
			JobID:   id,
			RanAt:   ran,
			Error:   jobs[i].LastRunError,
			Created: ran,
		}
		if err := s.appendRunRecord(record); err != nil {
			return Job{}, err
		}
		if jobs[i].DeleteAfterRun {
			filtered := make([]Job, 0, len(jobs)-1)
			for _, job := range jobs {
				if job.ID != id {
					filtered = append(filtered, job)
				}
			}
			if err := s.save(filtered); err != nil {
				return Job{}, err
			}
			return jobs[i], nil
		}
		if err := s.save(jobs); err != nil {
			return Job{}, err
		}
		return jobs[i], nil
	}
	return Job{}, fmt.Errorf("job not found: %s", id)
}

func (s *Store) ListRuns(id string, limit int) ([]RunRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	runs, err := s.loadRuns()
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	filtered := make([]RunRecord, 0, min(limit, len(runs)))
	for i := len(runs) - 1; i >= 0; i-- {
		if runs[i].JobID != id {
			continue
		}
		filtered = append(filtered, runs[i])
		if len(filtered) >= limit {
			break
		}
	}
	return filtered, nil
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

func (s *Store) loadRuns() ([]RunRecord, error) {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return nil, fmt.Errorf("create cron directory: %w", err)
	}
	f, err := os.Open(s.runsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []RunRecord{}, nil
		}
		return nil, fmt.Errorf("open cron runs: %w", err)
	}
	defer f.Close()

	records := make([]RunRecord, 0, 64)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec RunRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("decode cron run: %w", err)
		}
		records = append(records, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan cron runs: %w", err)
	}
	return records, nil
}

func (s *Store) appendRunRecord(record RunRecord) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create cron directory: %w", err)
	}
	f, err := os.OpenFile(s.runsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open cron runs: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode cron run: %w", err)
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write cron run: %w", err)
	}
	return nil
}

func (s *Store) pruneRunsForJob(jobID string) error {
	runs, err := s.loadRuns()
	if err != nil {
		return err
	}
	kept := make([]RunRecord, 0, len(runs))
	for _, rec := range runs {
		if rec.JobID != jobID {
			kept = append(kept, rec)
		}
	}
	f, err := os.OpenFile(s.runsPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open cron runs: %w", err)
	}
	defer f.Close()
	for _, rec := range kept {
		data, err := json.Marshal(rec)
		if err != nil {
			return fmt.Errorf("encode cron run: %w", err)
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			return fmt.Errorf("write cron run: %w", err)
		}
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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

func normalizeSchedule(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "every:1h", nil
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "every:") {
		dur := strings.TrimSpace(s[len("every:"):])
		if dur == "" {
			return "", fmt.Errorf("invalid schedule: %s (expected every:<duration> or valid cron expression)", s)
		}
		if _, err := time.ParseDuration(dur); err != nil {
			return "", fmt.Errorf("invalid schedule: %s (expected every:<duration> or valid cron expression)", s)
		}
		return "every:" + dur, nil
	}
	if strings.HasPrefix(lower, "@every ") {
		dur := strings.TrimSpace(s[len("@every "):])
		if _, err := time.ParseDuration(dur); err != nil {
			return "", fmt.Errorf("invalid schedule: %s (expected every:<duration> or valid cron expression)", s)
		}
		return "@every " + dur, nil
	}
	if _, err := cronv3.ParseStandard(s); err != nil {
		return "", fmt.Errorf("invalid schedule: %s (expected every:<duration> or valid cron expression)", s)
	}
	return s, nil
}

func newJobID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("job_%d", time.Now().UTC().UnixNano())
	}
	return "job_" + hex.EncodeToString(b[:])
}
