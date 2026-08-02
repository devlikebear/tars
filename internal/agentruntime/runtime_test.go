package agentruntime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/session"
)

func closeAgentRuntime(t *testing.T, rt *Runtime) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rt.Close(ctx); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
}

func TestRuntimeSpawnAndWait(t *testing.T) {
	store := session.NewStore(t.TempDir())
	rt := NewRuntime(RuntimeOptions{
		Enabled:      true,
		SessionStore: store,
		RunPrompt: func(_ context.Context, _ string, prompt string) (string, error) {
			return "echo: " + prompt, nil
		},
	})
	t.Cleanup(func() { closeAgentRuntime(t, rt) })

	run, err := rt.Spawn(context.Background(), SpawnRequest{Prompt: "hello"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if !run.Accepted {
		t.Fatalf("expected accepted run")
	}
	if strings.TrimSpace(run.SessionID) == "" {
		t.Fatalf("expected session id")
	}
	if run.Agent != "default" {
		t.Fatalf("expected default agent, got %q", run.Agent)
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	final, err := rt.Wait(waitCtx, run.ID)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if final.Status != RunStatusCompleted {
		t.Fatalf("expected completed status, got %s", final.Status)
	}
	if final.Response != "echo: hello" {
		t.Fatalf("unexpected response: %q", final.Response)
	}

	msgs, err := session.ReadMessages(store.TranscriptPath(run.SessionID))
	if err != nil {
		t.Fatalf("read messages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (user+assistant), got %d", len(msgs))
	}
}

func TestRuntimePublishesRunsSnapshotWithoutFilePersistence(t *testing.T) {
	snapshots := make(chan []Run, 16)
	rt := NewRuntime(RuntimeOptions{
		Enabled:      true,
		SessionStore: session.NewStore(t.TempDir()),
		RunPrompt: func(_ context.Context, _ string, prompt string) (string, error) {
			return "observed: " + prompt, nil
		},
		OnRunsSnapshot: func(runs []Run) {
			snapshots <- runs
		},
	})
	t.Cleanup(func() { closeAgentRuntime(t, rt) })

	run, err := rt.Spawn(context.Background(), SpawnRequest{Prompt: "durable mirror"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	final, err := rt.Wait(waitCtx, run.ID)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case observed := <-snapshots:
			if len(observed) == 1 && observed[0].ID == run.ID && observed[0].Status == RunStatusCompleted {
				if observed[0].Response != final.Response {
					t.Fatalf("observed response = %q, want %q", observed[0].Response, final.Response)
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for completed runs snapshot")
		}
	}
}

func TestRuntimeSystemPromptAppendReachesPromptExecutor(t *testing.T) {
	store := session.NewStore(t.TempDir())
	var seenAppend string
	rt := NewRuntime(RuntimeOptions{
		Enabled:      true,
		SessionStore: store,
		RunPrompt: func(ctx context.Context, _ string, prompt string) (string, error) {
			seenAppend = SystemPromptAppendFromContext(ctx)
			return "echo: " + prompt, nil
		},
	})
	t.Cleanup(func() { closeAgentRuntime(t, rt) })

	run, err := rt.Spawn(context.Background(), SpawnRequest{
		Prompt:             "hello",
		SystemPromptAppend: "embodied context block",
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := rt.Wait(waitCtx, run.ID); err != nil {
		t.Fatalf("wait: %v", err)
	}
	if seenAppend != "embodied context block" {
		t.Fatalf("system prompt append = %q", seenAppend)
	}
}

func TestRuntimeRestartFromCheckpointSpawnsDerivedRunWithOverrides(t *testing.T) {
	store := session.NewStore(t.TempDir())
	var callCount int
	var retryAllowedTools []string
	var retryTier string
	var retryOverride *ProviderOverride
	runPrompt := func(_ context.Context, _ string, prompt string, allowedTools []string, tier string, providerOverride *ProviderOverride) (string, error) {
		callCount++
		if callCount == 1 {
			return "", fmt.Errorf("first attempt failed")
		}
		retryAllowedTools = append([]string(nil), allowedTools...)
		retryTier = tier
		retryOverride = CloneProviderOverride(providerOverride)
		if !strings.Contains(prompt, "Use the cached migration result") {
			t.Fatalf("expected prompt adjustment in retry prompt, got %q", prompt)
		}
		return "retry ok", nil
	}
	defaultExecutor, err := NewPromptExecutorWithOptions(PromptExecutorOptions{
		Name:       "default",
		ToolsAllow: []string{"read_file"},
		Tier:       "standard",
		RunPrompt:  runPrompt,
	})
	if err != nil {
		t.Fatalf("new default executor: %v", err)
	}
	reviewerExecutor, err := NewPromptExecutorWithOptions(PromptExecutorOptions{
		Name:       "reviewer",
		ToolsAllow: []string{"read_file", "list_dir"},
		Tier:       "heavy",
		RunPrompt:  runPrompt,
	})
	if err != nil {
		t.Fatalf("new reviewer executor: %v", err)
	}
	rt := NewRuntime(RuntimeOptions{
		Enabled:      true,
		SessionStore: store,
		Executors:    []AgentExecutor{defaultExecutor, reviewerExecutor},
		DefaultAgent: "default",
	})
	t.Cleanup(func() { closeAgentRuntime(t, rt) })

	first, err := rt.Spawn(context.Background(), SpawnRequest{Prompt: "fix failed migration"})
	if err != nil {
		t.Fatalf("spawn first: %v", err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	failed, err := rt.Wait(waitCtx, first.ID)
	if err != nil {
		t.Fatalf("wait first: %v", err)
	}
	if failed.Status != RunStatusFailed {
		t.Fatalf("expected failed first run, got %+v", failed)
	}
	if len(failed.Checkpoints) == 0 {
		t.Fatalf("expected failed run checkpoints, got %+v", failed)
	}

	retry, err := rt.RestartFromCheckpoint(context.Background(), RestartRequest{
		RunID:            failed.ID,
		CheckpointID:     failed.Checkpoints[0].ID,
		Agent:            "reviewer",
		Tier:             "heavy",
		ProviderOverride: &ProviderOverride{Alias: "codex", Model: "gpt-5.5"},
		PromptAdjustment: "Use the cached migration result",
	})
	if err != nil {
		t.Fatalf("restart from checkpoint: %v", err)
	}
	if retry.ParentRunID != failed.ID || retry.RootRunID != failed.ID {
		t.Fatalf("expected retry provenance parent/root, got %+v", retry)
	}
	if retry.RestartedFromRunID != failed.ID || retry.RestartedFromCheckpointID != failed.Checkpoints[0].ID || retry.RestartAttempt != 1 {
		t.Fatalf("expected restart provenance, got %+v", retry)
	}
	if retry.Agent != "reviewer" {
		t.Fatalf("expected reviewer retry agent, got %q", retry.Agent)
	}

	final, err := rt.Wait(waitCtx, retry.ID)
	if err != nil {
		t.Fatalf("wait retry: %v", err)
	}
	if final.Status != RunStatusCompleted || final.Response != "retry ok" {
		t.Fatalf("expected completed retry, got %+v", final)
	}
	if retryTier != "heavy" {
		t.Fatalf("expected heavy retry tier, got %q", retryTier)
	}
	if retryOverride == nil || retryOverride.Alias != "codex" || retryOverride.Model != "gpt-5.5" {
		t.Fatalf("expected retry provider override, got %+v", retryOverride)
	}
	if strings.Join(retryAllowedTools, ",") != "read_file,list_dir" {
		t.Fatalf("expected reviewer permissions, got %+v", retryAllowedTools)
	}
}

func TestRuntimeCapturesFileToolCallSummaryAndEvent(t *testing.T) {
	store := session.NewStore(t.TempDir())
	entered := make(chan struct{})
	release := make(chan struct{})
	rt := NewRuntime(RuntimeOptions{
		Enabled:      true,
		SessionStore: store,
		RunPrompt: func(ctx context.Context, _ string, _ string) (string, error) {
			close(entered)
			<-release
			recorder := RuntimeToolCallRecorderFromContext(ctx)
			if recorder == nil {
				t.Fatal("expected runtime tool call recorder in context")
			}
			recorder(RuntimeToolCall{
				ToolName:   "read_file",
				ToolCallID: "call_read_1",
				ToolArgs:   `{"path":"internal/agentruntime/types.go"}`,
			})
			return "done", nil
		},
	})
	t.Cleanup(func() { closeAgentRuntime(t, rt) })

	run, err := rt.Spawn(context.Background(), SpawnRequest{Prompt: "inspect files"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for run prompt")
	}
	events, unsubscribe := rt.SubscribeRunEvents(run.ID)
	defer unsubscribe()
	close(release)

	var sawToolCall bool
	deadline := time.After(2 * time.Second)
	for !sawToolCall {
		select {
		case evt := <-events:
			if evt.Type == "tool.call" {
				sawToolCall = true
				if evt.ToolName != "read_file" || evt.ToolCallID != "call_read_1" {
					t.Fatalf("unexpected tool call event: %+v", evt)
				}
				if evt.Path != "internal/agentruntime/types.go" || evt.Action != "read" {
					t.Fatalf("unexpected file event path/action: %+v", evt)
				}
			}
		case <-deadline:
			t.Fatal("timed out waiting for tool.call event")
		}
	}

	final, err := rt.Wait(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if len(final.FileAttention) != 1 {
		t.Fatalf("expected one file attention summary, got %+v", final.FileAttention)
	}
	summary := final.FileAttention[0]
	if summary.Path != "internal/agentruntime/types.go" || summary.Reads != 1 || summary.Total != 1 {
		t.Fatalf("unexpected file attention summary: %+v", summary)
	}
	if len(summary.Sparkline) == 0 || summary.Sparkline[0] != 1 {
		t.Fatalf("expected non-empty sparkline with first access, got %+v", summary.Sparkline)
	}
	if final.FileOpsTotal != 1 {
		t.Fatalf("expected file ops total 1, got %d", final.FileOpsTotal)
	}
}

func TestRuntimeSpawn_PersistsSubagentLineageMetadata(t *testing.T) {
	store := session.NewStore(t.TempDir())
	rt := NewRuntime(RuntimeOptions{
		Enabled:                         true,
		SessionStore:                    store,
		AgentRuntimeSubagentsMaxDepth:   1,
		AgentRuntimeSubagentsMaxThreads: 4,
		Executors: []AgentExecutor{
			stubExecutor{
				info: AgentInfo{
					Name:        "explorer",
					Description: "read-only explorer",
					Enabled:     true,
					Kind:        "prompt",
					PolicyMode:  "allowlist",
					ToolsAllow:  []string{"read_file", "list_dir", "glob"},
				},
				exec: func(_ context.Context, req ExecuteRequest) (string, error) {
					return "summary:" + req.Prompt, nil
				},
			},
		},
		DefaultAgent: "explorer",
	})
	t.Cleanup(func() { closeAgentRuntime(t, rt) })

	parent, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create parent session: %v", err)
	}
	run, err := rt.Spawn(context.Background(), SpawnRequest{
		Title:           "scan backend",
		Prompt:          "inspect backend",
		Agent:           "explorer",
		ParentRunID:     "run_parent",
		RootRunID:       "run_root",
		ParentSessionID: parent.ID,
		Depth:           1,
		SessionKind:     "subagent",
		SessionHidden:   true,
	})
	if err != nil {
		t.Fatalf("spawn subagent run: %v", err)
	}
	final, err := rt.Wait(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("wait subagent run: %v", err)
	}
	if final.ParentRunID != "run_parent" || final.RootRunID != "run_root" {
		t.Fatalf("expected lineage ids on run, got %+v", final)
	}
	if final.ParentSessionID != parent.ID {
		t.Fatalf("expected parent session id %q, got %+v", parent.ID, final)
	}
	if final.Depth != 1 || final.SessionKind != "subagent" {
		t.Fatalf("expected depth/session kind metadata, got %+v", final)
	}
	sess, err := store.Get(final.SessionID)
	if err != nil {
		t.Fatalf("get spawned session: %v", err)
	}
	if sess.Kind != "subagent" || !sess.Hidden {
		t.Fatalf("unexpected spawned session metadata: %+v", sess)
	}
}

type stubExecutor struct {
	info AgentInfo
	exec func(ctx context.Context, req ExecuteRequest) (string, error)
}

func (s stubExecutor) Info() AgentInfo {
	return s.info
}

func (s stubExecutor) Execute(ctx context.Context, req ExecuteRequest) (string, error) {
	if s.exec == nil {
		return "", nil
	}
	return s.exec(ctx, req)
}

func TestRuntimeSpawn_UsesExecutorTierWhenRequestTierEmpty(t *testing.T) {
	store := session.NewStore(t.TempDir())
	var seenTier string
	rt := NewRuntime(RuntimeOptions{
		Enabled:      true,
		SessionStore: store,
		Executors: []AgentExecutor{
			stubExecutor{
				info: AgentInfo{Name: "researcher", Enabled: true, Tier: "deep"},
				exec: func(_ context.Context, req ExecuteRequest) (string, error) {
					seenTier = req.Tier
					return "ok", nil
				},
			},
		},
		DefaultAgent: "researcher",
	})
	t.Cleanup(func() { closeAgentRuntime(t, rt) })

	run, err := rt.Spawn(context.Background(), SpawnRequest{Prompt: "inspect"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if run.Tier != "deep" {
		t.Fatalf("expected accepted run tier to use executor tier, got %+v", run)
	}
	final, err := rt.Wait(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if final.Tier != "deep" {
		t.Fatalf("expected final run tier to use executor tier, got %+v", final)
	}
	if seenTier != "deep" {
		t.Fatalf("expected executor request tier deep, got %q", seenTier)
	}
}

func TestRuntimeSpawn_UnknownAgent(t *testing.T) {
	store := session.NewStore(t.TempDir())
	rt := NewRuntime(RuntimeOptions{
		Enabled:      true,
		SessionStore: store,
		RunPrompt: func(_ context.Context, _ string, prompt string) (string, error) {
			return prompt, nil
		},
	})
	t.Cleanup(func() { closeAgentRuntime(t, rt) })

	_, err := rt.Spawn(context.Background(), SpawnRequest{
		Prompt: "hello",
		Agent:  "not-exists",
	})
	if err == nil {
		t.Fatalf("expected unknown agent error")
	}
	if !strings.Contains(err.Error(), "unknown agent") {
		t.Fatalf("expected unknown agent error, got %v", err)
	}
}

func TestRuntimeRunWorkspaceScope(t *testing.T) {
	store := session.NewStore(t.TempDir())
	rt := NewRuntime(RuntimeOptions{
		Enabled:      true,
		SessionStore: store,
		RunPrompt: func(_ context.Context, _ string, prompt string) (string, error) {
			return "echo: " + prompt, nil
		},
	})
	t.Cleanup(func() { closeAgentRuntime(t, rt) })

	runA, err := rt.Spawn(context.Background(), SpawnRequest{WorkspaceID: "ws-a", Prompt: "a"})
	if err != nil {
		t.Fatalf("spawn ws-a: %v", err)
	}
	runB, err := rt.Spawn(context.Background(), SpawnRequest{WorkspaceID: "ws-b", Prompt: "b"})
	if err != nil {
		t.Fatalf("spawn ws-b: %v", err)
	}

	listA := rt.ListByWorkspace("ws-a", 10)
	if len(listA) != 1 || listA[0].ID != runA.ID {
		t.Fatalf("expected only ws-a run, got %+v", listA)
	}
	listB := rt.ListByWorkspace("ws-b", 10)
	if len(listB) != 1 || listB[0].ID != runB.ID {
		t.Fatalf("expected only ws-b run, got %+v", listB)
	}

	if _, ok := rt.GetByWorkspace("ws-a", runB.ID); ok {
		t.Fatalf("expected ws-a get on ws-b run to be blocked")
	}
	if _, err := rt.CancelByWorkspace("ws-a", runB.ID); err == nil {
		t.Fatalf("expected ws-a cancel on ws-b run to be blocked")
	}
}

func TestRuntimeSpawn_WithCustomExecutor(t *testing.T) {
	store := session.NewStore(t.TempDir())
	rt := NewRuntime(RuntimeOptions{
		Enabled:      true,
		SessionStore: store,
		Executors: []AgentExecutor{
			stubExecutor{
				info: AgentInfo{Name: "worker", Description: "worker executor", Enabled: true, Kind: "stub"},
				exec: func(_ context.Context, req ExecuteRequest) (string, error) {
					return "worker:" + req.Prompt, nil
				},
			},
		},
		DefaultAgent: "worker",
	})
	t.Cleanup(func() { closeAgentRuntime(t, rt) })

	run, err := rt.Spawn(context.Background(), SpawnRequest{Prompt: "hello"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if run.Agent != "worker" {
		t.Fatalf("expected worker agent, got %q", run.Agent)
	}
	final, err := rt.Wait(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if final.Response != "worker:hello" {
		t.Fatalf("unexpected final response: %q", final.Response)
	}
	agents := rt.Agents()
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent descriptor, got %d", len(agents))
	}
	if agents[0]["name"] != "worker" {
		t.Fatalf("unexpected agents payload: %+v", agents)
	}
}

func TestResolveRunAllowedTools_PassesThroughExecutorTools(t *testing.T) {
	got := resolveRunAllowedTools("", []string{"memory_get", "memory_save", "web_search", "exec"})
	want := []string{"memory_get", "memory_save", "web_search", "exec"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected allowed tools: got=%v want=%v", got, want)
	}
}

func TestRuntimeSpawn_PromptExecutorSessionRoutingModes(t *testing.T) {
	store := session.NewStore(t.TempDir())
	fixedSession, err := store.Create("fixed")
	if err != nil {
		t.Fatalf("create fixed session: %v", err)
	}
	callerSession, err := store.Create("caller")
	if err != nil {
		t.Fatalf("create caller session: %v", err)
	}

	newExecutor, err := NewPromptExecutorWithOptions(PromptExecutorOptions{
		Name:               "new_agent",
		PolicyMode:         "allowlist",
		ToolsAllow:         []string{"read_file"},
		SessionRoutingMode: "new",
		RunPrompt: func(_ context.Context, _ string, prompt string, _ []string, _ string, _ *ProviderOverride) (string, error) {
			return "new:" + prompt, nil
		},
	})
	if err != nil {
		t.Fatalf("new prompt executor(new): %v", err)
	}
	callerExecutor, err := NewPromptExecutorWithOptions(PromptExecutorOptions{
		Name:               "caller_agent",
		PolicyMode:         "allowlist",
		ToolsAllow:         []string{"read_file"},
		SessionRoutingMode: "caller",
		RunPrompt: func(_ context.Context, _ string, prompt string, _ []string, _ string, _ *ProviderOverride) (string, error) {
			return "caller:" + prompt, nil
		},
	})
	if err != nil {
		t.Fatalf("new prompt executor(caller): %v", err)
	}
	fixedExecutor, err := NewPromptExecutorWithOptions(PromptExecutorOptions{
		Name:               "fixed_agent",
		PolicyMode:         "allowlist",
		ToolsAllow:         []string{"read_file"},
		SessionRoutingMode: "fixed",
		SessionFixedID:     fixedSession.ID,
		RunPrompt: func(_ context.Context, _ string, prompt string, _ []string, _ string, _ *ProviderOverride) (string, error) {
			return "fixed:" + prompt, nil
		},
	})
	if err != nil {
		t.Fatalf("new prompt executor(fixed): %v", err)
	}

	rt := NewRuntime(RuntimeOptions{
		Enabled:      true,
		SessionStore: store,
		Executors:    []AgentExecutor{newExecutor, callerExecutor, fixedExecutor},
		DefaultAgent: "new_agent",
	})
	t.Cleanup(func() { closeAgentRuntime(t, rt) })

	newRun, err := rt.Spawn(context.Background(), SpawnRequest{
		Agent:     "new_agent",
		Prompt:    "hello",
		SessionID: callerSession.ID,
	})
	if err != nil {
		t.Fatalf("spawn new_agent: %v", err)
	}
	if newRun.SessionID == callerSession.ID {
		t.Fatalf("expected new routing to ignore caller session, got %+v", newRun)
	}

	callerRun, err := rt.Spawn(context.Background(), SpawnRequest{
		Agent:     "caller_agent",
		Prompt:    "hello",
		SessionID: callerSession.ID,
	})
	if err != nil {
		t.Fatalf("spawn caller_agent: %v", err)
	}
	if callerRun.SessionID != callerSession.ID {
		t.Fatalf("expected caller routing to preserve session id, got %+v", callerRun)
	}

	fixedRun, err := rt.Spawn(context.Background(), SpawnRequest{
		Agent:     "fixed_agent",
		Prompt:    "hello",
		SessionID: callerSession.ID,
	})
	if err != nil {
		t.Fatalf("spawn fixed_agent: %v", err)
	}
	if fixedRun.SessionID != fixedSession.ID {
		t.Fatalf("expected fixed routing to use fixed session id, got %+v", fixedRun)
	}
}

func TestRuntimeSetExecutors_ReplacesAgentSetForNextSpawn(t *testing.T) {
	store := session.NewStore(t.TempDir())
	rt := NewRuntime(RuntimeOptions{
		Enabled:      true,
		SessionStore: store,
		RunPrompt: func(_ context.Context, _ string, prompt string) (string, error) {
			return "default:" + prompt, nil
		},
		Executors: []AgentExecutor{
			stubExecutor{
				info: AgentInfo{Name: "worker1", Enabled: true, Kind: "stub"},
				exec: func(_ context.Context, req ExecuteRequest) (string, error) {
					return "worker1:" + req.Prompt, nil
				},
			},
		},
		DefaultAgent: "worker1",
	})
	t.Cleanup(func() { closeAgentRuntime(t, rt) })

	rt.SetExecutors([]AgentExecutor{
		stubExecutor{
			info: AgentInfo{Name: "worker2", Enabled: true, Kind: "stub"},
			exec: func(_ context.Context, req ExecuteRequest) (string, error) {
				return "worker2:" + req.Prompt, nil
			},
		},
	}, "worker2")

	run, err := rt.Spawn(context.Background(), SpawnRequest{Prompt: "hello"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if run.Agent != "worker2" {
		t.Fatalf("expected updated default agent worker2, got %q", run.Agent)
	}
	final, err := rt.Wait(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if final.Response != "worker2:hello" {
		t.Fatalf("unexpected final response: %q", final.Response)
	}
}

func TestRuntimeCancelRun(t *testing.T) {
	store := session.NewStore(t.TempDir())
	rt := NewRuntime(RuntimeOptions{
		Enabled:      true,
		SessionStore: store,
		RunPrompt: func(ctx context.Context, _ string, _ string) (string, error) {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(2 * time.Second):
				return "done", nil
			}
		},
	})
	t.Cleanup(func() { closeAgentRuntime(t, rt) })

	run, err := rt.Spawn(context.Background(), SpawnRequest{Prompt: "long"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	canceled, err := rt.Cancel(run.ID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if canceled.Status != RunStatusCanceled {
		t.Fatalf("expected canceled status, got %s", canceled.Status)
	}

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer waitCancel()
	final, err := rt.Wait(waitCtx, run.ID)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if final.Status != RunStatusCanceled {
		t.Fatalf("expected canceled status after wait, got %s", final.Status)
	}
}

func TestRuntimeRunFailure_SetsPolicyDiagnosticCode(t *testing.T) {
	store := session.NewStore(t.TempDir())
	rt := NewRuntime(RuntimeOptions{
		Enabled:      true,
		SessionStore: store,
		Executors: []AgentExecutor{
			stubExecutor{
				info: AgentInfo{
					Name:            "researcher",
					Enabled:         true,
					Kind:            "stub",
					PolicyMode:      "allowlist",
					ToolsAllow:      []string{"read_file", "list_dir"},
					ToolsDeny:       []string{"exec"},
					ToolsRiskMax:    "medium",
					ToolsAllowCount: 2,
				},
				exec: func(_ context.Context, _ ExecuteRequest) (string, error) {
					return "", fmt.Errorf("tool not injected for this request: exec")
				},
			},
		},
		DefaultAgent: "researcher",
	})
	t.Cleanup(func() { closeAgentRuntime(t, rt) })

	run, err := rt.Spawn(context.Background(), SpawnRequest{Prompt: "hello"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	final, err := rt.Wait(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if final.Status != RunStatusFailed {
		t.Fatalf("expected failed run status, got %+v", final)
	}
	if final.DiagnosticCode != "policy_tool_blocked" {
		t.Fatalf("expected policy diagnostic code, got %+v", final)
	}
	if !strings.Contains(final.DiagnosticReason, "tool not injected") {
		t.Fatalf("expected diagnostic reason to include tool block message, got %+v", final)
	}
	if final.PolicyBlockedTool != "exec" {
		t.Fatalf("expected blocked tool to be exec, got %+v", final)
	}
	if len(final.PolicyAllowedTools) != 2 || final.PolicyAllowedTools[0] != "read_file" || final.PolicyAllowedTools[1] != "list_dir" {
		t.Fatalf("expected policy allowed tools to be propagated, got %+v", final.PolicyAllowedTools)
	}
	if len(final.PolicyDeniedTools) != 1 || final.PolicyDeniedTools[0] != "exec" {
		t.Fatalf("expected policy denied tools to be propagated, got %+v", final.PolicyDeniedTools)
	}
	if final.PolicyRiskMax != "medium" {
		t.Fatalf("expected policy risk max to be propagated, got %+v", final.PolicyRiskMax)
	}
}

func TestRuntimeChannelNodes(t *testing.T) {
	rt := NewRuntime(RuntimeOptions{
		Enabled:                 true,
		WorkspaceDir:            t.TempDir(),
		ChannelsLocalEnabled:    true,
		ChannelsWebhookEnabled:  true,
		ChannelsTelegramEnabled: true,
	})
	t.Cleanup(func() { closeAgentRuntime(t, rt) })

	if _, err := rt.MessageSend("general", "", "hello"); err != nil {
		t.Fatalf("message send: %v", err)
	}
	if _, err := rt.InboundWebhook("general", "", "inbound", nil); err != nil {
		t.Fatalf("inbound webhook: %v", err)
	}
	messages, err := rt.MessageRead("general", 10)
	if err != nil {
		t.Fatalf("message read: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 channel messages, got %d", len(messages))
	}
}

func TestRuntime_OutboundTelegramRecordedAsTelegramSource(t *testing.T) {
	rt := NewRuntime(RuntimeOptions{
		Enabled:                 true,
		WorkspaceDir:            t.TempDir(),
		ChannelsTelegramEnabled: true,
	})
	t.Cleanup(func() { closeAgentRuntime(t, rt) })

	msg, err := rt.OutboundTelegram("bot-main", "chat-1", "", "hello telegram", map[string]any{"provider": "telegram"})
	if err != nil {
		t.Fatalf("OutboundTelegram: %v", err)
	}
	if msg.Source != "telegram" || msg.Direction != "outbound" {
		t.Fatalf("unexpected outbound telegram message: %+v", msg)
	}
	if msg.ChannelID != "chat-1" {
		t.Fatalf("expected channel_id chat-1, got %+v", msg)
	}
}

func TestRuntimeClose_CancelsRunningAndBlocksNewSpawn(t *testing.T) {
	store := session.NewStore(t.TempDir())
	rt := NewRuntime(RuntimeOptions{
		Enabled:      true,
		SessionStore: store,
		RunPrompt: func(ctx context.Context, _ string, _ string) (string, error) {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(2 * time.Second):
				return "done", nil
			}
		},
	})

	run, err := rt.Spawn(context.Background(), SpawnRequest{Prompt: "long"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rt.Close(ctx); err != nil {
		t.Fatalf("close runtime: %v", err)
	}

	final, err := rt.Wait(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if final.Status != RunStatusCanceled {
		t.Fatalf("expected canceled status, got %s", final.Status)
	}

	if _, err := rt.Spawn(context.Background(), SpawnRequest{Prompt: "next"}); err == nil {
		t.Fatal("expected spawn to fail after runtime close")
	}
}

func TestRuntimeStatus_AgentsTelemetry(t *testing.T) {
	store := session.NewStore(t.TempDir())
	rt := NewRuntime(RuntimeOptions{
		Enabled:                        true,
		SessionStore:                   store,
		AgentRuntimeAgentsWatchEnabled: true,
		RunPrompt: func(_ context.Context, _ string, prompt string) (string, error) {
			return prompt, nil
		},
		Executors: []AgentExecutor{
			stubExecutor{
				info: AgentInfo{Name: "worker", Enabled: true, Kind: "stub"},
				exec: func(_ context.Context, req ExecuteRequest) (string, error) {
					return req.Prompt, nil
				},
			},
		},
		DefaultAgent: "worker",
	})
	t.Cleanup(func() { closeAgentRuntime(t, rt) })

	status := rt.Status()
	if status.AgentsCount < 1 {
		t.Fatalf("expected at least one agent in status, got %d", status.AgentsCount)
	}
	if !status.AgentsWatchEnabled {
		t.Fatalf("expected agents watch enabled in status")
	}
	if status.AgentsReloadVersion <= 0 {
		t.Fatalf("expected positive agents reload version, got %d", status.AgentsReloadVersion)
	}
	if strings.TrimSpace(status.AgentsLastReloadAt) == "" {
		t.Fatalf("expected agents_last_reload_at in status")
	}

	before := status.AgentsReloadVersion
	rt.SetExecutors([]AgentExecutor{
		stubExecutor{
			info: AgentInfo{Name: "worker2", Enabled: true, Kind: "stub"},
			exec: func(_ context.Context, req ExecuteRequest) (string, error) {
				return req.Prompt, nil
			},
		},
	}, "worker2")
	after := rt.Status()
	if after.AgentsReloadVersion <= before {
		t.Fatalf("expected agents reload version to increase, before=%d after=%d", before, after.AgentsReloadVersion)
	}
}

func TestRuntimePersistence_RestoreSnapshotAndResumeSequences(t *testing.T) {
	persistDir := filepath.Join(t.TempDir(), "agentruntime")
	store := newSnapshotStore(persistDir)
	if err := store.writeRuns([]Run{
		{
			ID:          "run_2",
			SessionID:   "sess_1",
			Agent:       "default",
			Prompt:      "done",
			Status:      RunStatusCompleted,
			Accepted:    true,
			CreatedAt:   "2026-02-17T10:00:00Z",
			StartedAt:   "2026-02-17T10:00:01Z",
			CompletedAt: "2026-02-17T10:00:02Z",
			UpdatedAt:   "2026-02-17T10:00:02Z",
		},
		{
			ID:        "run_3",
			SessionID: "sess_2",
			Agent:     "default",
			Prompt:    "running",
			Status:    RunStatusRunning,
			Accepted:  true,
			CreatedAt: "2026-02-17T10:01:00Z",
			StartedAt: "2026-02-17T10:01:01Z",
			UpdatedAt: "2026-02-17T10:01:01Z",
		},
	}); err != nil {
		t.Fatalf("write runs snapshot: %v", err)
	}
	if err := store.writeChannels(map[string][]ChannelMessage{
		"general": {
			{ID: "msg_2", ChannelID: "general", Direction: "outbound", Source: "local", Text: "hello", Timestamp: "2026-02-17T10:00:00Z"},
		},
	}); err != nil {
		t.Fatalf("write channels snapshot: %v", err)
	}

	rt := NewRuntime(RuntimeOptions{
		Enabled:                                true,
		WorkspaceDir:                           t.TempDir(),
		SessionStore:                           session.NewStore(t.TempDir()),
		ChannelsLocalEnabled:                   true,
		AgentRuntimePersistenceEnabled:         true,
		AgentRuntimeRunsPersistenceEnabled:     true,
		AgentRuntimeChannelsPersistenceEnabled: true,
		AgentRuntimePersistenceDir:             persistDir,
		AgentRuntimeRestoreOnStartup:           true,
		RunPrompt: func(_ context.Context, _ string, prompt string) (string, error) {
			return "ok: " + prompt, nil
		},
	})
	t.Cleanup(func() { closeAgentRuntime(t, rt) })

	status := rt.Status()
	if status.RunsRestored != 2 {
		t.Fatalf("expected runs_restored=2, got %d", status.RunsRestored)
	}
	if status.ChannelsRestored != 1 {
		t.Fatalf("expected channels_restored=1, got %d", status.ChannelsRestored)
	}
	if strings.TrimSpace(status.LastRestoreAt) == "" {
		t.Fatalf("expected last_restore_at to be set")
	}

	run2, ok := rt.Get("run_2")
	if !ok {
		t.Fatalf("expected restored run_2")
	}
	if run2.Status != RunStatusCompleted {
		t.Fatalf("expected run_2 completed, got %s", run2.Status)
	}
	run3, ok := rt.Get("run_3")
	if !ok {
		t.Fatalf("expected restored run_3")
	}
	if run3.Status != RunStatusCanceled {
		t.Fatalf("expected run_3 canceled by recovery, got %s", run3.Status)
	}
	if run3.Error != "canceled by restart recovery" {
		t.Fatalf("expected restart recovery error, got %q", run3.Error)
	}

	newRun, err := rt.Spawn(context.Background(), SpawnRequest{Prompt: "fresh"})
	if err != nil {
		t.Fatalf("spawn after restore: %v", err)
	}
	if newRun.ID != "run_4" {
		t.Fatalf("expected next run id run_4, got %q", newRun.ID)
	}
	if _, err := rt.Wait(context.Background(), newRun.ID); err != nil {
		t.Fatalf("wait new run: %v", err)
	}

	msg, err := rt.MessageSend("general", "", "next")
	if err != nil {
		t.Fatalf("message send: %v", err)
	}
	if msg.ID != "msg_3" {
		t.Fatalf("expected next message id msg_3, got %q", msg.ID)
	}
}

func TestRuntimePersistence_TrimsRunsAndChannelMessages(t *testing.T) {
	persistDir := filepath.Join(t.TempDir(), "agentruntime")
	rt := NewRuntime(RuntimeOptions{
		Enabled:                                   true,
		WorkspaceDir:                              t.TempDir(),
		SessionStore:                              session.NewStore(t.TempDir()),
		ChannelsLocalEnabled:                      true,
		AgentRuntimePersistenceEnabled:            true,
		AgentRuntimeRunsPersistenceEnabled:        true,
		AgentRuntimeChannelsPersistenceEnabled:    true,
		AgentRuntimeRunsMaxRecords:                2,
		AgentRuntimeChannelsMaxMessagesPerChannel: 2,
		AgentRuntimePersistenceDir:                persistDir,
		RunPrompt: func(_ context.Context, _ string, prompt string) (string, error) {
			return "ok: " + prompt, nil
		},
	})

	for _, prompt := range []string{"a", "b", "c"} {
		run, err := rt.Spawn(context.Background(), SpawnRequest{Prompt: prompt})
		if err != nil {
			t.Fatalf("spawn %q: %v", prompt, err)
		}
		if _, err := rt.Wait(context.Background(), run.ID); err != nil {
			t.Fatalf("wait run %q: %v", prompt, err)
		}
	}
	for _, text := range []string{"m1", "m2", "m3"} {
		if _, err := rt.MessageSend("general", "", text); err != nil {
			t.Fatalf("message send %q: %v", text, err)
		}
	}
	closeAgentRuntime(t, rt)

	store := newSnapshotStore(persistDir)
	runs, err := store.readRuns()
	if err != nil {
		t.Fatalf("read runs snapshot: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 persisted runs, got %d", len(runs))
	}
	if runs[0].ID != "run_2" || runs[1].ID != "run_3" {
		t.Fatalf("unexpected persisted runs: %+v", runs)
	}

	channels, err := store.readChannels()
	if err != nil {
		t.Fatalf("read channels snapshot: %v", err)
	}
	msgs := channels[workspaceChannelKey(defaultWorkspaceID, "general")]
	if len(msgs) != 2 {
		t.Fatalf("expected 2 persisted channel messages, got %d", len(msgs))
	}
	if msgs[0].ID != "msg_2" || msgs[1].ID != "msg_3" {
		t.Fatalf("unexpected persisted channel messages: %+v", msgs)
	}
}

func TestRuntimePersistence_ConcurrentSnapshotsKeepLatestChannels(t *testing.T) {
	persistDir := filepath.Join(t.TempDir(), "agentruntime")
	rt := NewRuntime(RuntimeOptions{
		Enabled:                                   true,
		WorkspaceDir:                              t.TempDir(),
		SessionStore:                              session.NewStore(t.TempDir()),
		ChannelsLocalEnabled:                      true,
		AgentRuntimePersistenceEnabled:            true,
		AgentRuntimeRunsPersistenceEnabled:        true,
		AgentRuntimeChannelsPersistenceEnabled:    true,
		AgentRuntimeChannelsMaxMessagesPerChannel: 5,
		AgentRuntimePersistenceDir:                persistDir,
	})

	const totalMessages = 150
	errCh := make(chan error, totalMessages)
	for i := 0; i < totalMessages; i++ {
		i := i
		go func() {
			_, err := rt.MessageSend("general", "", fmt.Sprintf("m%03d", i))
			errCh <- err
		}()
	}
	for i := 0; i < totalMessages; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("message send %d: %v", i, err)
		}
	}

	want, err := rt.MessageRead("general", 5)
	if err != nil {
		t.Fatalf("read in-memory messages: %v", err)
	}
	closeAgentRuntime(t, rt)

	store := newSnapshotStore(persistDir)
	channels, err := store.readChannels()
	if err != nil {
		t.Fatalf("read channels snapshot: %v", err)
	}
	got := channels[workspaceChannelKey(defaultWorkspaceID, "general")]
	if len(got) != len(want) {
		t.Fatalf("expected %d persisted channel messages, got %d: %+v", len(want), len(got), got)
	}
	for i := range want {
		if got[i].ID != want[i].ID {
			t.Fatalf("persisted channel messages diverged from memory: want %+v got %+v", want, got)
		}
	}
}

func TestRuntimeArchive_RotateAndRetention(t *testing.T) {
	workspace := t.TempDir()
	archiveDir := filepath.Join(workspace, "archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatalf("mkdir archive dir: %v", err)
	}
	oldFile := filepath.Join(archiveDir, "agentruntime-old.jsonl")
	if err := os.WriteFile(oldFile, []byte("{\"old\":true}\n"), 0o644); err != nil {
		t.Fatalf("write old archive file: %v", err)
	}
	oldTime := time.Now().Add(-72 * time.Hour)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatalf("chtime old archive file: %v", err)
	}

	rt := NewRuntime(RuntimeOptions{
		Enabled:                          true,
		WorkspaceDir:                     workspace,
		SessionStore:                     session.NewStore(t.TempDir()),
		ChannelsLocalEnabled:             true,
		AgentRuntimeReportSummaryEnabled: true,
		AgentRuntimePersistenceEnabled:   true,
		AgentRuntimeArchiveEnabled:       true,
		AgentRuntimeArchiveDir:           archiveDir,
		AgentRuntimeArchiveRetentionDays: 1,
		AgentRuntimeArchiveMaxFileBytes:  256,
		RunPrompt: func(_ context.Context, _ string, prompt string) (string, error) {
			return "ok: " + prompt, nil
		},
	})
	for i := 0; i < 6; i++ {
		run, err := rt.Spawn(context.Background(), SpawnRequest{Prompt: "run"})
		if err != nil {
			t.Fatalf("spawn run %d: %v", i, err)
		}
		if _, err := rt.Wait(context.Background(), run.ID); err != nil {
			t.Fatalf("wait run %d: %v", i, err)
		}
	}
	if _, err := rt.MessageSend("general", "", "archive-message"); err != nil {
		t.Fatalf("message send: %v", err)
	}
	closeAgentRuntime(t, rt)

	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatalf("expected old archive file removed by retention, err=%v", err)
	}
	files, err := os.ReadDir(archiveDir)
	if err != nil {
		t.Fatalf("read archive dir: %v", err)
	}
	archiveFiles := 0
	for _, entry := range files {
		if entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), "agentruntime-") && strings.HasSuffix(entry.Name(), ".jsonl") {
			archiveFiles++
		}
	}
	if archiveFiles == 0 {
		t.Fatalf("expected archive files to be created")
	}
}
