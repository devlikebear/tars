package gateway

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/project"
	"github.com/devlikebear/tars/internal/session"
)

func TestResolveSpawnProjectID_UsesSessionProjectAndPersistsOverride(t *testing.T) {
	store := session.NewStore(t.TempDir())
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	projectStore := project.NewStore(t.TempDir(), nil)
	original, err := projectStore.Create(project.CreateInput{Name: "Original"})
	if err != nil {
		t.Fatalf("create original project: %v", err)
	}
	override, err := projectStore.Create(project.CreateInput{Name: "Override"})
	if err != nil {
		t.Fatalf("create override project: %v", err)
	}
	if err := store.SetProjectID(sess.ID, original.ID); err != nil {
		t.Fatalf("set session project: %v", err)
	}

	resolved, err := resolveSpawnProjectID(store, sess.ID, "")
	if err != nil {
		t.Fatalf("resolve project from session: %v", err)
	}
	if resolved != original.ID {
		t.Fatalf("expected session project %q, got %q", original.ID, resolved)
	}

	resolved, err = resolveSpawnProjectID(store, sess.ID, override.ID)
	if err != nil {
		t.Fatalf("resolve override project: %v", err)
	}
	if resolved != override.ID {
		t.Fatalf("expected override project %q, got %q", override.ID, resolved)
	}

	updated, err := store.Get(sess.ID)
	if err != nil {
		t.Fatalf("get updated session: %v", err)
	}
	if updated.ProjectID != override.ID {
		t.Fatalf("expected session project override %q, got %q", override.ID, updated.ProjectID)
	}
}

func TestResolveSpawnSessionID_CreatesDistinctHiddenWorkerSessionsPerProjectRun(t *testing.T) {
	store := session.NewStore(t.TempDir())
	mainSession, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}

	firstSessionID, err := resolveSpawnSessionID(store, SpawnRequest{ProjectID: "proj_demo"}, AgentInfo{}, "worker")
	if err != nil {
		t.Fatalf("resolve worker session: %v", err)
	}
	if firstSessionID == "" || firstSessionID == mainSession.ID {
		t.Fatalf("expected project worker session distinct from main, got %q", firstSessionID)
	}

	secondSessionID, err := resolveSpawnSessionID(store, SpawnRequest{ProjectID: "proj_demo"}, AgentInfo{}, "worker")
	if err != nil {
		t.Fatalf("resolve second worker session: %v", err)
	}
	if secondSessionID == "" || secondSessionID == mainSession.ID {
		t.Fatalf("expected second project worker session distinct from main, got %q", secondSessionID)
	}
	if firstSessionID == secondSessionID {
		t.Fatalf("expected project worker runs to use distinct hidden sessions, got %q", firstSessionID)
	}

	first, err := store.Get(firstSessionID)
	if err != nil {
		t.Fatalf("get first worker session: %v", err)
	}
	if first.Kind != "worker" || !first.Hidden {
		t.Fatalf("unexpected first worker session metadata: %+v", first)
	}
	second, err := store.Get(secondSessionID)
	if err != nil {
		t.Fatalf("get second worker session: %v", err)
	}
	if second.Kind != "worker" || !second.Hidden {
		t.Fatalf("unexpected second worker session metadata: %+v", second)
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
			ProjectID:   "proj_1",
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
			ProjectID:   "proj_demo",
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
	if got := mainMessages[0].Content; !strings.Contains(got, "[RUN SUMMARY]") || !strings.Contains(got, "project_id: proj_demo") || !strings.Contains(got, "status: completed") {
		t.Fatalf("unexpected run summary: %q", got)
	}
}
