package computeruse

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/devlikebear/tars/pkg/llm"
)

func TestLLMBackendValidatedActionsAndUsage(t *testing.T) {
	client := &llm.FakeClient{ChatResponse: llm.ChatResponse{Message: llm.ChatMessage{Content: `{"op":"click","target":"e1","input_key":"none","risky":false,"done":false}`}, Usage: llm.Usage{InputTokens: 100, OutputTokens: 20, CostUSD: 0.012}}}
	backend := NewLLMBackend(client, "test-model", nil)
	d, spent, err := backend.Decide(context.Background(), "GOAL: click", BuildQuestions([]Element{{Index: 1, Role: "AXButton", Enabled: true}}, nil))
	if err != nil || d.Op != OpClick || d.TargetIndex != 1 {
		t.Fatalf("decision = %+v, %v", d, err)
	}
	if d.Probabilistic || d.OpConfidence != 0 {
		t.Fatalf("LLM decision claims calibrated confidence: %+v", d)
	}
	if spent.InputTokens != 100 || spent.OutputTokens != 20 || spent.EstUSD != 0.012 {
		t.Fatalf("usage = %+v", spent)
	}
	if !client.LastChatOptions.ClaudeCodeHarness.SafeMode {
		t.Fatal("decision calls must disable Claude hooks and customizations")
	}
	if client.LastChatOptions.ToolChoice.Mode != llm.ToolChoiceModeNone {
		t.Fatal("decision calls must not execute tools")
	}
}

func TestLLMBackendRejectsInvalidDecision(t *testing.T) {
	for _, content := range []string{
		`{"op":"click","target":"none","input_key":"none","risky":false,"done":false}`,
		`{"op":"type","target":"e1","input_key":"none","risky":false,"done":false}`,
		`{"op":"done","target":"e1","input_key":"none","risky":false,"done":true}`,
		`not-json`,
		`{"op":"shell","target":"none","input_key":"none","risky":false,"done":false}`,
		`{"op":"click","target":"e999","input_key":"none","risky":false,"done":false}`,
		`{"op":"type","target":"e1","input_key":"secret","risky":false,"done":false}`,
		`{"op":"click","target":"e1"}`,
		`{"op":"done","target":"none","input_key":"none","risky":false,"done":false}`,
		`{"op":"click","target":"e1","input_key":"none","risky":false,"done":false,"confidence":1}`,
		`{"op":"click","target":"e1","input_key":"none","risky":false,"done":false} {}`,
	} {
		t.Run(content, func(t *testing.T) {
			client := &llm.FakeClient{ChatResponse: llm.ChatResponse{Message: llm.ChatMessage{Content: content}, Usage: llm.Usage{InputTokens: 10}}}
			_, spent, err := NewLLMBackend(client, "test", nil).Decide(context.Background(), "screen", BuildQuestions([]Element{{Index: 1, Role: "AXTextField", Enabled: true}}, []string{"name"}))
			if err == nil {
				t.Fatal("invalid decision accepted")
			}
			if spent.InputTokens != 10 {
				t.Fatal("invalid output must still account for spent tokens")
			}
		})
	}
}

func TestLLMEngineActsWithoutFakeConfidenceAndConfirmsRisk(t *testing.T) {
	for _, risky := range []bool{false, true} {
		content := `{"op":"click","target":"e1","input_key":"none","risky":false,"done":false}`
		if risky {
			content = `{"op":"click","target":"e1","input_key":"none","risky":true,"done":false}`
		}
		client := &llm.FakeClient{ChatResponse: llm.ChatResponse{Message: llm.ChatMessage{Content: content}}}
		driver := &FakeDriver{snaps: []Snapshot{snap("A")}}
		cfg := DefaultConfig()
		cfg.MaxSteps = 1
		res := NewEngineWithBackend(driver, NewLLMBackend(client, "test", nil), cfg).Run(context.Background(), Request{Goal: "click"})
		if risky {
			if res.Status != StatusNeedsConfirmation || len(driver.actions) != 0 {
				t.Fatalf("risk bypassed: %+v", res)
			}
		} else if len(driver.actions) != 1 {
			t.Fatalf("validated action did not execute: %+v", res)
		}
	}
}

func TestLLMEngineConfirmsHardToUndoControlEvenWhenModelSaysSafe(t *testing.T) {
	client := &llm.FakeClient{ChatResponse: llm.ChatResponse{Message: llm.ChatMessage{Content: `{"op":"click","target":"e1","input_key":"none","risky":false,"done":false}`}}}
	driver := &FakeDriver{snaps: []Snapshot{snap("Send")}}
	res := NewEngineWithBackend(driver, NewLLMBackend(client, "test", nil), DefaultConfig()).Run(context.Background(), Request{Goal: "send message"})
	if res.Status != StatusNeedsConfirmation || len(driver.actions) != 0 {
		t.Fatalf("send executed without confirmation: %+v", res)
	}
}

type decisionClient struct {
	responses []llm.ChatResponse
	err       error
	messages  []llm.ChatMessage
}

func (c *decisionClient) Ask(context.Context, string) (string, error) { return "", c.err }
func (c *decisionClient) Chat(_ context.Context, messages []llm.ChatMessage, _ llm.ChatOptions) (llm.ChatResponse, error) {
	c.messages = append(c.messages, messages...)
	if c.err != nil {
		return llm.ChatResponse{}, c.err
	}
	response := c.responses[0]
	if len(c.responses) > 1 {
		c.responses = c.responses[1:]
	}
	return response, nil
}

func TestLLMEngineMetersFailedCall(t *testing.T) {
	client := &decisionClient{err: &llm.PartialUsageError{Err: fmt.Errorf("provider disconnected"), Usage: llm.Usage{InputTokens: 40, OutputTokens: 8, CostUSD: 0.01}}}
	driver := &FakeDriver{snaps: []Snapshot{snap("A")}}
	res := NewEngineWithBackend(driver, NewLLMBackend(client, "test", nil), DefaultConfig()).Run(context.Background(), Request{Goal: "click"})
	if res.Status != StatusError || res.Usage.InputTokens != 40 || res.Usage.OutputTokens != 8 || res.Usage.EstUSD != 0.01 || len(driver.actions) != 0 {
		t.Fatalf("result: %+v", res)
	}
}

func TestLLMBackendRefusesToolResponsesAndMissingClient(t *testing.T) {
	qs := BuildQuestions([]Element{{Index: 1, Role: "AXButton", Enabled: true}}, nil)
	if _, _, err := NewLLMBackend(nil, "test", nil).Decide(context.Background(), "screen", qs); err == nil {
		t.Fatal("nil client accepted")
	}
	client := &decisionClient{responses: []llm.ChatResponse{{Message: llm.ChatMessage{ToolCalls: []llm.ToolCall{{Name: "exec"}}}, Usage: llm.Usage{InputTokens: 10}}}}
	_, spent, err := NewLLMBackend(client, "test", func(u llm.Usage) (float64, bool) { return float64(u.InputTokens) * 0.001, true }).Decide(context.Background(), "screen", qs)
	if err == nil || spent.EstUSD != 0.01 || !spent.PricingKnown {
		t.Fatalf("tool response or metering: %+v %v", spent, err)
	}
}

func TestLLMEngineSecureInputResumeAndVisibleCompletion(t *testing.T) {
	first := snap("Name")
	first.Elements[0].Secure = true
	last := snap("Name")
	last.Elements[0].Secure = true
	last.Elements[0].Value = "private-input"
	last.Texts = []string{"Submitted"}
	client := &decisionClient{responses: []llm.ChatResponse{
		{Message: llm.ChatMessage{Content: `{"op":"type","target":"e1","input_key":"name","risky":false,"done":false}`}},
		{Message: llm.ChatMessage{Content: `{"op":"done","target":"none","input_key":"none","risky":false,"done":true}`}},
	}}
	driver := &FakeDriver{snaps: []Snapshot{first, last}}
	engine := NewEngineWithBackend(driver, NewLLMBackend(client, "test", nil), DefaultConfig())
	res := engine.Run(context.Background(), Request{Goal: "enter name", Inputs: map[string]string{"name": "private-input"}})
	if res.Status != StatusNeedsConfirmation || len(driver.actions) != 0 {
		t.Fatalf("secure gate: %+v", res)
	}
	res = engine.Resume(context.Background(), res.Resume, true)
	if res.Status != StatusDone || len(driver.actions) != 1 || driver.actions[0] != "type:t1=private-input" {
		t.Fatalf("resume: %+v, actions=%v", res, driver.actions)
	}
	for _, msg := range client.messages {
		if strings.Contains(msg.Content, "private-input") {
			t.Fatal("secure input leaked")
		}
	}
}

func TestLLMEngineDisabledControlCannotExecute(t *testing.T) {
	screen := snap("A")
	screen.Elements[0].Enabled = false
	client := &llm.FakeClient{ChatResponse: llm.ChatResponse{Message: llm.ChatMessage{Content: `{"op":"click","target":"e1","input_key":"none","risky":false,"done":false}`}}}
	driver := &FakeDriver{snaps: []Snapshot{screen}}
	res := NewEngineWithBackend(driver, NewLLMBackend(client, "test", nil), DefaultConfig()).Run(context.Background(), Request{Goal: "click"})
	if res.Status != StatusStuck || len(driver.actions) != 0 {
		t.Fatalf("disabled control executed: %+v", res)
	}
}
