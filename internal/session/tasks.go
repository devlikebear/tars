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

	// AutoContinueEnabled, when true, lets pulse auto-continue this session
	// after the current plan completes — either by proposing a follow-up
	// plan or marking the goal achieved. The hard iteration cap is enforced
	// via the automation audit log (counting successful auto-continue turns
	// in a rolling window) so it survives plan replacement. Opt-in (default
	// false).
	AutoContinueEnabled bool `json:"auto_continue_enabled,omitempty"`

	// AutoContinueMaxIterations caps how many auto-continue turns may run
	// for this session in the rolling AutoContinueIterationWindow. Zero
	// means use DefaultAutoContinueMaxIterations.
	AutoContinueMaxIterations int `json:"auto_continue_max_iterations,omitempty"`
}

// Auto-continue iteration limits. The hard upper bound is enforced
// regardless of per-plan overrides, and the rolling window is the period
// over which audit-log entries are counted toward the cap.
const (
	DefaultAutoContinueMaxIterations = 5
	AutoContinueIterationsHardCap    = 10
	AutoContinueIterationWindow      = 24 * time.Hour
)

// EffectiveAutoContinueMaxIterations returns the cap that applies to this
// plan, clamping to the hard upper bound and falling back to the default
// when unset.
func (p *Plan) EffectiveAutoContinueMaxIterations() int {
	if p == nil {
		return DefaultAutoContinueMaxIterations
	}
	limit := p.AutoContinueMaxIterations
	if limit <= 0 {
		limit = DefaultAutoContinueMaxIterations
	}
	if limit > AutoContinueIterationsHardCap {
		limit = AutoContinueIterationsHardCap
	}
	return limit
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

// TaskContract makes the implicit work agreement explicit for a session plan.
// It is stored next to the active plan/tasks so reload, compaction, and archive
// flows can keep success criteria attached to the work rather than only in chat.
type TaskContract struct {
	Goal                 string   `json:"goal,omitempty"`
	Scope                string   `json:"scope,omitempty"`
	DoneCriteria         []string `json:"done_criteria,omitempty"`
	VerificationCommands []string `json:"verification_commands,omitempty"`
	Artifacts            []string `json:"artifacts,omitempty"`
	Status               string   `json:"status,omitempty"`
	CreatedAt            string   `json:"created_at,omitempty"`
	UpdatedAt            string   `json:"updated_at,omitempty"`
}

const (
	ContractStatusDraft    = "draft"
	ContractStatusApproved = "approved"
)

const (
	EvidenceTypeTestResult           = "test_result"
	EvidenceTypeImage                = "image"
	EvidenceTypeLogExcerpt           = "log_excerpt"
	EvidenceTypePRLink               = "pr_link"
	EvidenceTypeReleaseTag           = "release_tag"
	EvidenceTypeCommandOutputSummary = "command_output_summary"
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

type TaskEvidence struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Title     string `json:"title,omitempty"`
	Summary   string `json:"summary,omitempty"`
	URL       string `json:"url,omitempty"`
	Command   string `json:"command,omitempty"`
	Path      string `json:"path,omitempty"`
	Status    string `json:"status,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// Task represents a single work item linked to the session plan.
type Task struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Status      string         `json:"status"` // pending, in_progress, completed, cancelled
	Description string         `json:"description,omitempty"`
	Evidence    []TaskEvidence `json:"evidence,omitempty"`
	// RunID, when set, names the agentruntime run that is (or was) executing
	// this task. Set when the run is spawned with a task_id; read-only
	// metadata for UI consumers that want to navigate from a task to the
	// run that worked on it.
	RunID string `json:"run_id,omitempty"`
}

// SessionTasks holds the current plan and its associated tasks for a session.
type SessionTasks struct {
	Plan     *Plan         `json:"plan,omitempty"`
	Contract *TaskContract `json:"contract,omitempty"`
	Tasks    []Task        `json:"tasks"`
}

type SessionWithPlanTasks struct {
	Session   Session        `json:"session"`
	Plan      *Plan          `json:"plan,omitempty"`
	Contract  *TaskContract  `json:"contract,omitempty"`
	Tasks     []Task         `json:"tasks"`
	Summary   map[string]int `json:"summary"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// MarshalJSON keeps the API contract stable by always emitting tasks as an array.
func (st SessionTasks) MarshalJSON() ([]byte, error) {
	type sessionTasksJSON struct {
		Plan     *Plan         `json:"plan,omitempty"`
		Contract *TaskContract `json:"contract,omitempty"`
		Tasks    []Task        `json:"tasks"`
	}

	normalized := normalizeSessionTasks(st)
	return json.Marshal(sessionTasksJSON{
		Plan:     normalized.Plan,
		Contract: normalized.Contract,
		Tasks:    normalized.Tasks,
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
			Contract:  tasks.Contract,
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
	if tasks.Contract != nil {
		tasks.Contract = normalizeTaskContract(tasks.Contract)
	}
	for i := range tasks.Tasks {
		tasks.Tasks[i].Evidence = normalizeTaskEvidenceList(tasks.Tasks[i].Evidence)
	}
	return tasks
}

func normalizeTaskContract(contract *TaskContract) *TaskContract {
	if contract == nil {
		return nil
	}
	contract.Goal = strings.TrimSpace(contract.Goal)
	contract.Scope = strings.TrimSpace(contract.Scope)
	contract.DoneCriteria = cleanStringSlice(contract.DoneCriteria)
	contract.VerificationCommands = cleanStringSlice(contract.VerificationCommands)
	contract.Artifacts = cleanStringSlice(contract.Artifacts)
	contract.Status = strings.ToLower(strings.TrimSpace(contract.Status))
	if contract.Status == "" {
		contract.Status = ContractStatusDraft
	}
	return contract
}

func cleanStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			out = append(out, text)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeTaskEvidenceList(items []TaskEvidence) []TaskEvidence {
	if len(items) == 0 {
		return nil
	}
	out := make([]TaskEvidence, 0, len(items))
	for _, item := range items {
		out = append(out, normalizeTaskEvidence(item))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeTaskEvidence(ev TaskEvidence) TaskEvidence {
	ev.ID = strings.TrimSpace(ev.ID)
	ev.Type = strings.ToLower(strings.TrimSpace(ev.Type))
	if ev.Type == "" {
		ev.Type = EvidenceTypeCommandOutputSummary
	}
	ev.Title = strings.TrimSpace(ev.Title)
	ev.Summary = strings.TrimSpace(ev.Summary)
	ev.URL = strings.TrimSpace(ev.URL)
	ev.Command = strings.TrimSpace(ev.Command)
	ev.Path = strings.TrimSpace(ev.Path)
	ev.Status = strings.ToLower(strings.TrimSpace(ev.Status))
	ev.CreatedAt = strings.TrimSpace(ev.CreatedAt)
	return ev
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

func NextEvidenceID(tasks []Task) string {
	maxID := 0
	for _, task := range tasks {
		for _, ev := range task.Evidence {
			var n int
			if _, err := fmt.Sscanf(ev.ID, "ev_%d", &n); err == nil && n > maxID {
				maxID = n
			}
		}
	}
	return fmt.Sprintf("ev_%d", maxID+1)
}

// ValidTaskStatus checks if a status string is valid.
func ValidTaskStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending", "in_progress", "completed", "cancelled":
		return true
	}
	return false
}

func ValidEvidenceType(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case EvidenceTypeTestResult,
		EvidenceTypeImage,
		EvidenceTypeLogExcerpt,
		EvidenceTypePRLink,
		EvidenceTypeReleaseTag,
		EvidenceTypeCommandOutputSummary:
		return true
	}
	return false
}

func EvidenceTypeLabel(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case EvidenceTypeTestResult:
		return "Test result"
	case EvidenceTypeImage:
		return "Image"
	case EvidenceTypeLogExcerpt:
		return "Log excerpt"
	case EvidenceTypePRLink:
		return "PR link"
	case EvidenceTypeReleaseTag:
		return "Release tag"
	case EvidenceTypeCommandOutputSummary:
		return "Command output"
	default:
		return "Evidence"
	}
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
	if st.Contract != nil {
		writeContractForInjection(&b, st.Contract)
	}
	for _, t := range activeTasks {
		marker := "[ ]"
		if strings.EqualFold(strings.TrimSpace(t.Status), "in_progress") {
			marker = "[>]"
		}
		b.WriteString(fmt.Sprintf("- %s %s: %s\n", marker, t.ID, t.Title))
		writeEvidenceForInjection(&b, t.Evidence)
	}
	return b.String()
}

func writeEvidenceForInjection(b *strings.Builder, evidence []TaskEvidence) {
	if len(evidence) == 0 {
		return
	}
	b.WriteString("  Evidence:\n")
	for _, ev := range evidence {
		label := strings.TrimSpace(ev.Title)
		if label == "" {
			label = EvidenceTypeLabel(ev.Type)
		}
		parts := []string{label}
		if ev.Status != "" {
			parts = append(parts, "status="+ev.Status)
		}
		if ev.Summary != "" {
			parts = append(parts, ev.Summary)
		}
		if ev.Command != "" {
			parts = append(parts, "`"+ev.Command+"`")
		}
		if ev.URL != "" {
			parts = append(parts, ev.URL)
		}
		if ev.Path != "" {
			parts = append(parts, ev.Path)
		}
		b.WriteString("  - " + strings.Join(parts, " — ") + "\n")
	}
}

func writeContractForInjection(b *strings.Builder, contract *TaskContract) {
	if contract == nil {
		return
	}
	if strings.TrimSpace(contract.Goal) != "" {
		b.WriteString("**Contract Goal:** " + strings.TrimSpace(contract.Goal) + "\n")
	}
	if strings.TrimSpace(contract.Scope) != "" {
		b.WriteString("**Contract Scope:** " + strings.TrimSpace(contract.Scope) + "\n")
	}
	if len(contract.DoneCriteria) > 0 {
		b.WriteString("**Done Criteria:**\n")
		for _, item := range contract.DoneCriteria {
			b.WriteString("- " + item + "\n")
		}
	}
	if len(contract.VerificationCommands) > 0 {
		b.WriteString("**Verification Commands:**\n")
		for _, cmd := range contract.VerificationCommands {
			b.WriteString("- `" + cmd + "`\n")
		}
	}
	if len(contract.Artifacts) > 0 {
		b.WriteString("**Expected Artifacts:**\n")
		for _, item := range contract.Artifacts {
			b.WriteString("- " + item + "\n")
		}
	}
	b.WriteString("\n")
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
	if st.Contract != nil {
		b.WriteString("Contract: " + strings.TrimSpace(st.Contract.Goal) + " (" + st.Contract.Status + ")\n")
		if st.Contract.Scope != "" {
			b.WriteString("Scope: " + st.Contract.Scope + "\n")
		}
		if len(st.Contract.DoneCriteria) > 0 {
			b.WriteString("Done criteria:\n")
			for _, item := range st.Contract.DoneCriteria {
				b.WriteString("  - " + item + "\n")
			}
		}
		if len(st.Contract.VerificationCommands) > 0 {
			b.WriteString("Verification:\n")
			for _, cmd := range st.Contract.VerificationCommands {
				b.WriteString("  - " + cmd + "\n")
			}
		}
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
		for _, ev := range t.Evidence {
			label := strings.TrimSpace(ev.Title)
			if label == "" {
				label = EvidenceTypeLabel(ev.Type)
			}
			detail := strings.TrimSpace(ev.Summary)
			if detail == "" {
				detail = strings.TrimSpace(firstNonEmpty(ev.URL, ev.Path, ev.Command))
			}
			if detail == "" {
				b.WriteString(fmt.Sprintf("    evidence: %s\n", label))
			} else {
				b.WriteString(fmt.Sprintf("    evidence: %s — %s\n", label, detail))
			}
		}
	}
	return b.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// NowRFC3339 returns current time in RFC3339 format.
func NowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
