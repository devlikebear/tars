package tool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/gateway"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/usage"
)

func TestSubagentsOrchestrateTool_ExecutesParallelThenSequentialSteps(t *testing.T) {
	started := make(chan string, 3)
	releaseResearch := make(chan struct{})
	rt, _ := newGatewayRuntimeForSubagentToolTests(t, 4, 1, func(_ context.Context, _ string, prompt string, allowedTools []string, _ string) (string, error) {
		if len(allowedTools) == 0 {
			t.Fatalf("expected explorer allowlist to be forwarded")
		}
		switch strings.TrimSpace(prompt) {
		case "inspect backend":
			started <- "backend"
			<-releaseResearch
			return "backend findings", nil
		case "inspect docs":
			started <- "docs"
			<-releaseResearch
			return "docs findings", nil
		default:
			started <- prompt
			return "combined report", nil
		}
	})

	ctx := serverauth.WithWorkspaceID(context.Background(), "ws-orchestrate")
	ctx = usage.WithCallMeta(ctx, usage.CallMeta{
		Source:    "chat",
		SessionID: "sess-main",
	})
	runTool := NewSubagentsOrchestrateTool(rt)

	type execResult struct {
		res Result
		err error
	}
	done := make(chan execResult, 1)
	go func() {
		res, execErr := runTool.Execute(ctx, json.RawMessage(`{
			"steps":[
				{
					"id":"research",
					"mode":"parallel",
					"tasks":[
						{"id":"backend","title":"backend","prompt":"inspect backend","tier":"light"},
						{"id":"docs","title":"docs","prompt":"inspect docs","tier":"light"}
					]
				},
				{
					"id":"combine",
					"mode":"sequential",
					"tasks":[
						{
							"id":"report",
							"title":"report",
							"prompt":"combine {{task.backend.summary}} and {{task.docs.summary}}",
							"tier":"heavy",
							"depends_on":["backend","docs"]
						}
					]
				}
			]
		}`))
		done <- execResult{res: res, err: execErr}
	}()

	got := map[string]struct{}{}
	for i := 0; i < 2; i++ {
		select {
		case item := <-started:
			got[item] = struct{}{}
		case <-time.After(300 * time.Millisecond):
			t.Fatal("expected both research tasks to start in parallel")
		}
	}
	if _, ok := got["backend"]; !ok {
		t.Fatalf("expected backend task to start, got %+v", got)
	}
	if _, ok := got["docs"]; !ok {
		t.Fatalf("expected docs task to start, got %+v", got)
	}

	close(releaseResearch)

	select {
	case item := <-started:
		if item != "combine backend findings and docs findings" {
			t.Fatalf("expected sequential step to receive resolved placeholders, got %q", item)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected sequential aggregation task to start after research step")
	}

	select {
	case result := <-done:
		if result.err != nil {
			t.Fatalf("subagents_orchestrate execute: %v", result.err)
		}
		if result.res.IsError {
			t.Fatalf("expected success payload, got %s", result.res.Text())
		}
		var payload struct {
			StepCount int `json:"step_count"`
			TaskCount int `json:"task_count"`
			Steps     []struct {
				ID    string `json:"id"`
				Mode  string `json:"mode"`
				Tasks []struct {
					ID      string `json:"id"`
					Status  string `json:"status"`
					Summary string `json:"summary"`
					Tier    string `json:"tier"`
				} `json:"tasks"`
			} `json:"steps"`
		}
		if err := json.Unmarshal([]byte(result.res.Text()), &payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload.StepCount != 2 || payload.TaskCount != 3 {
			t.Fatalf("unexpected counts: %+v", payload)
		}
		if len(payload.Steps) != 2 {
			t.Fatalf("expected 2 steps, got %+v", payload)
		}
		if payload.Steps[1].Tasks[0].Tier != "heavy" {
			t.Fatalf("expected heavy tier on final task, got %+v", payload.Steps[1].Tasks[0])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for orchestration result")
	}
}

func TestSubagentsOrchestrateTool_RejectsParallelDependencyWithinSameStep(t *testing.T) {
	rt, _ := newGatewayRuntimeForSubagentToolTests(t, 4, 1, func(_ context.Context, _ string, prompt string, _ []string, _ string) (string, error) {
		return "summary for " + prompt, nil
	})

	ctx := serverauth.WithWorkspaceID(context.Background(), "ws-orchestrate")
	ctx = usage.WithCallMeta(ctx, usage.CallMeta{
		Source:    "chat",
		SessionID: "sess-main",
	})
	runTool := NewSubagentsOrchestrateTool(rt)
	res, err := runTool.Execute(ctx, json.RawMessage(`{
		"steps":[
			{
				"id":"research",
				"mode":"parallel",
				"tasks":[
					{"id":"backend","prompt":"inspect backend"},
					{"id":"docs","prompt":"inspect docs","depends_on":["backend"]}
				]
			}
		]
	}`))
	if err != nil {
		t.Fatalf("subagents_orchestrate execute: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected validation error, got %s", res.Text())
	}
	if !strings.Contains(res.Text(), "parallel step") {
		t.Fatalf("expected parallel dependency diagnostic, got %s", res.Text())
	}
}

func TestSubagentsOrchestrateTool_RejectsIncompletePlaceholderReference(t *testing.T) {
	rt, _ := newGatewayRuntimeForSubagentToolTests(t, 4, 1, func(_ context.Context, _ string, prompt string, _ []string, _ string) (string, error) {
		return "summary for " + prompt, nil
	})

	ctx := serverauth.WithWorkspaceID(context.Background(), "ws-orchestrate")
	ctx = usage.WithCallMeta(ctx, usage.CallMeta{
		Source:    "chat",
		SessionID: "sess-main",
	})
	runTool := NewSubagentsOrchestrateTool(rt)
	res, err := runTool.Execute(ctx, json.RawMessage(`{
		"steps":[
			{
				"id":"combine",
				"mode":"sequential",
				"tasks":[
					{"id":"report","prompt":"combine {{task.missing.summary}}"}
				]
			}
		]
	}`))
	if err != nil {
		t.Fatalf("subagents_orchestrate execute: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected placeholder error, got %s", res.Text())
	}
	if !strings.Contains(res.Text(), "incomplete task: missing") {
		t.Fatalf("expected placeholder diagnostic, got %s", res.Text())
	}
}

func TestSubagentsOrchestrateTool_CancelsSpawnedRunsWhenParallelSpawnFails(t *testing.T) {
	rt, _ := newGatewayRuntimeForSubagentToolTests(t, 4, 1, func(ctx context.Context, _ string, _ string, _ []string, _ string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})

	origSpawn := subagentFlowSpawn
	origCancel := subagentFlowCancel
	t.Cleanup(func() {
		subagentFlowSpawn = origSpawn
		subagentFlowCancel = origCancel
	})

	var firstRunID string
	spawnCalls := 0
	subagentFlowSpawn = func(runtime *gateway.Runtime, ctx context.Context, req gateway.SpawnRequest) (gateway.Run, error) {
		spawnCalls++
		if spawnCalls == 2 {
			return gateway.Run{}, errors.New("forced spawn failure")
		}
		run, err := origSpawn(runtime, ctx, req)
		if err == nil {
			firstRunID = run.ID
		}
		return run, err
	}

	canceledRunIDs := []string{}
	subagentFlowCancel = func(runtime *gateway.Runtime, workspaceID string, runs []gateway.Run) {
		for _, run := range runs {
			canceledRunIDs = append(canceledRunIDs, run.ID)
		}
		origCancel(runtime, workspaceID, runs)
	}

	ctx := serverauth.WithWorkspaceID(context.Background(), "ws-orchestrate")
	ctx = usage.WithCallMeta(ctx, usage.CallMeta{
		Source:    "chat",
		SessionID: "sess-main",
	})
	runTool := NewSubagentsOrchestrateTool(rt)
	res, err := runTool.Execute(ctx, json.RawMessage(`{
		"steps":[
			{
				"id":"research",
				"mode":"parallel",
				"tasks":[
					{"id":"one","prompt":"inspect one"},
					{"id":"two","prompt":"inspect two"}
				]
			}
		]
	}`))
	if err != nil {
		t.Fatalf("subagents_orchestrate execute: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected spawn failure payload, got %s", res.Text())
	}
	if !strings.Contains(res.Text(), "forced spawn failure") {
		t.Fatalf("expected spawn failure diagnostic, got %s", res.Text())
	}
	if firstRunID == "" {
		t.Fatal("expected first run to be spawned before injected failure")
	}
	if len(canceledRunIDs) != 1 || canceledRunIDs[0] != firstRunID {
		t.Fatalf("expected spawned run %q to be canceled, got %+v", firstRunID, canceledRunIDs)
	}
	canceled, ok := rt.GetByWorkspace("ws-orchestrate", firstRunID)
	if !ok {
		t.Fatalf("expected canceled run %q to remain queryable", firstRunID)
	}
	if canceled.Status != gateway.RunStatusCanceled {
		t.Fatalf("expected canceled run status, got %+v", canceled)
	}
}

func TestSubagentsOrchestrateTool_StopsSequentialStepAfterFailure(t *testing.T) {
	started := []string{}
	rt, _ := newGatewayRuntimeForSubagentToolTests(t, 4, 1, func(_ context.Context, _ string, prompt string, _ []string, _ string) (string, error) {
		started = append(started, prompt)
		if prompt == "inspect backend" {
			return "", errors.New("backend failed")
		}
		return "summary for " + prompt, nil
	})

	ctx := serverauth.WithWorkspaceID(context.Background(), "ws-orchestrate")
	ctx = usage.WithCallMeta(ctx, usage.CallMeta{
		Source:    "chat",
		SessionID: "sess-main",
	})
	runTool := NewSubagentsOrchestrateTool(rt)
	res, err := runTool.Execute(ctx, json.RawMessage(`{
		"steps":[
			{
				"id":"research",
				"mode":"sequential",
				"tasks":[
					{"id":"backend","prompt":"inspect backend"},
					{"id":"docs","prompt":"inspect docs"}
				]
			}
		]
	}`))
	if err != nil {
		t.Fatalf("subagents_orchestrate execute: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected sequential failure payload, got %s", res.Text())
	}
	if len(started) != 1 || started[0] != "inspect backend" {
		t.Fatalf("expected only failing task to run, got %+v", started)
	}

	var payload struct {
		Steps []struct {
			Status      string `json:"status"`
			FailedTasks int    `json:"failed_tasks"`
			Tasks       []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
				Error  string `json:"error"`
			} `json:"tasks"`
		} `json:"steps"`
	}
	if err := json.Unmarshal([]byte(res.Text()), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if len(payload.Steps) != 1 {
		t.Fatalf("expected one step, got %+v", payload)
	}
	if payload.Steps[0].Status != "failed" || payload.Steps[0].FailedTasks != 1 {
		t.Fatalf("expected failed sequential step with one failed task, got %+v", payload.Steps[0])
	}
	if len(payload.Steps[0].Tasks) != 1 || payload.Steps[0].Tasks[0].ID != "backend" {
		t.Fatalf("expected only backend task output, got %+v", payload.Steps[0].Tasks)
	}
	if payload.Steps[0].Tasks[0].Status != string(gateway.RunStatusFailed) {
		t.Fatalf("expected backend task to fail, got %+v", payload.Steps[0].Tasks[0])
	}
}
