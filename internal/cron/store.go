package cron

import (
	"bufio"
	"bytes"
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

const defaultRunHistoryLimit = 200

type Job struct {
	ID                  string          `json:"id"`
	Name                string          `json:"name"`
	Prompt              string          `json:"prompt"`
	Schedule            string          `json:"schedule"`
	Enabled             bool            `json:"enabled"`
	SessionTarget       string          `json:"session_target,omitempty"`
	WakeMode            string          `json:"wake_mode,omitempty"`
	DeliveryMode        string          `json:"delivery_mode,omitempty"`
	Payload             json.RawMessage `json:"payload,omitempty"`
	DeleteAfterRun      bool            `json:"delete_after_run,omitempty"`
	LastRunAt           *time.Time      `json:"last_run_at,omitempty"`
	LastRunError        string          `json:"last_run_error,omitempty"`
	ConsecutiveFailures int             `json:"consecutive_failures,omitempty"`
	BackoffUntil        *time.Time      `json:"backoff_until,omitempty"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

type RunRecord struct {
	JobID    string    `json:"job_id"`
	RanAt    time.Time `json:"ran_at"`
	Response string    `json:"response,omitempty"`
	Error    string    `json:"error,omitempty"`
	Created  time.Time `json:"created_at"`
}

type CreateInput struct {
	Name           string
	Prompt         string
	Schedule       string
	Enabled        bool
	HasEnable      bool
	SessionTarget  string
	WakeMode       string
	DeliveryMode   string
	Payload        json.RawMessage
	DeleteAfterRun bool
}

type UpdateInput struct {
	Name           *string
	Prompt         *string
	Schedule       *string
	Enabled        *bool
	SessionTarget  *string
	WakeMode       *string
	DeliveryMode   *string
	Payload        *json.RawMessage
	DeleteAfterRun *bool
}

type StoreOptions struct {
	RunHistoryLimit int
}

type Store struct {
	mu              sync.Mutex
	dir             string
	path            string
	runsDir         string
	runHistoryLimit int
	running         map[string]struct{}
}

func NewStore(workspaceDir string) *Store {
	return NewStoreWithOptions(workspaceDir, StoreOptions{})
}

func NewStoreWithOptions(workspaceDir string, opts StoreOptions) *Store {
	limit := opts.RunHistoryLimit
	if limit <= 0 {
		limit = defaultRunHistoryLimit
	}
	dir := filepath.Join(workspaceDir, "cron")
	return &Store{
		dir:             dir,
		path:            filepath.Join(dir, "jobs.json"),
		runsDir:         filepath.Join(dir, "runs"),
		runHistoryLimit: limit,
		running:         map[string]struct{}{},
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
	sessionTarget, err := normalizeSessionTarget(input.SessionTarget)
	if err != nil {
		return Job{}, err
	}
	wakeMode, err := normalizeWakeMode(input.WakeMode)
	if err != nil {
		return Job{}, err
	}
	deliveryMode, err := normalizeDeliveryMode(input.DeliveryMode, sessionTarget)
	if err != nil {
		return Job{}, err
	}
	payload, err := normalizePayload(input.Payload)
	if err != nil {
		return Job{}, err
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
		SessionTarget:  sessionTarget,
		WakeMode:       wakeMode,
		DeliveryMode:   deliveryMode,
		Payload:        payload,
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
		if input.SessionTarget != nil {
			sessionTarget, err := normalizeSessionTarget(*input.SessionTarget)
			if err != nil {
				return Job{}, err
			}
			jobs[i].SessionTarget = sessionTarget
			if strings.TrimSpace(jobs[i].DeliveryMode) == "" {
				jobs[i].DeliveryMode, _ = normalizeDeliveryMode("", sessionTarget)
			}
		}
		if input.WakeMode != nil {
			wakeMode, err := normalizeWakeMode(*input.WakeMode)
			if err != nil {
				return Job{}, err
			}
			jobs[i].WakeMode = wakeMode
		}
		if input.DeliveryMode != nil {
			deliveryMode, err := normalizeDeliveryMode(*input.DeliveryMode, jobs[i].SessionTarget)
			if err != nil {
				return Job{}, err
			}
			jobs[i].DeliveryMode = deliveryMode
		}
		if input.Payload != nil {
			payload, err := normalizePayload(*input.Payload)
			if err != nil {
				return Job{}, err
			}
			jobs[i].Payload = payload
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
	delete(s.running, id)
	return s.deleteRunFile(id)
}

func (s *Store) MarkRunResult(id string, ranAt time.Time, response string, runErr error) (Job, error) {
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
			jobs[i].ConsecutiveFailures++
			backoff := computeBackoffDuration(jobs[i].Schedule, jobs[i].ConsecutiveFailures)
			until := ran.Add(backoff)
			jobs[i].BackoffUntil = &until
		} else {
			jobs[i].LastRunError = ""
			jobs[i].ConsecutiveFailures = 0
			jobs[i].BackoffUntil = nil
		}
		jobs[i].UpdatedAt = ran
		record := RunRecord{
			JobID:    id,
			RanAt:    ran,
			Response: strings.TrimSpace(response),
			Error:    jobs[i].LastRunError,
			Created:  ran,
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
			delete(s.running, id)
			_ = s.deleteRunFile(id)
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

	runs, err := s.loadRuns(id)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	filtered := make([]RunRecord, 0, min(limit, len(runs)))
	for i := len(runs) - 1; i >= 0; i-- {
		filtered = append(filtered, runs[i])
		if len(filtered) >= limit {
			break
		}
	}
	return filtered, nil
}

func (s *Store) TryStartRun(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.running[id]; exists {
		return false
	}
	s.running[id] = struct{}{}
	return true
}

func (s *Store) FinishRun(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running, id)
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
	for i := range jobs {
		normalized, err := normalizeSessionTarget(jobs[i].SessionTarget)
		if err != nil {
			normalized = "isolated"
		}
		jobs[i].SessionTarget = normalized
		if mode, err := normalizeWakeMode(jobs[i].WakeMode); err == nil {
			jobs[i].WakeMode = mode
		} else {
			jobs[i].WakeMode = "agent_loop"
		}
		if mode, err := normalizeDeliveryMode(jobs[i].DeliveryMode, jobs[i].SessionTarget); err == nil {
			jobs[i].DeliveryMode = mode
		} else {
			jobs[i].DeliveryMode = "daily_log"
		}
		payload, err := normalizePayload(jobs[i].Payload)
		if err == nil {
			jobs[i].Payload = payload
		}
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

func (s *Store) loadRuns(jobID string) ([]RunRecord, error) {
	if err := os.MkdirAll(s.runsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create cron runs directory: %w", err)
	}
	f, err := os.Open(s.runPath(jobID))
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
	if err := os.MkdirAll(s.runsDir, 0o755); err != nil {
		return fmt.Errorf("create cron runs directory: %w", err)
	}
	path := s.runPath(record.JobID)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open cron runs: %w", err)
	}
	data, err := json.Marshal(record)
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("encode cron run: %w", err)
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		_ = f.Close()
		return fmt.Errorf("write cron run: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close cron runs: %w", err)
	}
	return s.pruneRunFile(path, s.runHistoryLimit)
}

func (s *Store) pruneRunFile(path string, limit int) error {
	if limit <= 0 {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read cron runs: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) <= limit {
		return nil
	}
	keep := lines[len(lines)-limit:]
	rewritten := strings.Join(keep, "\n") + "\n"
	if err := os.WriteFile(path, []byte(rewritten), 0o644); err != nil {
		return fmt.Errorf("write pruned cron runs: %w", err)
	}
	return nil
}

func (s *Store) deleteRunFile(jobID string) error {
	err := os.Remove(s.runPath(jobID))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete cron run file: %w", err)
	}
	return nil
}

func (s *Store) runPath(jobID string) string {
	return filepath.Join(s.runsDir, strings.TrimSpace(jobID)+".jsonl")
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
	if strings.HasPrefix(lower, "at:") {
		ts := strings.TrimSpace(s[len("at:"):])
		at, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			return "", fmt.Errorf("invalid schedule: %s (expected at:<rfc3339>, every:<duration>, or valid cron expression)", s)
		}
		return "at:" + at.UTC().Format(time.RFC3339), nil
	}
	if strings.HasPrefix(lower, "every:") {
		dur := strings.TrimSpace(s[len("every:"):])
		if dur == "" {
			return "", fmt.Errorf("invalid schedule: %s (expected at:<rfc3339>, every:<duration>, or valid cron expression)", s)
		}
		if _, err := time.ParseDuration(dur); err != nil {
			return "", fmt.Errorf("invalid schedule: %s (expected at:<rfc3339>, every:<duration>, or valid cron expression)", s)
		}
		return "every:" + dur, nil
	}
	if strings.HasPrefix(lower, "@every ") {
		dur := strings.TrimSpace(s[len("@every "):])
		if _, err := time.ParseDuration(dur); err != nil {
			return "", fmt.Errorf("invalid schedule: %s (expected at:<rfc3339>, every:<duration>, or valid cron expression)", s)
		}
		return "@every " + dur, nil
	}
	if _, err := cronv3.ParseStandard(s); err != nil {
		return "", fmt.Errorf("invalid schedule: %s (expected at:<rfc3339>, every:<duration>, or valid cron expression)", s)
	}
	return s, nil
}

func parseAtTime(schedule string) (time.Time, bool, error) {
	s := strings.TrimSpace(schedule)
	if !strings.HasPrefix(strings.ToLower(s), "at:") {
		return time.Time{}, false, nil
	}
	v := strings.TrimSpace(s[len("at:"):])
	at, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, true, err
	}
	return at.UTC(), true, nil
}

func normalizeSessionTarget(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "isolated", nil
	}
	switch strings.ToLower(v) {
	case "isolated":
		return "isolated", nil
	case "main":
		return "main", nil
	default:
		return v, nil
	}
}

func normalizeWakeMode(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "agent_loop", nil
	}
	if strings.EqualFold(v, "agent_loop") {
		return "agent_loop", nil
	}
	return "", fmt.Errorf("invalid wake_mode: %s (expected agent_loop)", v)
}

func normalizeDeliveryMode(raw, sessionTarget string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		if strings.EqualFold(sessionTarget, "main") {
			return "session", nil
		}
		return "daily_log", nil
	}
	switch strings.ToLower(v) {
	case "none":
		return "none", nil
	case "daily_log":
		return "daily_log", nil
	case "session":
		return "session", nil
	case "both":
		return "both", nil
	default:
		return "", fmt.Errorf("invalid delivery_mode: %s (expected none|daily_log|session|both)", v)
	}
}

func normalizePayload(raw json.RawMessage) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	if !json.Valid([]byte(trimmed)) {
		return nil, fmt.Errorf("payload must be valid json")
	}
	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return nil, fmt.Errorf("payload decode failed: %w", err)
	}
	if _, ok := decoded.(map[string]any); !ok {
		return nil, fmt.Errorf("payload must be a json object")
	}
	buf := &bytes.Buffer{}
	if err := json.Compact(buf, []byte(trimmed)); err != nil {
		return nil, fmt.Errorf("payload compact failed: %w", err)
	}
	return json.RawMessage(buf.Bytes()), nil
}

func computeBackoffDuration(schedule string, failures int) time.Duration {
	if failures <= 0 {
		return 0
	}
	base := 30 * time.Second
	if interval, ok := parseEveryDuration(schedule); ok && interval > base {
		base = interval
	}
	multiplier := failures - 1
	if multiplier > 6 {
		multiplier = 6
	}
	backoff := base * time.Duration(1<<multiplier)
	capDur := 12 * time.Hour
	if backoff > capDur {
		return capDur
	}
	return backoff
}

func newJobID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("job_%d", time.Now().UTC().UnixNano())
	}
	return "job_" + hex.EncodeToString(b[:])
}
