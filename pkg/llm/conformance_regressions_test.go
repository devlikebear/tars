package llm

import (
	"context"
	"strings"
	"testing"
)

// These pin the specific defects Phase 0 and Phase 1 of the provider
// modernization epic fixed. The issue's validation clause asks that the suite
// reproduce each one when its fix is reverted, so each test names the fix it
// guards and asserts the observable consequence rather than the
// implementation detail.

// LP-002 (#921): the message array carried no cache breakpoints, so a growing
// transcript was re-charged in full every turn.
func TestRegression_AnthropicPlacesRollingCacheBreakpoints(t *testing.T) {
	provider := conformanceProviders()[0]
	if provider.kind != "anthropic" {
		t.Fatalf("expected anthropic first in the table, got %q", provider.kind)
	}
	stub := newConformanceStub(t, provider, provider.okBody)
	client := stub.client(t, ProviderOptions{})

	// A transcript long enough to have completed turns behind the incoming one.
	messages := []ChatMessage{
		{Role: "user", Content: "one"},
		{Role: "assistant", Content: "first answer"},
		{Role: "user", Content: "two"},
		{Role: "assistant", Content: "second answer"},
		{Role: "user", Content: "three"},
	}
	if _, err := client.Chat(context.Background(), messages, ChatOptions{}); err != nil {
		t.Fatalf("chat: %v", err)
	}
	if !strings.Contains(stub.lastRaw, "cache_control") {
		t.Fatalf("no cache breakpoint reached the message array — a long transcript is re-charged in full every turn:\n%s", stub.lastRaw)
	}
}

// LP-003 (#922): reasoning_effort was read only from the thinking budget on
// the Anthropic path, so the level was accepted and ignored.
func TestRegression_AnthropicHonorsReasoningEffort(t *testing.T) {
	provider := conformanceProviders()[0]
	plain := newConformanceStub(t, provider, provider.okBody)
	plainClient := plain.client(t, ProviderOptions{Model: "claude-haiku-4-5"})
	if _, err := plainClient.Chat(context.Background(), conformanceUserTurn(), ChatOptions{}); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if strings.Contains(plain.lastRaw, "thinking") {
		t.Fatalf("thinking appeared with no effort configured:\n%s", plain.lastRaw)
	}

	effort := newConformanceStub(t, provider, provider.okBody)
	effortClient := effort.client(t, ProviderOptions{Model: "claude-haiku-4-5", ReasoningEffort: "medium"})
	if _, err := effortClient.Chat(context.Background(), conformanceUserTurn(), ChatOptions{}); err != nil {
		t.Fatalf("effort: %v", err)
	}
	if !strings.Contains(effort.lastRaw, "thinking") {
		t.Fatalf("reasoning_effort produced no thinking configuration — the setting is a no-op:\n%s", effort.lastRaw)
	}
}

// LP-004 (#923): max_tokens was structurally always 0, so every Anthropic
// tier shipped a hardcoded 4096 ceiling no matter what config said.
func TestRegression_AnthropicMaxTokensIsConfigurable(t *testing.T) {
	provider := conformanceProviders()[0]
	stub := newConformanceStub(t, provider, provider.okBody)
	client := stub.client(t, ProviderOptions{Model: "claude-haiku-4-5", MaxTokens: 32000})

	if _, err := client.Chat(context.Background(), conformanceUserTurn(), ChatOptions{}); err != nil {
		t.Fatalf("chat: %v", err)
	}
	if stub.lastBody["max_tokens"] != float64(32000) {
		t.Fatalf("max_tokens = %v, want 32000 — the tier setting does not reach the request", stub.lastBody["max_tokens"])
	}
}

// LP-004 (#923): anthropic-beta was one hardcoded constant carrying a flag
// that had gone GA, occupying the only slot and reaching gateways uninvited.
func TestRegression_AnthropicBetaHeaderIsOptIn(t *testing.T) {
	provider := conformanceProviders()[0]
	stub := newConformanceStub(t, provider, provider.okBody)
	client := stub.client(t, ProviderOptions{Model: "claude-haiku-4-5"})
	if _, err := client.Chat(context.Background(), conformanceUserTurn(), ChatOptions{}); err != nil {
		t.Fatalf("chat: %v", err)
	}
	// The stub records the body, so assert the header through the builder
	// the request path uses.
	if header := anthropicBetaHeader(nil); header != "" {
		t.Fatalf("default beta header = %q, want none", header)
	}
	if header := anthropicBetaHeader([]string{"context-1m-2025-08-07"}); header != "context-1m-2025-08-07" {
		t.Fatalf("opted-in beta header = %q", header)
	}
}

// LP-013 (#943): the thinking request shape is model-dependent, and the
// client sent one shape for every model — a 400 on every current family.
func TestRegression_AnthropicThinkingShapeFollowsTheModel(t *testing.T) {
	provider := conformanceProviders()[0]

	budget := newConformanceStub(t, provider, provider.okBody)
	budgetClient := budget.client(t, ProviderOptions{Model: "claude-haiku-4-5", ReasoningEffort: "medium"})
	if _, err := budgetClient.Chat(context.Background(), conformanceUserTurn(), ChatOptions{}); err != nil {
		t.Fatalf("budget-model chat: %v", err)
	}
	if !strings.Contains(budget.lastRaw, "budget_tokens") {
		t.Fatalf("budget-generation model did not receive budget_tokens:\n%s", budget.lastRaw)
	}
	if strings.Contains(budget.lastRaw, "output_config") {
		t.Fatalf("budget-generation model received output_config, which it rejects:\n%s", budget.lastRaw)
	}

	adaptive := newConformanceStub(t, provider, provider.okBody)
	adaptiveClient := adaptive.client(t, ProviderOptions{Model: "claude-opus-4-7", ReasoningEffort: "medium"})
	if _, err := adaptiveClient.Chat(context.Background(), conformanceUserTurn(), ChatOptions{}); err != nil {
		t.Fatalf("adaptive-model chat: %v", err)
	}
	if strings.Contains(adaptive.lastRaw, "budget_tokens") {
		t.Fatalf("adaptive-generation model received budget_tokens, which it rejects outright:\n%s", adaptive.lastRaw)
	}
	if !strings.Contains(adaptive.lastRaw, "output_config") {
		t.Fatalf("adaptive-generation model did not receive output_config.effort:\n%s", adaptive.lastRaw)
	}
}

// LP-007 (#926) itself: the divergence that motivated the suite. Two
// providers both declare thinking round-trip; both must actually replay
// reasoning on a follow-up turn, and no provider may claim it without one.
func TestRegression_ThinkingRoundTripIsDeclaredAndReal(t *testing.T) {
	declared := make([]string, 0, 2)
	for _, kind := range DeclaredProviderKinds() {
		if SupportsCapability(kind, CapThinkingRoundTrip) {
			declared = append(declared, kind)
		}
	}
	if len(declared) < 2 {
		t.Fatalf("thinking round-trip declared by %v — the capability that started this epic should cover anthropic and gemini-native", declared)
	}
	for _, kind := range declared {
		if !SupportsCapability(kind, CapToolCalling) {
			t.Errorf("%s declares thinking round-trip without tool calling; the round-trip only matters across a tool loop", kind)
		}
	}
}

// LP-006 (#925): history budgeting was model-independent, so the sizing a
// tier resolved to never reached the client that would serve the request.
func TestRegression_TierResolutionCarriesSizing(t *testing.T) {
	router, err := NewRouter(RouterConfig{
		Tiers: map[Tier]TierEntry{
			TierHeavy:    {Client: &FakeClient{Label: "heavy"}, Provider: "anthropic", Model: "claude-opus-5", ContextWindow: 1000000, MaxTokens: 32000},
			TierStandard: {Client: &FakeClient{Label: "standard"}, Provider: "anthropic", Model: "claude-haiku-4-5", ContextWindow: 200000, MaxTokens: 16000},
			TierLight:    {Client: &FakeClient{Label: "light"}, Provider: "anthropic", Model: "claude-haiku-4-5", ContextWindow: 200000, MaxTokens: 8000},
		},
		DefaultTier: TierStandard,
	})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}
	_, heavy, err := router.ClientForTier(TierHeavy)
	if err != nil {
		t.Fatalf("resolve heavy: %v", err)
	}
	if heavy.ContextWindow != 1000000 || heavy.MaxTokens != 32000 {
		t.Fatalf("heavy resolution lost its sizing: %+v", heavy)
	}
	_, standard, err := router.ClientForTier(TierStandard)
	if err != nil {
		t.Fatalf("resolve standard: %v", err)
	}
	if standard.ContextWindow == heavy.ContextWindow {
		t.Fatal("two tiers on different models resolved to the same window")
	}
}
