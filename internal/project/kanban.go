package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const projectBoardDocumentName = "KANBAN.md"

var defaultBoardColumns = []string{"todo", "in_progress", "review", "done"}

type BoardTask struct {
	ID               string `json:"id" yaml:"id"`
	Title            string `json:"title" yaml:"title"`
	Status           string `json:"status" yaml:"status"`
	Assignee         string `json:"assignee,omitempty" yaml:"assignee,omitempty"`
	Role             string `json:"role,omitempty" yaml:"role,omitempty"`
	WorkerKind       string `json:"worker_kind,omitempty" yaml:"worker_kind,omitempty"`
	ReviewApprovedBy string `json:"review_approved_by,omitempty" yaml:"review_approved_by,omitempty"`
	ReviewRequired   bool   `json:"review_required,omitempty" yaml:"review_required,omitempty"`
	TestCommand      string `json:"test_command,omitempty" yaml:"test_command,omitempty"`
	BuildCommand     string `json:"build_command,omitempty" yaml:"build_command,omitempty"`
}

type Board struct {
	ProjectID string      `json:"project_id" yaml:"project_id"`
	UpdatedAt string      `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`
	Columns   []string    `json:"columns" yaml:"columns"`
	Tasks     []BoardTask `json:"tasks,omitempty" yaml:"tasks,omitempty"`
	Path      string      `json:"path,omitempty" yaml:"-"`
}

type BoardUpdateInput struct {
	Columns []string
	Tasks   []BoardTask
}

func (s *Store) BoardPath(projectID string) string {
	return filepath.Join(s.workspaceDir, "projects", strings.TrimSpace(projectID), projectBoardDocumentName)
}

func (s *Store) GetBoard(projectID string) (Board, error) {
	if s == nil {
		return Board{}, fmt.Errorf("project store is nil")
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return Board{}, fmt.Errorf("project id is required")
	}
	if _, err := s.Get(projectID); err != nil {
		return Board{}, err
	}
	path := s.BoardPath(projectID)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			board := defaultBoard(projectID, s.nowFn().UTC())
			if err := s.writeBoard(board); err != nil {
				return Board{}, err
			}
			return s.GetBoard(projectID)
		}
		return Board{}, err
	}
	board, err := parseBoardDocument(string(raw))
	if err != nil {
		return Board{}, err
	}
	if strings.TrimSpace(board.ProjectID) == "" {
		board.ProjectID = projectID
	}
	board.Columns = normalizeBoardColumns(board.Columns)
	board.Tasks = normalizeBoardTasks(board.Tasks, board.Columns)
	board.Path = filepath.Dir(path)
	return board, nil
}

func (s *Store) UpdateBoard(projectID string, input BoardUpdateInput) (Board, error) {
	if s == nil {
		return Board{}, fmt.Errorf("project store is nil")
	}
	board, err := s.GetBoard(projectID)
	if err != nil {
		return Board{}, err
	}
	previous := board
	if input.Columns != nil {
		board.Columns = normalizeBoardColumns(input.Columns)
	}
	if input.Tasks != nil {
		board.Tasks = normalizeBoardTasks(input.Tasks, board.Columns)
	}
	board.UpdatedAt = s.nowFn().UTC().Format(time.RFC3339)
	if err := validateBoard(board); err != nil {
		return Board{}, err
	}
	if err := s.writeBoard(board); err != nil {
		return Board{}, err
	}
	updated, err := s.GetBoard(projectID)
	if err != nil {
		return Board{}, err
	}
	previousTasks := make(map[string]BoardTask, len(previous.Tasks))
	for _, task := range previous.Tasks {
		previousTasks[task.ID] = task
	}
	for _, task := range updated.Tasks {
		before, existed := previousTasks[task.ID]
		if !existed {
			if err := s.appendSystemActivity(projectID, ActivityAppendInput{
				TaskID:  task.ID,
				Kind:    ActivityKindBoardTaskCreated,
				Status:  task.Status,
				Message: "Board task created",
				Meta: map[string]string{
					"title":    task.Title,
					"assignee": task.Assignee,
					"role":     task.Role,
				},
			}); err != nil {
				return Board{}, err
			}
			continue
		}
		if !boardTaskActivityChanged(before, task) {
			continue
		}
		if err := s.appendSystemActivity(projectID, ActivityAppendInput{
			TaskID:  task.ID,
			Kind:    ActivityKindBoardTaskUpdated,
			Status:  task.Status,
			Message: "Board task updated",
			Meta: map[string]string{
				"title":    task.Title,
				"assignee": task.Assignee,
				"role":     task.Role,
			},
		}); err != nil {
			return Board{}, err
		}
	}
	return updated, nil
}

func (s *Store) writeBoard(board Board) error {
	board.ProjectID = strings.TrimSpace(board.ProjectID)
	if board.ProjectID == "" {
		return fmt.Errorf("project id is required")
	}
	board.Columns = normalizeBoardColumns(board.Columns)
	board.Tasks = normalizeBoardTasks(board.Tasks, board.Columns)
	if err := validateBoard(board); err != nil {
		return err
	}
	dir := filepath.Dir(s.BoardPath(board.ProjectID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.BoardPath(board.ProjectID), []byte(buildBoardDocument(board)), 0o644)
}

func defaultBoard(projectID string, now time.Time) Board {
	return Board{
		ProjectID: strings.TrimSpace(projectID),
		UpdatedAt: now.UTC().Format(time.RFC3339),
		Columns:   append([]string(nil), defaultBoardColumns...),
		Tasks:     []BoardTask{},
	}
}

func parseBoardDocument(raw string) (Board, error) {
	metaRaw, _, hasMeta, err := splitFrontmatter(raw)
	if err != nil {
		return Board{}, err
	}
	if !hasMeta {
		return Board{Columns: append([]string(nil), defaultBoardColumns...)}, nil
	}
	var item Board
	if err := yaml.Unmarshal([]byte(metaRaw), &item); err != nil {
		return Board{}, fmt.Errorf("parse board frontmatter: %w", err)
	}
	return item, nil
}

func buildBoardDocument(board Board) string {
	item := Board{
		ProjectID: strings.TrimSpace(board.ProjectID),
		UpdatedAt: strings.TrimSpace(board.UpdatedAt),
		Columns:   normalizeBoardColumns(board.Columns),
		Tasks:     normalizeBoardTasks(board.Tasks, board.Columns),
	}
	meta, _ := yaml.Marshal(item)
	return "---\n" + string(meta) + "---\n"
}

func validateBoard(board Board) error {
	if strings.TrimSpace(board.ProjectID) == "" {
		return fmt.Errorf("project id is required")
	}
	for _, task := range board.Tasks {
		if strings.TrimSpace(task.ID) == "" {
			return fmt.Errorf("task id is required")
		}
		if strings.TrimSpace(task.Title) == "" {
			return fmt.Errorf("task title is required")
		}
	}
	return nil
}

func normalizeBoardColumns(raw []string) []string {
	if len(raw) == 0 {
		return append([]string(nil), defaultBoardColumns...)
	}
	out := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, item := range raw {
		trimmed := strings.ToLower(strings.TrimSpace(item))
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return append([]string(nil), defaultBoardColumns...)
	}
	return out
}

func normalizeBoardTasks(raw []BoardTask, columns []string) []BoardTask {
	if len(raw) == 0 {
		return []BoardTask{}
	}
	allowedStatus := map[string]struct{}{}
	normalizedColumns := normalizeBoardColumns(columns)
	for _, col := range normalizedColumns {
		allowedStatus[col] = struct{}{}
	}
	out := make([]BoardTask, 0, len(raw))
	for _, task := range raw {
		item := BoardTask{
			ID:               strings.TrimSpace(task.ID),
			Title:            strings.TrimSpace(task.Title),
			Status:           strings.ToLower(strings.TrimSpace(task.Status)),
			Assignee:         strings.TrimSpace(task.Assignee),
			Role:             strings.TrimSpace(task.Role),
			WorkerKind:       strings.ToLower(strings.TrimSpace(task.WorkerKind)),
			ReviewApprovedBy: strings.TrimSpace(task.ReviewApprovedBy),
			ReviewRequired:   task.ReviewRequired,
			TestCommand:      strings.TrimSpace(task.TestCommand),
			BuildCommand:     strings.TrimSpace(task.BuildCommand),
		}
		if _, ok := allowedStatus[item.Status]; !ok {
			item.Status = normalizedColumns[0]
		}
		out = append(out, item)
	}
	return out
}
