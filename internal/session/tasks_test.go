package session

import (
	"encoding/json"
	"testing"
)

func TestSessionTasks_CRUD(t *testing.T) {
	store := NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}

	// Initially empty
	st, err := store.GetTasks(main.ID)
	if err != nil {
		t.Fatalf("get tasks: %v", err)
	}
	if st.Plan != nil || len(st.Tasks) != 0 {
		t.Fatalf("expected empty tasks, got %+v", st)
	}
	if st.Tasks == nil {
		t.Fatal("expected empty tasks slice, got nil")
	}

	// Set plan and add tasks
	st.Plan = &Plan{Goal: "Refactor auth module", CreatedAt: NowRFC3339()}
	st.Tasks = []Task{
		{ID: "1", Title: "Extract interfaces", Status: "pending"},
		{ID: "2", Title: "Write tests", Status: "pending"},
	}
	if err := store.SaveTasks(main.ID, st); err != nil {
		t.Fatalf("save tasks: %v", err)
	}

	// Read back
	loaded, err := store.GetTasks(main.ID)
	if err != nil {
		t.Fatalf("get tasks after save: %v", err)
	}
	if loaded.Plan == nil || loaded.Plan.Goal != "Refactor auth module" {
		t.Fatalf("expected plan goal, got %+v", loaded.Plan)
	}
	if len(loaded.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(loaded.Tasks))
	}
	if loaded.Tasks[0].Title != "Extract interfaces" {
		t.Fatalf("unexpected first task: %+v", loaded.Tasks[0])
	}
}

func TestSessionTasks_JSONIncludesEmptyTasksArray(t *testing.T) {
	raw, err := json.Marshal(SessionTasks{})
	if err != nil {
		t.Fatalf("marshal session tasks: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal session tasks: %v", err)
	}

	tasks, ok := payload["tasks"]
	if !ok {
		t.Fatalf("expected tasks field in payload, got %s", string(raw))
	}

	items, ok := tasks.([]any)
	if !ok {
		t.Fatalf("expected tasks array, got %T", tasks)
	}
	if len(items) != 0 {
		t.Fatalf("expected empty tasks array, got %+v", items)
	}
}

func TestNextTaskID(t *testing.T) {
	tests := []struct {
		tasks []Task
		want  string
	}{
		{nil, "1"},
		{[]Task{{ID: "1"}, {ID: "2"}}, "3"},
		{[]Task{{ID: "5"}, {ID: "2"}}, "6"},
	}
	for _, tt := range tests {
		got := NextTaskID(tt.tasks)
		if got != tt.want {
			t.Errorf("NextTaskID(%v) = %q, want %q", tt.tasks, got, tt.want)
		}
	}
}

func TestValidPlanStatus(t *testing.T) {
	for _, valid := range []string{
		PlanStatusDrafting, PlanStatusProposed, PlanStatusExecuting,
		PlanStatusPaused, PlanStatusCompleted, PlanStatusAborted,
	} {
		if !ValidPlanStatus(valid) {
			t.Errorf("expected %q to be valid", valid)
		}
	}
	for _, invalid := range []string{"", "running", "DONE", "queued"} {
		if invalid == "" {
			continue // empty is treated as legacy default by callers, not by ValidPlanStatus
		}
		if ValidPlanStatus(invalid) {
			t.Errorf("expected %q to be invalid", invalid)
		}
	}
	if ValidPlanStatus("") {
		t.Error("empty string should not validate as a plan status")
	}
}

func TestLegacyPlanWithoutStatusDefaultsToExecuting(t *testing.T) {
	store := NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	// Save a plan with empty Status to simulate a session pre-dating the
	// state machine (the file format is otherwise unchanged).
	if err := store.SaveTasks(main.ID, SessionTasks{
		Plan: &Plan{Goal: "legacy plan", CreatedAt: NowRFC3339()},
		Tasks: []Task{
			{ID: "1", Title: "task one", Status: "in_progress"},
		},
	}); err != nil {
		t.Fatalf("save tasks: %v", err)
	}

	loaded, err := store.GetTasks(main.ID)
	if err != nil {
		t.Fatalf("get tasks: %v", err)
	}
	if loaded.Plan == nil {
		t.Fatal("expected plan to be loaded")
	}
	if loaded.Plan.Status != PlanStatusExecuting {
		t.Errorf("expected legacy plan status to default to %q, got %q", PlanStatusExecuting, loaded.Plan.Status)
	}
}

func TestValidTaskStatus(t *testing.T) {
	for _, valid := range []string{"pending", "in_progress", "completed", "cancelled"} {
		if !ValidTaskStatus(valid) {
			t.Errorf("expected %q to be valid", valid)
		}
	}
	for _, invalid := range []string{"", "done", "unknown"} {
		if ValidTaskStatus(invalid) {
			t.Errorf("expected %q to be invalid", invalid)
		}
	}
}

func TestTaskSummary(t *testing.T) {
	tasks := []Task{
		{ID: "1", Status: "completed"},
		{ID: "2", Status: "pending"},
		{ID: "3", Status: "in_progress"},
		{ID: "4", Status: "cancelled"},
		{ID: "5", Status: "pending"},
	}
	summary := TaskSummary(tasks)
	if summary["total"] != 5 || summary["completed"] != 1 || summary["pending"] != 2 || summary["in_progress"] != 1 || summary["cancelled"] != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestFormatTasksForInjection(t *testing.T) {
	st := SessionTasks{
		Plan: &Plan{Goal: "Build feature X"},
		Tasks: []Task{
			{ID: "1", Title: "Design API", Status: "completed"},
			{ID: "2", Title: "Implement handler", Status: "in_progress"},
			{ID: "3", Title: "Write tests", Status: "pending"},
		},
	}
	result := FormatTasksForInjection(st)
	if result == "" {
		t.Fatal("expected non-empty injection")
	}
	// Should not include completed tasks
	if contains(result, "Design API") {
		t.Fatal("should not include completed task")
	}
	// Should include active tasks
	if !contains(result, "Implement handler") || !contains(result, "Write tests") {
		t.Fatal("should include active tasks")
	}
	if !contains(result, "Build feature X") {
		t.Fatal("should include plan goal")
	}
	if !contains(result, "## Active Plan (preserved across compression)") {
		t.Fatal("should identify the block as compression-preserved plan state")
	}
}

func TestFormatTasksForInjection_ExcludesInactivePlanWithNoActiveTasks(t *testing.T) {
	st := SessionTasks{
		Plan: &Plan{Goal: "Completed cleanup", Status: PlanStatusCompleted},
		Tasks: []Task{
			{ID: "1", Title: "Ship cleanup", Status: "completed"},
			{ID: "2", Title: "Drop old branch", Status: "cancelled"},
		},
	}

	result := FormatTasksForInjection(st)
	if result != "" {
		t.Fatalf("expected no injection for inactive plan/tasks, got %q", result)
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && stringContains(s, substr)
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
