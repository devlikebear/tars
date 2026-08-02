package session

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
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

func TestSessionTasks_SaveNotifiesObserverAfterDurableWrite(t *testing.T) {
	store := NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}

	notifications := 0
	store.SetTasksSavedHook(func(sessionID string, tasks SessionTasks) {
		notifications++
		if sessionID != main.ID || tasks.Plan == nil || tasks.Plan.Goal != "Observe durable save" {
			t.Fatalf("observer payload session=%q tasks=%#v", sessionID, tasks)
		}
		persisted, getErr := store.GetTasks(sessionID)
		if getErr != nil || persisted.Plan == nil || persisted.Plan.Goal != tasks.Plan.Goal {
			t.Fatalf("observer ran before durable read tasks=%#v err=%v", persisted, getErr)
		}
	})

	if err := store.SaveTasks(main.ID, SessionTasks{
		Plan:  &Plan{Goal: "Observe durable save", Status: PlanStatusDrafting},
		Tasks: []Task{},
	}); err != nil {
		t.Fatalf("save tasks: %v", err)
	}
	if notifications != 1 {
		t.Fatalf("observer notifications = %d, want 1", notifications)
	}

	store.SetTasksSavedHook(nil)
	if err := store.SaveTasks(main.ID, SessionTasks{}); err != nil {
		t.Fatalf("save tasks after clearing observer: %v", err)
	}
	if notifications != 1 {
		t.Fatalf("cleared observer notifications = %d, want 1", notifications)
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

func TestSessionTasks_ContractPersistsAndInjects(t *testing.T) {
	store := NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}

	now := NowRFC3339()
	if err := store.SaveTasks(main.ID, SessionTasks{
		Plan: &Plan{Goal: "Ship contract mode", CreatedAt: now, Status: PlanStatusProposed},
		Contract: &TaskContract{
			Goal:                 "Ship contract mode",
			Scope:                "Console and session task state only",
			DoneCriteria:         []string{"Contract survives reload", "Contract survives compaction"},
			VerificationCommands: []string{"make test"},
			Artifacts:            []string{"PR link"},
			Status:               ContractStatusDraft,
			CreatedAt:            now,
			UpdatedAt:            now,
		},
		Tasks: []Task{{ID: "1", Title: "Add model", Status: "pending"}},
	}); err != nil {
		t.Fatalf("save tasks: %v", err)
	}

	loaded, err := store.GetTasks(main.ID)
	if err != nil {
		t.Fatalf("get tasks: %v", err)
	}
	if loaded.Contract == nil {
		t.Fatal("expected contract to persist")
	}
	if loaded.Contract.Status != ContractStatusDraft {
		t.Fatalf("expected draft contract, got %+v", loaded.Contract)
	}

	injection := FormatTasksForInjection(loaded)
	for _, want := range []string{
		"**Contract Scope:** Console and session task state only",
		"- Contract survives reload",
		"`make test`",
		"PR link",
	} {
		if !strings.Contains(injection, want) {
			t.Fatalf("expected injection to contain %q, got:\n%s", want, injection)
		}
	}
}

func TestSessionTasks_EvidencePersistsAndInjects(t *testing.T) {
	store := NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}

	now := NowRFC3339()
	if err := store.SaveTasks(main.ID, SessionTasks{
		Plan: &Plan{Goal: "Ship evidence panel", CreatedAt: now, Status: PlanStatusExecuting},
		Tasks: []Task{
			{
				ID:     "1",
				Title:  "Run tests",
				Status: "in_progress",
				Evidence: []TaskEvidence{
					{
						ID:        "ev_1",
						Type:      EvidenceTypeTestResult,
						Title:     "Go tests",
						Summary:   "internal/session passed",
						Command:   "go test ./internal/session",
						Status:    "passed",
						CreatedAt: now,
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save tasks: %v", err)
	}

	loaded, err := store.GetTasks(main.ID)
	if err != nil {
		t.Fatalf("get tasks: %v", err)
	}
	if len(loaded.Tasks) != 1 || len(loaded.Tasks[0].Evidence) != 1 {
		t.Fatalf("expected evidence to persist, got %+v", loaded.Tasks)
	}
	ev := loaded.Tasks[0].Evidence[0]
	if ev.Type != EvidenceTypeTestResult || ev.Command != "go test ./internal/session" || ev.Status != "passed" {
		t.Fatalf("unexpected evidence after reload: %+v", ev)
	}

	injection := FormatTasksForInjection(loaded)
	for _, want := range []string{
		"Evidence:",
		"Go tests",
		"internal/session passed",
		"`go test ./internal/session`",
	} {
		if !strings.Contains(injection, want) {
			t.Fatalf("expected injection to contain %q, got:\n%s", want, injection)
		}
	}
}

func TestListSessionsWithPlansFiltersAndSortsActivePlans(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	older, err := store.Create("Older plan")
	if err != nil {
		t.Fatalf("create older session: %v", err)
	}
	newer, err := store.Create("Newer plan")
	if err != nil {
		t.Fatalf("create newer session: %v", err)
	}
	completed, err := store.Create("Completed plan")
	if err != nil {
		t.Fatalf("create completed session: %v", err)
	}
	hidden, err := store.CreateWithOptions("Hidden plan", "worker", true)
	if err != nil {
		t.Fatalf("create hidden session: %v", err)
	}

	olderUpdated := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	newerUpdated := olderUpdated.Add(2 * time.Hour)
	if err := store.SaveTasks(older.ID, SessionTasks{
		Plan: &Plan{Goal: "Older goal", Status: PlanStatusExecuting, CreatedAt: olderUpdated.Add(-time.Hour).Format(time.RFC3339), UpdatedAt: olderUpdated.Format(time.RFC3339)},
		Tasks: []Task{
			{ID: "1", Title: "Done", Status: "completed"},
			{ID: "2", Title: "Next", Status: "pending"},
		},
	}); err != nil {
		t.Fatalf("save older tasks: %v", err)
	}
	if err := store.SaveTasks(newer.ID, SessionTasks{
		Plan:  &Plan{Goal: "Newer goal", Status: PlanStatusPaused, CreatedAt: newerUpdated.Add(-time.Hour).Format(time.RFC3339), UpdatedAt: newerUpdated.Format(time.RFC3339)},
		Tasks: []Task{{ID: "1", Title: "Active", Status: "in_progress"}},
	}); err != nil {
		t.Fatalf("save newer tasks: %v", err)
	}
	if err := store.SaveTasks(completed.ID, SessionTasks{
		Plan:  &Plan{Goal: "Finished goal", Status: PlanStatusCompleted, CreatedAt: newerUpdated.Format(time.RFC3339), UpdatedAt: newerUpdated.Add(time.Hour).Format(time.RFC3339)},
		Tasks: []Task{{ID: "1", Title: "Done", Status: "completed"}},
	}); err != nil {
		t.Fatalf("save completed tasks: %v", err)
	}
	if err := store.SaveTasks(hidden.ID, SessionTasks{
		Plan:  &Plan{Goal: "Hidden goal", Status: PlanStatusExecuting, CreatedAt: newerUpdated.Format(time.RFC3339), UpdatedAt: newerUpdated.Add(2 * time.Hour).Format(time.RFC3339)},
		Tasks: []Task{{ID: "1", Title: "Hidden", Status: "pending"}},
	}); err != nil {
		t.Fatalf("save hidden tasks: %v", err)
	}

	active, err := store.ListSessionsWithPlans(false, true)
	if err != nil {
		t.Fatalf("list active plans: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("expected visible active plans only, got %+v", active)
	}
	if active[0].Session.ID != newer.ID || active[1].Session.ID != older.ID {
		t.Fatalf("expected newest active plans first, got %+v", active)
	}
	if active[0].Summary["in_progress"] != 1 || active[1].Summary["pending"] != 1 || active[1].Summary["completed"] != 1 {
		t.Fatalf("expected task summaries, got %+v", active)
	}

	withHidden, err := store.ListSessionsWithPlans(true, true)
	if err != nil {
		t.Fatalf("list active plans with hidden: %v", err)
	}
	if len(withHidden) != 3 || withHidden[0].Session.ID != hidden.ID {
		t.Fatalf("expected hidden plans included and sorted when requested, got %+v", withHidden)
	}

	allPlans, err := store.ListSessionsWithPlans(false, false)
	if err != nil {
		t.Fatalf("list all plans: %v", err)
	}
	if len(allPlans) != 3 || allPlans[0].Session.ID != completed.ID {
		t.Fatalf("expected inactive plans included when activeOnly=false, got %+v", allPlans)
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
