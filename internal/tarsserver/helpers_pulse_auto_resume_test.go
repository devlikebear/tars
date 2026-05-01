package tarsserver

import (
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/pulse"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func setupStalledChatSession(t *testing.T, now time.Time) (string, *session.Store, session.Session) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := store.SetAutomationConsent(sess.ID, &session.SessionAutomationConsent{
		AutoResumeEnabled:      true,
		AutoResumeAfterMinutes: 5,
		AllowedResumeModes:     []string{session.AutoResumeModeRecordAssumptionAndProceed},
	}); err != nil {
		t.Fatalf("set consent: %v", err)
	}
	if err := store.SaveTasks(sess.ID, session.SessionTasks{
		Plan:  &session.Plan{Goal: "ship", Status: session.PlanStatusExecuting},
		Tasks: []session.Task{{ID: "t1", Title: "finish", Status: "in_progress"}},
	}); err != nil {
		t.Fatalf("save tasks: %v", err)
	}
	if err := session.AppendMessage(store.TranscriptPath(sess.ID), session.Message{
		ID:        "msg_waiting",
		Role:      "assistant",
		Content:   "Should I proceed with a documented assumption?",
		Timestamp: now.Add(-10 * time.Minute),
	}); err != nil {
		t.Fatalf("append message: %v", err)
	}
	reloaded, err := store.Get(sess.ID)
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	return root, store, reloaded
}

func TestSessionAutoResumeController_ContinuesOptedInSession(t *testing.T) {
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	root, store, sess := setupStalledChatSession(t, now)
	manager := ops.NewManager(root, ops.Options{HomeDir: t.TempDir(), Now: func() time.Time { return now }})
	var prompts []string

	controller := &sessionAutoResumeController{
		workspaceDir: root,
		store:        store,
		manager:      manager,
		now:          func() time.Time { return now },
		logger:       zerolog.New(io.Discard),
		runTurn: func(ctx context.Context, candidate pulse.StalledChatCandidate, prompt string) (string, error) {
			prompts = append(prompts, prompt)
			return "continued safely", nil
		},
	}

	result, err := controller.AutoContinueStalledChats(context.Background())
	if err != nil {
		t.Fatalf("auto continue: %v", err)
	}
	if result.Resumed != 1 || result.Skipped != 0 || result.Escalated != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(prompts) != 1 || prompts[0] == "" {
		t.Fatalf("expected one generated prompt, got %+v", prompts)
	}
	audit, err := manager.ListAutomationAudit(ops.AutomationAuditListOptions{SessionID: sess.ID})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(audit) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(audit))
	}
	if audit[0].Action != "auto_continue_chat" || audit[0].Result != "success" {
		t.Fatalf("unexpected audit entry: %+v", audit[0])
	}
}

func TestSessionAutoResumeController_EscalatesAfterThreeResumesInWindow(t *testing.T) {
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	root, store, sess := setupStalledChatSession(t, now)
	current := now.Add(-20 * time.Minute)
	manager := ops.NewManager(root, ops.Options{HomeDir: t.TempDir(), Now: func() time.Time { return current }})
	for i := 0; i < 3; i++ {
		if _, err := manager.RecordAutomationAudit(ops.AutomationAuditEntry{
			Actor:     "pulse",
			Action:    "auto_continue_chat",
			Reason:    "previous resume",
			SessionID: sess.ID,
			Result:    "success",
		}); err != nil {
			t.Fatalf("record audit: %v", err)
		}
		current = current.Add(5 * time.Minute)
	}
	current = now
	ran := false
	controller := &sessionAutoResumeController{
		workspaceDir: root,
		store:        store,
		manager:      manager,
		now:          func() time.Time { return now },
		logger:       zerolog.New(io.Discard),
		runTurn: func(ctx context.Context, candidate pulse.StalledChatCandidate, prompt string) (string, error) {
			ran = true
			return "should not run", nil
		},
	}

	result, err := controller.AutoContinueStalledChats(context.Background())
	if err != nil {
		t.Fatalf("auto continue: %v", err)
	}
	if ran {
		t.Fatalf("resume turn should not run after escalation threshold")
	}
	if result.Resumed != 0 || result.Escalated != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	audit, err := manager.ListAutomationAudit(ops.AutomationAuditListOptions{SessionID: sess.ID, Limit: 10})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if audit[0].Result != "escalated" {
		t.Fatalf("latest audit should be escalated, got %+v", audit[0])
	}
}
