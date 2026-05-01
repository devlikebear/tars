package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/atomicwrite"
)

const (
	// TasksInjectionHeader marks task state that was deliberately reinserted
	// after context compression so later compactions can replace stale copies.
	TasksInjectionHeader       = "## Active Plan (preserved across compression)"
	legacyTasksInjectionHeader = "## Active Session Tasks"
)

// Plan represents a high-level goal for the current session.
// At most one plan is active per session; setting a new plan archives the previous one.
//
// Status follows a small state machine:
//
//	drafting ──plan_propose──► proposed ──plan_approve──► executing
//	   ▲                                                     │
//	   │                                                     │ plan_pause
//	   │                                                     ▼
//	   │                                                  paused
//	   │                                                     │
//	   │ user edit                       plan_resume         │
//	   └────────────────────                 ◄───────────────┘
//
//	executing ─(all tasks completed/cancelled)──► completed
//	any (except completed/aborted) ──plan_abort──► aborted
//
// Empty Status (legacy plans saved before this field existed) is treated as
// "executing" on load so existing sessions keep their prior behavior.
type Plan struct {
	Goal        string `json:"goal"`
	Constraints string `json:"constraints,omitempty"`
	CreatedAt   string `json:"created_at"`
	Status      string `json:"status,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// Plan status constants — enumerate the states a plan can be in.
const (
	PlanStatusDrafting  = "drafting"
	PlanStatusProposed  = "proposed"
	PlanStatusExecuting = "executing"
	PlanStatusPaused    = "paused"
	PlanStatusCompleted = "completed"
	PlanStatusAborted   = "aborted"
)

// ValidPlanStatus reports whether s is a recognized plan status.
func ValidPlanStatus(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case PlanStatusDrafting, PlanStatusProposed, PlanStatusExecuting,
		PlanStatusPaused, PlanStatusCompleted, PlanStatusAborted:
		return true
	}
	return false
}

// Task represents a single work item linked to the session plan.
type Task struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"` // pending, in_progress, completed, cancelled
	Description string `json:"description,omitempty"`
}

// SessionTasks holds the current plan and its associated tasks for a session.
type SessionTasks struct {
	Plan  *Plan  `json:"plan,omitempty"`
	Tasks []Task `json:"tasks"`
}

type SessionWithPlanTasks struct {
	Session   Session        `json:"session"`
	Plan      *Plan          `json:"plan,omitempty"`
	Tasks     []Task         `json:"tasks"`
	Summary   map[string]int `json:"summary"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// MarshalJSON keeps the API contract stable by always emitting tasks as an array.
func (st SessionTasks) MarshalJSON() ([]byte, error) {
	type sessionTasksJSON struct {
		Plan  *Plan  `json:"plan,omitempty"`
		Tasks []Task `json:"tasks"`
	}

	normalized := normalizeSessionTasks(st)
	return json.Marshal(sessionTasksJSON{
		Plan:  normalized.Plan,
		Tasks: normalized.Tasks,
	})
}

// GetTasks reads the tasks file for a session. Returns empty SessionTasks if not found.
func (s *Store) GetTasks(sessionID string) (SessionTasks, error) {
	path := s.tasksPath(sessionID)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return normalizeSessionTasks(SessionTasks{}), nil
		}
		return SessionTasks{}, fmt.Errorf("read tasks: %w", err)
	}
	var tasks SessionTasks
	if err := json.Unmarshal(raw, &tasks); err != nil {
		return SessionTasks{}, fmt.Errorf("unmarshal tasks: %w", err)
	}
	return normalizeSessionTasks(tasks), nil
}

// SaveTasks writes the tasks file for a session.
func (s *Store) SaveTasks(sessionID string, tasks SessionTasks) error {
	raw, err := json.MarshalIndent(normalizeSessionTasks(tasks), "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tasks: %w", err)
	}
	if err := atomicwrite.Write(s.tasksPath(sessionID), raw); err != nil {
		return fmt.Errorf("write tasks: %w", err)
	}
	return nil
}

func (s *Store) ListSessionsWithPlans(includeHidden bool, activeOnly bool) ([]SessionWithPlanTasks, error) {
	sessions, err := s.list(includeHidden)
	if err != nil {
		return nil, err
	}
	items := make([]SessionWithPlanTasks, 0, len(sessions))
	for _, sess := range sessions {
		tasks, err := s.GetTasks(sess.ID)
		if err != nil {
			return nil, err
		}
		if tasks.Plan == nil || strings.TrimSpace(tasks.Plan.Goal) == "" {
			continue
		}
		if activeOnly && !isActivePlan(tasks.Plan) {
			continue
		}
		items = append(items, SessionWithPlanTasks{
			Session:   sess,
			Plan:      tasks.Plan,
			Tasks:     tasks.Tasks,
			Summary:   TaskSummary(tasks.Tasks),
			UpdatedAt: planTasksUpdatedAt(sess, tasks),
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if !items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].UpdatedAt.After(items[j].UpdatedAt)
		}
		if !items[i].Session.CreatedAt.Equal(items[j].Session.CreatedAt) {
			return items[i].Session.CreatedAt.After(items[j].Session.CreatedAt)
		}
		return items[i].Session.ID < items[j].Session.ID
	})
	if items == nil {
		return []SessionWithPlanTasks{}, nil
	}
	return items, nil
}

func planTasksUpdatedAt(sess Session, tasks SessionTasks) time.Time {
	if tasks.Plan != nil {
		for _, raw := range []string{tasks.Plan.UpdatedAt, tasks.Plan.CreatedAt} {
			if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(raw)); err == nil {
				return parsed.UTC()
			}
		}
	}
	if !sess.UpdatedAt.IsZero() {
		return sess.UpdatedAt.UTC()
	}
	if !sess.CreatedAt.IsZero() {
		return sess.CreatedAt.UTC()
	}
	return time.Time{}
}

func normalizeSessionTasks(tasks SessionTasks) SessionTasks {
	if tasks.Tasks == nil {
		tasks.Tasks = []Task{}
	}
	// Legacy plans saved before the state machine existed have no Status —
	// treat them as already executing so their behavior is identical to
	// what users observed before this field was introduced.
	if tasks.Plan != nil && strings.TrimSpace(tasks.Plan.Status) == "" {
		tasks.Plan.Status = PlanStatusExecuting
	}
	return tasks
}

func (s *Store) tasksPath(sessionID string) string {
	return filepath.Join(s.dir, sessionID+".tasks.json")
}

// NextTaskID returns the next sequential task ID based on existing tasks.
func NextTaskID(tasks []Task) string {
	max := 0
	for _, t := range tasks {
		var n int
		if _, err := fmt.Sscanf(t.ID, "%d", &n); err == nil && n > max {
			max = n
		}
	}
	return fmt.Sprintf("%d", max+1)
}

// ValidTaskStatus checks if a status string is valid.
func ValidTaskStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending", "in_progress", "completed", "cancelled":
		return true
	}
	return false
}

// TaskSummary returns a compact summary of task statuses.
func TaskSummary(tasks []Task) map[string]int {
	counts := map[string]int{
		"total":       len(tasks),
		"pending":     0,
		"in_progress": 0,
		"completed":   0,
		"cancelled":   0,
	}
	for _, t := range tasks {
		counts[t.Status]++
	}
	return counts
}

// FormatTasksForInjection renders active tasks for system prompt injection
// after context compression. Only includes pending and in_progress tasks.
func FormatTasksForInjection(st SessionTasks) string {
	st = normalizeSessionTasks(st)
	activeTasks := make([]Task, 0, len(st.Tasks))
	for _, t := range st.Tasks {
		switch strings.ToLower(strings.TrimSpace(t.Status)) {
		case "pending", "in_progress":
			activeTasks = append(activeTasks, t)
		}
	}
	planActive := isActivePlan(st.Plan)
	if !planActive && len(activeTasks) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(TasksInjectionHeader + "\n\n")
	if planActive {
		b.WriteString("**Plan:** " + strings.TrimSpace(st.Plan.Goal) + "\n")
		if st.Plan.Constraints != "" {
			b.WriteString("**Constraints:** " + strings.TrimSpace(st.Plan.Constraints) + "\n")
		}
		b.WriteString("\n")
	}
	for _, t := range activeTasks {
		marker := "[ ]"
		if strings.EqualFold(strings.TrimSpace(t.Status), "in_progress") {
			marker = "[>]"
		}
		b.WriteString(fmt.Sprintf("- %s %s: %s\n", marker, t.ID, t.Title))
	}
	return b.String()
}

// IsTasksInjectionMessage reports whether msg is a previously injected active
// plan block. Compaction replaces these blocks with fresh task state.
func IsTasksInjectionMessage(msg Message) bool {
	if msg.Role != "system" {
		return false
	}
	content := strings.TrimSpace(msg.Content)
	return strings.Contains(content, TasksInjectionHeader) || strings.Contains(content, legacyTasksInjectionHeader)
}

func isActivePlan(plan *Plan) bool {
	if plan == nil || strings.TrimSpace(plan.Goal) == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(plan.Status)) {
	case "", PlanStatusDrafting, PlanStatusProposed, PlanStatusExecuting, PlanStatusPaused:
		return true
	case PlanStatusCompleted, PlanStatusAborted:
		return false
	default:
		return true
	}
}

// ArchiveSummary returns a human-readable summary of the plan and tasks for memory archival.
func ArchiveSummary(st SessionTasks) string {
	if st.Plan == nil && len(st.Tasks) == 0 {
		return ""
	}
	var b strings.Builder
	if st.Plan != nil {
		b.WriteString("Plan: " + strings.TrimSpace(st.Plan.Goal))
		if st.Plan.CreatedAt != "" {
			b.WriteString(" (created: " + st.Plan.CreatedAt + ")")
		}
		b.WriteString("\n")
	}
	summary := TaskSummary(st.Tasks)
	b.WriteString(fmt.Sprintf("Tasks: %d total, %d completed, %d cancelled, %d pending\n",
		summary["total"], summary["completed"], summary["cancelled"], summary["pending"]))
	for _, t := range st.Tasks {
		marker := "[ ]"
		switch t.Status {
		case "completed":
			marker = "[x]"
		case "in_progress":
			marker = "[>]"
		case "cancelled":
			marker = "[~]"
		}
		b.WriteString(fmt.Sprintf("  %s %s: %s\n", marker, t.ID, t.Title))
	}
	return b.String()
}

// NowRFC3339 returns current time in RFC3339 format.
func NowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
