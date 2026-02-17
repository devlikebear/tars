package gateway

import (
	"context"
	"path/filepath"
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

func TestRuntimePersistence_RestoreSnapshotAndResumeSequences(t *testing.T) {
	persistDir := filepath.Join(t.TempDir(), "gateway")
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
		Enabled:                           true,
		WorkspaceDir:                      t.TempDir(),
		SessionStore:                      session.NewStore(t.TempDir()),
		ChannelsLocalEnabled:              true,
		GatewayPersistenceEnabled:         true,
		GatewayRunsPersistenceEnabled:     true,
		GatewayChannelsPersistenceEnabled: true,
		GatewayPersistenceDir:             persistDir,
		GatewayRestoreOnStartup:           true,
		RunPrompt: func(_ context.Context, _ string, prompt string) (string, error) {
			return "ok: " + prompt, nil
		},
	})
	t.Cleanup(func() { closeGatewayRuntime(t, rt) })

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
	persistDir := filepath.Join(t.TempDir(), "gateway")
	rt := NewRuntime(RuntimeOptions{
		Enabled:                              true,
		WorkspaceDir:                         t.TempDir(),
		SessionStore:                         session.NewStore(t.TempDir()),
		ChannelsLocalEnabled:                 true,
		GatewayPersistenceEnabled:            true,
		GatewayRunsPersistenceEnabled:        true,
		GatewayChannelsPersistenceEnabled:    true,
		GatewayRunsMaxRecords:                2,
		GatewayChannelsMaxMessagesPerChannel: 2,
		GatewayPersistenceDir:                persistDir,
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
	closeGatewayRuntime(t, rt)

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
	msgs := channels["general"]
	if len(msgs) != 2 {
		t.Fatalf("expected 2 persisted channel messages, got %d", len(msgs))
	}
	if msgs[0].ID != "msg_2" || msgs[1].ID != "msg_3" {
		t.Fatalf("unexpected persisted channel messages: %+v", msgs)
	}
}
