package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// captureAnthropicBody runs one Chat call against a stub server and returns
// the decoded request body, so tests can assert the exact wire shape.
func captureAnthropicBody(t *testing.T, model string, config ClientConfig, opts ChatOptions) map[string]any {
	t.Helper()
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}]}`))
	}))
	defer srv.Close()

	if config.MaxTokens <= 0 {
		config.MaxTokens = 16000
	}
	client, err := newAnthropicClientWithConfig(srv.URL, "k", model, config)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, err := client.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}}, opts); err != nil {
		t.Fatalf("chat: %v", err)
	}
	return body
}

func thinkingBlock(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	raw, ok := body["thinking"]
	if !ok {
		t.Fatalf("request carries no thinking block: %v", body)
	}
	block, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("thinking is %T, want an object", raw)
	}
	return block
}

// TestAnthropicRequest_BudgetModelNeverCarriesOutputConfig is the regression
// this whole change must not introduce. output_config.effort errors on Sonnet
// 4.5 and Haiku 4.5 — and Haiku 4.5 is llmdefaults.AnthropicModel, the default
// model. Leaking effort onto the budget path would trade the old 400 for a new
// one on the shipped default.
func TestAnthropicRequest_BudgetModelNeverCarriesOutputConfig(t *testing.T) {
	for _, model := range []string{"claude-haiku-4-5", "claude-haiku-4-5-20251001", "claude-sonnet-4-5", "claude-sonnet-4-6", "claude-opus-4-6"} {
		for _, effort := range []string{"minimal", "low", "medium", "high"} {
			body := captureAnthropicBody(t, model, ClientConfig{ReasoningEffort: effort}, ChatOptions{})
			if _, present := body["output_config"]; present {
				t.Errorf("%s effort=%s: output_config present on a budget-mode model", model, effort)
			}
			block := thinkingBlock(t, body)
			if block["type"] != "enabled" {
				t.Errorf("%s effort=%s: thinking.type = %v, want enabled", model, effort, block["type"])
			}
			if _, ok := block["budget_tokens"]; !ok {
				t.Errorf("%s effort=%s: thinking carries no budget_tokens", model, effort)
			}
		}
	}
}

// TestAnthropicRequest_UnknownModelKeepsBudgetShape pins the behavior for
// gateway-hosted models reached over an Anthropic-compatible endpoint: they
// are unrecognized, and the shape they have always received is the safe one.
func TestAnthropicRequest_UnknownModelKeepsBudgetShape(t *testing.T) {
	body := captureAnthropicBody(t, "MiniMax-M2.7", ClientConfig{ReasoningEffort: "medium"}, ChatOptions{})
	if _, present := body["output_config"]; present {
		t.Error("output_config present for an unrecognized model")
	}
	block := thinkingBlock(t, body)
	if block["type"] != "enabled" {
		t.Errorf("thinking.type = %v, want enabled", block["type"])
	}
}

// TestAnthropicRequest_AdaptiveModelSendsEffortNotBudget covers the defect:
// budget_tokens is rejected outright by these families.
func TestAnthropicRequest_AdaptiveModelSendsEffortNotBudget(t *testing.T) {
	cases := []struct {
		model      string
		effort     string
		wantEffort string
	}{
		{"claude-opus-4-7", "high", "high"},
		{"claude-opus-4-8", "medium", "medium"},
		{"claude-opus-5", "low", "low"},
		{"claude-sonnet-5", "minimal", "low"},
		{"claude-fable-5", "high", "high"},
		// A dated snapshot must inherit its family's row.
		{"claude-opus-4-7-20260115", "medium", "medium"},
	}
	for _, tc := range cases {
		t.Run(tc.model+"/"+tc.effort, func(t *testing.T) {
			body := captureAnthropicBody(t, tc.model, ClientConfig{ReasoningEffort: tc.effort}, ChatOptions{})

			block := thinkingBlock(t, body)
			if block["type"] != "adaptive" {
				t.Errorf("thinking.type = %v, want adaptive", block["type"])
			}
			if _, present := block["budget_tokens"]; present {
				t.Error("budget_tokens present on an adaptive model — the provider rejects this outright")
			}
			// Without this the console's reasoning stream goes silent:
			// these models omit reasoning text by default.
			if block["display"] != "summarized" {
				t.Errorf("thinking.display = %v, want summarized", block["display"])
			}

			outputConfig, ok := body["output_config"].(map[string]any)
			if !ok {
				t.Fatalf("output_config missing or not an object: %v", body["output_config"])
			}
			if outputConfig["effort"] != tc.wantEffort {
				t.Errorf("output_config.effort = %v, want %q", outputConfig["effort"], tc.wantEffort)
			}
		})
	}
}

func TestAnthropicRequest_NoEffortLeavesReasoningUnset(t *testing.T) {
	// Neither knob set: assert nothing and take the model's own default,
	// on both generations.
	for _, model := range []string{"claude-opus-5", "claude-haiku-4-5", "MiniMax-M2.7"} {
		body := captureAnthropicBody(t, model, ClientConfig{}, ChatOptions{})
		if _, present := body["thinking"]; present {
			t.Errorf("%s: thinking present with no effort configured", model)
		}
		if _, present := body["output_config"]; present {
			t.Errorf("%s: output_config present with no effort configured", model)
		}
	}
}

func TestAnthropicRequest_EffortNoneDisablesThinkingWhereSupported(t *testing.T) {
	body := captureAnthropicBody(t, "claude-opus-5", ClientConfig{ReasoningEffort: "none"}, ChatOptions{})
	block := thinkingBlock(t, body)
	if block["type"] != "disabled" {
		t.Fatalf("thinking.type = %v, want disabled", block["type"])
	}
	// Disabling is accepted only at effort high or below; omitting the field
	// defaults to high, so asserting one would risk a 400 for no gain.
	if _, present := body["output_config"]; present {
		t.Error("output_config present alongside disabled thinking")
	}
}

func TestAnthropicRequest_EffortNoneOmittedWhereThinkingCannotBeDisabled(t *testing.T) {
	// Fable and Mythos think unconditionally and reject an explicit disable.
	// Sending it anyway would be a 400; the setting is reported as unhonored
	// and the field left off.
	for _, model := range []string{"claude-fable-5", "claude-mythos-5"} {
		body := captureAnthropicBody(t, model, ClientConfig{ReasoningEffort: "none"}, ChatOptions{})
		if _, present := body["thinking"]; present {
			t.Errorf("%s: thinking present though the model rejects an explicit disable", model)
		}
		if _, present := body["output_config"]; present {
			t.Errorf("%s: output_config present for effort=none", model)
		}
	}
}

func TestAnthropicRequest_ThinkingBudgetOnAdaptiveModelBecomesEffort(t *testing.T) {
	// TARS configs cannot reach this — ResolveLLMTier rejects the pairing —
	// but a caller building a provider directly through pkg/llm can, and it
	// must not ship a 400.
	cases := []struct {
		budget     int
		wantEffort string
	}{
		{1024, "low"},
		{2048, "low"},
		{8192, "medium"},
		{16384, "high"},
	}
	for _, tc := range cases {
		body := captureAnthropicBody(t, "claude-opus-5", ClientConfig{ThinkingBudget: tc.budget}, ChatOptions{})
		block := thinkingBlock(t, body)
		if _, present := block["budget_tokens"]; present {
			t.Errorf("budget %d: budget_tokens survived onto an adaptive model", tc.budget)
		}
		outputConfig, ok := body["output_config"].(map[string]any)
		if !ok {
			t.Fatalf("budget %d: output_config missing", tc.budget)
		}
		if outputConfig["effort"] != tc.wantEffort {
			t.Errorf("budget %d: effort = %v, want %q", tc.budget, outputConfig["effort"], tc.wantEffort)
		}
	}
}

func TestAnthropicRequest_ExplicitEffortOutranksBudgetOnAdaptiveModel(t *testing.T) {
	body := captureAnthropicBody(t, "claude-opus-5", ClientConfig{
		ReasoningEffort: "low",
		ThinkingBudget:  16384,
	}, ChatOptions{})
	outputConfig, ok := body["output_config"].(map[string]any)
	if !ok {
		t.Fatalf("output_config missing: %v", body)
	}
	if outputConfig["effort"] != "low" {
		t.Fatalf("effort = %v, want low — the expressible knob wins", outputConfig["effort"])
	}
}

func TestAnthropicEffortForLevel(t *testing.T) {
	cases := map[string]string{
		"minimal": "low",
		"low":     "low",
		"medium":  "medium",
		"high":    "high",
		"none":    "",
		"":        "",
		"bogus":   "",
	}
	for level, want := range cases {
		if got := anthropicEffortForLevel(level); got != want {
			t.Errorf("anthropicEffortForLevel(%q) = %q, want %q", level, got, want)
		}
	}
}

func TestAnthropicRequest_BudgetModelBehaviorUnchangedByThisChange(t *testing.T) {
	// The budget path must render exactly as it did before adaptive support
	// existed: the effort ladder clamped against max_tokens headroom.
	body := captureAnthropicBody(t, "claude-haiku-4-5", ClientConfig{
		MaxTokens:       16000,
		ReasoningEffort: "medium",
	}, ChatOptions{})
	block := thinkingBlock(t, body)
	if block["budget_tokens"] != float64(8192) {
		t.Fatalf("budget_tokens = %v, want 8192", block["budget_tokens"])
	}
}
