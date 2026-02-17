package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tarsncase/internal/config"
	"github.com/devlikebear/tarsncase/internal/gateway"
	"github.com/devlikebear/tarsncase/internal/session"
	"github.com/rs/zerolog"
)

func newTestGatewayRuntime(t *testing.T) *gateway.Runtime {
	t.Helper()
	store := session.NewStore(filepath.Join(t.TempDir(), "workspace"))
	rt := gateway.NewRuntime(gateway.RuntimeOptions{
		Enabled:                 true,
		WorkspaceDir:            t.TempDir(),
		SessionStore:            store,
		ChannelsLocalEnabled:    true,
		ChannelsWebhookEnabled:  true,
		ChannelsTelegramEnabled: true,
		RunPrompt: func(_ context.Context, _ string, prompt string) (string, error) {
			return "ok: " + prompt, nil
		},
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := rt.Close(ctx); err != nil {
			t.Fatalf("close gateway runtime: %v", err)
		}
	})
	return rt
}

func TestAgentRunsAPIHandler_ListAndGet(t *testing.T) {
	runtime := newTestGatewayRuntime(t)
	run, err := runtime.Spawn(context.Background(), gateway.SpawnRequest{Prompt: "hello"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	h := newAgentRunsAPIHandler(runtime, zerolog.New(io.Discard))

	recList := httptest.NewRecorder()
	reqList := httptest.NewRequest(http.MethodGet, "/v1/agent/runs", nil)
	h.ServeHTTP(recList, reqList)
	if recList.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recList.Code, recList.Body.String())
	}

	recGet := httptest.NewRecorder()
	reqGet := httptest.NewRequest(http.MethodGet, "/v1/agent/runs/"+run.ID, nil)
	h.ServeHTTP(recGet, reqGet)
	if recGet.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recGet.Code, recGet.Body.String())
	}

	waitForGatewayRun(t, runtime, run.ID)
}

func TestAgentRunsAPIHandler_AgentsList(t *testing.T) {
	runtime := newTestGatewayRuntime(t)
	h := newAgentRunsAPIHandler(runtime, zerolog.New(io.Discard))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/agent/agents", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Count  int              `json:"count"`
		Agents []map[string]any `json:"agents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Count < 1 || len(payload.Agents) < 1 {
		t.Fatalf("expected at least one agent, payload=%+v", payload)
	}
	firstName, _ := payload.Agents[0]["name"].(string)
	if strings.TrimSpace(firstName) == "" {
		t.Fatalf("expected agent name, payload=%+v", payload)
	}
}

func TestAgentRunsAPIHandler_AgentsListIncludesSourceEntryDefault(t *testing.T) {
	runtime := newTestGatewayRuntime(t)
	h := newAgentRunsAPIHandler(runtime, zerolog.New(io.Discard))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/agent/agents", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Count  int              `json:"count"`
		Agents []map[string]any `json:"agents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Count != len(payload.Agents) || len(payload.Agents) == 0 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	first := payload.Agents[0]
	source, _ := first["source"].(string)
	entry, _ := first["entry"].(string)
	isDefault, _ := first["default"].(bool)
	if strings.TrimSpace(source) == "" || strings.TrimSpace(entry) == "" {
		t.Fatalf("expected source/entry fields, payload=%+v", payload)
	}
	if !isDefault {
		t.Fatalf("expected default=true for in-process default executor, payload=%+v", payload)
	}
}

func TestAgentRunsAPIHandler_Spawn(t *testing.T) {
	runtime := newTestGatewayRuntime(t)
	h := newAgentRunsAPIHandler(runtime, zerolog.New(io.Discard))

	body, _ := json.Marshal(map[string]any{
		"message": "spawn hello",
		"agent":   "default",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/agent/runs", bytes.NewReader(body))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	runID, _ := payload["run_id"].(string)
	if runID == "" {
		t.Fatalf("expected run_id in response, payload=%+v", payload)
	}
	accepted, _ := payload["accepted"].(bool)
	if !accepted {
		t.Fatalf("expected accepted=true, payload=%+v", payload)
	}

	waitForGatewayRun(t, runtime, runID)
}

func TestAgentRunsAPIHandler_SpawnMissingMessage(t *testing.T) {
	runtime := newTestGatewayRuntime(t)
	h := newAgentRunsAPIHandler(runtime, zerolog.New(io.Discard))

	body, _ := json.Marshal(map[string]any{
		"agent": "default",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/agent/runs", bytes.NewReader(body))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAgentRunsAPIHandler_Cancel(t *testing.T) {
	store := session.NewStore(filepath.Join(t.TempDir(), "workspace"))
	runtime := gateway.NewRuntime(gateway.RuntimeOptions{
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
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := runtime.Close(ctx); err != nil {
			t.Fatalf("close gateway runtime: %v", err)
		}
	})
	run, err := runtime.Spawn(context.Background(), gateway.SpawnRequest{Prompt: "hello"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	h := newAgentRunsAPIHandler(runtime, zerolog.New(io.Discard))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/agent/runs/"+run.ID+"/cancel", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	waitForGatewayRun(t, runtime, run.ID)
}

func TestGatewayAPIHandler_StatusReloadRestart(t *testing.T) {
	runtime := newTestGatewayRuntime(t)
	h := newGatewayAPIHandler(runtime, zerolog.New(io.Discard), nil)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v1/gateway/status"},
		{http.MethodPost, "/v1/gateway/reload"},
		{http.MethodPost, "/v1/gateway/restart"},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, nil)
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s %s expected 200, got %d body=%s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

func TestGatewayAPIHandler_ReloadCallsRefreshHook(t *testing.T) {
	runtime := newTestGatewayRuntime(t)
	called := false
	h := newGatewayAPIHandler(runtime, zerolog.New(io.Discard), func() {
		called = true
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/gateway/reload", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !called {
		t.Fatal("expected gateway reload hook to be called")
	}
}

func TestGatewayAPIHandler_ReloadRefreshesWorkspaceAgents(t *testing.T) {
	workspace := t.TempDir()
	store := session.NewStore(filepath.Join(workspace, "workspace"))
	runPrompt := func(_ context.Context, _ string, _ string) (string, error) {
		return "ok", nil
	}
	runtime := gateway.NewRuntime(gateway.RuntimeOptions{
		Enabled:      true,
		WorkspaceDir: workspace,
		SessionStore: store,
		RunPrompt:    runPrompt,
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := runtime.Close(ctx); err != nil {
			t.Fatalf("close gateway runtime: %v", err)
		}
	})
	cfg := config.Config{WorkspaceDir: workspace}
	refresh := func() {
		executors := buildGatewayExecutors(cfg, runPrompt, zerolog.New(io.Discard))
		runtime.SetExecutors(executors, "")
	}

	h := newGatewayAPIHandler(runtime, zerolog.New(io.Discard), refresh)
	if len(runtime.Agents()) != 1 {
		t.Fatalf("expected only default agent before reload, got %+v", runtime.Agents())
	}

	agentFile := filepath.Join(workspace, "agents", "researcher", "AGENT.md")
	if err := os.MkdirAll(filepath.Dir(agentFile), 0o755); err != nil {
		t.Fatalf("mkdir agent dir: %v", err)
	}
	if err := os.WriteFile(agentFile, []byte("# Researcher\nFocus on evidence"), 0o644); err != nil {
		t.Fatalf("write agent file: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/gateway/reload", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	agents := runtime.Agents()
	if len(agents) < 2 {
		t.Fatalf("expected markdown agent to be registered after reload, got %+v", agents)
	}
	found := false
	for _, item := range agents {
		name, _ := item["name"].(string)
		if name == "researcher" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected researcher in agents after reload, got %+v", agents)
	}
}

func TestChannelsAPIHandler_WebhookAndTelegramInbound(t *testing.T) {
	runtime := newTestGatewayRuntime(t)
	h := newChannelsAPIHandler(runtime, zerolog.New(io.Discard))

	payload, _ := json.Marshal(map[string]any{"text": "hello"})
	recWebhook := httptest.NewRecorder()
	reqWebhook := httptest.NewRequest(http.MethodPost, "/v1/channels/webhook/inbound/general", bytes.NewReader(payload))
	h.ServeHTTP(recWebhook, reqWebhook)
	if recWebhook.Code != http.StatusOK {
		t.Fatalf("webhook expected 200, got %d body=%s", recWebhook.Code, recWebhook.Body.String())
	}

	telPayload, _ := json.Marshal(map[string]any{"message": map[string]any{"text": "hello"}})
	recTelegram := httptest.NewRecorder()
	reqTelegram := httptest.NewRequest(http.MethodPost, "/v1/channels/telegram/webhook/bot-1", bytes.NewReader(telPayload))
	h.ServeHTTP(recTelegram, reqTelegram)
	if recTelegram.Code != http.StatusOK {
		t.Fatalf("telegram expected 200, got %d body=%s", recTelegram.Code, recTelegram.Body.String())
	}
}

func waitForGatewayRun(t *testing.T, runtime *gateway.Runtime, runID string) {
	t.Helper()
	waitCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := runtime.Wait(waitCtx, runID); err != nil {
		t.Fatalf("wait run %s: %v", runID, err)
	}
}
