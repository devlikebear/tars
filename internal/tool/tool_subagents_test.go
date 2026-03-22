package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/gateway"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/usage"
)

func newGatewayRuntimeForSubagentToolTests(
	t *testing.T,
	maxThreads int,
	maxDepth int,
	runPrompt func(ctx context.Context, runLabel string, prompt string, allowedTools []string) (string, error),
) (*gateway.Runtime, *session.Store) {
	t.Helper()
	workspaceDir := t.TempDir()
	store := session.NewStore(workspaceDir)
	explorer, err := gateway.NewPromptExecutorWithOptions(gateway.PromptExecutorOptions{
		Name:        "explorer",
		Description: "Read-only explorer",
		PolicyMode:  "allowlist",
		ToolsAllow:  []string{"read_file", "list_dir", "glob", "memory_search"},
		RunPrompt:   runPrompt,
	})
	if err != nil {
		t.Fatalf("new prompt executor: %v", err)
	}
	rt := gateway.NewRuntime(gateway.RuntimeOptions{
		Enabled:                    true,
		WorkspaceDir:               workspaceDir,
		SessionStore:               store,
		Executors:                  []gateway.AgentExecutor{explorer},
		DefaultAgent:               "explorer",
		GatewaySubagentsMaxThreads: maxThreads,
		GatewaySubagentsMaxDepth:   maxDepth,
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := rt.Close(ctx); err != nil {
			t.Fatalf("close gateway runtime: %v", err)
		}
	})
	return rt, store
}

func TestSubagentsRunTool_SpawnsParallelExplorerChildrenAndReturnsSummaries(t *testing.T) {
	startedCh := make(chan string, 2)
	release := make(chan struct{})
	rt, store := newGatewayRuntimeForSubagentToolTests(t, 4, 1, func(_ context.Context, _ string, prompt string, allowedTools []string) (string, error) {
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
		case <-time.After(300 * time.Millisecond):
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
			if item.Status != string(gateway.RunStatusCompleted) {
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
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for subagents_run")
	}
}

func TestSubagentsRunTool_RejectsTaskCountAboveThreadLimit(t *testing.T) {
	rt, store := newGatewayRuntimeForSubagentToolTests(t, 1, 1, func(_ context.Context, _ string, prompt string, _ []string) (string, error) {
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
	if !strings.Contains(res.Text(), "gateway_subagents_max_threads") {
		t.Fatalf("expected thread limit diagnostic, got %s", res.Text())
	}
}

func TestSubagentsRunTool_RejectsDepthAboveLimit(t *testing.T) {
	rt, store := newGatewayRuntimeForSubagentToolTests(t, 4, 1, func(_ context.Context, _ string, prompt string, _ []string) (string, error) {
		return "summary for " + prompt, nil
	})
	parent, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create parent session: %v", err)
	}
	ctx := serverauth.WithWorkspaceID(context.Background(), "ws-subagents")
	rootRun, err := rt.Spawn(ctx, gateway.SpawnRequest{
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
	if !strings.Contains(res.Text(), "gateway_subagents_max_depth") {
		t.Fatalf("expected depth limit diagnostic, got %s", res.Text())
	}
}
