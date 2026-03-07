package schedule

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tarsncase/internal/cron"
	"github.com/devlikebear/tarsncase/internal/scheduleexpr"
)

type Options struct {
	Now      func() time.Time
	Timezone string
}

type Item struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Prompt    string    `json:"prompt,omitempty"`
	Natural   string    `json:"natural,omitempty"`
	Schedule  string    `json:"schedule"`
	Status    string    `json:"status"`
	ProjectID string    `json:"project_id,omitempty"`
	CronJobID string    `json:"cron_job_id,omitempty"`
	Timezone  string    `json:"timezone,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateInput struct {
	Natural   string
	Title     string
	Prompt    string
	Schedule  string
	ProjectID string
	Timezone  string
}

type UpdateInput struct {
	Title     *string
	Prompt    *string
	Schedule  *string
	Status    *string
	ProjectID *string
	Timezone  *string
}

type Store struct {
	mu             sync.Mutex
	workspace      string
	legacyItems    string
	legacyCronMap  string
	cronStore      *cron.Store
	nowFn          func() time.Time
	timezone       string
	migrationTried bool
}

type scheduleMeta struct {
	Natural   string `json:"natural,omitempty"`
	Status    string `json:"status,omitempty"`
	Timezone  string `json:"timezone,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type legacyRecord struct {
	Op        string    `json:"op"`
	Item      Item      `json:"item"`
	Timestamp time.Time `json:"timestamp"`
}

const schedulePayloadKey = "_tars_schedule"

func NewStore(workspaceDir string, cronStore *cron.Store, opts Options) *Store {
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	tz := strings.TrimSpace(opts.Timezone)
	if tz == "" {
		tz = "Asia/Seoul"
	}
	root := strings.TrimSpace(workspaceDir)
	if root == "" {
		root = "./workspace"
	}
	base := filepath.Join(root, "schedule")
	return &Store{
		workspace:     root,
		legacyItems:   filepath.Join(base, "items.jsonl"),
		legacyCronMap: filepath.Join(base, "cron_map.json"),
		cronStore:     cronStore,
		nowFn:         nowFn,
		timezone:      tz,
	}
}

func (s *Store) List() ([]Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureReadyLocked(); err != nil {
		return nil, err
	}
	jobs, err := s.cronStore.List()
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(jobs))
	for _, job := range jobs {
		item, ok := s.itemFromJob(job)
		if !ok {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	return items, nil
}

func (s *Store) Get(id string) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureReadyLocked(); err != nil {
		return Item{}, err
	}
	job, err := s.cronStore.Get(strings.TrimSpace(id))
	if err != nil {
		return Item{}, fmt.Errorf("schedule not found: %s", strings.TrimSpace(id))
	}
	item, ok := s.itemFromJob(job)
	if !ok {
		return Item{}, fmt.Errorf("schedule not found: %s", strings.TrimSpace(id))
	}
	return item, nil
}

func (s *Store) Create(input CreateInput) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureReadyLocked(); err != nil {
		return Item{}, err
	}

	now := s.nowFn().UTC()
	natural := strings.TrimSpace(input.Natural)
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = natural
	}
	if title == "" {
		title = "schedule"
	}
	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" {
		prompt = "Reminder: " + title
	}
	tz := strings.TrimSpace(input.Timezone)
	if tz == "" {
		tz = s.timezone
	}
	scheduleValue, err := resolveSchedule(strings.TrimSpace(input.Schedule), natural, tz, s.nowFn())
	if err != nil {
		return Item{}, err
	}
	meta := scheduleMeta{
		Natural:   natural,
		Status:    "active",
		Timezone:  tz,
		CreatedAt: now.Format(time.RFC3339),
		UpdatedAt: now.Format(time.RFC3339),
	}
	payload, err := mergeScheduleMeta(nil, meta)
	if err != nil {
		return Item{}, err
	}

	job, err := s.cronStore.CreateWithOptions(cron.CreateInput{
		Name:      title,
		Prompt:    prompt,
		Schedule:  scheduleValue,
		Enabled:   true,
		HasEnable: true,
		ProjectID: strings.TrimSpace(input.ProjectID),
		Payload:   payload,
	})
	if err != nil {
		return Item{}, err
	}
	item, _ := s.itemFromJob(job)
	return item, nil
}

func (s *Store) Update(id string, input UpdateInput) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureReadyLocked(); err != nil {
		return Item{}, err
	}
	key := strings.TrimSpace(id)
	if key == "" {
		return Item{}, fmt.Errorf("schedule id is required")
	}
	job, err := s.cronStore.Get(key)
	if err != nil {
		return Item{}, fmt.Errorf("schedule not found: %s", key)
	}

	meta, hasMeta := extractScheduleMeta(job.Payload)
	if !hasMeta {
		return Item{}, fmt.Errorf("schedule not found: %s", key)
	}

	now := s.nowFn().UTC()
	name := strings.TrimSpace(job.Name)
	prompt := strings.TrimSpace(job.Prompt)
	scheduleValue := strings.TrimSpace(job.Schedule)
	projectID := strings.TrimSpace(job.ProjectID)
	status := normalizeStatus(meta.Status)
	if status == "" {
		status = inferStatus(job.Enabled, "")
	}
	timezone := strings.TrimSpace(meta.Timezone)
	if timezone == "" {
		timezone = s.timezone
	}

	if input.Title != nil {
		v := strings.TrimSpace(*input.Title)
		if v == "" {
			return Item{}, fmt.Errorf("title is required")
		}
		name = v
	}
	if input.Prompt != nil {
		prompt = strings.TrimSpace(*input.Prompt)
	}
	if input.ProjectID != nil {
		projectID = strings.TrimSpace(*input.ProjectID)
	}
	if input.Timezone != nil {
		timezone = strings.TrimSpace(*input.Timezone)
		if timezone == "" {
			timezone = s.timezone
		}
	}
	if input.Schedule != nil {
		resolved, resolveErr := resolveSchedule(strings.TrimSpace(*input.Schedule), "", timezone, s.nowFn())
		if resolveErr != nil {
			return Item{}, resolveErr
		}
		scheduleValue = resolved
	}
	if input.Status != nil {
		status = normalizeStatus(*input.Status)
	}

	meta.Status = status
	meta.Timezone = timezone
	if strings.TrimSpace(meta.CreatedAt) == "" {
		meta.CreatedAt = job.CreatedAt.UTC().Format(time.RFC3339)
	}
	meta.UpdatedAt = now.Format(time.RFC3339)
	payload, err := mergeScheduleMeta(job.Payload, meta)
	if err != nil {
		return Item{}, err
	}
	enabled := status == "active"

	updated, err := s.cronStore.Update(key, cron.UpdateInput{
		Name:      &name,
		Prompt:    &prompt,
		Schedule:  &scheduleValue,
		Enabled:   &enabled,
		ProjectID: &projectID,
		Payload:   &payload,
	})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return Item{}, fmt.Errorf("schedule not found: %s", key)
		}
		return Item{}, err
	}
	item, _ := s.itemFromJob(updated)
	return item, nil
}

func (s *Store) Complete(id string) (Item, error) {
	status := "completed"
	return s.Update(id, UpdateInput{Status: &status})
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureReadyLocked(); err != nil {
		return err
	}
	key := strings.TrimSpace(id)
	if key == "" {
		return fmt.Errorf("schedule id is required")
	}
	job, err := s.cronStore.Get(key)
	if err != nil {
		return fmt.Errorf("schedule not found: %s", key)
	}
	if _, ok := extractScheduleMeta(job.Payload); !ok {
		return fmt.Errorf("schedule not found: %s", key)
	}
	if err := s.cronStore.Delete(key); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return fmt.Errorf("schedule not found: %s", key)
		}
		return err
	}
	return nil
}

func (s *Store) ensureReadyLocked() error {
	if s.cronStore == nil {
		return fmt.Errorf("cron store is not configured")
	}
	if s.migrationTried {
		return nil
	}
	s.migrationTried = true
	if err := s.migrateLegacyLocked(); err != nil {
		return err
	}
	return nil
}

func (s *Store) itemFromJob(job cron.Job) (Item, bool) {
	meta, ok := extractScheduleMeta(job.Payload)
	if !ok {
		return Item{}, false
	}
	createdAt := parseMetaTime(meta.CreatedAt, job.CreatedAt)
	updatedAt := parseMetaTime(meta.UpdatedAt, job.UpdatedAt)
	status := inferStatus(job.Enabled, meta.Status)
	timezone := strings.TrimSpace(meta.Timezone)
	if timezone == "" {
		timezone = s.timezone
	}
	return Item{
		ID:        job.ID,
		Title:     job.Name,
		Prompt:    job.Prompt,
		Natural:   strings.TrimSpace(meta.Natural),
		Schedule:  job.Schedule,
		Status:    status,
		ProjectID: strings.TrimSpace(job.ProjectID),
		CronJobID: job.ID,
		Timezone:  timezone,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, true
}

func (s *Store) migrateLegacyLocked() error {
	state, err := loadLegacyState(s.legacyItems)
	if err != nil {
		return err
	}
	if len(state) == 0 {
		return nil
	}
	legacyCronMap, err := loadLegacyCronMap(s.legacyCronMap)
	if err != nil {
		return err
	}

	items := make([]Item, 0, len(state))
	for _, item := range state {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].UpdatedAt.Before(items[j].UpdatedAt)
	})

	for _, legacy := range items {
		if err := s.upsertLegacyItemLocked(legacy, legacyCronMap); err != nil {
			return err
		}
	}
	if err := backupMigratedFile(s.legacyItems); err != nil {
		return err
	}
	if err := backupMigratedFile(s.legacyCronMap); err != nil {
		return err
	}
	return nil
}

func (s *Store) upsertLegacyItemLocked(item Item, legacyCronMap map[string]string) error {
	natural := strings.TrimSpace(item.Natural)
	title := strings.TrimSpace(item.Title)
	if title == "" {
		title = natural
	}
	if title == "" {
		title = "schedule"
	}
	prompt := strings.TrimSpace(item.Prompt)
	if prompt == "" {
		prompt = "Reminder: " + title
	}
	projectID := strings.TrimSpace(item.ProjectID)
	tz := strings.TrimSpace(item.Timezone)
	if tz == "" {
		tz = s.timezone
	}
	scheduleValue, err := resolveSchedule(strings.TrimSpace(item.Schedule), natural, tz, s.nowFn())
	if err != nil {
		return err
	}
	status := normalizeStatus(item.Status)
	createdAt := item.CreatedAt
	if createdAt.IsZero() {
		createdAt = s.nowFn().UTC()
	}
	updatedAt := item.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}
	meta := scheduleMeta{
		Natural:   natural,
		Status:    status,
		Timezone:  tz,
		CreatedAt: createdAt.UTC().Format(time.RFC3339),
		UpdatedAt: updatedAt.UTC().Format(time.RFC3339),
	}

	targetJobID := strings.TrimSpace(item.CronJobID)
	if targetJobID == "" {
		targetJobID = strings.TrimSpace(legacyCronMap[strings.TrimSpace(item.ID)])
	}
	if targetJobID != "" {
		existing, getErr := s.cronStore.Get(targetJobID)
		if getErr == nil {
			payload, mergeErr := mergeScheduleMeta(existing.Payload, meta)
			if mergeErr != nil {
				return mergeErr
			}
			enabled := status == "active"
			_, updateErr := s.cronStore.Update(targetJobID, cron.UpdateInput{
				Name:      &title,
				Prompt:    &prompt,
				Schedule:  &scheduleValue,
				Enabled:   &enabled,
				ProjectID: &projectID,
				Payload:   &payload,
			})
			if updateErr != nil {
				return updateErr
			}
			return nil
		}
	}

	payload, err := mergeScheduleMeta(nil, meta)
	if err != nil {
		return err
	}
	_, err = s.cronStore.CreateWithOptions(cron.CreateInput{
		Name:      title,
		Prompt:    prompt,
		Schedule:  scheduleValue,
		Enabled:   status == "active",
		HasEnable: true,
		ProjectID: projectID,
		Payload:   payload,
	})
	return err
}

func mergeScheduleMeta(payload json.RawMessage, meta scheduleMeta) (json.RawMessage, error) {
	base := map[string]any{}
	trimmed := strings.TrimSpace(string(payload))
	if trimmed != "" && trimmed != "null" {
		if err := json.Unmarshal([]byte(trimmed), &base); err != nil {
			return nil, fmt.Errorf("invalid payload json: %w", err)
		}
	}
	if base == nil {
		base = map[string]any{}
	}
	metaMap := map[string]any{}
	if strings.TrimSpace(meta.Natural) != "" {
		metaMap["natural"] = strings.TrimSpace(meta.Natural)
	}
	if strings.TrimSpace(meta.Status) != "" {
		metaMap["status"] = normalizeStatus(meta.Status)
	}
	if strings.TrimSpace(meta.Timezone) != "" {
		metaMap["timezone"] = strings.TrimSpace(meta.Timezone)
	}
	if strings.TrimSpace(meta.CreatedAt) != "" {
		metaMap["created_at"] = strings.TrimSpace(meta.CreatedAt)
	}
	if strings.TrimSpace(meta.UpdatedAt) != "" {
		metaMap["updated_at"] = strings.TrimSpace(meta.UpdatedAt)
	}
	base[schedulePayloadKey] = metaMap
	raw, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}
	buf := &bytes.Buffer{}
	if err := json.Compact(buf, raw); err != nil {
		return nil, err
	}
	return json.RawMessage(buf.Bytes()), nil
}

func extractScheduleMeta(payload json.RawMessage) (scheduleMeta, bool) {
	trimmed := strings.TrimSpace(string(payload))
	if trimmed == "" || trimmed == "null" {
		return scheduleMeta{}, false
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
		return scheduleMeta{}, false
	}
	raw, ok := obj[schedulePayloadKey]
	if !ok {
		return scheduleMeta{}, false
	}
	metaMap, ok := raw.(map[string]any)
	if !ok {
		return scheduleMeta{}, false
	}
	meta := scheduleMeta{}
	if v, ok := metaMap["natural"].(string); ok {
		meta.Natural = strings.TrimSpace(v)
	}
	if v, ok := metaMap["status"].(string); ok {
		meta.Status = normalizeStatus(v)
	}
	if v, ok := metaMap["timezone"].(string); ok {
		meta.Timezone = strings.TrimSpace(v)
	}
	if v, ok := metaMap["created_at"].(string); ok {
		meta.CreatedAt = strings.TrimSpace(v)
	}
	if v, ok := metaMap["updated_at"].(string); ok {
		meta.UpdatedAt = strings.TrimSpace(v)
	}
	return meta, true
}

func inferStatus(enabled bool, raw string) string {
	status := normalizeStatus(raw)
	switch status {
	case "completed":
		return "completed"
	case "paused":
		return "paused"
	case "active":
		if !enabled {
			return "paused"
		}
		return "active"
	default:
		if enabled {
			return "active"
		}
		return "paused"
	}
}

func parseMetaTime(raw string, fallback time.Time) time.Time {
	if strings.TrimSpace(raw) != "" {
		if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(raw)); err == nil {
			return parsed.UTC()
		}
	}
	if fallback.IsZero() {
		return time.Now().UTC()
	}
	return fallback.UTC()
}

func loadLegacyState(path string) (map[string]Item, error) {
	if strings.TrimSpace(path) == "" {
		return map[string]Item{}, nil
	}
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]Item{}, nil
		}
		return nil, err
	}
	defer file.Close()

	state := map[string]Item{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec legacyRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		id := strings.TrimSpace(rec.Item.ID)
		if id == "" {
			continue
		}
		switch strings.TrimSpace(rec.Op) {
		case "delete":
			delete(state, id)
		default:
			state[id] = rec.Item
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return state, nil
}

func loadLegacyCronMap(path string) (map[string]string, error) {
	if strings.TrimSpace(path) == "" {
		return map[string]string{}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	if strings.TrimSpace(string(raw)) == "" {
		return map[string]string{}, nil
	}
	out := map[string]string{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out == nil {
		return map[string]string{}, nil
	}
	return out, nil
}

func backupMigratedFile(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	backup := path + ".migrated.bak"
	_ = os.Remove(backup)
	return os.Rename(path, backup)
}

func resolveSchedule(explicit string, natural string, timezone string, now time.Time) (string, error) {
	return scheduleexpr.ResolveSchedule(explicit, natural, timezone, now)
}

func parseNaturalSchedule(natural string, timezone string, now time.Time) (string, error) {
	return scheduleexpr.ParseNaturalSchedule(natural, timezone, now)
}

func normalizeStatus(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "active", "completed", "paused":
		return v
	default:
		return "active"
	}
}
