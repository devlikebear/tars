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

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func newTestAgentRuntime(t *testing.T) *agentruntime.Runtime {
	t.Helper()
	rt, _ := newTestAgentRuntimeWithStore(t)
	return rt
}

func newTestAgentRuntimeWithStore(t *testing.T) (*agentruntime.Runtime, *session.Store) {
	t.Helper()
	store := session.NewStore(filepath.Join(t.TempDir(), "workspace"))
	rt := agentruntime.NewRuntime(agentruntime.RuntimeOptions{
		Enabled:                          true,
		WorkspaceDir:                     t.TempDir(),
		SessionStore:                     store,
		ChannelsLocalEnabled:             true,
		ChannelsWebhookEnabled:           true,
		ChannelsTelegramEnabled:          true,
		AgentRuntimeReportSummaryEnabled: true,
		RunPrompt: func(_ context.Context, _ string, prompt string) (string, error) {
			return "ok: " + prompt, nil
		},
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := rt.Close(ctx); err != nil {
			t.Fatalf("close agent runtime: %v", err)
		}
	})
	return rt, store
}

func TestAgentRunsAPIHandler_ListAndGet(t *testing.T) {
	runtime := newTestAgentRuntime(t)
	run, err := runtime.Spawn(context.Background(), agentruntime.SpawnRequest{Prompt: "hello"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	h := newAgentRunsAPIHandler(runtime, zerolog.New(io.Discard))

	recList := httptest.NewRecorder()
	reqList := httptest.NewRequest(http.MethodGet, "/v1/agentruntime/runs", nil)
	h.ServeHTTP(recList, reqList)
	if recList.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recList.Code, recList.Body.String())
	}

	recGet := httptest.NewRecorder()
	reqGet := httptest.NewRequest(http.MethodGet, "/v1/agentruntime/runs/"+run.ID, nil)
	h.ServeHTTP(recGet, reqGet)
	if recGet.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recGet.Code, recGet.Body.String())
	}

	waitForAgentRuntimeRun(t, runtime, run.ID)
}

func TestAgentRunsAPIHandler_ListFiltersStatusSinceAndSearch(t *testing.T) {
	runtime, store := newTestAgentRuntimeWithStore(t)
	alphaSession, err := store.Create("Alpha chat")
	if err != nil {
		t.Fatalf("create alpha session: %v", err)
	}
	betaSession, err := store.Create("Beta chat")
	if err != nil {
		t.Fatalf("create beta session: %v", err)
	}
	first, err := runtime.Spawn(context.Background(), agentruntime.SpawnRequest{
		SessionID: alphaSession.ID,
		Prompt:    "alpha budget review",
		Agent:     "default",
	})
	if err != nil {
		t.Fatalf("spawn first: %v", err)
	}
	second, err := runtime.Spawn(context.Background(), agentruntime.SpawnRequest{
		SessionID: betaSession.ID,
		Prompt:    "beta cleanup",
		Agent:     "default",
	})
	if err != nil {
		t.Fatalf("spawn second: %v", err)
	}
	waitForAgentRuntimeRun(t, runtime, first.ID)
	waitForAgentRuntimeRun(t, runtime, second.ID)

	h := newAgentRunsAPIHandler(runtime, zerolog.New(io.Discard))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/agentruntime/runs?status=done&since=24h&search=budget&limit=10", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Count int                `json:"count"`
		Runs  []agentruntime.Run `json:"runs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Count != 1 || len(payload.Runs) != 1 {
		t.Fatalf("expected one filtered run, payload=%+v", payload)
	}
	if payload.Runs[0].ID != first.ID || payload.Runs[0].SessionID != alphaSession.ID {
		t.Fatalf("expected first run with session id, payload=%+v", payload)
	}

	recRunning := httptest.NewRecorder()
	reqRunning := httptest.NewRequest(http.MethodGet, "/v1/agentruntime/runs?status=running&limit=10", nil)
	h.ServeHTTP(recRunning, reqRunning)
	if recRunning.Code != http.StatusOK {
		t.Fatalf("expected 200 for running filter, got %d body=%s", recRunning.Code, recRunning.Body.String())
	}
	var runningPayload struct {
		Count int                `json:"count"`
		Runs  []agentruntime.Run `json:"runs"`
	}
	if err := json.Unmarshal(recRunning.Body.Bytes(), &runningPayload); err != nil {
		t.Fatalf("decode running response: %v", err)
	}
	if runningPayload.Count != 0 || len(runningPayload.Runs) != 0 {
		t.Fatalf("expected no running runs after completion, payload=%+v", runningPayload)
	}

	recInvalid := httptest.NewRecorder()
	reqInvalid := httptest.NewRequest(http.MethodGet, "/v1/agentruntime/runs?since=not-a-range", nil)
	h.ServeHTTP(recInvalid, reqInvalid)
	if recInvalid.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid since, got %d body=%s", recInvalid.Code, recInvalid.Body.String())
	}
}

func TestAgentRuntimeAPIHandler_HardCutRoutes(t *testing.T) {
	runtime := newTestAgentRuntime(t)
	h := newAgentRuntimeAPIHandler(runtime, zerolog.New(io.Discard), nil)

	recNew := httptest.NewRecorder()
	reqNew := httptest.NewRequest(http.MethodGet, "/v1/agentruntime/status", nil)
	h.ServeHTTP(recNew, reqNew)
	if recNew.Code != http.StatusOK {
		t.Fatalf("expected agentruntime status route to return 200, got %d body=%s", recNew.Code, recNew.Body.String())
	}

	recLegacy := httptest.NewRecorder()
	reqLegacy := httptest.NewRequest(http.MethodGet, "/v1/gateway/status", nil)
	h.ServeHTTP(recLegacy, reqLegacy)
	if recLegacy.Code != http.StatusNotFound {
		t.Fatalf("expected legacy gateway status route to be removed, got %d body=%s", recLegacy.Code, recLegacy.Body.String())
	}
}

func TestAgentRunsAPIHandler_AgentsList(t *testing.T) {
	runtime := newTestAgentRuntime(t)
	h := newAgentRunsAPIHandler(runtime, zerolog.New(io.Discard))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/agentruntime/agents", nil)
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

func TestAgentRunsAPIHandler_LegacyAgentRoutesRemainAliases(t *testing.T) {
	runtime := newTestAgentRuntime(t)
	run, err := runtime.Spawn(context.Background(), agentruntime.SpawnRequest{Prompt: "legacy alias"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	h := newAgentRunsAPIHandler(runtime, zerolog.New(io.Discard))

	for _, path := range []string{"/v1/agent/agents", "/v1/agent/runs", "/v1/agent/runs/" + run.ID} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected legacy route %s to return 200, got %d body=%s", path, rec.Code, rec.Body.String())
		}
	}

	waitForAgentRuntimeRun(t, runtime, run.ID)
}

func TestAgentRunsAPIHandler_AgentsListIncludesSourceEntryDefault(t *testing.T) {
	runtime := newTestAgentRuntime(t)
	h := newAgentRunsAPIHandler(runtime, zerolog.New(io.Discard))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/agentruntime/agents", nil)
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
	if _, ok := first["tools_deny_groups"]; !ok {
		t.Fatalf("expected tools_deny_groups field, payload=%+v", payload)
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
	promptExecutor, err := agentruntime.NewPromptExecutorWithOptions(agentruntime.PromptExecutorOptions{
		Name:               "researcher",
		Description:        "research worker",
		Source:             "workspace",
		Entry:              "workspace/agents/researcher/AGENT.md",
		PolicyMode:         "allowlist",
		ToolsAllow:         []string{"read_file", "list_dir"},
		ToolsDeny:          []string{"exec"},
		ToolsRiskMax:       "medium",
		ToolsAllowGroups:   []string{"memory"},
		ToolsDenyGroups:    []string{"shell"},
		ToolsAllowPatterns: []string{"^read"},
		SessionRoutingMode: "fixed",
		SessionFixedID:     "sess_fixed",
		Tier:               "light",
		ProviderOverride:   &agentruntime.ProviderOverride{Alias: "gemini_fast", Model: "gemini-2.5-flash"},
		RunPrompt: func(_ context.Context, _ string, _ string, _ []string, _ string, _ *agentruntime.ProviderOverride) (string, error) {
			return "ok", nil
		},
	})
	if err != nil {
		t.Fatalf("new prompt executor: %v", err)
	}
	runtime := agentruntime.NewRuntime(agentruntime.RuntimeOptions{
		Enabled:      true,
		SessionStore: store,
		Executors:    []agentruntime.AgentExecutor{promptExecutor},
		DefaultAgent: "researcher",
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if closeErr := runtime.Close(ctx); closeErr != nil {
			t.Fatalf("close agent runtime: %v", closeErr)
		}
	})

	h := newAgentRunsAPIHandler(runtime, zerolog.New(io.Discard))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/agentruntime/agents", nil)
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
	denyGroups, ok := researcher["tools_deny_groups"].([]any)
	if !ok || len(denyGroups) != 1 {
		t.Fatalf("expected tools_deny_groups list, got %+v", researcher)
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
	tier, _ := researcher["tier"].(string)
	if tier != "light" {
		t.Fatalf("expected tier=light, got %+v", researcher)
	}
	providerOverride, ok := researcher["provider_override"].(map[string]any)
	if !ok {
		t.Fatalf("expected provider_override object, got %+v", researcher)
	}
	alias, _ := providerOverride["alias"].(string)
	if alias != "gemini_fast" {
		t.Fatalf("expected provider_override.alias=gemini_fast, got %+v", researcher)
	}
	fixedID, _ := researcher["session_fixed_id"].(string)
	if fixedID != "sess_fixed" {
		t.Fatalf("expected session_fixed_id=sess_fixed, got %+v", researcher)
	}
}

func TestAgentRuntimeSubagentsAPIHandler_ListIncludesTiersAndRunTelemetry(t *testing.T) {
	store := session.NewStore(filepath.Join(t.TempDir(), "workspace"))
	promptExecutor, err := agentruntime.NewPromptExecutorWithOptions(agentruntime.PromptExecutorOptions{
		Name:        "researcher",
		Description: "research worker",
		Source:      "workspace",
		Entry:       "workspace/agents/researcher/AGENT.md",
		PolicyMode:  "allowlist",
		ToolsAllow:  []string{"read_file", "list_dir"},
		Tier:        "deep",
		RunPrompt: func(_ context.Context, _ string, _ string, _ []string, tier string, _ *agentruntime.ProviderOverride) (string, error) {
			if tier != "deep" {
				t.Fatalf("expected run prompt tier deep, got %q", tier)
			}
			return "ok", nil
		},
	})
	if err != nil {
		t.Fatalf("new prompt executor: %v", err)
	}
	runtime := agentruntime.NewRuntime(agentruntime.RuntimeOptions{
		Enabled:      true,
		SessionStore: store,
		Executors:    []agentruntime.AgentExecutor{promptExecutor},
		DefaultAgent: "researcher",
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if closeErr := runtime.Close(ctx); closeErr != nil {
			t.Fatalf("close agent runtime: %v", closeErr)
		}
	})

	run, err := runtime.Spawn(context.Background(), agentruntime.SpawnRequest{Agent: "researcher", Prompt: "map code"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	waitForAgentRuntimeRun(t, runtime, run.ID)

	cfg := config.Config{
		LLMConfig: config.LLMConfig{
			LLMProviders: map[string]config.LLMProviderSettings{
				"codex": {Kind: "openai-codex", AuthMode: "oauth"},
			},
			LLMTiers: map[string]config.LLMTierBinding{
				"deep":  {Provider: "codex", Model: "gpt-5.5", ReasoningEffort: "high"},
				"swift": {Provider: "codex", Model: "gpt-5.4", ReasoningEffort: "low"},
			},
			LLMDefaultTier: "swift",
			LLMRoleDefaults: map[string]string{
				"agentruntime_default": "deep",
			},
		},
	}
	h := newAgentRuntimeSubagentsAPIHandler(runtime, cfg, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/agentruntime/subagents", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Count int `json:"count"`
		Tiers []struct {
			Name          string `json:"name"`
			ProviderAlias string `json:"provider_alias"`
			Kind          string `json:"kind"`
			Model         string `json:"model"`
		} `json:"tiers"`
		Agents []struct {
			Name            string   `json:"name"`
			DefaultTier     string   `json:"default_tier"`
			EffectiveTier   string   `json:"effective_tier"`
			TierSource      string   `json:"tier_source"`
			TierMissing     bool     `json:"tier_missing"`
			ResolvedAlias   string   `json:"resolved_alias"`
			ResolvedKind    string   `json:"resolved_kind"`
			ResolvedModel   string   `json:"resolved_model"`
			ToolsAllow      []string `json:"tools_allow"`
			ToolsAllowCount int      `json:"tools_allow_count"`
			RunCount        int      `json:"run_count"`
			LastRun         *struct {
				RunID  string `json:"run_id"`
				Status string `json:"status"`
				Tier   string `json:"tier"`
			} `json:"last_run"`
			RecentRuns []struct {
				RunID string `json:"run_id"`
			} `json:"recent_runs"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Count != 1 || len(payload.Agents) != 1 {
		t.Fatalf("unexpected agents payload: %+v", payload)
	}
	if len(payload.Tiers) != 2 || payload.Tiers[0].Name != "deep" || payload.Tiers[0].Model != "gpt-5.5" {
		t.Fatalf("expected sorted tier options with resolved model, got %+v", payload.Tiers)
	}
	agent := payload.Agents[0]
	if agent.Name != "researcher" || agent.DefaultTier != "deep" || agent.EffectiveTier != "deep" || agent.TierSource != "agent" {
		t.Fatalf("unexpected tier fields: %+v", agent)
	}
	if agent.TierMissing {
		t.Fatalf("did not expect tier_missing for configured tier: %+v", agent)
	}
	if agent.ResolvedAlias != "codex" || agent.ResolvedKind != "openai-codex" || agent.ResolvedModel != "gpt-5.5" {
		t.Fatalf("unexpected resolved preview: %+v", agent)
	}
	if agent.ToolsAllowCount != 2 || len(agent.ToolsAllow) != 2 {
		t.Fatalf("expected policy values in subagent payload: %+v", agent)
	}
	if agent.RunCount != 1 || agent.LastRun == nil || agent.LastRun.RunID != run.ID || agent.LastRun.Status != string(agentruntime.RunStatusCompleted) || agent.LastRun.Tier != "deep" {
		t.Fatalf("expected last run summary for researcher, got %+v", agent)
	}
	if len(agent.RecentRuns) != 1 || agent.RecentRuns[0].RunID != run.ID {
		t.Fatalf("expected recent run link data, got %+v", agent.RecentRuns)
	}
}

func TestAgentRuntimeSubagentsAPIHandler_DetailMarksMissingTier(t *testing.T) {
	store := session.NewStore(filepath.Join(t.TempDir(), "workspace"))
	promptExecutor, err := agentruntime.NewPromptExecutorWithOptions(agentruntime.PromptExecutorOptions{
		Name:        "researcher",
		Description: "research worker",
		Tier:        "removed-tier",
		RunPrompt: func(_ context.Context, _ string, _ string, _ []string, _ string, _ *agentruntime.ProviderOverride) (string, error) {
			return "ok", nil
		},
	})
	if err != nil {
		t.Fatalf("new prompt executor: %v", err)
	}
	runtime := agentruntime.NewRuntime(agentruntime.RuntimeOptions{
		Enabled:      true,
		SessionStore: store,
		Executors:    []agentruntime.AgentExecutor{promptExecutor},
		DefaultAgent: "researcher",
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if closeErr := runtime.Close(ctx); closeErr != nil {
			t.Fatalf("close agent runtime: %v", closeErr)
		}
	})

	cfg := config.Config{
		LLMConfig: config.LLMConfig{
			LLMProviders: map[string]config.LLMProviderSettings{
				"codex": {Kind: "openai-codex", AuthMode: "oauth"},
			},
			LLMTiers: map[string]config.LLMTierBinding{
				"deep": {Provider: "codex", Model: "gpt-5.5"},
			},
			LLMDefaultTier: "deep",
		},
	}
	h := newAgentRuntimeSubagentsAPIHandler(runtime, cfg, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/agentruntime/subagents/researcher", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Name          string `json:"name"`
		DefaultTier   string `json:"default_tier"`
		EffectiveTier string `json:"effective_tier"`
		TierMissing   bool   `json:"tier_missing"`
		TierError     string `json:"tier_error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Name != "researcher" || payload.DefaultTier != "removed-tier" || payload.EffectiveTier != "removed-tier" {
		t.Fatalf("unexpected detail payload: %+v", payload)
	}
	if !payload.TierMissing || !strings.Contains(payload.TierError, "removed-tier") {
		t.Fatalf("expected missing tier diagnostic, got %+v", payload)
	}
}

func TestAgentRuntimeSubagentsAPIHandler_PatchWorkspaceTierReloadsExecutor(t *testing.T) {
	workspaceDir := t.TempDir()
	agentDir := filepath.Join(workspaceDir, "agents", "researcher")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("mkdir agent dir: %v", err)
	}
	agentPath := filepath.Join(agentDir, "AGENT.md")
	if err := os.WriteFile(agentPath, []byte("---\nname: researcher\ndescription: Research worker\ntier: light\n---\nResearch the codebase.\n"), 0o644); err != nil {
		t.Fatalf("write agent: %v", err)
	}

	cfg := config.Config{
		RuntimeConfig: config.RuntimeConfig{WorkspaceDir: workspaceDir},
		LLMConfig: config.LLMConfig{
			LLMProviders: map[string]config.LLMProviderSettings{
				"codex": {Kind: "openai-codex", AuthMode: "oauth"},
			},
			LLMTiers: map[string]config.LLMTierBinding{
				"deep":  {Provider: "codex", Model: "gpt-5.5"},
				"light": {Provider: "codex", Model: "gpt-5.4"},
			},
			LLMDefaultTier: "light",
		},
		AgentRuntimeConfig: config.AgentRuntimeConfig{
			AgentRuntimeDefaultAgent: "researcher",
		},
	}
	store := session.NewStore(filepath.Join(workspaceDir, "sessions"))
	runPrompt := func(_ context.Context, _ string, _ string, _ []string, _ string, _ *agentruntime.ProviderOverride) (string, error) {
		return "ok", nil
	}
	executors := buildAgentRuntimeExecutors(cfg, runPrompt, zerolog.New(io.Discard))
	runtime := agentruntime.NewRuntime(agentruntime.RuntimeOptions{
		Enabled:      true,
		SessionStore: store,
		Executors:    executors,
		DefaultAgent: "researcher",
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if closeErr := runtime.Close(ctx); closeErr != nil {
			t.Fatalf("close agent runtime: %v", closeErr)
		}
	})

	reloaded := 0
	h := newAgentRuntimeSubagentsAPIHandler(runtime, cfg, func() {
		reloaded++
		runtime.SetExecutors(buildAgentRuntimeExecutors(cfg, runPrompt, zerolog.New(io.Discard)), cfg.AgentRuntimeDefaultAgent)
	})

	body := bytes.NewBufferString(`{"default_tier":"deep"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/v1/agentruntime/subagents/researcher", body)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if reloaded != 1 {
		t.Fatalf("expected one executor reload, got %d", reloaded)
	}

	var payload struct {
		Name          string `json:"name"`
		DefaultTier   string `json:"default_tier"`
		EffectiveTier string `json:"effective_tier"`
		TierEditable  bool   `json:"tier_editable"`
		ResolvedModel string `json:"resolved_model"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Name != "researcher" || payload.DefaultTier != "deep" || payload.EffectiveTier != "deep" || !payload.TierEditable || payload.ResolvedModel != "gpt-5.5" {
		t.Fatalf("unexpected patch payload: %+v", payload)
	}
	raw, err := os.ReadFile(agentPath)
	if err != nil {
		t.Fatalf("read updated agent: %v", err)
	}
	if !strings.Contains(string(raw), "tier: deep") {
		t.Fatalf("expected AGENT.md tier update, got:\n%s", string(raw))
	}

	if info, ok := runtime.LookupAgent("researcher"); !ok || info.Tier != "deep" {
		t.Fatalf("expected runtime executor tier deep after reload, ok=%t info=%+v", ok, info)
	}
}

func TestAgentRuntimeSubagentsAPIHandler_PatchRejectsUnknownTier(t *testing.T) {
	workspaceDir := t.TempDir()
	agentDir := filepath.Join(workspaceDir, "agents", "researcher")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("mkdir agent dir: %v", err)
	}
	agentPath := filepath.Join(agentDir, "AGENT.md")
	if err := os.WriteFile(agentPath, []byte("---\nname: researcher\ntier: light\n---\nResearch the codebase.\n"), 0o644); err != nil {
		t.Fatalf("write agent: %v", err)
	}

	cfg := config.Config{
		RuntimeConfig: config.RuntimeConfig{WorkspaceDir: workspaceDir},
		LLMConfig: config.LLMConfig{
			LLMProviders: map[string]config.LLMProviderSettings{
				"codex": {Kind: "openai-codex", AuthMode: "oauth"},
			},
			LLMTiers: map[string]config.LLMTierBinding{
				"light": {Provider: "codex", Model: "gpt-5.4"},
			},
			LLMDefaultTier: "light",
		},
	}
	store := session.NewStore(filepath.Join(workspaceDir, "sessions"))
	runtime := agentruntime.NewRuntime(agentruntime.RuntimeOptions{
		Enabled:      true,
		SessionStore: store,
		Executors: buildAgentRuntimeExecutors(cfg, func(_ context.Context, _ string, _ string, _ []string, _ string, _ *agentruntime.ProviderOverride) (string, error) {
			return "ok", nil
		}, zerolog.New(io.Discard)),
		DefaultAgent: "researcher",
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if closeErr := runtime.Close(ctx); closeErr != nil {
			t.Fatalf("close agent runtime: %v", closeErr)
		}
	})

	h := newAgentRuntimeSubagentsAPIHandler(runtime, cfg, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/v1/agentruntime/subagents/researcher", bytes.NewBufferString(`{"default_tier":"missing"}`))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	raw, err := os.ReadFile(agentPath)
	if err != nil {
		t.Fatalf("read agent: %v", err)
	}
	if strings.Contains(string(raw), "tier: missing") {
		t.Fatalf("unexpected invalid tier write:\n%s", string(raw))
	}
}

func TestAgentRuntimeSubagentsAPIHandler_BuilderCreateDraftAndApply(t *testing.T) {
	workspaceDir := t.TempDir()
	cfg := config.Config{
		RuntimeConfig: config.RuntimeConfig{WorkspaceDir: workspaceDir},
		LLMConfig: config.LLMConfig{
			LLMProviders: map[string]config.LLMProviderSettings{
				"codex": {Kind: "openai-codex", AuthMode: "oauth"},
			},
			LLMTiers: map[string]config.LLMTierBinding{
				"standard": {Provider: "codex", Model: "gpt-5.4"},
				"heavy":    {Provider: "codex", Model: "gpt-5.5"},
			},
			LLMDefaultTier: "standard",
		},
		AgentRuntimeConfig: config.AgentRuntimeConfig{
			AgentRuntimeDefaultAgent: "frontend-reviewer",
		},
	}
	store := session.NewStore(filepath.Join(workspaceDir, "sessions"))
	runPrompt := func(_ context.Context, _ string, _ string, _ []string, _ string, _ *agentruntime.ProviderOverride) (string, error) {
		return "ok", nil
	}
	runtime := agentruntime.NewRuntime(agentruntime.RuntimeOptions{
		Enabled:      true,
		SessionStore: store,
		Executors:    buildAgentRuntimeExecutors(cfg, runPrompt, zerolog.New(io.Discard)),
		DefaultAgent: "frontend-reviewer",
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if closeErr := runtime.Close(ctx); closeErr != nil {
			t.Fatalf("close agent runtime: %v", closeErr)
		}
	})

	reloaded := 0
	h := newAgentRuntimeSubagentsAPIHandler(runtime, cfg, func() {
		reloaded++
		runtime.SetExecutors(buildAgentRuntimeExecutors(cfg, runPrompt, zerolog.New(io.Discard)), cfg.AgentRuntimeDefaultAgent)
	})

	draftBody := bytes.NewBufferString(`{"mode":"create","request":"Create a frontend reviewer agent","default_tier":"heavy"}`)
	draftRec := httptest.NewRecorder()
	draftReq := httptest.NewRequest(http.MethodPost, "/v1/agentruntime/subagents/builder/draft", draftBody)
	h.ServeHTTP(draftRec, draftReq)
	if draftRec.Code != http.StatusOK {
		t.Fatalf("expected draft 200, got %d body=%s", draftRec.Code, draftRec.Body.String())
	}

	var draftPayload struct {
		Draft struct {
			Action      string   `json:"action"`
			Name        string   `json:"name"`
			Description string   `json:"description"`
			DefaultTier string   `json:"default_tier"`
			Prompt      string   `json:"prompt"`
			ToolsAllow  []string `json:"tools_allow"`
		} `json:"draft"`
		DraftSource string `json:"draft_source"`
	}
	if err := json.Unmarshal(draftRec.Body.Bytes(), &draftPayload); err != nil {
		t.Fatalf("decode draft response: %v", err)
	}
	if draftPayload.Draft.Name != "frontend-reviewer" || draftPayload.Draft.DefaultTier != "heavy" || !strings.Contains(draftPayload.Draft.Prompt, "frontend reviewer") {
		t.Fatalf("unexpected draft payload: %+v", draftPayload)
	}
	if got := strings.Join(draftPayload.Draft.ToolsAllow, ","); got != "glob,list_dir,read_file" {
		t.Fatalf("expected safe read-only allowlist, got %q", got)
	}

	applyBody := bytes.NewBuffer(draftRec.Body.Bytes())
	applyRec := httptest.NewRecorder()
	applyReq := httptest.NewRequest(http.MethodPost, "/v1/agentruntime/subagents/builder/apply", applyBody)
	h.ServeHTTP(applyRec, applyReq)
	if applyRec.Code != http.StatusOK {
		t.Fatalf("expected apply 200, got %d body=%s", applyRec.Code, applyRec.Body.String())
	}
	if reloaded != 1 {
		t.Fatalf("expected one reload after apply, got %d", reloaded)
	}

	agentPath := filepath.Join(workspaceDir, "agents", "frontend-reviewer", "AGENT.md")
	raw, err := os.ReadFile(agentPath)
	if err != nil {
		t.Fatalf("read created agent: %v", err)
	}
	text := string(raw)
	for _, want := range []string{"name: frontend-reviewer", "description:", "tier: heavy", "tools_allow:", "read_file", "Create a frontend reviewer agent"} {
		if !strings.Contains(text, want) {
			t.Fatalf("created AGENT.md missing %q:\n%s", want, text)
		}
	}
	if info, ok := runtime.LookupAgent("frontend-reviewer"); !ok || info.Tier != "heavy" {
		t.Fatalf("expected runtime executor after apply, ok=%t info=%+v", ok, info)
	}
}

func TestAgentRuntimeSubagentsAPIHandler_BuilderEditAndArchiveRequiresConfirm(t *testing.T) {
	workspaceDir := t.TempDir()
	agentDir := filepath.Join(workspaceDir, "agents", "researcher")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("mkdir agent dir: %v", err)
	}
	agentPath := filepath.Join(agentDir, "AGENT.md")
	if err := os.WriteFile(agentPath, []byte("---\nname: researcher\ndescription: Research worker\ntier: standard\ntools_allow:\n  - read_file\n---\nResearch the codebase.\n"), 0o644); err != nil {
		t.Fatalf("write agent: %v", err)
	}

	cfg := config.Config{
		RuntimeConfig: config.RuntimeConfig{WorkspaceDir: workspaceDir},
		LLMConfig: config.LLMConfig{
			LLMProviders: map[string]config.LLMProviderSettings{
				"codex": {Kind: "openai-codex", AuthMode: "oauth"},
			},
			LLMTiers: map[string]config.LLMTierBinding{
				"standard": {Provider: "codex", Model: "gpt-5.4"},
				"heavy":    {Provider: "codex", Model: "gpt-5.5"},
			},
			LLMDefaultTier: "standard",
		},
		AgentRuntimeConfig: config.AgentRuntimeConfig{
			AgentRuntimeDefaultAgent: "researcher",
		},
	}
	store := session.NewStore(filepath.Join(workspaceDir, "sessions"))
	runPrompt := func(_ context.Context, _ string, _ string, _ []string, _ string, _ *agentruntime.ProviderOverride) (string, error) {
		return "ok", nil
	}
	runtime := agentruntime.NewRuntime(agentruntime.RuntimeOptions{
		Enabled:      true,
		SessionStore: store,
		Executors:    buildAgentRuntimeExecutors(cfg, runPrompt, zerolog.New(io.Discard)),
		DefaultAgent: "researcher",
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if closeErr := runtime.Close(ctx); closeErr != nil {
			t.Fatalf("close agent runtime: %v", closeErr)
		}
	})

	h := newAgentRuntimeSubagentsAPIHandler(runtime, cfg, func() {
		runtime.SetExecutors(buildAgentRuntimeExecutors(cfg, runPrompt, zerolog.New(io.Discard)), cfg.AgentRuntimeDefaultAgent)
	})

	editBody := bytes.NewBufferString(`{"mode":"edit","base_name":"researcher","request":"Make it focus on frontend accessibility","default_tier":"heavy"}`)
	editRec := httptest.NewRecorder()
	editReq := httptest.NewRequest(http.MethodPost, "/v1/agentruntime/subagents/builder/draft", editBody)
	h.ServeHTTP(editRec, editReq)
	if editRec.Code != http.StatusOK {
		t.Fatalf("expected edit draft 200, got %d body=%s", editRec.Code, editRec.Body.String())
	}
	var editPayload struct {
		Draft agentRuntimeSubagentDraft `json:"draft"`
	}
	if err := json.Unmarshal(editRec.Body.Bytes(), &editPayload); err != nil {
		t.Fatalf("decode edit draft: %v", err)
	}
	if editPayload.Draft.Action != "update" || editPayload.Draft.Name != "researcher" || editPayload.Draft.DefaultTier != "heavy" || !strings.Contains(editPayload.Draft.Prompt, "frontend accessibility") {
		t.Fatalf("unexpected edit draft: %+v", editPayload.Draft)
	}

	applyBody, _ := json.Marshal(map[string]any{"draft": editPayload.Draft})
	applyRec := httptest.NewRecorder()
	applyReq := httptest.NewRequest(http.MethodPost, "/v1/agentruntime/subagents/builder/apply", bytes.NewReader(applyBody))
	h.ServeHTTP(applyRec, applyReq)
	if applyRec.Code != http.StatusOK {
		t.Fatalf("expected edit apply 200, got %d body=%s", applyRec.Code, applyRec.Body.String())
	}
	raw, err := os.ReadFile(agentPath)
	if err != nil {
		t.Fatalf("read edited agent: %v", err)
	}
	if !strings.Contains(string(raw), "frontend accessibility") || !strings.Contains(string(raw), "tier: heavy") {
		t.Fatalf("expected edited profile content, got:\n%s", string(raw))
	}

	noConfirmRec := httptest.NewRecorder()
	noConfirmReq := httptest.NewRequest(http.MethodPost, "/v1/agentruntime/subagents/researcher/archive", bytes.NewBufferString(`{"confirm":false}`))
	h.ServeHTTP(noConfirmRec, noConfirmReq)
	if noConfirmRec.Code != http.StatusBadRequest {
		t.Fatalf("expected archive confirmation 400, got %d body=%s", noConfirmRec.Code, noConfirmRec.Body.String())
	}
	if _, err := os.Stat(agentPath); err != nil {
		t.Fatalf("agent should still exist before confirm: %v", err)
	}

	confirmRec := httptest.NewRecorder()
	confirmReq := httptest.NewRequest(http.MethodPost, "/v1/agentruntime/subagents/researcher/archive", bytes.NewBufferString(`{"confirm":true}`))
	h.ServeHTTP(confirmRec, confirmReq)
	if confirmRec.Code != http.StatusOK {
		t.Fatalf("expected archive 200, got %d body=%s", confirmRec.Code, confirmRec.Body.String())
	}
	if _, err := os.Stat(agentPath); !os.IsNotExist(err) {
		t.Fatalf("expected AGENT.md archived away, stat err=%v", err)
	}
	if _, ok := runtime.LookupAgent("researcher"); ok {
		t.Fatalf("expected runtime executor removed after archive")
	}
	matches, err := filepath.Glob(filepath.Join(agentDir, "AGENT.archived.*.md"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("expected archived file, matches=%+v err=%v", matches, err)
	}
}

func TestAgentRuntimeSubagentBuilderLLMPromptMentionsJSON(t *testing.T) {
	if !strings.Contains(agentRuntimeSubagentBuilderLLMSystemPrompt, "json") {
		t.Fatalf("OpenAI JSON object response format requires a prompt message that mentions json")
	}
	if agentRuntimeSubagentBuilderLLMResponseHint != "json" {
		t.Fatalf("OpenAI JSON object response format requires the input message to mention json")
	}
}

func TestNormalizeAgentRuntimeSubagentDraftMapsLLMEditAction(t *testing.T) {
	draft := normalizeAgentRuntimeSubagentDraft(agentRuntimeSubagentDraft{
		Action:      "edit",
		Name:        "researcher",
		DefaultTier: "standard",
		Prompt:      "Focus on frontend accessibility.",
	}, config.Config{}, &agentRuntimeSubagentView{Name: "researcher"})
	if draft.Action != "update" {
		t.Fatalf("expected edit action to normalize to update, got %q", draft.Action)
	}
}

func TestAgentRunsAPIHandler_Spawn(t *testing.T) {
	runtime := newTestAgentRuntime(t)
	h := newAgentRunsAPIHandler(runtime, zerolog.New(io.Discard))

	body, _ := json.Marshal(map[string]any{
		"message": "spawn hello",
		"agent":   "default",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/agentruntime/runs", bytes.NewReader(body))
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

	waitForAgentRuntimeRun(t, runtime, runID)
}

func TestAgentRunsAPIHandler_SpawnMissingMessage(t *testing.T) {
	runtime := newTestAgentRuntime(t)
	h := newAgentRunsAPIHandler(runtime, zerolog.New(io.Discard))

	body, _ := json.Marshal(map[string]any{
		"agent": "default",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/agentruntime/runs", bytes.NewReader(body))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAgentRunsAPIHandler_SpawnUnknownAgentReturnsDiagnosticCode(t *testing.T) {
	runtime := newTestAgentRuntime(t)
	h := newAgentRunsAPIHandler(runtime, zerolog.New(io.Discard))

	body, _ := json.Marshal(map[string]any{
		"message": "spawn hello",
		"agent":   "unknown-agent",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/agentruntime/runs", bytes.NewReader(body))
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
	runtime := agentruntime.NewRuntime(agentruntime.RuntimeOptions{
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
			t.Fatalf("close agent runtime: %v", err)
		}
	})
	run, err := runtime.Spawn(context.Background(), agentruntime.SpawnRequest{Prompt: "hello"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	h := newAgentRunsAPIHandler(runtime, zerolog.New(io.Discard))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/agentruntime/runs/"+run.ID+"/cancel", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	waitForAgentRuntimeRun(t, runtime, run.ID)
}

func TestAgentRunsAPIHandler_IgnoresWorkspaceHeaderAndUsesSingleNamespace(t *testing.T) {
	runtime := newTestAgentRuntime(t)
	baseHandler := newAgentRunsAPIHandler(runtime, zerolog.New(io.Discard))
	handler := applyAPIMiddleware(config.Config{
		APIConfig: config.APIConfig{APIAuthMode: "off"},
	}, zerolog.New(io.Discard), baseHandler, io.Discard)

	spawn := func(workspaceID, message string) map[string]any {
		t.Helper()
		body, _ := json.Marshal(map[string]any{
			"message": message,
			"agent":   "default",
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/agentruntime/runs", bytes.NewReader(body))
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
	reqListA := httptest.NewRequest(http.MethodGet, "/v1/agentruntime/runs", nil)
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
	reqGet := httptest.NewRequest(http.MethodGet, "/v1/agentruntime/runs/"+runIDA, nil)
	reqGet.Header.Set("Tars-Workspace-Id", "ws-b")
	handler.ServeHTTP(recGet, reqGet)
	if recGet.Code != http.StatusOK {
		t.Fatalf("expected 200 in single workspace mode, got %d body=%s", recGet.Code, recGet.Body.String())
	}

	recCancel := httptest.NewRecorder()
	reqCancel := httptest.NewRequest(http.MethodPost, "/v1/agentruntime/runs/"+runIDA+"/cancel", nil)
	reqCancel.Header.Set("Tars-Workspace-Id", "ws-b")
	handler.ServeHTTP(recCancel, reqCancel)
	if recCancel.Code != http.StatusOK {
		t.Fatalf("expected 200 in single workspace mode, got %d body=%s", recCancel.Code, recCancel.Body.String())
	}
}

func TestAgentRuntimeAPIHandler_StatusReloadRestart(t *testing.T) {
	runtime := newTestAgentRuntime(t)
	h := newAgentRuntimeAPIHandler(runtime, zerolog.New(io.Discard), nil)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v1/agentruntime/status"},
		{http.MethodPost, "/v1/agentruntime/reload"},
		{http.MethodPost, "/v1/agentruntime/restart"},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, nil)
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s %s expected 200, got %d body=%s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

func TestAgentRuntimeAPIHandler_StatusIncludesAgentsTelemetry(t *testing.T) {
	runtime := newTestAgentRuntime(t)
	runtime.SetAgentsWatchEnabled(true)
	h := newAgentRuntimeAPIHandler(runtime, zerolog.New(io.Discard), nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/agentruntime/status", nil)
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

func TestAgentRuntimeAPIHandler_StatusWhenRuntimeMissingHasConsistentDefaults(t *testing.T) {
	h := newAgentRuntimeAPIHandler(nil, zerolog.New(io.Discard), nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/agentruntime/status", nil)
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

func TestAgentRuntimeAPIHandler_StatusIncludesPersistenceTelemetryValues(t *testing.T) {
	workspaceDir := t.TempDir()
	runtime := agentruntime.NewRuntime(agentruntime.RuntimeOptions{
		Enabled:                                   true,
		WorkspaceDir:                              workspaceDir,
		SessionStore:                              session.NewStore(filepath.Join(workspaceDir, "workspace")),
		RunPrompt:                                 func(_ context.Context, _ string, prompt string) (string, error) { return prompt, nil },
		AgentRuntimePersistenceEnabled:            true,
		AgentRuntimeRunsPersistenceEnabled:        true,
		AgentRuntimeChannelsPersistenceEnabled:    false,
		AgentRuntimeRunsMaxRecords:                50,
		AgentRuntimeChannelsMaxMessagesPerChannel: 10,
		AgentRuntimePersistenceDir:                filepath.Join(workspaceDir, "_shared", "agentruntime"),
		AgentRuntimeRestoreOnStartup:              true,
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := runtime.Close(ctx); err != nil {
			t.Fatalf("close agent runtime: %v", err)
		}
	})

	h := newAgentRuntimeAPIHandler(runtime, zerolog.New(io.Discard), nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/agentruntime/status", nil)
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

func TestAgentRuntimeAPIHandler_ReloadCallsRefreshHook(t *testing.T) {
	runtime := newTestAgentRuntime(t)
	called := false
	h := newAgentRuntimeAPIHandler(runtime, zerolog.New(io.Discard), func() {
		called = true
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/agentruntime/reload", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !called {
		t.Fatal("expected agent runtime reload hook to be called")
	}
}

func TestAgentRuntimeAPIHandler_ReportsSummary(t *testing.T) {
	runtime := newTestAgentRuntime(t)
	run, err := runtime.Spawn(context.Background(), agentruntime.SpawnRequest{Prompt: "hello"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	waitForAgentRuntimeRun(t, runtime, run.ID)

	h := newAgentRuntimeAPIHandler(runtime, zerolog.New(io.Discard), nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/agentruntime/reports/summary", nil)
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

func TestAgentRuntimeAPIHandler_ReportsSummarySingleWorkspaceNamespace(t *testing.T) {
	runtime := newTestAgentRuntime(t)
	baseAgentHandler := newAgentRunsAPIHandler(runtime, zerolog.New(io.Discard))
	agentHandler := applyAPIMiddleware(config.Config{
		APIConfig: config.APIConfig{APIAuthMode: "off"},
	}, zerolog.New(io.Discard), baseAgentHandler, io.Discard)
	baseAgentRuntimeHandler := newAgentRuntimeAPIHandler(runtime, zerolog.New(io.Discard), nil)
	agentRuntimeHandler := applyAPIMiddleware(config.Config{
		APIConfig: config.APIConfig{APIAuthMode: "off"},
	}, zerolog.New(io.Discard), baseAgentRuntimeHandler, io.Discard)

	spawn := func(workspaceID, message string) {
		t.Helper()
		body, _ := json.Marshal(map[string]any{
			"message": message,
			"agent":   "default",
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/agentruntime/runs", bytes.NewReader(body))
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
		req := httptest.NewRequest(http.MethodGet, "/v1/agentruntime/reports/summary", nil)
		req.Header.Set("Tars-Workspace-Id", workspaceID)
		agentRuntimeHandler.ServeHTTP(rec, req)
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

func TestAgentRuntimeAPIHandler_ReportDetailEndpointsBehindArchiveFlag(t *testing.T) {
	runtime := newTestAgentRuntime(t)
	h := newAgentRuntimeAPIHandler(runtime, zerolog.New(io.Discard), nil)

	recRuns := httptest.NewRecorder()
	reqRuns := httptest.NewRequest(http.MethodGet, "/v1/agentruntime/reports/runs", nil)
	h.ServeHTTP(recRuns, reqRuns)
	if recRuns.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for runs report when archive disabled, got %d body=%s", recRuns.Code, recRuns.Body.String())
	}

	recChannels := httptest.NewRecorder()
	reqChannels := httptest.NewRequest(http.MethodGet, "/v1/agentruntime/reports/channels", nil)
	h.ServeHTTP(recChannels, reqChannels)
	if recChannels.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for channels report when archive disabled, got %d body=%s", recChannels.Code, recChannels.Body.String())
	}

	store := session.NewStore(filepath.Join(t.TempDir(), "workspace"))
	archiveRuntime := agentruntime.NewRuntime(agentruntime.RuntimeOptions{
		Enabled:                          true,
		WorkspaceDir:                     t.TempDir(),
		SessionStore:                     store,
		ChannelsLocalEnabled:             true,
		AgentRuntimeReportSummaryEnabled: true,
		AgentRuntimeArchiveEnabled:       true,
		AgentRuntimeArchiveDir:           filepath.Join(t.TempDir(), "archive"),
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
	run, err := archiveRuntime.Spawn(context.Background(), agentruntime.SpawnRequest{Prompt: "hello"})
	if err != nil {
		t.Fatalf("spawn archive runtime: %v", err)
	}
	waitForAgentRuntimeRun(t, archiveRuntime, run.ID)
	if _, err := archiveRuntime.MessageSend("general", "", "ping"); err != nil {
		t.Fatalf("message send: %v", err)
	}

	archiveHandler := newAgentRuntimeAPIHandler(archiveRuntime, zerolog.New(io.Discard), nil)
	recRunsOn := httptest.NewRecorder()
	reqRunsOn := httptest.NewRequest(http.MethodGet, "/v1/agentruntime/reports/runs?limit=5", nil)
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
	reqChannelsOn := httptest.NewRequest(http.MethodGet, "/v1/agentruntime/reports/channels?limit=5", nil)
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

func TestAgentRuntimeAPIHandler_ReportsRunsRejectsInvalidLimit(t *testing.T) {
	runtime := newTestAgentRuntime(t)
	h := newAgentRuntimeAPIHandler(runtime, zerolog.New(io.Discard), nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/agentruntime/reports/runs?limit=0", nil)
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

func TestAgentRuntimeAPIHandler_ReloadRefreshesWorkspaceAgents(t *testing.T) {
	workspace := t.TempDir()
	store := session.NewStore(filepath.Join(workspace, "workspace"))
	runPrompt := func(_ context.Context, _ string, _ string, _ []string, _ string, _ *agentruntime.ProviderOverride) (string, error) {
		return "ok", nil
	}
	runtime := agentruntime.NewRuntime(agentruntime.RuntimeOptions{
		Enabled:      true,
		WorkspaceDir: workspace,
		SessionStore: store,
		RunPrompt: func(ctx context.Context, runLabel string, prompt string) (string, error) {
			return runPrompt(ctx, runLabel, prompt, nil, "", nil)
		},
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := runtime.Close(ctx); err != nil {
			t.Fatalf("close agent runtime: %v", err)
		}
	})
	cfg := config.Config{RuntimeConfig: config.RuntimeConfig{WorkspaceDir: workspace}}
	refresh := func() {
		executors := buildAgentRuntimeExecutors(cfg, runPrompt, zerolog.New(io.Discard))
		runtime.SetExecutors(executors, "")
	}

	h := newAgentRuntimeAPIHandler(runtime, zerolog.New(io.Discard), refresh)
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
	req := httptest.NewRequest(http.MethodPost, "/v1/agentruntime/reload", nil)
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
	runtime := newTestAgentRuntime(t)
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
	runtime := newTestAgentRuntime(t)
	sender := telegramSendFunc(func(ctx context.Context, req telegramSendRequest) (telegramSendResult, error) {
		return telegramSendResult{
			MessageID: 77,
			ChatID:    req.ChatID,
			Text:      req.Text,
		}, nil
	})
	h := applyAPIMiddleware(config.Config{
		APIConfig: config.APIConfig{
			APIAuthMode:  "required",
			APIUserToken: "user-token",
		},
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
	runtime := newTestAgentRuntime(t)
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
	runtime := newTestAgentRuntime(t)
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
	runtime := newTestAgentRuntime(t)
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

func waitForAgentRuntimeRun(t *testing.T, runtime *agentruntime.Runtime, runID string) {
	t.Helper()
	waitCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := runtime.Wait(waitCtx, runID); err != nil {
		t.Fatalf("wait run %s: %v", runID, err)
	}
}
