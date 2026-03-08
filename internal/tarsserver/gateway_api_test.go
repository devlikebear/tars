package tarsserver

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

	"github.com/devlikebear/tarsncase/internal/browser"
	"github.com/devlikebear/tarsncase/internal/config"
	"github.com/devlikebear/tarsncase/internal/gateway"
	"github.com/devlikebear/tarsncase/internal/session"
	"github.com/rs/zerolog"
)

func newTestGatewayRuntime(t *testing.T) *gateway.Runtime {
	t.Helper()
	store := session.NewStore(filepath.Join(t.TempDir(), "workspace"))
	rt := gateway.NewRuntime(gateway.RuntimeOptions{
		Enabled:                     true,
		WorkspaceDir:                t.TempDir(),
		SessionStore:                store,
		ChannelsLocalEnabled:        true,
		ChannelsWebhookEnabled:      true,
		ChannelsTelegramEnabled:     true,
		GatewayReportSummaryEnabled: true,
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
	policyMode, _ := first["policy_mode"].(string)
	if strings.TrimSpace(policyMode) == "" {
		t.Fatalf("expected policy_mode field, payload=%+v", payload)
	}
	if _, ok := first["tools_allow_count"]; !ok {
		t.Fatalf("expected tools_allow_count field, payload=%+v", payload)
	}
	if _, ok := first["tools_allow"]; !ok {
		t.Fatalf("expected tools_allow field, payload=%+v", payload)
	}
	if _, ok := first["tools_allow_groups"]; !ok {
		t.Fatalf("expected tools_allow_groups field, payload=%+v", payload)
	}
	if _, ok := first["tools_allow_patterns"]; !ok {
		t.Fatalf("expected tools_allow_patterns field, payload=%+v", payload)
	}
	if _, ok := first["session_routing_mode"]; !ok {
		t.Fatalf("expected session_routing_mode field, payload=%+v", payload)
	}
	if _, ok := first["session_fixed_id"]; !ok {
		t.Fatalf("expected session_fixed_id field, payload=%+v", payload)
	}
}

func TestAgentRunsAPIHandler_AgentsListIncludesAllowlistPolicyValues(t *testing.T) {
	store := session.NewStore(filepath.Join(t.TempDir(), "workspace"))
	promptExecutor, err := gateway.NewPromptExecutorWithOptions(gateway.PromptExecutorOptions{
		Name:               "researcher",
		Description:        "research worker",
		Source:             "workspace",
		Entry:              "workspace/agents/researcher/AGENT.md",
		PolicyMode:         "allowlist",
		ToolsAllow:         []string{"read_file", "list_dir"},
		ToolsDeny:          []string{"exec"},
		ToolsRiskMax:       "medium",
		ToolsAllowGroups:   []string{"memory"},
		ToolsAllowPatterns: []string{"^read"},
		SessionRoutingMode: "fixed",
		SessionFixedID:     "sess_fixed",
		RunPrompt: func(_ context.Context, _ string, _ string, _ []string) (string, error) {
			return "ok", nil
		},
	})
	if err != nil {
		t.Fatalf("new prompt executor: %v", err)
	}
	runtime := gateway.NewRuntime(gateway.RuntimeOptions{
		Enabled:      true,
		SessionStore: store,
		Executors:    []gateway.AgentExecutor{promptExecutor},
		DefaultAgent: "researcher",
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if closeErr := runtime.Close(ctx); closeErr != nil {
			t.Fatalf("close gateway runtime: %v", closeErr)
		}
	})

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
	if payload.Count == 0 || len(payload.Agents) == 0 {
		t.Fatalf("expected non-empty agents payload: %+v", payload)
	}
	var researcher map[string]any
	for _, item := range payload.Agents {
		name, _ := item["name"].(string)
		if name == "researcher" {
			researcher = item
			break
		}
	}
	if researcher == nil {
		t.Fatalf("expected researcher agent in payload: %+v", payload)
	}
	policyMode, _ := researcher["policy_mode"].(string)
	if policyMode != "allowlist" {
		t.Fatalf("expected allowlist policy mode, got %+v", researcher)
	}
	count, _ := researcher["tools_allow_count"].(float64)
	if int(count) != 2 {
		t.Fatalf("expected tools_allow_count=2, got %+v", researcher)
	}
	tools, ok := researcher["tools_allow"].([]any)
	if !ok || len(tools) != 2 {
		t.Fatalf("expected tools_allow list, got %+v", researcher)
	}
	groups, ok := researcher["tools_allow_groups"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("expected tools_allow_groups list, got %+v", researcher)
	}
	patterns, ok := researcher["tools_allow_patterns"].([]any)
	if !ok || len(patterns) != 1 {
		t.Fatalf("expected tools_allow_patterns list, got %+v", researcher)
	}
	deny, ok := researcher["tools_deny"].([]any)
	if !ok || len(deny) != 1 {
		t.Fatalf("expected tools_deny list, got %+v", researcher)
	}
	denyCount, _ := researcher["tools_deny_count"].(float64)
	if int(denyCount) != 1 {
		t.Fatalf("expected tools_deny_count=1, got %+v", researcher)
	}
	riskMax, _ := researcher["tools_risk_max"].(string)
	if riskMax != "medium" {
		t.Fatalf("expected tools_risk_max=medium, got %+v", researcher)
	}
	routing, _ := researcher["session_routing_mode"].(string)
	if routing != "fixed" {
		t.Fatalf("expected session_routing_mode=fixed, got %+v", researcher)
	}
	fixedID, _ := researcher["session_fixed_id"].(string)
	if fixedID != "sess_fixed" {
		t.Fatalf("expected session_fixed_id=sess_fixed, got %+v", researcher)
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

func TestAgentRunsAPIHandler_SpawnUnknownAgentReturnsDiagnosticCode(t *testing.T) {
	runtime := newTestGatewayRuntime(t)
	h := newAgentRunsAPIHandler(runtime, zerolog.New(io.Discard))

	body, _ := json.Marshal(map[string]any{
		"message": "spawn hello",
		"agent":   "unknown-agent",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/agent/runs", bytes.NewReader(body))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	code, _ := payload["code"].(string)
	if code != "agent_not_found" {
		t.Fatalf("expected code=agent_not_found, payload=%+v", payload)
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

func TestAgentRunsAPIHandler_IgnoresWorkspaceHeaderAndUsesSingleNamespace(t *testing.T) {
	runtime := newTestGatewayRuntime(t)
	baseHandler := newAgentRunsAPIHandler(runtime, zerolog.New(io.Discard))
	handler := applyAPIMiddleware(config.Config{
		APIAuthMode: "off",
	}, zerolog.New(io.Discard), baseHandler, io.Discard)

	spawn := func(workspaceID, message string) map[string]any {
		t.Helper()
		body, _ := json.Marshal(map[string]any{
			"message": message,
			"agent":   "default",
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/agent/runs", bytes.NewReader(body))
		req.Header.Set("Tars-Workspace-Id", workspaceID)
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("spawn expected 202, got %d body=%s", rec.Code, rec.Body.String())
		}
		var payload map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode spawn response: %v", err)
		}
		return payload
	}

	runA := spawn("ws-a", "from a")
	runB := spawn("ws-b", "from b")
	runIDA, _ := runA["run_id"].(string)
	runIDB, _ := runB["run_id"].(string)
	if strings.TrimSpace(runIDA) == "" || strings.TrimSpace(runIDB) == "" {
		t.Fatalf("expected run ids, runA=%+v runB=%+v", runA, runB)
	}

	recListA := httptest.NewRecorder()
	reqListA := httptest.NewRequest(http.MethodGet, "/v1/agent/runs", nil)
	reqListA.Header.Set("Tars-Workspace-Id", "ws-a")
	handler.ServeHTTP(recListA, reqListA)
	if recListA.Code != http.StatusOK {
		t.Fatalf("list expected 200, got %d body=%s", recListA.Code, recListA.Body.String())
	}
	var listPayload map[string]any
	if err := json.Unmarshal(recListA.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	count, _ := listPayload["count"].(float64)
	if int(count) != 2 {
		t.Fatalf("expected run count=2 in single workspace mode, payload=%+v", listPayload)
	}
	runs, _ := listPayload["runs"].([]any)
	if len(runs) != 2 {
		t.Fatalf("expected two runs in single workspace mode, payload=%+v", listPayload)
	}
	seen := map[string]bool{}
	for _, item := range runs {
		run, _ := item.(map[string]any)
		runID, _ := run["run_id"].(string)
		seen[runID] = true
	}
	if !seen[runIDA] || !seen[runIDB] {
		t.Fatalf("expected both runs visible in single workspace mode, payload=%+v", listPayload)
	}

	recGet := httptest.NewRecorder()
	reqGet := httptest.NewRequest(http.MethodGet, "/v1/agent/runs/"+runIDA, nil)
	reqGet.Header.Set("Tars-Workspace-Id", "ws-b")
	handler.ServeHTTP(recGet, reqGet)
	if recGet.Code != http.StatusOK {
		t.Fatalf("expected 200 in single workspace mode, got %d body=%s", recGet.Code, recGet.Body.String())
	}

	recCancel := httptest.NewRecorder()
	reqCancel := httptest.NewRequest(http.MethodPost, "/v1/agent/runs/"+runIDA+"/cancel", nil)
	reqCancel.Header.Set("Tars-Workspace-Id", "ws-b")
	handler.ServeHTTP(recCancel, reqCancel)
	if recCancel.Code != http.StatusOK {
		t.Fatalf("expected 200 in single workspace mode, got %d body=%s", recCancel.Code, recCancel.Body.String())
	}
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

func TestGatewayAPIHandler_StatusIncludesAgentsTelemetry(t *testing.T) {
	runtime := newTestGatewayRuntime(t)
	runtime.SetAgentsWatchEnabled(true)
	h := newGatewayAPIHandler(runtime, zerolog.New(io.Discard), nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/gateway/status", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode status payload: %v", err)
	}
	if _, ok := payload["agents_count"]; !ok {
		t.Fatalf("expected agents_count in status payload: %+v", payload)
	}
	if _, ok := payload["agents_watch_enabled"]; !ok {
		t.Fatalf("expected agents_watch_enabled in status payload: %+v", payload)
	}
	if _, ok := payload["agents_reload_version"]; !ok {
		t.Fatalf("expected agents_reload_version in status payload: %+v", payload)
	}
	if _, ok := payload["agents_last_reload_at"]; !ok {
		t.Fatalf("expected agents_last_reload_at in status payload: %+v", payload)
	}
	if _, ok := payload["persistence_enabled"]; !ok {
		t.Fatalf("expected persistence_enabled in status payload: %+v", payload)
	}
	if _, ok := payload["runs_persistence_enabled"]; !ok {
		t.Fatalf("expected runs_persistence_enabled in status payload: %+v", payload)
	}
	if _, ok := payload["channels_persistence_enabled"]; !ok {
		t.Fatalf("expected channels_persistence_enabled in status payload: %+v", payload)
	}
	if _, ok := payload["restore_on_startup"]; !ok {
		t.Fatalf("expected restore_on_startup in status payload: %+v", payload)
	}
	if _, ok := payload["runs_restored"]; !ok {
		t.Fatalf("expected runs_restored in status payload: %+v", payload)
	}
	if _, ok := payload["channels_restored"]; !ok {
		t.Fatalf("expected channels_restored in status payload: %+v", payload)
	}
}

func TestGatewayAPIHandler_StatusWhenRuntimeMissingHasConsistentDefaults(t *testing.T) {
	h := newGatewayAPIHandler(nil, zerolog.New(io.Discard), nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/gateway/status", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode status payload: %v", err)
	}
	enabled, _ := payload["enabled"].(bool)
	if enabled {
		t.Fatalf("expected enabled=false, payload=%+v", payload)
	}
	if _, ok := payload["agents_count"]; !ok {
		t.Fatalf("expected agents_count in status payload: %+v", payload)
	}
	if _, ok := payload["agents_watch_enabled"]; !ok {
		t.Fatalf("expected agents_watch_enabled in status payload: %+v", payload)
	}
	if _, ok := payload["agents_reload_version"]; !ok {
		t.Fatalf("expected agents_reload_version in status payload: %+v", payload)
	}
	if _, ok := payload["persistence_enabled"]; !ok {
		t.Fatalf("expected persistence_enabled in status payload: %+v", payload)
	}
	if _, ok := payload["runs_persistence_enabled"]; !ok {
		t.Fatalf("expected runs_persistence_enabled in status payload: %+v", payload)
	}
	if _, ok := payload["channels_persistence_enabled"]; !ok {
		t.Fatalf("expected channels_persistence_enabled in status payload: %+v", payload)
	}
	if _, ok := payload["restore_on_startup"]; !ok {
		t.Fatalf("expected restore_on_startup in status payload: %+v", payload)
	}
	if _, ok := payload["runs_restored"]; !ok {
		t.Fatalf("expected runs_restored in status payload: %+v", payload)
	}
	if _, ok := payload["channels_restored"]; !ok {
		t.Fatalf("expected channels_restored in status payload: %+v", payload)
	}
}

func TestGatewayAPIHandler_StatusIncludesPersistenceTelemetryValues(t *testing.T) {
	workspaceDir := t.TempDir()
	runtime := gateway.NewRuntime(gateway.RuntimeOptions{
		Enabled:                              true,
		WorkspaceDir:                         workspaceDir,
		SessionStore:                         session.NewStore(filepath.Join(workspaceDir, "workspace")),
		RunPrompt:                            func(_ context.Context, _ string, prompt string) (string, error) { return prompt, nil },
		GatewayPersistenceEnabled:            true,
		GatewayRunsPersistenceEnabled:        true,
		GatewayChannelsPersistenceEnabled:    false,
		GatewayRunsMaxRecords:                50,
		GatewayChannelsMaxMessagesPerChannel: 10,
		GatewayPersistenceDir:                filepath.Join(workspaceDir, "_shared", "gateway"),
		GatewayRestoreOnStartup:              true,
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := runtime.Close(ctx); err != nil {
			t.Fatalf("close gateway runtime: %v", err)
		}
	})

	h := newGatewayAPIHandler(runtime, zerolog.New(io.Discard), nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/gateway/status", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode status payload: %v", err)
	}
	if enabled, _ := payload["persistence_enabled"].(bool); !enabled {
		t.Fatalf("expected persistence_enabled=true, payload=%+v", payload)
	}
	if runsEnabled, _ := payload["runs_persistence_enabled"].(bool); !runsEnabled {
		t.Fatalf("expected runs_persistence_enabled=true, payload=%+v", payload)
	}
	if channelsEnabled, _ := payload["channels_persistence_enabled"].(bool); channelsEnabled {
		t.Fatalf("expected channels_persistence_enabled=false, payload=%+v", payload)
	}
	if restoreOnStartup, _ := payload["restore_on_startup"].(bool); !restoreOnStartup {
		t.Fatalf("expected restore_on_startup=true, payload=%+v", payload)
	}
	persistenceDir, _ := payload["persistence_dir"].(string)
	if strings.TrimSpace(persistenceDir) == "" {
		t.Fatalf("expected persistence_dir to be set, payload=%+v", payload)
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

func TestGatewayAPIHandler_ReportsSummary(t *testing.T) {
	runtime := newTestGatewayRuntime(t)
	run, err := runtime.Spawn(context.Background(), gateway.SpawnRequest{Prompt: "hello"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	waitForGatewayRun(t, runtime, run.ID)

	h := newGatewayAPIHandler(runtime, zerolog.New(io.Discard), nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/gateway/reports/summary", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode summary payload: %v", err)
	}
	if _, ok := payload["runs_total"]; !ok {
		t.Fatalf("expected runs_total field, payload=%+v", payload)
	}
	if _, ok := payload["runs_by_status"]; !ok {
		t.Fatalf("expected runs_by_status field, payload=%+v", payload)
	}
	if _, ok := payload["messages_by_source"]; !ok {
		t.Fatalf("expected messages_by_source field, payload=%+v", payload)
	}
}

func TestGatewayAPIHandler_ReportsSummarySingleWorkspaceNamespace(t *testing.T) {
	runtime := newTestGatewayRuntime(t)
	baseAgentHandler := newAgentRunsAPIHandler(runtime, zerolog.New(io.Discard))
	agentHandler := applyAPIMiddleware(config.Config{
		APIAuthMode: "off",
	}, zerolog.New(io.Discard), baseAgentHandler, io.Discard)
	baseGatewayHandler := newGatewayAPIHandler(runtime, zerolog.New(io.Discard), nil)
	gatewayHandler := applyAPIMiddleware(config.Config{
		APIAuthMode: "off",
	}, zerolog.New(io.Discard), baseGatewayHandler, io.Discard)

	spawn := func(workspaceID, message string) {
		t.Helper()
		body, _ := json.Marshal(map[string]any{
			"message": message,
			"agent":   "default",
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/agent/runs", bytes.NewReader(body))
		req.Header.Set("Tars-Workspace-Id", workspaceID)
		agentHandler.ServeHTTP(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("spawn expected 202, got %d body=%s", rec.Code, rec.Body.String())
		}
	}
	spawn("ws-a", "hello-a")
	spawn("ws-b", "hello-b")

	summaryFor := func(workspaceID string) map[string]any {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/gateway/reports/summary", nil)
		req.Header.Set("Tars-Workspace-Id", workspaceID)
		gatewayHandler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("summary expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		var payload map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode summary payload: %v", err)
		}
		return payload
	}

	summaryA := summaryFor("ws-a")
	if runsTotal, _ := summaryA["runs_total"].(float64); int(runsTotal) != 2 {
		t.Fatalf("expected ws-a runs_total=2 in single workspace mode, payload=%+v", summaryA)
	}
	summaryB := summaryFor("ws-b")
	if runsTotal, _ := summaryB["runs_total"].(float64); int(runsTotal) != 2 {
		t.Fatalf("expected ws-b runs_total=2 in single workspace mode, payload=%+v", summaryB)
	}
}

func TestGatewayAPIHandler_ReportDetailEndpointsBehindArchiveFlag(t *testing.T) {
	runtime := newTestGatewayRuntime(t)
	h := newGatewayAPIHandler(runtime, zerolog.New(io.Discard), nil)

	recRuns := httptest.NewRecorder()
	reqRuns := httptest.NewRequest(http.MethodGet, "/v1/gateway/reports/runs", nil)
	h.ServeHTTP(recRuns, reqRuns)
	if recRuns.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for runs report when archive disabled, got %d body=%s", recRuns.Code, recRuns.Body.String())
	}

	recChannels := httptest.NewRecorder()
	reqChannels := httptest.NewRequest(http.MethodGet, "/v1/gateway/reports/channels", nil)
	h.ServeHTTP(recChannels, reqChannels)
	if recChannels.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for channels report when archive disabled, got %d body=%s", recChannels.Code, recChannels.Body.String())
	}

	store := session.NewStore(filepath.Join(t.TempDir(), "workspace"))
	archiveRuntime := gateway.NewRuntime(gateway.RuntimeOptions{
		Enabled:                     true,
		WorkspaceDir:                t.TempDir(),
		SessionStore:                store,
		ChannelsLocalEnabled:        true,
		GatewayReportSummaryEnabled: true,
		GatewayArchiveEnabled:       true,
		GatewayArchiveDir:           filepath.Join(t.TempDir(), "archive"),
		RunPrompt: func(_ context.Context, _ string, prompt string) (string, error) {
			return "ok: " + prompt, nil
		},
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := archiveRuntime.Close(ctx); err != nil {
			t.Fatalf("close archive runtime: %v", err)
		}
	})
	run, err := archiveRuntime.Spawn(context.Background(), gateway.SpawnRequest{Prompt: "hello"})
	if err != nil {
		t.Fatalf("spawn archive runtime: %v", err)
	}
	waitForGatewayRun(t, archiveRuntime, run.ID)
	if _, err := archiveRuntime.MessageSend("general", "", "ping"); err != nil {
		t.Fatalf("message send: %v", err)
	}

	archiveHandler := newGatewayAPIHandler(archiveRuntime, zerolog.New(io.Discard), nil)
	recRunsOn := httptest.NewRecorder()
	reqRunsOn := httptest.NewRequest(http.MethodGet, "/v1/gateway/reports/runs?limit=5", nil)
	archiveHandler.ServeHTTP(recRunsOn, reqRunsOn)
	if recRunsOn.Code != http.StatusOK {
		t.Fatalf("expected 200 for runs report when archive enabled, got %d body=%s", recRunsOn.Code, recRunsOn.Body.String())
	}
	var runsPayload map[string]any
	if err := json.Unmarshal(recRunsOn.Body.Bytes(), &runsPayload); err != nil {
		t.Fatalf("decode runs payload: %v", err)
	}
	if _, ok := runsPayload["runs"]; !ok {
		t.Fatalf("expected runs field in payload: %+v", runsPayload)
	}

	recChannelsOn := httptest.NewRecorder()
	reqChannelsOn := httptest.NewRequest(http.MethodGet, "/v1/gateway/reports/channels?limit=5", nil)
	archiveHandler.ServeHTTP(recChannelsOn, reqChannelsOn)
	if recChannelsOn.Code != http.StatusOK {
		t.Fatalf("expected 200 for channels report when archive enabled, got %d body=%s", recChannelsOn.Code, recChannelsOn.Body.String())
	}
	var channelsPayload map[string]any
	if err := json.Unmarshal(recChannelsOn.Body.Bytes(), &channelsPayload); err != nil {
		t.Fatalf("decode channels payload: %v", err)
	}
	if _, ok := channelsPayload["messages"]; !ok {
		t.Fatalf("expected messages field in payload: %+v", channelsPayload)
	}
}

func TestGatewayAPIHandler_ReportsRunsRejectsInvalidLimit(t *testing.T) {
	runtime := newTestGatewayRuntime(t)
	h := newGatewayAPIHandler(runtime, zerolog.New(io.Discard), nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/gateway/reports/runs?limit=0", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["error"] != "limit must be a positive integer" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestGatewayAPIHandler_ReloadRefreshesWorkspaceAgents(t *testing.T) {
	workspace := t.TempDir()
	store := session.NewStore(filepath.Join(workspace, "workspace"))
	runPrompt := func(_ context.Context, _ string, _ string, _ []string) (string, error) {
		return "ok", nil
	}
	runtime := gateway.NewRuntime(gateway.RuntimeOptions{
		Enabled:      true,
		WorkspaceDir: workspace,
		SessionStore: store,
		RunPrompt: func(ctx context.Context, runLabel string, prompt string) (string, error) {
			return runPrompt(ctx, runLabel, prompt, nil)
		},
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

func TestBrowserAPIHandler_LoginRejectsInvalidBody(t *testing.T) {
	runtime := newTestGatewayRuntime(t)
	h := newBrowserAPIHandler(runtime, vaultStatusSnapshot{}, zerolog.New(io.Discard))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/browser/login", strings.NewReader("{"))
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["error"] != "invalid request body" {
		t.Fatalf("unexpected payload: %+v", payload)
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

func TestChannelsAPI_TelegramSend_UserAllowed(t *testing.T) {
	runtime := newTestGatewayRuntime(t)
	sender := telegramSendFunc(func(ctx context.Context, req telegramSendRequest) (telegramSendResult, error) {
		return telegramSendResult{
			MessageID: 77,
			ChatID:    req.ChatID,
			Text:      req.Text,
		}, nil
	})
	h := applyAPIMiddleware(config.Config{
		APIAuthMode:  "required",
		APIUserToken: "user-token",
	}, zerolog.New(io.Discard), newChannelsAPIHandlerWithTelegramSender(runtime, sender, zerolog.New(io.Discard)), io.Discard)

	body := bytes.NewBufferString(`{"chat_id":"chat-1","text":"hello"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/channels/telegram/send", body)
	req.Header.Set("Authorization", "Bearer user-token")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("telegram send expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if strings.TrimSpace(asString(payload["source"])) != "telegram" {
		t.Fatalf("expected source=telegram, got %+v", payload)
	}
	if strings.TrimSpace(asString(payload["direction"])) != "outbound" {
		t.Fatalf("expected direction=outbound, got %+v", payload)
	}
}

func TestChannelsAPI_TelegramSendRejectsInvalidBody(t *testing.T) {
	runtime := newTestGatewayRuntime(t)
	sender := telegramSendFunc(func(ctx context.Context, req telegramSendRequest) (telegramSendResult, error) {
		return telegramSendResult{}, nil
	})
	h := newChannelsAPIHandlerWithTelegramSender(runtime, sender, zerolog.New(io.Discard))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/channels/telegram/send", strings.NewReader("{"))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("telegram send expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["error"] != "invalid request body" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestChannelsAPI_TelegramPairings_Approve(t *testing.T) {
	runtime := newTestGatewayRuntime(t)
	pairings, err := newTelegramPairingStore(filepath.Join(t.TempDir(), "telegram_pairings.json"), nil)
	if err != nil {
		t.Fatalf("newTelegramPairingStore: %v", err)
	}
	issued, _, err := pairings.issue(telegramPairingIdentity{
		UserID:   41,
		ChatID:   "4101",
		Username: "alice",
	}, telegramPairingTTL)
	if err != nil {
		t.Fatalf("issue pairing: %v", err)
	}
	h := newChannelsAPIHandlerWithTelegramPairings(
		runtime,
		nil,
		pairings,
		"pairing",
		true,
		zerolog.New(io.Discard),
	)

	recList := httptest.NewRecorder()
	reqList := httptest.NewRequest(http.MethodGet, "/v1/channels/telegram/pairings", nil)
	h.ServeHTTP(recList, reqList)
	if recList.Code != http.StatusOK {
		t.Fatalf("pairings list expected 200, got %d body=%s", recList.Code, recList.Body.String())
	}
	var listPayload struct {
		Pending []telegramPairingEntry `json:"pending"`
		Allowed []telegramAllowedUser  `json:"allowed"`
	}
	if err := json.Unmarshal(recList.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode list payload: %v", err)
	}
	if len(listPayload.Pending) != 1 || listPayload.Pending[0].Code != issued.Code {
		t.Fatalf("unexpected pending payload: %+v", listPayload)
	}

	approveBody := bytes.NewBufferString(`{"code":"` + issued.Code + `"}`)
	recApprove := httptest.NewRecorder()
	reqApprove := httptest.NewRequest(http.MethodPost, "/v1/channels/telegram/pairings/approve", approveBody)
	h.ServeHTTP(recApprove, reqApprove)
	if recApprove.Code != http.StatusOK {
		t.Fatalf("pairings approve expected 200, got %d body=%s", recApprove.Code, recApprove.Body.String())
	}
	var approvePayload struct {
		Approved telegramAllowedUser `json:"approved"`
	}
	if err := json.Unmarshal(recApprove.Body.Bytes(), &approvePayload); err != nil {
		t.Fatalf("decode approve payload: %v", err)
	}
	if approvePayload.Approved.UserID != 41 || approvePayload.Approved.ChatID != "4101" {
		t.Fatalf("unexpected approve payload: %+v", approvePayload)
	}
}

func TestChannelsAPI_TelegramPairingsApproveUnknownCodeReturnsNotFound(t *testing.T) {
	runtime := newTestGatewayRuntime(t)
	pairings, err := newTelegramPairingStore(filepath.Join(t.TempDir(), "telegram_pairings.json"), nil)
	if err != nil {
		t.Fatalf("newTelegramPairingStore: %v", err)
	}
	h := newChannelsAPIHandlerWithTelegramPairings(
		runtime,
		nil,
		pairings,
		"pairing",
		true,
		zerolog.New(io.Discard),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/channels/telegram/pairings/approve", bytes.NewBufferString(`{"code":"missing"}`))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("pairings approve expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if strings.TrimSpace(asString(payload["error"])) == "" {
		t.Fatalf("expected error payload, got %+v", payload)
	}
}

func TestBrowserAPIHandler_StatusProfilesAndVaultStatus(t *testing.T) {
	runtime := newTestGatewayRuntime(t)
	handler := newBrowserAPIHandler(runtime, vaultStatusSnapshot{
		Enabled:        true,
		Ready:          false,
		AuthMode:       "token",
		Addr:           "http://127.0.0.1:8200",
		AllowlistCount: 2,
		LastError:      "not configured",
	}, zerolog.New(io.Discard))

	recStatus := httptest.NewRecorder()
	reqStatus := httptest.NewRequest(http.MethodGet, "/v1/browser/status", nil)
	handler.ServeHTTP(recStatus, reqStatus)
	if recStatus.Code != http.StatusOK {
		t.Fatalf("browser status expected 200, got %d body=%s", recStatus.Code, recStatus.Body.String())
	}

	recProfiles := httptest.NewRecorder()
	reqProfiles := httptest.NewRequest(http.MethodGet, "/v1/browser/profiles", nil)
	handler.ServeHTTP(recProfiles, reqProfiles)
	if recProfiles.Code != http.StatusOK {
		t.Fatalf("browser profiles expected 200, got %d body=%s", recProfiles.Code, recProfiles.Body.String())
	}
	var profilesPayload struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(recProfiles.Body.Bytes(), &profilesPayload); err != nil {
		t.Fatalf("decode browser profiles payload: %v", err)
	}
	if profilesPayload.Count < 1 {
		t.Fatalf("expected non-empty browser profiles payload: %s", recProfiles.Body.String())
	}

	recVault := httptest.NewRecorder()
	reqVault := httptest.NewRequest(http.MethodGet, "/v1/vault/status", nil)
	handler.ServeHTTP(recVault, reqVault)
	if recVault.Code != http.StatusOK {
		t.Fatalf("vault status expected 200, got %d body=%s", recVault.Code, recVault.Body.String())
	}
	var vaultPayload map[string]any
	if err := json.Unmarshal(recVault.Body.Bytes(), &vaultPayload); err != nil {
		t.Fatalf("decode vault status payload: %v", err)
	}
	if vaultPayload["enabled"] != true {
		t.Fatalf("expected vault enabled in payload: %+v", vaultPayload)
	}
}

func TestBrowserAPIHandler_LoginCheckRun(t *testing.T) {
	workspace := t.TempDir()
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `<html><body><div id="ready">hello</div><button id="export">export</button></body></html>`)
	}))
	defer site.Close()
	flowDir := filepath.Join(workspace, "automation", "sites")
	if err := os.MkdirAll(flowDir, 0o755); err != nil {
		t.Fatalf("mkdir flow dir: %v", err)
	}
	flow := strings.Join([]string{
		"id: sample",
		"enabled: true",
		"profile: managed",
		"url: '" + site.URL + "'",
		"checks:",
		"  - selector: '#ready'",
		"    contains: 'hello'",
		"actions:",
		"  ping:",
		"    steps:",
		"      - open: '" + site.URL + "'",
		"      - click: '#export'",
	}, "\n")
	if err := os.WriteFile(filepath.Join(flowDir, "sample.yaml"), []byte(flow), 0o644); err != nil {
		t.Fatalf("write sample flow: %v", err)
	}

	store := session.NewStore(filepath.Join(workspace, "sessions"))
	runtime := gateway.NewRuntime(gateway.RuntimeOptions{
		Enabled:      true,
		WorkspaceDir: workspace,
		SessionStore: store,
		BrowserService: browser.NewService(browser.Config{
			WorkspaceDir:   workspace,
			SiteFlowsDir:   flowDir,
			DefaultProfile: "managed",
		}),
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := runtime.Close(ctx); err != nil {
			t.Fatalf("close gateway runtime: %v", err)
		}
	})

	runtime.BrowserStartWithProfile("managed")
	handler := newBrowserAPIHandler(runtime, vaultStatusSnapshot{Enabled: false}, zerolog.New(io.Discard))

	loginBody := bytes.NewBufferString(`{"site_id":"sample"}`)
	recLogin := httptest.NewRecorder()
	reqLogin := httptest.NewRequest(http.MethodPost, "/v1/browser/login", loginBody)
	handler.ServeHTTP(recLogin, reqLogin)
	if recLogin.Code != http.StatusOK {
		t.Fatalf("browser login expected 200, got %d body=%s", recLogin.Code, recLogin.Body.String())
	}

	checkBody := bytes.NewBufferString(`{"site_id":"sample"}`)
	recCheck := httptest.NewRecorder()
	reqCheck := httptest.NewRequest(http.MethodPost, "/v1/browser/check", checkBody)
	handler.ServeHTTP(recCheck, reqCheck)
	if recCheck.Code != http.StatusOK {
		t.Fatalf("browser check expected 200, got %d body=%s", recCheck.Code, recCheck.Body.String())
	}

	runBody := bytes.NewBufferString(`{"site_id":"sample","flow_action":"ping"}`)
	recRun := httptest.NewRecorder()
	reqRun := httptest.NewRequest(http.MethodPost, "/v1/browser/run", runBody)
	handler.ServeHTTP(recRun, reqRun)
	if recRun.Code != http.StatusOK {
		t.Fatalf("browser run expected 200, got %d body=%s", recRun.Code, recRun.Body.String())
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
