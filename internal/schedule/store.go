package schedule

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tarsncase/internal/cron"
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

type record struct {
	Op        string    `json:"op"`
	Item      Item      `json:"item"`
	Timestamp time.Time `json:"timestamp"`
}

type Store struct {
	mu          sync.Mutex
	workspace   string
	itemsPath   string
	cronMapPath string
	cronStore   *cron.Store
	nowFn       func() time.Time
	timezone    string
}

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
		workspace:   root,
		itemsPath:   filepath.Join(base, "items.jsonl"),
		cronMapPath: filepath.Join(base, "cron_map.json"),
		cronStore:   cronStore,
		nowFn:       nowFn,
		timezone:    tz,
	}
}

func (s *Store) List() ([]Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadStateLocked()
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(state))
	for _, item := range state {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	if items == nil {
		return []Item{}, nil
	}
	return items, nil
}

func (s *Store) Get(id string) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadStateLocked()
	if err != nil {
		return Item{}, err
	}
	item, ok := state[strings.TrimSpace(id)]
	if !ok {
		return Item{}, fmt.Errorf("schedule not found: %s", strings.TrimSpace(id))
	}
	return item, nil
}

func (s *Store) Create(input CreateInput) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.nowFn().UTC()
	item, err := s.buildNewItemLocked(input, now)
	if err != nil {
		return Item{}, err
	}
	if err := s.appendRecordLocked(record{Op: "upsert", Item: item, Timestamp: now}); err != nil {
		return Item{}, err
	}
	return item, nil
}

func (s *Store) Update(id string, input UpdateInput) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := strings.TrimSpace(id)
	if key == "" {
		return Item{}, fmt.Errorf("schedule id is required")
	}
	state, err := s.loadStateLocked()
	if err != nil {
		return Item{}, err
	}
	item, ok := state[key]
	if !ok {
		return Item{}, fmt.Errorf("schedule not found: %s", key)
	}
	if input.Title != nil {
		v := strings.TrimSpace(*input.Title)
		if v == "" {
			return Item{}, fmt.Errorf("title is required")
		}
		item.Title = v
	}
	if input.Prompt != nil {
		item.Prompt = strings.TrimSpace(*input.Prompt)
	}
	if input.ProjectID != nil {
		item.ProjectID = strings.TrimSpace(*input.ProjectID)
	}
	if input.Timezone != nil {
		tz := strings.TrimSpace(*input.Timezone)
		if tz == "" {
			tz = s.timezone
		}
		item.Timezone = tz
	}
	if input.Schedule != nil {
		scheduleValue, err := resolveSchedule(strings.TrimSpace(*input.Schedule), "", item.Timezone, s.nowFn())
		if err != nil {
			return Item{}, err
		}
		item.Schedule = scheduleValue
	}
	if input.Status != nil {
		status := normalizeStatus(*input.Status)
		item.Status = status
	}
	if err := s.syncCronLocked(&item); err != nil {
		return Item{}, err
	}
	item.UpdatedAt = s.nowFn().UTC()
	if err := s.appendRecordLocked(record{Op: "upsert", Item: item, Timestamp: item.UpdatedAt}); err != nil {
		return Item{}, err
	}
	return item, nil
}

func (s *Store) Complete(id string) (Item, error) {
	status := "completed"
	return s.Update(id, UpdateInput{Status: &status})
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := strings.TrimSpace(id)
	if key == "" {
		return fmt.Errorf("schedule id is required")
	}
	state, err := s.loadStateLocked()
	if err != nil {
		return err
	}
	item, ok := state[key]
	if !ok {
		return fmt.Errorf("schedule not found: %s", key)
	}
	if s.cronStore != nil && strings.TrimSpace(item.CronJobID) != "" {
		_ = s.cronStore.Delete(strings.TrimSpace(item.CronJobID))
	}
	cronMap, err := s.loadCronMapLocked()
	if err == nil {
		delete(cronMap, key)
		_ = s.saveCronMapLocked(cronMap)
	}
	return s.appendRecordLocked(record{Op: "delete", Item: item, Timestamp: s.nowFn().UTC()})
}

func (s *Store) buildNewItemLocked(input CreateInput, now time.Time) (Item, error) {
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
	item := Item{
		ID:        newScheduleID(now),
		Title:     title,
		Prompt:    prompt,
		Natural:   natural,
		Schedule:  scheduleValue,
		Status:    "active",
		ProjectID: strings.TrimSpace(input.ProjectID),
		Timezone:  tz,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.syncCronLocked(&item); err != nil {
		return Item{}, err
	}
	return item, nil
}

func (s *Store) syncCronLocked(item *Item) error {
	if item == nil {
		return fmt.Errorf("item is required")
	}
	if s.cronStore == nil {
		return nil
	}
	status := normalizeStatus(item.Status)
	item.Status = status
	enabled := status == "active"
	if strings.TrimSpace(item.CronJobID) == "" {
		job, err := s.cronStore.CreateWithOptions(cron.CreateInput{
			Name:      strings.TrimSpace(item.Title),
			Prompt:    strings.TrimSpace(item.Prompt),
			Schedule:  strings.TrimSpace(item.Schedule),
			Enabled:   enabled,
			HasEnable: true,
			ProjectID: strings.TrimSpace(item.ProjectID),
		})
		if err != nil {
			return err
		}
		item.CronJobID = job.ID
		item.Schedule = job.Schedule
		cronMap, _ := s.loadCronMapLocked()
		if cronMap == nil {
			cronMap = map[string]string{}
		}
		cronMap[item.ID] = job.ID
		_ = s.saveCronMapLocked(cronMap)
		return nil
	}

	scheduleValue := strings.TrimSpace(item.Schedule)
	name := strings.TrimSpace(item.Title)
	prompt := strings.TrimSpace(item.Prompt)
	projectID := strings.TrimSpace(item.ProjectID)
	job, err := s.cronStore.Update(strings.TrimSpace(item.CronJobID), cron.UpdateInput{
		Name:      &name,
		Prompt:    &prompt,
		Schedule:  &scheduleValue,
		Enabled:   &enabled,
		ProjectID: &projectID,
	})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			item.CronJobID = ""
			return s.syncCronLocked(item)
		}
		return err
	}
	item.Schedule = job.Schedule
	cronMap, _ := s.loadCronMapLocked()
	if cronMap == nil {
		cronMap = map[string]string{}
	}
	cronMap[item.ID] = job.ID
	_ = s.saveCronMapLocked(cronMap)
	return nil
}

func (s *Store) loadStateLocked() (map[string]Item, error) {
	if err := os.MkdirAll(filepath.Dir(s.itemsPath), 0o755); err != nil {
		return nil, err
	}
	file, err := os.Open(s.itemsPath)
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
		var rec record
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

func (s *Store) appendRecordLocked(rec record) error {
	if err := os.MkdirAll(filepath.Dir(s.itemsPath), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(s.itemsPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	line, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return err
}

func (s *Store) loadCronMapLocked() (map[string]string, error) {
	raw, err := os.ReadFile(s.cronMapPath)
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

func (s *Store) saveCronMapLocked(value map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(s.cronMapPath), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.cronMapPath, payload, 0o644)
}

func resolveSchedule(explicit string, natural string, timezone string, now time.Time) (string, error) {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		return explicit, nil
	}
	natural = strings.TrimSpace(natural)
	if natural == "" {
		return "", fmt.Errorf("natural or schedule is required")
	}
	schedule, err := parseNaturalSchedule(natural, timezone, now)
	if err != nil {
		return "", err
	}
	return schedule, nil
}

var tomorrowHourPattern = regexp.MustCompile(`(오전|오후)?\s*(\d{1,2})시`)
var weeklyPattern = regexp.MustCompile(`매주\s*([월화수목금토일])요일?\s*(오전|오후)?\s*(\d{1,2})시`)

func parseNaturalSchedule(natural string, timezone string, now time.Time) (string, error) {
	input := strings.TrimSpace(natural)
	if input == "" {
		return "", fmt.Errorf("natural schedule is required")
	}
	if strings.HasPrefix(strings.ToLower(input), "at:") || strings.HasPrefix(strings.ToLower(input), "every:") {
		return input, nil
	}
	loc, err := time.LoadLocation(strings.TrimSpace(timezone))
	if err != nil {
		loc = time.FixedZone("UTC+9", 9*3600)
	}
	if strings.Contains(input, "매주") {
		match := weeklyPattern.FindStringSubmatch(input)
		if len(match) == 4 {
			dow := weekdayToCron(match[1])
			hour, hourErr := parseHour(match[2], match[3])
			if hourErr != nil {
				return "", hourErr
			}
			return fmt.Sprintf("0 %d * * %d", hour, dow), nil
		}
	}
	if strings.Contains(input, "내일") {
		match := tomorrowHourPattern.FindStringSubmatch(input)
		if len(match) == 3 {
			hour, hourErr := parseHour(match[1], match[2])
			if hourErr != nil {
				return "", hourErr
			}
			base := now.In(loc).AddDate(0, 0, 1)
			at := time.Date(base.Year(), base.Month(), base.Day(), hour, 0, 0, 0, loc)
			return "at:" + at.Format(time.RFC3339), nil
		}
	}
	return "", fmt.Errorf("could not parse natural schedule; use at:<rfc3339> or every:<duration>")
}

func parseHour(marker string, raw string) (int, error) {
	hour := 0
	if _, err := fmt.Sscanf(strings.TrimSpace(raw), "%d", &hour); err != nil {
		return 0, fmt.Errorf("invalid hour: %s", raw)
	}
	if hour < 0 || hour > 23 {
		if hour < 1 || hour > 12 {
			return 0, fmt.Errorf("invalid hour: %d", hour)
		}
	}
	mark := strings.TrimSpace(marker)
	switch mark {
	case "오후":
		if hour < 12 {
			hour += 12
		}
	case "오전":
		if hour == 12 {
			hour = 0
		}
	}
	if hour > 23 {
		return 0, fmt.Errorf("invalid hour: %d", hour)
	}
	return hour, nil
}

func weekdayToCron(token string) int {
	switch strings.TrimSpace(token) {
	case "월":
		return 1
	case "화":
		return 2
	case "수":
		return 3
	case "목":
		return 4
	case "금":
		return 5
	case "토":
		return 6
	case "일":
		return 0
	default:
		return 0
	}
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

func newScheduleID(now time.Time) string {
	return "sch_" + now.UTC().Format("20060102T150405.000000000")
}
