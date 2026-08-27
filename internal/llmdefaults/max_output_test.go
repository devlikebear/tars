package llmdefaults

import "testing"

func TestMaxOutputTokens_DocumentedFamilies(t *testing.T) {
	cases := map[string]int{
		"claude-fable-5":    128000,
		"claude-mythos-5":   128000,
		"claude-opus-5":     128000,
		"claude-opus-4-8":   128000,
		"claude-opus-4-7":   128000,
		"claude-opus-4-6":   128000,
		"claude-sonnet-5":   128000,
		"claude-sonnet-4-6": 128000,
		"claude-haiku-4-5":  64000,
	}
	for model, want := range cases {
		if got := MaxOutputTokens(model); got != want {
			t.Errorf("MaxOutputTokens(%q) = %d, want %d", model, got, want)
		}
	}
}

func TestMaxOutputTokens_DatedSnapshotInheritsFamily(t *testing.T) {
	// AnthropicModel is a dated ID; it must resolve through the family
	// prefix rather than needing its own row.
	if got := MaxOutputTokens(AnthropicModel); got != 64000 {
		t.Fatalf("MaxOutputTokens(%q) = %d, want 64000", AnthropicModel, got)
	}
	if got := MaxOutputTokens("claude-opus-4-7-20260115"); got != 128000 {
		t.Fatalf("dated opus snapshot = %d, want 128000", got)
	}
}

func TestMaxOutputTokens_UnknownModelsReturnZero(t *testing.T) {
	// Gateway-hosted models and older families must miss rather than
	// inherit a ceiling the provider would reject. Sonnet 4.5 in
	// particular must not prefix-match the 128K Sonnet 4.6 row.
	for _, model := range []string{
		"",
		"   ",
		"MiniMax-M2.7",
		"gpt-5.4",
		"claude-sonnet-4-5",
		"claude-opus-4-5",
		"claude-opus-4-1",
		"claude",
	} {
		if got := MaxOutputTokens(model); got != 0 {
			t.Errorf("MaxOutputTokens(%q) = %d, want 0", model, got)
		}
	}
}

func TestMaxOutputTokens_NormalizesCaseAndSpace(t *testing.T) {
	if got := MaxOutputTokens("  Claude-Opus-5  "); got != 128000 {
		t.Fatalf("MaxOutputTokens with padding/case = %d, want 128000", got)
	}
}

func TestModelBehaviorFor_ThinkingModeSplit(t *testing.T) {
	// The Adaptive set is exactly the families where budget_tokens is a hard
	// 400. Opus 4.6 and Sonnet 4.6 accept adaptive thinking too, but
	// budget_tokens still works there, so they stay on Budget — that is what
	// keeps callers from turning a working config into a startup error.
	adaptive := []string{
		"claude-fable-5", "claude-mythos-5", "claude-opus-5",
		"claude-opus-4-8", "claude-opus-4-7", "claude-sonnet-5",
	}
	budget := []string{
		"claude-opus-4-6", "claude-sonnet-4-6", "claude-haiku-4-5",
	}
	for _, model := range adaptive {
		behavior, ok := ModelBehaviorFor(model)
		if !ok {
			t.Errorf("%s: not recognized", model)
			continue
		}
		if behavior.Thinking != ThinkingModeAdaptive {
			t.Errorf("%s: Thinking = %v, want adaptive", model, behavior.Thinking)
		}
	}
	for _, model := range budget {
		behavior, ok := ModelBehaviorFor(model)
		if !ok {
			t.Errorf("%s: not recognized", model)
			continue
		}
		if behavior.Thinking != ThinkingModeBudget {
			t.Errorf("%s: Thinking = %v, want budget", model, behavior.Thinking)
		}
	}
}

func TestModelBehaviorFor_UnknownModelIsBudgetMode(t *testing.T) {
	// The zero value must be the conservative choice: an unrecognized model
	// is every gateway-hosted one, and the budget shape is what this client
	// has always sent them.
	for _, model := range []string{"", "MiniMax-M2.7", "claude-sonnet-4-5", "gpt-5.4"} {
		behavior, ok := ModelBehaviorFor(model)
		if ok {
			t.Errorf("%s: unexpectedly recognized", model)
		}
		if behavior.Thinking != ThinkingModeBudget {
			t.Errorf("%s: Thinking = %v, want budget for an unknown model", model, behavior.Thinking)
		}
		if behavior.MaxOutputTokens != 0 {
			t.Errorf("%s: MaxOutputTokens = %d, want 0", model, behavior.MaxOutputTokens)
		}
	}
}

func TestModelBehaviorFor_ThinkingCanBeDisabledExceptOnAlwaysThinkingModels(t *testing.T) {
	for _, model := range []string{"claude-fable-5", "claude-mythos-5"} {
		behavior, _ := ModelBehaviorFor(model)
		if behavior.CanDisableThinking {
			t.Errorf("%s: CanDisableThinking = true, but this model rejects an explicit disable", model)
		}
	}
	for _, model := range []string{"claude-opus-5", "claude-opus-4-8", "claude-opus-4-7", "claude-sonnet-5"} {
		behavior, _ := ModelBehaviorFor(model)
		if !behavior.CanDisableThinking {
			t.Errorf("%s: CanDisableThinking = false, want true", model)
		}
	}
}

func TestModelBehaviorFor_DatedSnapshotInheritsTheFamilyRow(t *testing.T) {
	behavior, ok := ModelBehaviorFor("claude-opus-4-7-20260115")
	if !ok {
		t.Fatal("dated opus 4.7 snapshot not recognized")
	}
	if behavior.Thinking != ThinkingModeAdaptive || behavior.MaxOutputTokens != 128000 {
		t.Fatalf("dated snapshot behavior = %+v, want the opus-4-7 row", behavior)
	}
}
