package initiative

import "testing"

func TestPlanTextJev(t *testing.T) {
	cases := []struct {
		name string
		in   PlanInput
		want BackendPlan
	}{
		{
			name: "not configured",
			in:   PlanInput{Backend: "jev"},
			want: BackendPlan{Backend: "jev", Usable: false, SendsText: false, Reason: "jev_not_configured", Kind: "jev"},
		},
		{
			name: "loopback",
			in:   PlanInput{Backend: "jev", JevConfigured: true, JevLoopback: true, JevHost: "127.0.0.1:8009"},
			want: BackendPlan{Backend: "jev", Usable: true, SendsText: true, Reason: "jev_loopback", Kind: "jev", Provider: "127.0.0.1:8009"},
		},
		{
			name: "remote",
			in:   PlanInput{Backend: "jev", JevConfigured: true, JevLoopback: false, JevHost: "api.typesafe.ai"},
			want: BackendPlan{Backend: "jev", Usable: true, SendsText: false, Reason: "jev_remote", Kind: "jev", Provider: "api.typesafe.ai"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := PlanText(c.in)
			if got != c.want {
				t.Fatalf("PlanText(%+v) = %+v, want %+v", c.in, got, c.want)
			}
		})
	}
}

func TestPlanTextLLM(t *testing.T) {
	cases := []struct {
		name string
		in   PlanInput
		want BackendPlan
	}{
		{
			name: "unavailable",
			in:   PlanInput{Backend: "llm", LLMResolved: false},
			want: BackendPlan{Backend: "llm", Usable: false, SendsText: false, Reason: "llm_unavailable"},
		},
		{
			name: "unsupported provider (antigravity-cli)",
			in: PlanInput{Backend: "llm", LLMResolved: true, LLMSupportsDecisionOnly: false,
				LLMKind: "antigravity-cli", LLMProviderAlias: "agy", LLMModel: "m", LLMTier: "light"},
			want: BackendPlan{Backend: "llm", Usable: false, SendsText: false, Reason: "llm_unsupported_provider",
				Kind: "antigravity-cli", Provider: "agy", Model: "m", Tier: "light"},
		},
		{
			name: "same provider alias as chat",
			in: PlanInput{Backend: "llm", LLMResolved: true, LLMSupportsDecisionOnly: true,
				LLMKind: "anthropic", LLMProviderAlias: "anthropic-main", LLMChatProviderAlias: "anthropic-main",
				LLMModel: "haiku", LLMTier: "light"},
			want: BackendPlan{Backend: "llm", Usable: true, SendsText: true, Reason: "llm_same_provider",
				Kind: "anthropic", Provider: "anthropic-main", Model: "haiku", Tier: "light"},
		},
		{
			name: "different provider alias than chat",
			in: PlanInput{Backend: "llm", LLMResolved: true, LLMSupportsDecisionOnly: true,
				LLMKind: "anthropic", LLMProviderAlias: "anthropic-light", LLMChatProviderAlias: "anthropic-main",
				LLMModel: "haiku", LLMTier: "light"},
			want: BackendPlan{Backend: "llm", Usable: true, SendsText: false, Reason: "llm_other_provider",
				Kind: "anthropic", Provider: "anthropic-light", Model: "haiku", Tier: "light"},
		},
		{
			name: "same kind different alias (different credentials) does not send text",
			in: PlanInput{Backend: "llm", LLMResolved: true, LLMSupportsDecisionOnly: true,
				LLMKind: "openai", LLMProviderAlias: "openai-a", LLMChatProviderAlias: "openai-b",
				LLMModel: "gpt", LLMTier: "light"},
			want: BackendPlan{Backend: "llm", Usable: true, SendsText: false, Reason: "llm_other_provider",
				Kind: "openai", Provider: "openai-a", Model: "gpt", Tier: "light"},
		},
		{
			name: "empty chat alias never matches",
			in: PlanInput{Backend: "llm", LLMResolved: true, LLMSupportsDecisionOnly: true,
				LLMKind: "openai", LLMProviderAlias: "", LLMChatProviderAlias: "",
				LLMModel: "gpt", LLMTier: "light"},
			want: BackendPlan{Backend: "llm", Usable: true, SendsText: false, Reason: "llm_other_provider",
				Kind: "openai", Provider: "", Model: "gpt", Tier: "light"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := PlanText(c.in)
			if got != c.want {
				t.Fatalf("PlanText(%+v) = %+v, want %+v", c.in, got, c.want)
			}
		})
	}
}

func TestPlanTextDefaultsToLLM(t *testing.T) {
	got := PlanText(PlanInput{Backend: ""})
	if got.Backend != "llm" || got.Usable || got.SendsText {
		t.Fatalf("got %+v", got)
	}
}
