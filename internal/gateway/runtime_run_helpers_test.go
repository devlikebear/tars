package gateway

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/session"
)

func TestResolveSpawnSessionID_CreatesHiddenSubagentSessionWhenRequested(t *testing.T) {
	store := session.NewStore(t.TempDir())
	parent, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create parent session: %v", err)
	}

	sessionID, err := resolveSpawnSessionID(store, SpawnRequest{
		Title:           "scan docs",
		ParentSessionID: parent.ID,
		SessionKind:     "subagent",
		SessionHidden:   true,
	}, AgentInfo{}, "explorer")
	if err != nil {
		t.Fatalf("resolve subagent session: %v", err)
	}
	if sessionID == "" || sessionID == parent.ID {
		t.Fatalf("expected hidden subagent session distinct from parent, got %q", sessionID)
	}
	sess, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("get subagent session: %v", err)
	}
	if sess.Kind != "subagent" || !sess.Hidden {
		t.Fatalf("unexpected subagent session metadata: %+v", sess)
	}
}

func TestFinalizeRunLocked_PopulatesPolicyFailureMetadata(t *testing.T) {
	fixedNow := time.Date(2026, 3, 7, 1, 2, 3, 0, time.UTC)
	rt := NewRuntime(RuntimeOptions{
		Enabled:      true,
		SessionStore: session.NewStore(t.TempDir()),
		Now: func() time.Time {
			return fixedNow
		},
	})
	t.Cleanup(func() { closeGatewayRuntime(t, rt) })

	state := &runState{
		run: Run{
			ID:          "run_1",
			Status:      RunStatusRunning,
			CreatedAt:   fixedNow.Add(-time.Minute).Format(time.RFC3339),
			UpdatedAt:   fixedNow.Add(-time.Minute).Format(time.RFC3339),
			SessionID:   "sess_1",
			Accepted:    true,
			Prompt:      "hello",
			Agent:       "researcher",
			WorkspaceID: DefaultWorkspaceID,
		},
		executor: stubExecutor{
			info: AgentInfo{
				Name:         "researcher",
				ToolsAllow:   []string{"read_file", "list_dir"},
				ToolsDeny:    []string{"exec"},
				ToolsRiskMax: "medium",
			},
			exec: func(_ context.Context, _ ExecuteRequest) (string, error) {
				return "", fmt.Errorf("tool not injected for this request: exec")
			},
		},
		done: make(chan struct{}),
	}
	rt.runs[state.run.ID] = state
	rt.runOrder = append(rt.runOrder, state.run.ID)

	rt.mu.Lock()
	rt.finalizeRunLocked(state, "", fmt.Errorf("tool not injected for this request: exec"))
	rt.mu.Unlock()

	if state.run.Status != RunStatusFailed {
		t.Fatalf("expected failed status, got %+v", state.run)
	}
	if state.run.DiagnosticCode != "policy_tool_blocked" {
		t.Fatalf("expected policy diagnostic, got %+v", state.run)
	}
	if state.run.PolicyBlockedTool != "exec" {
		t.Fatalf("expected blocked tool exec, got %+v", state.run)
	}
	if len(state.run.PolicyAllowedTools) != 2 || state.run.PolicyAllowedTools[0] != "read_file" || state.run.PolicyAllowedTools[1] != "list_dir" {
		t.Fatalf("expected allowed tools metadata, got %+v", state.run.PolicyAllowedTools)
	}
	if len(state.run.PolicyDeniedTools) != 1 || state.run.PolicyDeniedTools[0] != "exec" {
		t.Fatalf("expected denied tools metadata, got %+v", state.run.PolicyDeniedTools)
	}
	if state.run.PolicyRiskMax != "medium" {
		t.Fatalf("expected risk metadata, got %+v", state.run.PolicyRiskMax)
	}
	if state.run.CompletedAt != fixedNow.Format(time.RFC3339) {
		t.Fatalf("expected completed_at %s, got %s", fixedNow.Format(time.RFC3339), state.run.CompletedAt)
	}
	if state.run.UpdatedAt != fixedNow.Format(time.RFC3339) {
		t.Fatalf("expected updated_at %s, got %s", fixedNow.Format(time.RFC3339), state.run.UpdatedAt)
	}
	if !state.closed {
		t.Fatalf("expected run to be closed")
	}
}

func TestFinalizeRunLocked_HiddenWorkerSessionAppendsSummaryToMain(t *testing.T) {
	store := session.NewStore(t.TempDir())
	mainSession, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	worker, err := store.EnsureWorker("proj_demo")
	if err != nil {
		t.Fatalf("ensure worker: %v", err)
	}
	fixedNow := time.Date(2026, 3, 7, 3, 4, 5, 0, time.UTC)
	rt := NewRuntime(RuntimeOptions{
		Enabled:      true,
		SessionStore: store,
		Now: func() time.Time {
			return fixedNow
		},
	})
	t.Cleanup(func() { closeGatewayRuntime(t, rt) })

	state := &runState{
		run: Run{
			ID:          "run_summary",
			Status:      RunStatusRunning,
			CreatedAt:   fixedNow.Add(-time.Minute).Format(time.RFC3339),
			UpdatedAt:   fixedNow.Add(-time.Minute).Format(time.RFC3339),
			SessionID:   worker.ID,
			Accepted:    true,
			Prompt:      "draft episode 3",
			Agent:       "novelist",
			WorkspaceID: DefaultWorkspaceID,
		},
		done: make(chan struct{}),
	}
	rt.runs[state.run.ID] = state
	rt.runOrder = append(rt.runOrder, state.run.ID)

	rt.mu.Lock()
	rt.finalizeRunLocked(state, "drafted episode 3 outline and updated state", nil)
	rt.mu.Unlock()

	mainMessages, err := session.ReadMessages(store.TranscriptPath(mainSession.ID))
	if err != nil {
		t.Fatalf("read main transcript: %v", err)
	}
	if len(mainMessages) != 1 {
		t.Fatalf("expected 1 main summary message, got %+v", mainMessages)
	}
	if got := mainMessages[0].Content; !strings.Contains(got, "[RUN SUMMARY]") || !strings.Contains(got, "status: completed") {
		t.Fatalf("unexpected run summary: %q", got)
	}
}
