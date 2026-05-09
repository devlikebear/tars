package tarsserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/rs/zerolog"
)

func TestSessionTasksVerificationRunsApprovedContractCommands(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	sess, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main session: %v", err)
	}
	if err := store.SaveTasks(sess.ID, session.SessionTasks{
		Plan: &session.Plan{Goal: "Ship verified work", Status: session.PlanStatusExecuting, CreatedAt: session.NowRFC3339()},
		Contract: &session.TaskContract{
			Goal:                 "Ship verified work",
			Status:               session.ContractStatusApproved,
			VerificationCommands: []string{"printf verification-ok"},
		},
		Tasks: []session.Task{{ID: "1", Title: "Run verification", Status: "in_progress"}},
	}); err != nil {
		t.Fatalf("save tasks: %v", err)
	}

	handler := newSessionAPIHandler(store, zerolog.Nop())
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/sessions/"+sess.ID+"/tasks/verify", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Tars-Debug-Auth-Role", "admin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	var payload struct {
		OK      bool `json:"ok"`
		Results []struct {
			Command    string `json:"command"`
			Status     string `json:"status"`
			EvidenceID string `json:"evidence_id"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.OK || len(payload.Results) != 1 {
		t.Fatalf("unexpected verification response: %+v", payload)
	}
	if payload.Results[0].Command != "printf verification-ok" || payload.Results[0].Status != "passed" || payload.Results[0].EvidenceID == "" {
		t.Fatalf("unexpected verification result: %+v", payload.Results[0])
	}

	st, err := store.GetTasks(sess.ID)
	if err != nil {
		t.Fatalf("get tasks: %v", err)
	}
	if len(st.Tasks) != 1 || len(st.Tasks[0].Evidence) != 1 {
		t.Fatalf("expected verification evidence on task, got %+v", st.Tasks)
	}
	ev := st.Tasks[0].Evidence[0]
	if ev.Type != session.EvidenceTypeTestResult || ev.Status != "passed" || ev.Command != "printf verification-ok" {
		t.Fatalf("unexpected evidence: %+v", ev)
	}
	if !strings.Contains(ev.Summary, "verification-ok") {
		t.Fatalf("expected stdout in evidence summary, got %q", ev.Summary)
	}
}

func TestSessionTasksVerificationRequiresApprovedContract(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	sess, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main session: %v", err)
	}
	if err := store.SaveTasks(sess.ID, session.SessionTasks{
		Plan: &session.Plan{Goal: "Draft work", Status: session.PlanStatusDrafting, CreatedAt: session.NowRFC3339()},
		Contract: &session.TaskContract{
			Goal:                 "Draft work",
			Status:               session.ContractStatusDraft,
			VerificationCommands: []string{"printf should-not-run"},
		},
		Tasks: []session.Task{{ID: "1", Title: "Run verification", Status: "pending"}},
	}); err != nil {
		t.Fatalf("save tasks: %v", err)
	}

	handler := newSessionAPIHandler(store, zerolog.Nop())
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/sessions/"+sess.ID+"/tasks/verify", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Tars-Debug-Auth-Role", "admin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "approved") {
		t.Fatalf("expected approved-contract error, got %q", rec.Body.String())
	}
	st, err := store.GetTasks(sess.ID)
	if err != nil {
		t.Fatalf("get tasks: %v", err)
	}
	if len(st.Tasks[0].Evidence) != 0 {
		t.Fatalf("draft contract should not add evidence, got %+v", st.Tasks[0].Evidence)
	}
}

func TestSessionTasksVerificationRecordsFailedCommandEvidence(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	sess, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main session: %v", err)
	}
	if err := store.SaveTasks(sess.ID, session.SessionTasks{
		Plan: &session.Plan{Goal: "Ship verified work", Status: session.PlanStatusExecuting, CreatedAt: session.NowRFC3339()},
		Contract: &session.TaskContract{
			Goal:                 "Ship verified work",
			Status:               session.ContractStatusApproved,
			VerificationCommands: []string{"go env -definitely-not-a-real-flag"},
		},
		Tasks: []session.Task{{ID: "1", Title: "Run verification", Status: "in_progress"}},
	}); err != nil {
		t.Fatalf("save tasks: %v", err)
	}

	handler := newSessionAPIHandler(store, zerolog.Nop())
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/sessions/"+sess.ID+"/tasks/verify", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Tars-Debug-Auth-Role", "admin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	var payload struct {
		OK      bool `json:"ok"`
		Results []struct {
			Command  string `json:"command"`
			Status   string `json:"status"`
			ExitCode int    `json:"exit_code"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.OK || len(payload.Results) != 1 || payload.Results[0].Status != "failed" {
		t.Fatalf("expected failed verification result, got %+v", payload)
	}

	st, err := store.GetTasks(sess.ID)
	if err != nil {
		t.Fatalf("get tasks: %v", err)
	}
	if len(st.Tasks[0].Evidence) != 1 {
		t.Fatalf("expected failed evidence on task, got %+v", st.Tasks[0].Evidence)
	}
	ev := st.Tasks[0].Evidence[0]
	if ev.Status != "failed" || ev.Command != payload.Results[0].Command {
		t.Fatalf("unexpected failed evidence: %+v", ev)
	}
	if !strings.Contains(ev.Summary, "exit_code=") {
		t.Fatalf("expected exit code summary, got %q", ev.Summary)
	}
}

func TestSessionTasksVerificationUsesRequestedTaskID(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	sess, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main session: %v", err)
	}
	if err := store.SaveTasks(sess.ID, session.SessionTasks{
		Plan: &session.Plan{Goal: "Ship verified work", Status: session.PlanStatusExecuting, CreatedAt: session.NowRFC3339()},
		Contract: &session.TaskContract{
			Goal:                 "Ship verified work",
			Status:               session.ContractStatusApproved,
			VerificationCommands: []string{"printf selected-task"},
		},
		Tasks: []session.Task{
			{ID: "1", Title: "Default active task", Status: "in_progress"},
			{ID: "2", Title: "Requested task", Status: "pending"},
		},
	}); err != nil {
		t.Fatalf("save tasks: %v", err)
	}

	handler := newSessionAPIHandler(store, zerolog.Nop())
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/sessions/"+sess.ID+"/tasks/verify", strings.NewReader(`{"task_id":"2"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Tars-Debug-Auth-Role", "admin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	st, err := store.GetTasks(sess.ID)
	if err != nil {
		t.Fatalf("get tasks: %v", err)
	}
	if len(st.Tasks[0].Evidence) != 0 || len(st.Tasks[1].Evidence) != 1 {
		t.Fatalf("expected evidence only on requested task, got %+v", st.Tasks)
	}
}

func TestSessionTasksVerificationRejectsMissingContractCommandsAndTasks(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	sess, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main session: %v", err)
	}
	handler := newSessionAPIHandler(store, zerolog.Nop())

	post := func(t *testing.T) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/sessions/"+sess.ID+"/tasks/verify", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Tars-Debug-Auth-Role", "admin")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	if err := store.SaveTasks(sess.ID, session.SessionTasks{
		Plan:     &session.Plan{Goal: "No contract", Status: session.PlanStatusExecuting, CreatedAt: session.NowRFC3339()},
		Contract: nil,
		Tasks:    []session.Task{{ID: "1", Title: "Task", Status: "pending"}},
	}); err != nil {
		t.Fatalf("save no-contract tasks: %v", err)
	}
	rec := post(t)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "contract") {
		t.Fatalf("expected missing-contract error, got %d %q", rec.Code, rec.Body.String())
	}

	if err := store.SaveTasks(sess.ID, session.SessionTasks{
		Plan:     &session.Plan{Goal: "No commands", Status: session.PlanStatusExecuting, CreatedAt: session.NowRFC3339()},
		Contract: &session.TaskContract{Goal: "No commands", Status: session.ContractStatusApproved},
		Tasks:    []session.Task{{ID: "1", Title: "Task", Status: "pending"}},
	}); err != nil {
		t.Fatalf("save no-command tasks: %v", err)
	}
	rec = post(t)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "no verification commands") {
		t.Fatalf("expected no-command error, got %d %q", rec.Code, rec.Body.String())
	}

	if err := store.SaveTasks(sess.ID, session.SessionTasks{
		Plan:     &session.Plan{Goal: "No tasks", Status: session.PlanStatusExecuting, CreatedAt: session.NowRFC3339()},
		Contract: &session.TaskContract{Goal: "No tasks", Status: session.ContractStatusApproved, VerificationCommands: []string{"printf ok"}},
		Tasks:    nil,
	}); err != nil {
		t.Fatalf("save no-task tasks: %v", err)
	}
	rec = post(t)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "at least one task") {
		t.Fatalf("expected no-task error, got %d %q", rec.Code, rec.Body.String())
	}
}

func TestSelectVerificationTaskIndex(t *testing.T) {
	tasks := []session.Task{
		{ID: "1", Title: "Done", Status: "completed"},
		{ID: "2", Title: "Pending", Status: "pending"},
		{ID: "3", Title: "Active", Status: "in_progress"},
	}
	if idx, err := selectVerificationTaskIndex(tasks, "2"); err != nil || idx != 1 {
		t.Fatalf("requested index = %d, %v", idx, err)
	}
	if idx, err := selectVerificationTaskIndex(tasks, ""); err != nil || idx != 2 {
		t.Fatalf("active index = %d, %v", idx, err)
	}
	if idx, err := selectVerificationTaskIndex(tasks[:2], ""); err != nil || idx != 1 {
		t.Fatalf("pending fallback index = %d, %v", idx, err)
	}
	if idx, err := selectVerificationTaskIndex(tasks[:1], ""); err != nil || idx != 0 {
		t.Fatalf("first fallback index = %d, %v", idx, err)
	}
	if _, err := selectVerificationTaskIndex(tasks, "missing"); err == nil {
		t.Fatal("expected missing requested task to error")
	}
	if _, err := selectVerificationTaskIndex(nil, ""); err == nil {
		t.Fatal("expected empty task list to error")
	}
}

func TestParseVerificationExecResponseFallback(t *testing.T) {
	parsed := parseVerificationExecResponse("custom command", tool.Result{
		Content: []tool.ContentBlock{{Type: "text", Text: "plain failure"}},
		IsError: true,
	})
	if parsed.Command != "custom command" || parsed.ExitCode != -1 || parsed.Message != "plain failure" {
		t.Fatalf("unexpected fallback parse: %+v", parsed)
	}
}
