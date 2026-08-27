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
