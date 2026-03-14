package project

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const projectActivityDocumentName = "ACTIVITY.jsonl"

const defaultRecentActivityLimit = 50

const (
	ActivitySourceSystem = "system"
	ActivitySourcePM     = "pm"
	ActivitySourceAgent  = "agent"
)

const (
	ActivityKindAssignment       = "assignment"
	ActivityKindTaskStatus       = "task_status"
	ActivityKindProjectCreated   = "project_created"
	ActivityKindProjectUpdated   = "project_updated"
	ActivityKindProjectArchived  = "project_archived"
	ActivityKindStateChanged     = "state_changed"
	ActivityKindBoardTaskCreated = "board_task_created"
	ActivityKindBoardTaskUpdated = "board_task_updated"
	ActivityKindReviewStatus     = "review_status"
	ActivityKindTestStatus       = "test_status"
	ActivityKindBuildStatus      = "build_status"
	ActivityKindIssueStatus      = "issue_status"
	ActivityKindPRStatus         = "pr_status"
)

type Activity struct {
	ID        string            `json:"id"`
	ProjectID string            `json:"project_id"`
	TaskID    string            `json:"task_id,omitempty"`
	Source    string            `json:"source"`
	Agent     string            `json:"agent,omitempty"`
	Kind      string            `json:"kind"`
	Status    string            `json:"status,omitempty"`
	Message   string            `json:"message,omitempty"`
	Timestamp string            `json:"timestamp"`
	Meta      map[string]string `json:"meta,omitempty"`
}

type ActivityAppendInput struct {
	TaskID    string
	Source    string
	Agent     string
	Kind      string
	Status    string
	Message   string
	Timestamp string
	Meta      map[string]string
}

func (s *Store) ActivityPath(projectID string) string {
	return filepath.Join(s.workspaceDir, "projects", strings.TrimSpace(projectID), projectActivityDocumentName)
}

func (s *Store) AppendActivity(projectID string, input ActivityAppendInput) (Activity, error) {
	if s == nil {
		return Activity{}, fmt.Errorf("project store is nil")
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return Activity{}, fmt.Errorf("project id is required")
	}
	if _, err := s.Get(projectID); err != nil {
		return Activity{}, err
	}
	source := strings.ToLower(strings.TrimSpace(input.Source))
	if source == "" {
		return Activity{}, fmt.Errorf("source is required")
	}
	kind := normalizeActivityKind(input.Kind)
	if kind == "" {
		return Activity{}, fmt.Errorf("kind is required")
	}

	existing, err := s.readActivity(projectID)
	if err != nil {
		return Activity{}, err
	}
	now := s.nowFn().UTC()
	timestamp := strings.TrimSpace(input.Timestamp)
	if timestamp == "" {
		timestamp = now.Format(time.RFC3339)
	}
	item := Activity{
		ID:        fmt.Sprintf("act_%s_%03d", now.Format("20060102T150405.000000000"), len(existing)+1),
		ProjectID: projectID,
		TaskID:    strings.TrimSpace(input.TaskID),
		Source:    source,
		Agent:     strings.TrimSpace(input.Agent),
		Kind:      kind,
		Status:    strings.ToLower(strings.TrimSpace(input.Status)),
		Message:   strings.TrimSpace(input.Message),
		Timestamp: timestamp,
		Meta:      normalizeActivityMeta(input.Meta),
	}

	path := s.ActivityPath(projectID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Activity{}, err
	}
	encoded, err := json.Marshal(item)
	if err != nil {
		return Activity{}, fmt.Errorf("marshal activity: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return Activity{}, err
	}
	defer f.Close()
	if _, err := f.Write(append(encoded, '\n')); err != nil {
		return Activity{}, err
	}
	return item, nil
}

func (s *Store) ListRecentActivity(projectID string) ([]Activity, error) {
	return s.ListActivity(projectID, defaultRecentActivityLimit)
}

func (s *Store) ListActivity(projectID string, limit int) ([]Activity, error) {
	if s == nil {
		return nil, fmt.Errorf("project store is nil")
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, fmt.Errorf("project id is required")
	}
	if _, err := s.Get(projectID); err != nil {
		return nil, err
	}
	items, err := s.readActivity(projectID)
	if err != nil {
		return nil, err
	}
	out := make([]Activity, 0, len(items))
	for i := len(items) - 1; i >= 0; i-- {
		out = append(out, items[i])
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	if out == nil {
		return []Activity{}, nil
	}
	return out, nil
}

func (s *Store) readActivity(projectID string) ([]Activity, error) {
	path := s.ActivityPath(projectID)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Activity{}, nil
		}
		return nil, err
	}
	defer f.Close()

	items := []Activity{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item Activity
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			return nil, fmt.Errorf("decode activity: %w", err)
		}
		item.ProjectID = strings.TrimSpace(item.ProjectID)
		items = append(items, item)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func normalizeActivityMeta(raw map[string]string) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string]string, len(raw))
	for key, value := range raw {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		out[trimmedKey] = strings.TrimSpace(value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeActivityKind(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}
