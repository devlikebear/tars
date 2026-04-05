package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Plan represents a high-level goal for the current session.
// At most one plan is active per session; setting a new plan archives the previous one.
type Plan struct {
	Goal        string `json:"goal"`
	Constraints string `json:"constraints,omitempty"`
	CreatedAt   string `json:"created_at"`
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
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create sessions directory: %w", err)
	}
	raw, err := json.MarshalIndent(normalizeSessionTasks(tasks), "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tasks: %w", err)
	}
	path := s.tasksPath(sessionID)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write tasks: %w", err)
	}
	return nil
}

func normalizeSessionTasks(tasks SessionTasks) SessionTasks {
	if tasks.Tasks == nil {
		tasks.Tasks = []Task{}
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
	if st.Plan == nil && len(st.Tasks) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Active Session Tasks\n\n")
	if st.Plan != nil {
		b.WriteString("**Plan:** " + strings.TrimSpace(st.Plan.Goal) + "\n")
		if st.Plan.Constraints != "" {
			b.WriteString("**Constraints:** " + strings.TrimSpace(st.Plan.Constraints) + "\n")
		}
		b.WriteString("\n")
	}
	active := false
	for _, t := range st.Tasks {
		if t.Status == "completed" || t.Status == "cancelled" {
			continue
		}
		marker := "[ ]"
		if t.Status == "in_progress" {
			marker = "[>]"
		}
		b.WriteString(fmt.Sprintf("- %s %s: %s\n", marker, t.ID, t.Title))
		active = true
	}
	if !active && st.Plan == nil {
		return ""
	}
	return b.String()
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
