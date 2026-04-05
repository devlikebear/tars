package pulse

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
)

type fakeLLMClient struct {
	resp llm.ChatResponse
	err  error
	// capture last call
	lastMessages []llm.ChatMessage
	lastOpts     llm.ChatOptions
}

func (f *fakeLLMClient) Ask(ctx context.Context, prompt string) (string, error) {
	return "", nil
}

func (f *fakeLLMClient) Chat(ctx context.Context, messages []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	f.lastMessages = messages
	f.lastOpts = opts
	if f.err != nil {
		return llm.ChatResponse{}, f.err
	}
	return f.resp, nil
}

// routerForClient wraps a single llm.Client into a three-tier router
// where every tier and role resolves to that client. Used by pulse tests
// that exercise Decider logic without caring about tier routing.
func routerForClient(client llm.Client) llm.Router {
	entry := llm.TierEntry{Client: client, Provider: "fake", Model: "fake-model"}
	router, err := llm.NewRouter(llm.RouterConfig{
		Tiers: map[llm.Tier]llm.TierEntry{
			llm.TierHeavy:    entry,
			llm.TierStandard: entry,
			llm.TierLight:    entry,
		},
		DefaultTier: llm.TierLight,
		RoleDefaults: map[llm.Role]llm.Tier{
			llm.RolePulseDecider: llm.TierLight,
		},
	})
	if err != nil {
		panic(err)
	}
	return router
}

func makeToolCall(args string) llm.ChatResponse {
	return llm.ChatResponse{
		Message: llm.ChatMessage{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{
				{ID: "c1", Name: PulseDecideToolName, Arguments: args},
			},
		},
		StopReason: "tool_use",
	}
}

func TestPulseDecideToolSchema(t *testing.T) {
	schema := PulseDecideToolSchema()
	if schema.Function.Name != "pulse_decide" {
		t.Errorf("schema name = %q, want pulse_decide", schema.Function.Name)
	}
	if len(schema.Function.Parameters) == 0 {
		t.Error("schema parameters should be non-empty")
	}
}

func TestDecider_SuccessNotify(t *testing.T) {
	client := &fakeLLMClient{
		resp: makeToolCall(`{"action":"notify","severity":"warn","title":"Disk high","summary":"usage 90%"}`),
	}
	d := NewDecider(routerForClient(client), DeciderPolicy{MinSeverity: SeverityWarn})
	got, err := d.Decide(context.Background(), []Signal{
		{Kind: SignalKindDiskUsage, Severity: SeverityWarn, Summary: "disk 90%"},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Action != ActionNotify || got.Severity != SeverityWarn || got.Title != "Disk high" {
		t.Errorf("unexpected decision: %+v", got)
	}
	// Verify options were set correctly.
	if client.lastOpts.ToolChoice != "required" {
		t.Errorf("ToolChoice = %q, want required", client.lastOpts.ToolChoice)
	}
	if len(client.lastOpts.Tools) != 1 || client.lastOpts.Tools[0].Function.Name != "pulse_decide" {
		t.Errorf("tools not wired correctly: %+v", client.lastOpts.Tools)
	}
}

func TestDecider_SuccessAutofix(t *testing.T) {
	client := &fakeLLMClient{
		resp: makeToolCall(`{"action":"autofix","severity":"info","autofix_name":"compress_old_logs"}`),
	}
	d := NewDecider(routerForClient(client), DeciderPolicy{AllowedAutofixes: []string{"compress_old_logs"}})
	got, err := d.Decide(context.Background(), []Signal{{Kind: SignalKindCronFailures, Severity: SeverityWarn, Summary: "x"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Action != ActionAutofix || got.AutofixName != "compress_old_logs" {
		t.Errorf("unexpected decision: %+v", got)
	}
}

func TestDecider_SuccessIgnore(t *testing.T) {
	client := &fakeLLMClient{
		resp: makeToolCall(`{"action":"ignore","severity":"info"}`),
	}
	d := NewDecider(routerForClient(client), DeciderPolicy{})
	got, err := d.Decide(context.Background(), []Signal{{Kind: SignalKindCronFailures, Severity: SeverityInfo, Summary: "x"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Action != ActionIgnore {
		t.Errorf("action = %v, want ignore", got.Action)
	}
}

func TestDecider_LLMErrorPropagates(t *testing.T) {
	client := &fakeLLMClient{err: errors.New("timeout")}
	d := NewDecider(routerForClient(client), DeciderPolicy{})
	_, err := d.Decide(context.Background(), []Signal{{Kind: SignalKindDiskUsage, Severity: SeverityWarn, Summary: "x"}})
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Errorf("want timeout error, got %v", err)
	}
}

func TestDecider_NoToolCall(t *testing.T) {
	client := &fakeLLMClient{
		resp: llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "sorry"}},
	}
	d := NewDecider(routerForClient(client), DeciderPolicy{})
	_, err := d.Decide(context.Background(), []Signal{{Kind: SignalKindDiskUsage, Severity: SeverityWarn, Summary: "x"}})
	if err == nil || !strings.Contains(err.Error(), "no tool calls") {
		t.Errorf("want no tool calls error, got %v", err)
	}
}

func TestDecider_WrongToolName(t *testing.T) {
	client := &fakeLLMClient{
		resp: llm.ChatResponse{Message: llm.ChatMessage{
			ToolCalls: []llm.ToolCall{{Name: "unrelated", Arguments: "{}"}},
		}},
	}
	d := NewDecider(routerForClient(client), DeciderPolicy{})
	_, err := d.Decide(context.Background(), []Signal{{Kind: SignalKindDiskUsage, Severity: SeverityWarn, Summary: "x"}})
	if err == nil || !strings.Contains(err.Error(), "pulse_decide") {
		t.Errorf("want wrong tool error, got %v", err)
	}
}

func TestDecider_MalformedArguments(t *testing.T) {
	client := &fakeLLMClient{resp: makeToolCall(`{bad json`)}
	d := NewDecider(routerForClient(client), DeciderPolicy{})
	_, err := d.Decide(context.Background(), []Signal{{Kind: SignalKindDiskUsage, Severity: SeverityWarn, Summary: "x"}})
	if err == nil {
		t.Error("expected parse error")
	}
}

func TestDecider_InvalidAction(t *testing.T) {
	client := &fakeLLMClient{resp: makeToolCall(`{"action":"run","severity":"warn"}`)}
	d := NewDecider(routerForClient(client), DeciderPolicy{})
	_, err := d.Decide(context.Background(), []Signal{{Kind: SignalKindDiskUsage, Severity: SeverityWarn, Summary: "x"}})
	if err == nil || !strings.Contains(err.Error(), "action") {
		t.Errorf("want invalid action error, got %v", err)
	}
}

func TestDecider_InvalidSeverity(t *testing.T) {
	client := &fakeLLMClient{resp: makeToolCall(`{"action":"ignore","severity":"wat"}`)}
	d := NewDecider(routerForClient(client), DeciderPolicy{})
	_, err := d.Decide(context.Background(), []Signal{{Kind: SignalKindDiskUsage, Severity: SeverityWarn, Summary: "x"}})
	if err == nil || !strings.Contains(err.Error(), "severity") {
		t.Errorf("want invalid severity error, got %v", err)
	}
}

func TestDecider_AutofixMissingName(t *testing.T) {
	client := &fakeLLMClient{resp: makeToolCall(`{"action":"autofix","severity":"warn"}`)}
	d := NewDecider(routerForClient(client), DeciderPolicy{AllowedAutofixes: []string{"compress_old_logs"}})
	_, err := d.Decide(context.Background(), []Signal{{Kind: SignalKindCronFailures, Severity: SeverityWarn, Summary: "x"}})
	if err == nil || !strings.Contains(err.Error(), "autofix_name") {
		t.Errorf("want missing name error, got %v", err)
	}
}

func TestDecider_AutofixNotAllowed(t *testing.T) {
	client := &fakeLLMClient{resp: makeToolCall(`{"action":"autofix","severity":"warn","autofix_name":"drop_all_tables"}`)}
	d := NewDecider(routerForClient(client), DeciderPolicy{AllowedAutofixes: []string{"compress_old_logs"}})
	_, err := d.Decide(context.Background(), []Signal{{Kind: SignalKindCronFailures, Severity: SeverityWarn, Summary: "x"}})
	if err == nil || !strings.Contains(err.Error(), "not in the allowed list") {
		t.Errorf("want not allowed error, got %v", err)
	}
}

func TestDecider_NotifyRequiresTitle(t *testing.T) {
	client := &fakeLLMClient{resp: makeToolCall(`{"action":"notify","severity":"warn"}`)}
	d := NewDecider(routerForClient(client), DeciderPolicy{})
	_, err := d.Decide(context.Background(), []Signal{{Kind: SignalKindDiskUsage, Severity: SeverityWarn, Summary: "x"}})
	if err == nil || !strings.Contains(err.Error(), "title") {
		t.Errorf("want title required error, got %v", err)
	}
}

func TestDecider_NoSignalsError(t *testing.T) {
	d := NewDecider(routerForClient(&fakeLLMClient{}), DeciderPolicy{})
	_, err := d.Decide(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "no signals") {
		t.Errorf("want no signals error, got %v", err)
	}
}

func TestDecider_NilClient(t *testing.T) {
	d := NewDecider(nil, DeciderPolicy{})
	_, err := d.Decide(context.Background(), []Signal{{Kind: SignalKindDiskUsage, Severity: SeverityWarn, Summary: "x"}})
	if err == nil {
		t.Error("expected error for nil client")
	}
}

func TestBuildDeciderPromptContainsSignals(t *testing.T) {
	signals := []Signal{
		{Kind: SignalKindDiskUsage, Severity: SeverityWarn, Summary: "disk 90%", Details: map[string]any{"pct": 90}},
		{Kind: SignalKindCronFailures, Severity: SeverityError, Summary: "2 jobs failing"},
	}
	policy := DeciderPolicy{AllowedAutofixes: []string{"compress_old_logs", "cleanup_stale_tmp"}, MinSeverity: SeverityWarn}
	out := buildDeciderPrompt(signals, policy)
	for _, want := range []string{"disk_usage", "cron_failures", "disk 90%", "2 jobs failing", "compress_old_logs", "min_severity: warn"} {
		if !strings.Contains(out, want) {
			t.Errorf("prompt missing %q:\n%s", want, out)
		}
	}
}

func TestBuildDeciderPromptEmptyAutofixes(t *testing.T) {
	out := buildDeciderPrompt(
		[]Signal{{Kind: SignalKindCronFailures, Severity: SeverityInfo, Summary: "x"}},
		DeciderPolicy{},
	)
	if !strings.Contains(out, "(none") {
		t.Errorf("prompt should note empty autofix list:\n%s", out)
	}
}
