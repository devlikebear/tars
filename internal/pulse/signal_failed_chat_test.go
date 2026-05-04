package pulse

import (
	"context"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/session"
)

func newOptedInSession(id string, title string, now time.Time) session.Session {
	return session.Session{
		ID:        id,
		Title:     title,
		UpdatedAt: now,
		AutomationConsent: &session.SessionAutomationConsent{
			AutoResumeEnabled:      true,
			AutoResumeAfterMinutes: 30,
			AllowedResumeModes:     []string{session.AutoResumeModeRecordAssumptionAndProceed},
		},
	}
}

func newActiveTasks(taskID string) session.SessionTasks {
	return session.SessionTasks{
		Plan:  &session.Plan{Goal: "ongoing", Status: session.PlanStatusExecuting},
		Tasks: []session.Task{{ID: taskID, Title: "wip", Status: "in_progress"}},
	}
}

func TestScanner_FailedChat_DetectsToolErrorTail(t *testing.T) {
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	root := t.TempDir()
	src := &fakeChatSessionSource{
		root:     root,
		sessions: []session.Session{newOptedInSession("sess_failed", "tool fail", now.Add(-45*time.Minute))},
		tasks:    map[string]session.SessionTasks{"sess_failed": newActiveTasks("t1")},
	}
	if err := session.AppendMessage(src.TranscriptPath("sess_failed"), session.Message{
		ID:        "msg_user",
		Role:      "user",
		Content:   "fetch the json",
		Timestamp: now.Add(-50 * time.Minute),
	}); err != nil {
		t.Fatalf("append user: %v", err)
	}
	if err := session.AppendMessage(src.TranscriptPath("sess_failed"), session.Message{
		ID:          "msg_tool_err",
		Role:        "tool",
		ToolName:    "web_fetch",
		ToolCallID:  "call_1",
		Content:     "DNS lookup failed: temporary failure in name resolution",
		ToolIsError: true,
		Timestamp:   now.Add(-45 * time.Minute),
	}); err != nil {
		t.Fatalf("append tool err: %v", err)
	}

	sc := buildScanner(ScannerSources{ChatSessions: src}, Thresholds{}, now)
	got := sc.Scan(context.Background())
	if len(got) != 1 {
		t.Fatalf("want 1 signal, got %d (%+v)", len(got), got)
	}
	sig := got[0]
	if sig.Kind != SignalKindFailedChat {
		t.Fatalf("kind = %s, want %s", sig.Kind, SignalKindFailedChat)
	}
	if sig.Details["failure_kind"] != FailedChatKindToolError {
		t.Fatalf("failure_kind = %+v", sig.Details["failure_kind"])
	}
	if sig.Details["failing_tool"] != "web_fetch" {
		t.Fatalf("failing_tool = %+v", sig.Details["failing_tool"])
	}
	if sig.Details["can_auto_resume"] != true {
		t.Fatalf("can_auto_resume = %+v", sig.Details["can_auto_resume"])
	}
	if sig.Details["autofix_candidate"] != "auto_resume_failed_chat" {
		t.Fatalf("autofix_candidate = %+v", sig.Details["autofix_candidate"])
	}
}

func TestScanner_FailedChat_BlocksHighRiskToolFailure(t *testing.T) {
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	root := t.TempDir()
	src := &fakeChatSessionSource{
		root:     root,
		sessions: []session.Session{newOptedInSession("sess_risky", "exec fail", now.Add(-60*time.Minute))},
		tasks:    map[string]session.SessionTasks{"sess_risky": newActiveTasks("t1")},
	}
	if err := session.AppendMessage(src.TranscriptPath("sess_risky"), session.Message{
		Role:      "user",
		Content:   "drop the table",
		Timestamp: now.Add(-65 * time.Minute),
	}); err != nil {
		t.Fatalf("append user: %v", err)
	}
	if err := session.AppendMessage(src.TranscriptPath("sess_risky"), session.Message{
		ID:          "msg_exec_err",
		Role:        "tool",
		ToolName:    "exec",
		ToolCallID:  "call_x",
		Content:     "exit code 1: connection lost mid-write",
		ToolIsError: true,
		Timestamp:   now.Add(-55 * time.Minute),
	}); err != nil {
		t.Fatalf("append exec err: %v", err)
	}

	sc := buildScanner(ScannerSources{ChatSessions: src}, Thresholds{}, now)
	got := sc.Scan(context.Background())
	if len(got) != 1 {
		t.Fatalf("want 1 signal, got %d", len(got))
	}
	if got[0].Details["can_auto_resume"] != false {
		t.Fatalf("high-risk tool failure must not auto-resume: %+v", got[0].Details)
	}
	if got[0].Details["block_reason"] != "high_risk_failure" {
		t.Fatalf("block_reason = %+v", got[0].Details["block_reason"])
	}
}

func TestScanner_FailedChat_DetectsUserMessageWithoutResponse(t *testing.T) {
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	root := t.TempDir()
	src := &fakeChatSessionSource{
		root:     root,
		sessions: []session.Session{newOptedInSession("sess_silent", "silent", now.Add(-45*time.Minute))},
		tasks:    map[string]session.SessionTasks{"sess_silent": newActiveTasks("t1")},
	}
	if err := session.AppendMessage(src.TranscriptPath("sess_silent"), session.Message{
		ID:        "msg_user",
		Role:      "user",
		Content:   "summarize the design doc",
		Timestamp: now.Add(-45 * time.Minute),
	}); err != nil {
		t.Fatalf("append user: %v", err)
	}

	sc := buildScanner(ScannerSources{ChatSessions: src}, Thresholds{}, now)
	got := sc.Scan(context.Background())
	if len(got) != 1 {
		t.Fatalf("want 1 signal, got %d", len(got))
	}
	if got[0].Details["failure_kind"] != FailedChatKindNoResponse {
		t.Fatalf("failure_kind = %+v", got[0].Details["failure_kind"])
	}
	if got[0].Details["can_auto_resume"] != true {
		t.Fatalf("can_auto_resume = %+v", got[0].Details["can_auto_resume"])
	}
}

func TestScanner_FailedChat_SkipsBeforeThreshold(t *testing.T) {
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	root := t.TempDir()
	src := &fakeChatSessionSource{
		root:     root,
		sessions: []session.Session{newOptedInSession("sess_recent", "recent", now)},
		tasks:    map[string]session.SessionTasks{"sess_recent": newActiveTasks("t1")},
	}
	if err := session.AppendMessage(src.TranscriptPath("sess_recent"), session.Message{
		Role:      "user",
		Content:   "hi",
		Timestamp: now.Add(-1 * time.Minute),
	}); err != nil {
		t.Fatalf("append: %v", err)
	}

	sc := buildScanner(ScannerSources{ChatSessions: src}, Thresholds{}, now)
	got := sc.Scan(context.Background())
	for _, sig := range got {
		if sig.Kind == SignalKindFailedChat {
			t.Fatalf("should not emit failed-chat signal under threshold: %+v", sig)
		}
	}
}

func TestScanner_FailedChat_SkipsCompletedTurn(t *testing.T) {
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	root := t.TempDir()
	src := &fakeChatSessionSource{
		root:     root,
		sessions: []session.Session{newOptedInSession("sess_done", "done", now.Add(-50*time.Minute))},
		tasks:    map[string]session.SessionTasks{"sess_done": newActiveTasks("t1")},
	}
	if err := session.AppendMessage(src.TranscriptPath("sess_done"), session.Message{
		Role:      "user",
		Content:   "hello",
		Timestamp: now.Add(-50 * time.Minute),
	}); err != nil {
		t.Fatalf("append user: %v", err)
	}
	if err := session.AppendMessage(src.TranscriptPath("sess_done"), session.Message{
		Role:      "assistant",
		Content:   "All done.",
		Timestamp: now.Add(-49 * time.Minute),
	}); err != nil {
		t.Fatalf("append assistant: %v", err)
	}

	sc := buildScanner(ScannerSources{ChatSessions: src}, Thresholds{}, now)
	got := sc.Scan(context.Background())
	for _, sig := range got {
		if sig.Kind == SignalKindFailedChat {
			t.Fatalf("should not emit failed-chat signal when assistant completed: %+v", sig)
		}
	}
}

func TestScanner_FailedChat_SkipsWhenNoActiveWork(t *testing.T) {
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	root := t.TempDir()
	src := &fakeChatSessionSource{
		root:     root,
		sessions: []session.Session{newOptedInSession("sess_idle", "idle", now.Add(-60*time.Minute))},
		tasks: map[string]session.SessionTasks{"sess_idle": {
			Plan:  &session.Plan{Status: session.PlanStatusCompleted},
			Tasks: []session.Task{{ID: "t1", Title: "done", Status: "completed"}},
		}},
	}
	if err := session.AppendMessage(src.TranscriptPath("sess_idle"), session.Message{
		Role:        "tool",
		ToolName:    "web_fetch",
		ToolCallID:  "c",
		Content:     "boom",
		ToolIsError: true,
		Timestamp:   now.Add(-55 * time.Minute),
	}); err != nil {
		t.Fatalf("append: %v", err)
	}

	sc := buildScanner(ScannerSources{ChatSessions: src}, Thresholds{}, now)
	got := sc.Scan(context.Background())
	for _, sig := range got {
		if sig.Kind == SignalKindFailedChat {
			t.Fatalf("should not emit failed-chat signal when no active work: %+v", sig)
		}
	}
}
