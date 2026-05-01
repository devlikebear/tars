package tarsserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func TestGlobalTasksAPIListsActivePlans(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	older, err := store.Create("Older session")
	if err != nil {
		t.Fatalf("create older session: %v", err)
	}
	newer, err := store.Create("Newer session")
	if err != nil {
		t.Fatalf("create newer session: %v", err)
	}
	done, err := store.Create("Done session")
	if err != nil {
		t.Fatalf("create done session: %v", err)
	}

	base := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	if err := store.SaveTasks(older.ID, session.SessionTasks{
		Plan: &session.Plan{Goal: "Older goal", Status: session.PlanStatusExecuting, CreatedAt: base.Add(-time.Hour).Format(time.RFC3339), UpdatedAt: base.Format(time.RFC3339)},
		Tasks: []session.Task{
			{ID: "1", Title: "Done", Status: "completed"},
			{ID: "2", Title: "Next", Status: "pending"},
		},
	}); err != nil {
		t.Fatalf("save older tasks: %v", err)
	}
	if err := store.SaveTasks(newer.ID, session.SessionTasks{
		Plan:     &session.Plan{Goal: "Newer goal", Status: session.PlanStatusPaused, CreatedAt: base.Format(time.RFC3339), UpdatedAt: base.Add(time.Hour).Format(time.RFC3339)},
		Contract: &session.TaskContract{Goal: "Newer goal", DoneCriteria: []string{"criteria"}, Status: session.ContractStatusDraft},
		Tasks:    []session.Task{{ID: "1", Title: "Active", Status: "in_progress"}},
	}); err != nil {
		t.Fatalf("save newer tasks: %v", err)
	}
	if err := store.SaveTasks(done.ID, session.SessionTasks{
		Plan:  &session.Plan{Goal: "Done goal", Status: session.PlanStatusCompleted, CreatedAt: base.Format(time.RFC3339), UpdatedAt: base.Add(2 * time.Hour).Format(time.RFC3339)},
		Tasks: []session.Task{{ID: "1", Title: "Done", Status: "completed"}},
	}); err != nil {
		t.Fatalf("save done tasks: %v", err)
	}

	handler := newSessionAPIHandler(store, zerolog.Nop())
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/tasks?active=true", nil)
	req.Header.Set("Tars-Debug-Auth-Role", "admin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	var payload struct {
		Count int `json:"count"`
		Items []struct {
			Session   session.Session       `json:"session"`
			Plan      *session.Plan         `json:"plan"`
			Contract  *session.TaskContract `json:"contract"`
			Tasks     []session.Task        `json:"tasks"`
			Summary   map[string]int        `json:"summary"`
			UpdatedAt string                `json:"updated_at"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode global tasks response: %v", err)
	}
	if payload.Count != 2 || len(payload.Items) != 2 {
		t.Fatalf("expected two active plan items, got %+v", payload)
	}
	if payload.Items[0].Session.ID != newer.ID || payload.Items[0].Plan.Goal != "Newer goal" {
		t.Fatalf("expected newest active plan first, got %+v", payload.Items)
	}
	if payload.Items[0].Contract == nil || payload.Items[0].Contract.DoneCriteria[0] != "criteria" {
		t.Fatalf("expected contract in global task item, got %+v", payload.Items[0].Contract)
	}
	if payload.Items[1].Summary["pending"] != 1 || payload.Items[1].Summary["completed"] != 1 {
		t.Fatalf("expected task summary counts, got %+v", payload.Items[1])
	}
	if payload.Items[0].UpdatedAt != base.Add(time.Hour).Format(time.RFC3339) {
		t.Fatalf("expected updated_at from plan updated_at, got %+v", payload.Items[0])
	}
}
