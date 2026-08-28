package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// This is the cross-provider conformance suite (LP-007).
//
// Provider tests used to be per-file and per-provider, each covering its own
// client on its own terms, and nothing asserted that the set was uniform.
// That is how the Anthropic gaps in this epic went unnoticed for so long:
// gemini-native learned to round-trip reasoning signatures while anthropic
// did not, and no test could observe the divergence.
//
// The goal is not that every provider supports everything — CLI-backed ones
// legitimately cannot. The goal is that support is *declared* in
// capabilities.go and *verified* here, so an unsupported capability is a
// documented false rather than a silent no-op.
//
// Every scenario runs against a stub HTTP transport. No network, no
// subprocess, no shell — the suite must stay Windows-clean, which is why the
// CLI-backed providers are covered by declaration checks rather than by
// driving their binaries.

// conformanceProvider is one HTTP-backed provider the scenario table runs
// against, with the knobs each needs to be constructed and observed.
type conformanceProvider struct {
	kind string

	// chatPath is the request path the client posts a chat turn to. The stub
	// answers anything else with the provider's preflight/discovery shape.
	chatPath string

	// okBody is a minimal successful response in this provider's own shape.
	okBody string

	// toolCallBody is a response containing exactly one tool call named
	// "lookup", with argument {"q":"x"}.
	toolCallBody string

	// usageBody is a response whose usage block reports 100 input, 20
	// output, and 60 cache-read tokens in this provider's own shape.
	usageBody string

	// baseURLSuffix is appended to the stub server URL to form BaseURL.
	baseURLSuffix string

	// model is a model this provider recognizes.
	model string
}

func conformanceProviders() []conformanceProvider {
	openAIOK := `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`
	openAITool := `{"choices":[{"message":{"content":"","tool_calls":[{"id":"call_1","function":{"name":"lookup","arguments":"{\"q\":\"x\"}"}}]},"finish_reason":"tool_calls"}]}`
	openAIUsage := `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":20,"prompt_tokens_details":{"cached_tokens":60}}}`

	return []conformanceProvider{
		{
			kind:         "anthropic",
			chatPath:     "/v1/messages",
			okBody:       `{"content":[{"type":"text","text":"ok"}]}`,
			toolCallBody: `{"content":[{"type":"tool_use","id":"call_1","name":"lookup","input":{"q":"x"}}],"stop_reason":"tool_use"}`,
			usageBody:    `{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":100,"output_tokens":20,"cache_read_input_tokens":60}}`,
			model:        "claude-haiku-4-5",
		},
		{
			kind:          "openai",
			chatPath:      "/chat/completions",
			okBody:        openAIOK,
			toolCallBody:  openAITool,
			usageBody:     openAIUsage,
			baseURLSuffix: "/v1",
			model:         "gpt-4.1-mini",
		},
		{
			kind:          "kimi",
			chatPath:      "/chat/completions",
			okBody:        openAIOK,
			toolCallBody:  openAITool,
			usageBody:     openAIUsage,
			baseURLSuffix: "/v1",
			model:         "moonshot-v1-8k",
		},
		{
			kind:          "gemini",
			chatPath:      "/chat/completions",
			okBody:        openAIOK,
			toolCallBody:  openAITool,
			usageBody:     openAIUsage,
			baseURLSuffix: "/v1beta/openai",
			model:         "gemini-2.5-flash",
		},
		{
			kind:          "gemini-native",
			chatPath:      ":generateContent",
			okBody:        `{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`,
			toolCallBody:  `{"candidates":[{"content":{"parts":[{"functionCall":{"name":"lookup","args":{"q":"x"}}}]},"finishReason":"STOP"}]}`,
			usageBody:     `{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":20}}`,
			baseURLSuffix: "/v1beta",
			model:         "gemini-2.5-flash",
		},
	}
}

// conformanceStub serves one canned body for chat turns and a permissive
// discovery response for anything else, and records the last chat request.
type conformanceStub struct {
	server   *httptest.Server
	provider conformanceProvider

	lastBody   map[string]any
	lastRaw    string
	lastPath   string
	chatCalls  int
	respondErr int
}

func newConformanceStub(t *testing.T, provider conformanceProvider, body string) *conformanceStub {
	t.Helper()
	stub := &conformanceStub{provider: provider}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// gemini-native performs a GET model preflight before its first
		// generateContent call; answer it without counting a chat turn.
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"models/gemini-2.5-flash","supportedGenerationMethods":["generateContent","streamGenerateContent"]}`))
			return
		}
		raw, _ := io.ReadAll(r.Body)
		stub.lastRaw = string(raw)
		stub.lastPath = r.URL.Path
		stub.lastBody = nil
		_ = json.Unmarshal(raw, &stub.lastBody)
		stub.chatCalls++
		if stub.respondErr != 0 {
			w.WriteHeader(stub.respondErr)
			_, _ = w.Write([]byte(`{"error":"stub failure"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

func (s *conformanceStub) client(t *testing.T, opts ProviderOptions) Client {
	t.Helper()
	opts.Provider = s.provider.kind
	opts.BaseURL = s.server.URL + s.provider.baseURLSuffix
	opts.APIKey = "test-key"
	if strings.TrimSpace(opts.Model) == "" {
		opts.Model = s.provider.model
	}
	client, err := NewProvider(opts)
	if err != nil {
		t.Fatalf("%s: new provider: %v", s.provider.kind, err)
	}
	return client
}

func conformanceUserTurn() []ChatMessage {
	return []ChatMessage{{Role: "user", Content: "hello"}}
}

// ---------------------------------------------------------------------------
// Declaration integrity — these are what stop a provider from shipping with
// its support undeclared.
// ---------------------------------------------------------------------------

// TestEveryProviderKindDeclaresCapabilities fails when NewProvider accepts a
// kind that capabilities.go does not describe. Adding a provider without
// declaring what it supports therefore fails the suite, which is the
// acceptance criterion for this issue.
func TestEveryProviderKindDeclaresCapabilities(t *testing.T) {
	// Every kind NewProvider routes to. Kept explicit rather than derived,
	// so that adding a case to that switch without touching this list is
	// itself the failure.
	constructible := []string{
		"anthropic",
		"openai",
		"kimi",
		"gemini",
		"gemini-native",
		"openai-codex",
		"claude-code-cli",
		"antigravity-cli",
	}
	for _, kind := range constructible {
		if _, declared := CapabilitiesFor(kind); !declared {
			t.Errorf("provider %q is constructible but declares no capabilities — add it to providerCapabilities", kind)
		}
	}

	// And the reverse: a declaration for a kind that cannot be built is a
	// promise about something that does not exist.
	known := make(map[string]bool, len(constructible))
	for _, kind := range constructible {
		known[kind] = true
	}
	for _, kind := range DeclaredProviderKinds() {
		if !known[kind] {
			t.Errorf("provider %q declares capabilities but NewProvider cannot build it", kind)
		}
	}
}

func TestNewProviderRejectsUnknownKind(t *testing.T) {
	if _, err := NewProvider(ProviderOptions{Provider: "not-a-provider", APIKey: "k", BaseURL: "http://example.invalid"}); err == nil {
		t.Fatal("expected an error for an unknown provider kind")
	}
}

func TestSupportsCapability_UnknownKindPromisesNothing(t *testing.T) {
	for _, capability := range AllCapabilities() {
		if SupportsCapability("not-a-provider", capability) {
			t.Errorf("unknown kind claims %s", capability)
		}
	}
}

// TestUnsupportedRequestedCapabilitiesIsObservable covers the rule this epic
// exists for: asking for something a provider cannot do must be visible.
func TestUnsupportedRequestedCapabilitiesIsObservable(t *testing.T) {
	cases := []struct {
		name string
		kind string
		opts ProviderOptions
		want []Capability
	}{
		{
			// The original defect: service_tier is an openai-family knob,
			// and anthropic has no equivalent.
			name: "service tier on anthropic is reported",
			kind: "anthropic",
			opts: ProviderOptions{ServiceTier: "priority"},
			want: []Capability{CapServiceTier},
		},
		{
			name: "reasoning effort on claude-code-cli is reported",
			kind: "claude-code-cli",
			opts: ProviderOptions{ReasoningEffort: "high"},
			want: []Capability{CapReasoningEffort},
		},
		{
			name: "reasoning effort on openai-codex is reported",
			kind: "openai-codex",
			opts: ProviderOptions{ReasoningEffort: "high"},
			want: []Capability{CapReasoningEffort},
		},
		{
			name: "both knobs unsupported are both reported",
			kind: "claude-code-cli",
			opts: ProviderOptions{ReasoningEffort: "high", ServiceTier: "priority"},
			want: []Capability{CapReasoningEffort, CapServiceTier},
		},
		{
			name: "supported knobs report nothing",
			kind: "openai",
			opts: ProviderOptions{ReasoningEffort: "high", ServiceTier: "priority"},
			want: nil,
		},
		{
			name: "anthropic reasoning effort is supported and reports nothing",
			kind: "anthropic",
			opts: ProviderOptions{ReasoningEffort: "high"},
			want: nil,
		},
		{
			name: "an unset knob is not a request",
			kind: "claude-code-cli",
			opts: ProviderOptions{},
			want: nil,
		},
		{
			// A value that normalizes away was never a coherent request,
			// so it must not produce a spurious diagnostic.
			name: "an unrecognized value is not a request",
			kind: "claude-code-cli",
			opts: ProviderOptions{ReasoningEffort: "banana", ServiceTier: "banana"},
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := unsupportedRequestedCapabilities(tc.kind, tc.opts)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// The shared scenario table. Every HTTP-backed provider runs all of it.
// ---------------------------------------------------------------------------

func TestConformance_BasicChatTurn(t *testing.T) {
	for _, provider := range conformanceProviders() {
		t.Run(provider.kind, func(t *testing.T) {
			stub := newConformanceStub(t, provider, provider.okBody)
			client := stub.client(t, ProviderOptions{})

			resp, err := client.Chat(context.Background(), conformanceUserTurn(), ChatOptions{})
			if err != nil {
				t.Fatalf("chat: %v", err)
			}
			if strings.TrimSpace(resp.Message.Content) != "ok" {
				t.Fatalf("content = %q, want ok", resp.Message.Content)
			}
			if stub.chatCalls != 1 {
				t.Fatalf("chat calls = %d, want 1", stub.chatCalls)
			}
		})
	}
}

func TestConformance_ToolCallRoundTrip(t *testing.T) {
	tools := []ToolSchema{{
		Type: "function",
		Function: ToolFunctionSchema{
			Name:        "lookup",
			Description: "look something up",
			Parameters:  []byte(`{"type":"object","properties":{"q":{"type":"string"}}}`),
		},
	}}

	for _, provider := range conformanceProviders() {
		t.Run(provider.kind, func(t *testing.T) {
			if !SupportsCapability(provider.kind, CapToolCalling) {
				t.Skipf("%s does not declare tool calling", provider.kind)
			}
			stub := newConformanceStub(t, provider, provider.toolCallBody)
			client := stub.client(t, ProviderOptions{})

			resp, err := client.Chat(context.Background(), conformanceUserTurn(), ChatOptions{Tools: tools})
			if err != nil {
				t.Fatalf("chat: %v", err)
			}
			if len(resp.Message.ToolCalls) != 1 {
				t.Fatalf("tool calls = %d, want 1 (%+v)", len(resp.Message.ToolCalls), resp.Message)
			}
			call := resp.Message.ToolCalls[0]
			if call.Name != "lookup" {
				t.Errorf("tool name = %q, want lookup", call.Name)
			}
			// Arguments must be parseable JSON on every provider, whatever
			// shape the wire used.
			var args map[string]any
			if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
				t.Fatalf("tool arguments are not JSON: %q (%v)", call.Arguments, err)
			}
			if args["q"] != "x" {
				t.Errorf("args = %v, want q=x", args)
			}
			// The schema must have reached the request, not just been
			// accepted by the client.
			if !strings.Contains(stub.lastRaw, "lookup") {
				t.Errorf("tool schema did not reach the request: %s", stub.lastRaw)
			}

			// Replay the assistant turn plus a tool result. Every provider
			// must accept its own parsed output back as input.
			followUp := append(conformanceUserTurn(), resp.Message, ChatMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    "42",
			})
			if _, err := client.Chat(context.Background(), followUp, ChatOptions{Tools: tools}); err != nil {
				t.Fatalf("tool result follow-up: %v", err)
			}
		})
	}
}

func TestConformance_UsageIsPopulatedFromEachProviderShape(t *testing.T) {
	for _, provider := range conformanceProviders() {
		t.Run(provider.kind, func(t *testing.T) {
			stub := newConformanceStub(t, provider, provider.usageBody)
			client := stub.client(t, ProviderOptions{})

			resp, err := client.Chat(context.Background(), conformanceUserTurn(), ChatOptions{})
			if err != nil {
				t.Fatalf("chat: %v", err)
			}
			if resp.Usage.InputTokens != 100 {
				t.Errorf("InputTokens = %d, want 100", resp.Usage.InputTokens)
			}
			if resp.Usage.OutputTokens != 20 {
				t.Errorf("OutputTokens = %d, want 20", resp.Usage.OutputTokens)
			}
			if !SupportsCapability(provider.kind, CapCacheUsageReporting) {
				return
			}
			if resp.Usage.CachedTokens == 0 && resp.Usage.CacheReadTokens == 0 {
				t.Errorf("%s declares cache usage reporting but reported none: %+v", provider.kind, resp.Usage)
			}
		})
	}
}

func TestConformance_ReasoningEffortReachesTheRequest(t *testing.T) {
	for _, provider := range conformanceProviders() {
		t.Run(provider.kind, func(t *testing.T) {
			if !SupportsCapability(provider.kind, CapReasoningEffort) {
				t.Skipf("%s does not declare reasoning effort", provider.kind)
			}
			// Baseline with no effort, then with effort: the request must
			// differ. Asserting the exact rendering would just restate each
			// provider's own conversion code; asserting that it changes
			// something is what catches a silent no-op, which is the defect
			// this suite exists to observe.
			plain := newConformanceStub(t, provider, provider.okBody)
			plainClient := plain.client(t, ProviderOptions{})
			if _, err := plainClient.Chat(context.Background(), conformanceUserTurn(), ChatOptions{}); err != nil {
				t.Fatalf("baseline chat: %v", err)
			}

			effort := newConformanceStub(t, provider, provider.okBody)
			effortClient := effort.client(t, ProviderOptions{ReasoningEffort: "high"})
			if _, err := effortClient.Chat(context.Background(), conformanceUserTurn(), ChatOptions{}); err != nil {
				t.Fatalf("effort chat: %v", err)
			}

			if plain.lastRaw == effort.lastRaw {
				t.Fatalf("%s declares reasoning_effort support but the request is byte-identical with and without it:\n%s", provider.kind, plain.lastRaw)
			}
		})
	}
}

func TestConformance_ServiceTierReachesTheRequestOnlyWhereDeclared(t *testing.T) {
	for _, provider := range conformanceProviders() {
		t.Run(provider.kind, func(t *testing.T) {
			plain := newConformanceStub(t, provider, provider.okBody)
			plainClient := plain.client(t, ProviderOptions{})
			if _, err := plainClient.Chat(context.Background(), conformanceUserTurn(), ChatOptions{}); err != nil {
				t.Fatalf("baseline chat: %v", err)
			}

			tiered := newConformanceStub(t, provider, provider.okBody)
			tieredClient := tiered.client(t, ProviderOptions{ServiceTier: "priority"})
			if _, err := tieredClient.Chat(context.Background(), conformanceUserTurn(), ChatOptions{}); err != nil {
				t.Fatalf("service tier chat: %v", err)
			}

			changed := plain.lastRaw != tiered.lastRaw
			declared := SupportsCapability(provider.kind, CapServiceTier)
			if declared && !changed {
				t.Fatalf("%s declares service_tier support but the request is unchanged", provider.kind)
			}
			if !declared && changed {
				t.Fatalf("%s does not declare service_tier but the request changed — update the capability matrix", provider.kind)
			}
		})
	}
}

func TestConformance_MultimodalInputReachesTheRequest(t *testing.T) {
	for _, provider := range conformanceProviders() {
		t.Run(provider.kind, func(t *testing.T) {
			if !SupportsCapability(provider.kind, CapMultimodalInput) {
				t.Skipf("%s does not declare multimodal input", provider.kind)
			}
			stub := newConformanceStub(t, provider, provider.okBody)
			client := stub.client(t, ProviderOptions{})

			messages := []ChatMessage{{
				Role: "user",
				ContentBlocks: []ContentBlock{
					{Type: "text", Text: "what is this"},
					{Type: "image", MediaType: "image/png", Data: "aGVsbG8="},
				},
			}}
			if _, err := client.Chat(context.Background(), messages, ChatOptions{}); err != nil {
				t.Fatalf("chat: %v", err)
			}
			if !strings.Contains(stub.lastRaw, "aGVsbG8=") {
				t.Fatalf("%s declares multimodal input but the image payload did not reach the request:\n%s", provider.kind, stub.lastRaw)
			}
		})
	}
}

func TestConformance_ErrorsCarryProviderContext(t *testing.T) {
	// A failed turn must say which provider failed. Without it, a multi-tier
	// deployment cannot tell which binding to fix.
	for _, provider := range conformanceProviders() {
		t.Run(provider.kind, func(t *testing.T) {
			stub := newConformanceStub(t, provider, provider.okBody)
			stub.respondErr = http.StatusUnauthorized
			client := stub.client(t, ProviderOptions{})

			_, err := client.Chat(context.Background(), conformanceUserTurn(), ChatOptions{})
			if err == nil {
				t.Fatal("expected an error from a 401")
			}
			if !strings.Contains(strings.ToLower(err.Error()), provider.kind) {
				t.Errorf("error does not name the provider %q: %v", provider.kind, err)
			}
		})
	}
}

func TestConformance_ContextCancellationIsHonored(t *testing.T) {
	for _, provider := range conformanceProviders() {
		t.Run(provider.kind, func(t *testing.T) {
			stub := newConformanceStub(t, provider, provider.okBody)
			client := stub.client(t, ProviderOptions{})

			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err := client.Chat(ctx, conformanceUserTurn(), ChatOptions{}); err == nil {
				t.Fatal("expected an error from a cancelled context")
			}
		})
	}
}

// TestConformance_DefaultPathMakesNoNetworkCalls is a guard on the suite
// itself: every stub above is an httptest server on loopback, and nothing
// here may reach a real provider. A base URL pointing anywhere else would
// make `make test` depend on credentials and connectivity.
func TestConformance_DefaultPathMakesNoNetworkCalls(t *testing.T) {
	for _, provider := range conformanceProviders() {
		stub := newConformanceStub(t, provider, provider.okBody)
		if !strings.HasPrefix(stub.server.URL, "http://127.0.0.1:") {
			t.Errorf("%s stub is not on loopback: %s", provider.kind, stub.server.URL)
		}
	}
}
