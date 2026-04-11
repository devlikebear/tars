package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/usage"
)

type plannerToolTestClient struct {
	response     string
	chatCalls    int
	seenMessages []llm.ChatMessage
	seenMeta     llm.SelectionMetadata
}

func (c *plannerToolTestClient) Ask(_ context.Context, prompt string) (string, error) {
	return c.response + prompt, nil
}

func (c *plannerToolTestClient) Chat(ctx context.Context, messages []llm.ChatMessage, _ llm.ChatOptions) (llm.ChatResponse, error) {
	c.chatCalls++
	c.seenMessages = append([]llm.ChatMessage(nil), messages...)
	c.seenMeta, _ = llm.SelectionMetadataFromContext(ctx)
	return llm.ChatResponse{
		Message: llm.ChatMessage{
			Role:    "assistant",
			Content: c.response,
		},
		StopReason: "stop",
	}, nil
}

func newPlannerToolTestRouter(t *testing.T, planner llm.Client) llm.Router {
	t.Helper()
	router, err := llm.NewRouter(llm.RouterConfig{
		Tiers: map[llm.Tier]llm.TierEntry{
			llm.TierHeavy: {
				Client:   planner,
				Provider: "openai",
				Model:    "gpt-5.4",
			},
			llm.TierStandard: {
				Client:   &llm.FakeClient{Label: "standard"},
				Provider: "openai",
				Model:    "gpt-5.4-mini",
			},
			llm.TierLight: {
				Client:   &llm.FakeClient{Label: "light"},
				Provider: "openai",
				Model:    "gpt-5.4-nano",
			},
		},
		DefaultTier: llm.TierStandard,
		RoleDefaults: map[llm.Role]llm.Tier{
			llm.RoleGatewayPlanner: llm.TierHeavy,
		},
	})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}
	return router
}

func TestSubagentsPlanTool_UsesGatewayPlannerRoleAndReturnsValidatedPlan(t *testing.T) {
	rt, _ := newGatewayRuntimeForSubagentToolTests(t, 4, 1, func(_ context.Context, _ string, prompt string, _ []string, _ string) (string, error) {
		return "summary for " + prompt, nil
	})
	planner := &plannerToolTestClient{
		response: "```json\n" + `{
  "steps":[
    {
      "id":"research",
      "mode":"parallel",
      "tasks":[
        {"id":"backend","title":"backend","prompt":"inspect backend auth"},
        {"id":"docs","title":"docs","prompt":"inspect docs auth"}
      ]
    },
    {
      "id":"combine",
      "mode":"sequential",
      "tasks":[
        {
          "id":"report",
          "title":"report",
          "prompt":"compare {{task.backend.summary}} with {{task.docs.summary}}",
          "depends_on":["backend","docs"]
        }
      ]
    }
  ]
}` + "\n```",
	}
	runTool := NewSubagentsPlanTool(rt, newPlannerToolTestRouter(t, planner))

	ctx := serverauth.WithWorkspaceID(context.Background(), "ws-plan")
	ctx = usage.WithCallMeta(ctx, usage.CallMeta{
		Source:    "chat",
		SessionID: "sess-main",
		RunID:     "run-main",
	})
	res, err := runTool.Execute(ctx, json.RawMessage(`{
		"goal":"analyze auth flow changes",
		"agent":"explorer",
		"flow_id":"flow-auth"
	}`))
	if err != nil {
		t.Fatalf("subagents_plan execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success payload, got %s", res.Text())
	}

	if planner.chatCalls != 1 {
		t.Fatalf("expected one planner llm call, got %d", planner.chatCalls)
	}
	if planner.seenMeta.Role != llm.RoleGatewayPlanner {
		t.Fatalf("expected gateway planner role, got %+v", planner.seenMeta)
	}
	if planner.seenMeta.Tier != llm.TierHeavy {
		t.Fatalf("expected heavy tier metadata, got %+v", planner.seenMeta)
	}
	if planner.seenMeta.Source != "role" {
		t.Fatalf("expected role-based planner source, got %+v", planner.seenMeta)
	}
	if planner.seenMeta.SessionID != "sess-main" || planner.seenMeta.RunID != "run-main" {
		t.Fatalf("expected session/run metadata, got %+v", planner.seenMeta)
	}
	if planner.seenMeta.FlowID != "flow-auth" || planner.seenMeta.StepID != "plan" {
		t.Fatalf("expected flow/step metadata, got %+v", planner.seenMeta)
	}
	if len(planner.seenMessages) < 2 || !strings.Contains(planner.seenMessages[1].Content, "analyze auth flow changes") {
		t.Fatalf("expected planner prompt to include goal, got %+v", planner.seenMessages)
	}

	var payload struct {
		FlowID      string                  `json:"flow_id"`
		Agent       string                  `json:"agent"`
		PlannerRole string                  `json:"planner_role"`
		PlannerTier string                  `json:"planner_tier"`
		Steps       []subagentFlowStepInput `json:"steps"`
	}
	if err := json.Unmarshal([]byte(res.Text()), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.FlowID != "flow-auth" {
		t.Fatalf("expected flow-auth, got %+v", payload)
	}
	if payload.Agent != "explorer" {
		t.Fatalf("expected explorer agent, got %+v", payload)
	}
	if payload.PlannerRole != "gateway_planner" || payload.PlannerTier != "heavy" {
		t.Fatalf("expected planner metadata in payload, got %+v", payload)
	}
	if len(payload.Steps) != 2 {
		t.Fatalf("expected 2 planned steps, got %+v", payload)
	}
}

func TestSubagentsPlanTool_RejectsInvalidPlannerOutput(t *testing.T) {
	rt, _ := newGatewayRuntimeForSubagentToolTests(t, 4, 1, func(_ context.Context, _ string, prompt string, _ []string, _ string) (string, error) {
		return "summary for " + prompt, nil
	})
	planner := &plannerToolTestClient{
		response: `{
  "steps":[
    {
      "mode":"parallel",
      "tasks":[
        {"id":"backend","prompt":"inspect backend"},
        {"id":"docs","prompt":"inspect docs","depends_on":["backend"]}
      ]
    }
  ]
}`,
	}
	runTool := NewSubagentsPlanTool(rt, newPlannerToolTestRouter(t, planner))

	ctx := serverauth.WithWorkspaceID(context.Background(), "ws-plan")
	ctx = usage.WithCallMeta(ctx, usage.CallMeta{
		Source:    "chat",
		SessionID: "sess-main",
	})
	res, err := runTool.Execute(ctx, json.RawMessage(`{"goal":"analyze auth flow changes"}`))
	if err != nil {
		t.Fatalf("subagents_plan execute: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected validation error, got %s", res.Text())
	}
	if !strings.Contains(res.Text(), "parallel step") {
		t.Fatalf("expected planner validation diagnostic, got %s", res.Text())
	}
}

func TestSubagentsPlanTool_NormalizesDuplicateTaskIDsAndReferences(t *testing.T) {
	rt, _ := newGatewayRuntimeForSubagentToolTests(t, 4, 1, func(_ context.Context, _ string, prompt string, _ []string, _ string) (string, error) {
		return "summary for " + prompt, nil
	})
	planner := &plannerToolTestClient{
		response: `{
  "steps":[
    {
      "id":"work",
      "mode":"parallel",
      "tasks":[
        {"id":"cfg","title":"config","prompt":"inspect planner config"},
        {"id":"impl","title":"implementation","prompt":"inspect subagent implementation"}
      ]
    },
    {
      "id":"work",
      "mode":"sequential",
      "tasks":[
        {"id":"cfg","title":"config again","prompt":"recheck planner config details","depends_on":["cfg"]},
        {"id":"report","title":"report","prompt":"compare {{task.cfg.summary}} with {{task.impl.summary}}","depends_on":["cfg","impl"]}
      ]
    }
  ]
}`,
	}
	runTool := NewSubagentsPlanTool(rt, newPlannerToolTestRouter(t, planner))

	ctx := serverauth.WithWorkspaceID(context.Background(), "ws-plan")
	ctx = usage.WithCallMeta(ctx, usage.CallMeta{
		Source:    "chat",
		SessionID: "sess-main",
	})
	res, err := runTool.Execute(ctx, json.RawMessage(`{"goal":"verify planner path"}`))
	if err != nil {
		t.Fatalf("subagents_plan execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected normalized planner output, got %s", res.Text())
	}

	var payload struct {
		Steps []subagentFlowStepInput `json:"steps"`
	}
	if err := json.Unmarshal([]byte(res.Text()), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if len(payload.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %+v", payload)
	}
	if payload.Steps[0].ID != "work" {
		t.Fatalf("expected first step id work, got %+v", payload.Steps[0])
	}
	if payload.Steps[1].ID == "work" {
		t.Fatalf("expected duplicate step id to be normalized, got %+v", payload.Steps[1])
	}
	if payload.Steps[1].Tasks[0].ID == "cfg" {
		t.Fatalf("expected duplicate task id to be normalized, got %+v", payload.Steps[1].Tasks[0])
	}
	if payload.Steps[1].Tasks[0].DependsOn[0] != "cfg" {
		t.Fatalf("expected first duplicate task to still depend on original cfg, got %+v", payload.Steps[1].Tasks[0])
	}
	if !strings.Contains(payload.Steps[1].Tasks[1].Prompt, "{{task."+payload.Steps[1].Tasks[0].ID+".summary}}") {
		t.Fatalf("expected prompt placeholder to be rewritten to normalized task id, got %+v", payload.Steps[1].Tasks[1])
	}
	if got := payload.Steps[1].Tasks[1].DependsOn[0]; got != payload.Steps[1].Tasks[0].ID {
		t.Fatalf("expected depends_on to be rewritten to normalized duplicate id, got %+v", payload.Steps[1].Tasks[1])
	}
}

func TestSubagentsPlanTool_EnsuresExactTargetsRemainInTaskPrompts(t *testing.T) {
	rt, _ := newGatewayRuntimeForSubagentToolTests(t, 4, 1, func(_ context.Context, _ string, prompt string, _ []string, _ string) (string, error) {
		return "summary for " + prompt, nil
	})
	planner := &plannerToolTestClient{
		response: `{
  "steps":[
    {
      "id":"review",
      "mode":"parallel",
      "tasks":[
        {"id":"cfg","title":"config review","prompt":"inspect planner config"},
        {"id":"docs","title":"docs review","prompt":"inspect workspace docs"}
      ]
    }
  ]
}`,
	}
	runTool := NewSubagentsPlanTool(rt, newPlannerToolTestRouter(t, planner))

	ctx := serverauth.WithWorkspaceID(context.Background(), "ws-plan")
	ctx = usage.WithCallMeta(ctx, usage.CallMeta{
		Source:    "chat",
		SessionID: "sess-main",
	})
	res, err := runTool.Execute(ctx, json.RawMessage(`{
		"goal":"verify planner path preservation",
		"targets":[
			"/abs/worktree/workspace/config/tars.config.yaml",
			"/abs/worktree/workspace/README.md"
		]
	}`))
	if err != nil {
		t.Fatalf("subagents_plan execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected target-preserving planner output, got %s", res.Text())
	}

	var payload struct {
		Steps []subagentFlowStepInput `json:"steps"`
	}
	if err := json.Unmarshal([]byte(res.Text()), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if len(payload.Steps) != 1 || len(payload.Steps[0].Tasks) != 2 {
		t.Fatalf("unexpected planner payload: %+v", payload)
	}

	prompts := []string{
		payload.Steps[0].Tasks[0].Prompt,
		payload.Steps[0].Tasks[1].Prompt,
	}
	for _, target := range []string{
		"/abs/worktree/workspace/config/tars.config.yaml",
		"/abs/worktree/workspace/README.md",
	} {
		found := false
		for _, prompt := range prompts {
			if strings.Contains(prompt, target) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected exact target %q to remain in at least one task prompt, got %+v", target, prompts)
		}
	}
}
