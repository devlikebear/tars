package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

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
