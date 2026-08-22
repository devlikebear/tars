package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/usage"
)

func newAgentRuntimeForSubagentToolTests(
	t *testing.T,
	maxThreads int,
	maxDepth int,
	runPrompt func(ctx context.Context, runLabel string, prompt string, allowedTools []string, tier string) (string, error),
) (*agentruntime.Runtime, *session.Store) {
	t.Helper()
	workspaceDir := t.TempDir()
	store := session.NewStore(workspaceDir)
	explorer, err := agentruntime.NewPromptExecutorWithOptions(agentruntime.PromptExecutorOptions{
		Name:        "explorer",
		Description: "Read-only explorer",
		PolicyMode:  "allowlist",
		ToolsAllow:  []string{"read_file", "list_dir", "glob", "memory_search"},
		RunPrompt: func(ctx context.Context, runLabel string, prompt string, allowedTools []string, tier string, _ *agentruntime.ProviderOverride) (string, error) {
			return runPrompt(ctx, runLabel, prompt, allowedTools, tier)
		},
	})
	if err != nil {
		t.Fatalf("new prompt executor: %v", err)
	}
	rt := agentruntime.NewRuntime(agentruntime.RuntimeOptions{
		Enabled:                         true,
		WorkspaceDir:                    workspaceDir,
		SessionStore:                    store,
		Executors:                       []agentruntime.AgentExecutor{explorer},
		DefaultAgent:                    "explorer",
		AgentRuntimeSubagentsMaxThreads: maxThreads,
		AgentRuntimeSubagentsMaxDepth:   maxDepth,
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

func newAgentRuntimeForSubagentCompareToolTests(
	t *testing.T,
	responses map[string]string,
) (*agentruntime.Runtime, *session.Store) {
	t.Helper()
	workspaceDir := t.TempDir()
	store := session.NewStore(workspaceDir)
	executors := make([]agentruntime.AgentExecutor, 0, len(responses))
	for name, response := range responses {
		name := name
		response := response
		executor, err := agentruntime.NewPromptExecutorWithOptions(agentruntime.PromptExecutorOptions{
			Name:        name,
			Description: "Read-only " + name,
			PolicyMode:  "allowlist",
			ToolsAllow:  []string{"read_file", "list_dir", "glob", "memory_search"},
			RunPrompt: func(_ context.Context, _ string, prompt string, allowedTools []string, _ string, _ *agentruntime.ProviderOverride) (string, error) {
				if len(allowedTools) == 0 {
					t.Fatalf("expected %s allowlist to be forwarded", name)
				}
				if strings.TrimSpace(prompt) == "" {
					t.Fatalf("expected prompt for %s", name)
				}
				return response, nil
			},
		})
		if err != nil {
			t.Fatalf("new prompt executor %s: %v", name, err)
		}
		executors = append(executors, executor)
	}
	rt := agentruntime.NewRuntime(agentruntime.RuntimeOptions{
		Enabled:                         true,
		WorkspaceDir:                    workspaceDir,
		SessionStore:                    store,
		Executors:                       executors,
		DefaultAgent:                    "explorer",
		AgentRuntimeSubagentsMaxThreads: 4,
		AgentRuntimeSubagentsMaxDepth:   1,
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

// subagentTestEventTimeout bounds waits for an event these tests require to
// happen. The property under test is enforced by the handshake around the wait
// — a stub signals that it started, then blocks until the test releases it —
// so this deadline only exists to fail a genuine hang instead of blocking
// forever. It is deliberately generous: a loaded two-core CI runner is far
// slower than a developer machine, and the earlier sub-second and two-second
// values turned that difference into flaky failures on the Windows job.
const subagentTestEventTimeout = 30 * time.Second

func TestSubagentsRunTool_SpawnsParallelExplorerChildrenAndReturnsSummaries(t *testing.T) {
	startedCh := make(chan string, 2)
	release := make(chan struct{})
	rt, store := newAgentRuntimeForSubagentToolTests(t, 4, 1, func(_ context.Context, _ string, prompt string, allowedTools []string, _ string) (string, error) {
		if len(allowedTools) == 0 {
			t.Fatalf("expected explorer allowlist to be forwarded")
		}
		startedCh <- prompt
		<-release
		return "summary for " + prompt, nil
	})
	parent, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create parent session: %v", err)
	}

	ctx := serverauth.WithWorkspaceID(context.Background(), "ws-subagents")
	ctx = usage.WithCallMeta(ctx, usage.CallMeta{
		Source:    "chat",
		SessionID: parent.ID,
	})
	runTool := NewSubagentsRunTool(rt)

	type execResult struct {
		res Result
		err error
	}
	done := make(chan execResult, 1)
	go func() {
		res, execErr := runTool.Execute(ctx, json.RawMessage(`{
			"tasks":[
				{"title":"scan backend","prompt":"inspect backend package"},
				{"title":"scan docs","prompt":"inspect README and docs"}
			]
		}`))
		done <- execResult{res: res, err: execErr}
	}()

	got := map[string]struct{}{}
	for i := 0; i < 2; i++ {
		select {
		case prompt := <-startedCh:
			got[prompt] = struct{}{}
		case <-time.After(subagentTestEventTimeout):
			t.Fatal("expected both subagent runs to start before completion")
		}
	}
	close(release)

	select {
	case result := <-done:
		if result.err != nil {
			t.Fatalf("subagents_run execute: %v", result.err)
		}
		if result.res.IsError {
			t.Fatalf("expected success payload, got %s", result.res.Text())
		}
		var payload struct {
			Count     int `json:"count"`
			Subagents []struct {
				RunID           string `json:"run_id"`
				SessionID       string `json:"session_id"`
				Agent           string `json:"agent"`
				Status          string `json:"status"`
				ParentSessionID string `json:"parent_session_id"`
				Summary         string `json:"summary"`
			} `json:"subagents"`
		}
		if err := json.Unmarshal([]byte(result.res.Text()), &payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload.Count != 2 || len(payload.Subagents) != 2 {
			t.Fatalf("expected 2 completed subagents, got %+v", payload)
		}
		for _, item := range payload.Subagents {
			if item.Agent != "explorer" {
				t.Fatalf("expected explorer agent, got %+v", item)
			}
			if item.Status != string(agentruntime.RunStatusCompleted) {
				t.Fatalf("expected completed status, got %+v", item)
			}
			if item.ParentSessionID != parent.ID {
				t.Fatalf("expected parent session id %q, got %+v", parent.ID, item)
			}
			if !strings.Contains(item.Summary, "summary for inspect") {
				t.Fatalf("expected compact summary, got %+v", item)
			}
			sess, err := store.Get(item.SessionID)
			if err != nil {
				t.Fatalf("get child session: %v", err)
			}
			if !sess.Hidden || sess.Kind != "subagent" {
				t.Fatalf("expected hidden subagent session, got %+v", sess)
			}
		}
	case <-time.After(subagentTestEventTimeout):
		t.Fatal("timed out waiting for subagents_run")
	}
}

func TestSubagentsRunTool_CompareModeRunsCompatibleAgentsAndReturnsComparison(t *testing.T) {
	rt, store := newAgentRuntimeForSubagentCompareToolTests(t, map[string]string{
		"explorer": "- API handler validates token\n- Shared migration path exists\n- No database writes found",
		"reviewer": "- API handler validates token\n- Shared migration path exists\n- Database writes are guarded",
	})
	parent, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create parent session: %v", err)
	}

	ctx := serverauth.WithWorkspaceID(context.Background(), "ws-subagents")
	ctx = usage.WithCallMeta(ctx, usage.CallMeta{
		Source:    "chat",
		SessionID: parent.ID,
	})
	runTool := NewSubagentsRunTool(rt)
	res, err := runTool.Execute(ctx, json.RawMessage(`{
		"mode":"compare",
		"tasks":[
			{"agent":"explorer","title":"Explorer pass","prompt":"Why does auth fail?"},
			{"agent":"reviewer","title":"Reviewer pass","prompt":"Why does auth fail?"}
		]
	}`))
	if err != nil {
		t.Fatalf("subagents_run execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success payload, got %s", res.Text())
	}

	var payload struct {
		Mode      string `json:"mode"`
		Count     int    `json:"count"`
		Subagents []struct {
			RunID  string `json:"run_id"`
			Agent  string `json:"agent"`
			Status string `json:"status"`
		} `json:"subagents"`
		Comparison struct {
			CommonFindings []string `json:"common_findings"`
			Conflicts      []string `json:"conflicts"`
			Evidence       []struct {
				RunID string `json:"run_id"`
				Text  string `json:"text"`
			} `json:"evidence"`
			SideBySide []struct {
				RunID    string `json:"run_id"`
				Agent    string `json:"agent"`
				Response string `json:"response"`
			} `json:"side_by_side"`
		} `json:"comparison"`
	}
	if err := json.Unmarshal([]byte(res.Text()), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.Mode != "compare" || payload.Count != 2 || len(payload.Subagents) != 2 {
		t.Fatalf("unexpected compare payload header: %+v", payload)
	}
	if len(payload.Comparison.SideBySide) != 2 {
		t.Fatalf("expected side-by-side outputs, got %+v", payload.Comparison.SideBySide)
	}
	if !strings.Contains(strings.Join(payload.Comparison.CommonFindings, "\n"), "API handler validates token") {
		t.Fatalf("expected shared finding in comparison, got %+v", payload.Comparison.CommonFindings)
	}
	if len(payload.Comparison.Evidence) == 0 || payload.Comparison.Evidence[0].RunID == "" {
		t.Fatalf("expected sourced evidence, got %+v", payload.Comparison.Evidence)
	}
	agents := map[string]bool{}
	for _, item := range payload.Subagents {
		if item.RunID == "" || item.Status != string(agentruntime.RunStatusCompleted) {
			t.Fatalf("unexpected subagent item: %+v", item)
		}
		agents[item.Agent] = true
	}
	if !agents["explorer"] || !agents["reviewer"] {
		t.Fatalf("expected both compare agents, got %+v", agents)
	}
}

func TestSubagentsRunTool_CompareModeValidatesTaskShape(t *testing.T) {
	rt, store := newAgentRuntimeForSubagentCompareToolTests(t, map[string]string{
		"explorer": "ok",
		"reviewer": "ok",
	})
	parent, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create parent session: %v", err)
	}
	ctx := serverauth.WithWorkspaceID(context.Background(), "ws-subagents")
	ctx = usage.WithCallMeta(ctx, usage.CallMeta{
		Source:    "chat",
		SessionID: parent.ID,
	})
	runTool := NewSubagentsRunTool(rt)

	res, err := runTool.Execute(ctx, json.RawMessage(`{
		"mode":"compare",
		"tasks":[{"agent":"explorer","prompt":"same"}]
	}`))
	if err != nil {
		t.Fatalf("subagents_run execute: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Text(), "compare mode requires 2-3 tasks") {
		t.Fatalf("expected compare task count error, got error=%t text=%s", res.IsError, res.Text())
	}

	res, err = runTool.Execute(ctx, json.RawMessage(`{
		"mode":"compare",
		"tasks":[
			{"agent":"explorer","prompt":"first"},
			{"agent":"reviewer","prompt":"second"}
		]
	}`))
	if err != nil {
		t.Fatalf("subagents_run execute: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Text(), "compare mode requires all task prompts to match") {
		t.Fatalf("expected compare prompt mismatch error, got error=%t text=%s", res.IsError, res.Text())
	}
}

func TestSubagentsRunTool_HidesConsensusSchemaWhenRuntimeGateDisabled(t *testing.T) {
	runTool := NewSubagentsRunTool(nil)
	params := string(runTool.Parameters)
	if strings.Contains(params, `"consensus"`) {
		t.Fatalf("expected default subagents_run schema to hide consensus, got %s", params)
	}
}

func TestSubagentsRunTool_ExposesConsensusSchemaWhenRuntimeGateEnabled(t *testing.T) {
	rt := agentruntime.NewRuntime(agentruntime.RuntimeOptions{
		Enabled:                        true,
		WorkspaceDir:                   t.TempDir(),
		AgentRuntimeConsensusEnabled:   true,
		AgentRuntimeConsensusMaxFanout: 2,
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := rt.Close(ctx); err != nil {
			t.Fatalf("close agent runtime: %v", err)
		}
	})

	runTool := NewSubagentsRunTool(rt)
	params := string(runTool.Parameters)
	if !strings.Contains(params, `"consensus"`) {
		t.Fatalf("expected consensus-enabled schema to expose consensus, got %s", params)
	}
	if !strings.Contains(params, `"enum":["parallel","consensus","compare"]`) {
		t.Fatalf("expected consensus-enabled schema to advertise consensus mode, got %s", params)
	}
}

func TestSubagentsRunTool_RejectsConsensusBeforeSpawnWhenRuntimeGateDisabled(t *testing.T) {
	rt, store := newAgentRuntimeForSubagentToolTests(t, 4, 1, func(_ context.Context, _ string, prompt string, _ []string, _ string) (string, error) {
		return "summary for " + prompt, nil
	})
	parent, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create parent session: %v", err)
	}

	ctx := serverauth.WithWorkspaceID(context.Background(), "ws-subagents")
	ctx = usage.WithCallMeta(ctx, usage.CallMeta{
		Source:    "chat",
		SessionID: parent.ID,
	})
	runTool := NewSubagentsRunTool(rt)
	res, err := runTool.Execute(ctx, json.RawMessage(`{
		"mode":"consensus",
		"consensus":{"variants":[{"alias":"codex"}]},
		"tasks":[{"prompt":"inspect backend"}]
	}`))
	if err != nil {
		t.Fatalf("subagents_run execute: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Text(), "agentruntime_consensus_enabled=false") {
		t.Fatalf("expected disabled consensus diagnostic, got error=%t text=%s", res.IsError, res.Text())
	}
	if got := rt.ListByWorkspace("ws-subagents", 10); len(got) != 0 {
		t.Fatalf("expected disabled consensus call to avoid spawning runs, got %+v", got)
	}
}

func TestSubagentsRunTool_RejectsTaskCountAboveThreadLimit(t *testing.T) {
	rt, store := newAgentRuntimeForSubagentToolTests(t, 1, 1, func(_ context.Context, _ string, prompt string, _ []string, _ string) (string, error) {
		return "summary for " + prompt, nil
	})
	parent, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create parent session: %v", err)
	}

	ctx := serverauth.WithWorkspaceID(context.Background(), "ws-subagents")
	ctx = usage.WithCallMeta(ctx, usage.CallMeta{
		Source:    "chat",
		SessionID: parent.ID,
	})
	runTool := NewSubagentsRunTool(rt)
	res, err := runTool.Execute(ctx, json.RawMessage(`{
		"tasks":[
			{"prompt":"inspect backend"},
			{"prompt":"inspect docs"}
		]
	}`))
	if err != nil {
		t.Fatalf("subagents_run execute: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected thread limit error, got %s", res.Text())
	}
	if !strings.Contains(res.Text(), "agentruntime_subagents_max_threads") {
		t.Fatalf("expected thread limit diagnostic, got %s", res.Text())
	}
}

func TestSubagentsRunTool_RejectsDepthAboveLimit(t *testing.T) {
	rt, store := newAgentRuntimeForSubagentToolTests(t, 4, 1, func(_ context.Context, _ string, prompt string, _ []string, _ string) (string, error) {
		return "summary for " + prompt, nil
	})
	parent, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create parent session: %v", err)
	}
	ctx := serverauth.WithWorkspaceID(context.Background(), "ws-subagents")
	rootRun, err := rt.Spawn(ctx, agentruntime.SpawnRequest{
		WorkspaceID:     "ws-subagents",
		Title:           "existing child",
		Prompt:          "already delegated",
		Agent:           "explorer",
		ParentSessionID: parent.ID,
		Depth:           1,
		SessionKind:     "subagent",
		SessionHidden:   true,
	})
	if err != nil {
		t.Fatalf("spawn root child run: %v", err)
	}
	ctx = usage.WithCallMeta(ctx, usage.CallMeta{
		Source:    "agent_run",
		RunID:     rootRun.ID,
		SessionID: rootRun.SessionID,
	})

	runTool := NewSubagentsRunTool(rt)
	res, err := runTool.Execute(ctx, json.RawMessage(`{"tasks":[{"prompt":"inspect docs"}]}`))
	if err != nil {
		t.Fatalf("subagents_run execute: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected depth limit error, got %s", res.Text())
	}
	if !strings.Contains(res.Text(), "agentruntime_subagents_max_depth") {
		t.Fatalf("expected depth limit diagnostic, got %s", res.Text())
	}
}
