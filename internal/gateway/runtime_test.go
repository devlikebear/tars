package gateway

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tarsncase/internal/session"
)

func closeGatewayRuntime(t *testing.T, rt *Runtime) {
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
	t.Cleanup(func() { closeGatewayRuntime(t, rt) })

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

func TestRuntimeSpawn_UnknownAgent(t *testing.T) {
	store := session.NewStore(t.TempDir())
	rt := NewRuntime(RuntimeOptions{
		Enabled:      true,
		SessionStore: store,
		RunPrompt: func(_ context.Context, _ string, prompt string) (string, error) {
			return prompt, nil
		},
	})
	t.Cleanup(func() { closeGatewayRuntime(t, rt) })

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
	t.Cleanup(func() { closeGatewayRuntime(t, rt) })

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
	t.Cleanup(func() { closeGatewayRuntime(t, rt) })

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
	t.Cleanup(func() { closeGatewayRuntime(t, rt) })

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

func TestRuntimeChannelBrowserNodes(t *testing.T) {
	rt := NewRuntime(RuntimeOptions{
		Enabled:                 true,
		WorkspaceDir:            t.TempDir(),
		ChannelsLocalEnabled:    true,
		ChannelsWebhookEnabled:  true,
		ChannelsTelegramEnabled: true,
	})
	t.Cleanup(func() { closeGatewayRuntime(t, rt) })

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

	state := rt.BrowserStart()
	if !state.Running {
		t.Fatalf("expected browser running")
	}
	if _, err := rt.BrowserOpen("https://example.com"); err != nil {
		t.Fatalf("browser open: %v", err)
	}
	if _, err := rt.BrowserSnapshot(); err != nil {
		t.Fatalf("browser snapshot: %v", err)
	}
	if _, err := rt.BrowserAct("click", "#app", ""); err != nil {
		t.Fatalf("browser act: %v", err)
	}
	shot, err := rt.BrowserScreenshot("")
	if err != nil {
		t.Fatalf("browser screenshot: %v", err)
	}
	if strings.TrimSpace(shot.LastScreenshot) == "" {
		t.Fatalf("expected screenshot path")
	}

	if _, err := rt.NodeDescribe("echo"); err != nil {
		t.Fatalf("node describe: %v", err)
	}
	resp, err := rt.NodeInvoke("echo", map[string]any{"a": 1})
	if err != nil {
		t.Fatalf("node invoke: %v", err)
	}
	if resp["node"] != "echo" {
		t.Fatalf("unexpected node invoke response: %+v", resp)
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
		Enabled:                   true,
		SessionStore:              store,
		GatewayAgentsWatchEnabled: true,
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
	t.Cleanup(func() { closeGatewayRuntime(t, rt) })

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
