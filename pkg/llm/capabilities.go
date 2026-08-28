package llm

import (
	"sort"
	"strings"

	zlog "github.com/rs/zerolog/log"
)

// Capability names one thing a provider client may or may not be able to do
// with a request. The set exists so that support is *declared* rather than
// discovered: an unsupported capability must produce a diagnostic, never a
// silent no-op.
//
// This is the mechanism the provider-modernization epic was opened over.
// reasoning_effort was accepted, validated, displayed in the console, and
// ignored by the Anthropic client for as long as that client existed,
// because nothing anywhere stated which providers honored it.
type Capability string

const (
	// CapToolCalling: the client can send tool schemas and parse tool calls
	// back out. CLI-backed providers execute tools themselves and report
	// them as observations, which is not this.
	CapToolCalling Capability = "tool_calling"

	// CapStreaming: ChatOptions.OnDelta produces incremental text.
	CapStreaming Capability = "streaming"

	// CapReasoningEffort: ChatOptions/ClientConfig ReasoningEffort changes
	// the request. How it is rendered is the provider's business.
	CapReasoningEffort Capability = "reasoning_effort"

	// CapThinkingRoundTrip: reasoning blocks survive a tool-calling loop —
	// parsed out of a response and replayed on the next request with
	// whatever signature the provider requires.
	CapThinkingRoundTrip Capability = "thinking_round_trip"

	// CapPromptCaching: the client places explicit cache breakpoints in the
	// request. Providers that cache implicitly do not have this; they may
	// still report cache usage.
	CapPromptCaching Capability = "prompt_caching"

	// CapCacheUsageReporting: Usage comes back with cache read/write counts
	// populated from the provider's own response shape.
	CapCacheUsageReporting Capability = "cache_usage_reporting"

	// CapJSONSchemaResponse: ChatOptions.ResponseFormat with a JSON schema
	// constrains the output.
	CapJSONSchemaResponse Capability = "json_schema_response"

	// CapServiceTier: ChatOptions/ClientConfig ServiceTier reaches the
	// request.
	CapServiceTier Capability = "service_tier"

	// CapSessionResume: the provider exposes a resumable upstream session,
	// so ResumeSessionID avoids replaying the transcript.
	CapSessionResume Capability = "session_resume"

	// CapMultimodalInput: ChatMessage.Blocks with images or documents reach
	// the request.
	CapMultimodalInput Capability = "multimodal_input"
)

// AllCapabilities lists every capability in a stable order, so the
// conformance suite and any report over it are deterministic.
func AllCapabilities() []Capability {
	return []Capability{
		CapToolCalling,
		CapStreaming,
		CapReasoningEffort,
		CapThinkingRoundTrip,
		CapPromptCaching,
		CapCacheUsageReporting,
		CapJSONSchemaResponse,
		CapServiceTier,
		CapSessionResume,
		CapMultimodalInput,
	}
}

// providerCapabilities declares what each provider kind supports.
//
// A kind missing from this map is a build-time omission, not a provider with
// no capabilities: TestEveryProviderKindDeclaresCapabilities fails the suite
// when NewProvider accepts a kind this map does not know, which is what stops
// a new provider from shipping with its support silently undeclared.
//
// Entries are asserted against real client behavior by the conformance suite
// wherever a stub transport can observe it, so a wrong declaration here is a
// failing test rather than documentation that drifts.
var providerCapabilities = map[string]map[Capability]bool{
	"anthropic": {
		CapToolCalling:         true,
		CapStreaming:           true,
		CapReasoningEffort:     true,
		CapThinkingRoundTrip:   true,
		CapPromptCaching:       true,
		CapCacheUsageReporting: true,
		CapMultimodalInput:     true,
	},
	"openai": {
		CapToolCalling:         true,
		CapStreaming:           true,
		CapReasoningEffort:     true,
		CapCacheUsageReporting: true,
		CapJSONSchemaResponse:  true,
		CapServiceTier:         true,
		CapMultimodalInput:     true,
	},
	// kimi honors both knobs, but drops them on tool-calling turns —
	// openai_compat_client.go excludes them when opts.Tools is non-empty,
	// because the Moonshot endpoint rejects the pair. Declared as supported:
	// the capability exists, with a provider-side condition, which is not
	// the same as never being implemented.
	"kimi": {
		CapToolCalling:         true,
		CapStreaming:           true,
		CapReasoningEffort:     true,
		CapCacheUsageReporting: true,
		CapJSONSchemaResponse:  true,
		CapServiceTier:         true,
		CapMultimodalInput:     true,
	},
	// gemini reaches Google's OpenAI-compatibility shim, which rejects
	// reasoning_effort and service_tier — openai_compat_client.go excludes
	// both for this label unconditionally. Setting either on a kind: gemini
	// tier has always been a no-op; declaring it false is what turns that
	// into a startup warning instead of silence. Use kind: gemini-native for
	// effort control on Gemini models.
	"gemini": {
		CapToolCalling:         true,
		CapStreaming:           true,
		CapCacheUsageReporting: true,
		CapJSONSchemaResponse:  true,
		CapMultimodalInput:     true,
	},
	"gemini-native": {
		CapToolCalling:       true,
		CapStreaming:         true,
		CapReasoningEffort:   true,
		CapThinkingRoundTrip: true,
		CapMultimodalInput:   true,
	},
	"openai-codex": {
		CapToolCalling:         true,
		CapStreaming:           true,
		CapCacheUsageReporting: true,
		CapJSONSchemaResponse:  true,
		CapMultimodalInput:     true,
	},
	// CLI-backed providers run their own tool loop against their own
	// permission policy. TARS observes what they executed; it does not
	// supply schemas or execute calls, so CapToolCalling is false.
	"claude-code-cli": {
		CapStreaming:           true,
		CapCacheUsageReporting: true,
		CapSessionResume:       true,
	},
	"antigravity-cli": {
		CapStreaming:           true,
		CapReasoningEffort:     true,
		CapCacheUsageReporting: true,
		CapJSONSchemaResponse:  true,
		CapSessionResume:       true,
	},
}

// SupportsCapability reports whether a provider kind declares support.
// An unknown kind reports false for everything — a gateway or a provider
// added without a declaration gets no promises made on its behalf.
func SupportsCapability(kind string, capability Capability) bool {
	declared, ok := providerCapabilities[normalizeProviderKind(kind)]
	if !ok {
		return false
	}
	return declared[capability]
}

// CapabilitiesFor returns the declared capabilities for a kind, sorted, plus
// whether the kind is declared at all.
func CapabilitiesFor(kind string) ([]Capability, bool) {
	declared, ok := providerCapabilities[normalizeProviderKind(kind)]
	if !ok {
		return nil, false
	}
	out := make([]Capability, 0, len(declared))
	for capability, supported := range declared {
		if supported {
			out = append(out, capability)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, true
}

// DeclaredProviderKinds lists every kind with a capability declaration.
func DeclaredProviderKinds() []string {
	out := make([]string, 0, len(providerCapabilities))
	for kind := range providerCapabilities {
		out = append(out, kind)
	}
	sort.Strings(out)
	return out
}

func normalizeProviderKind(kind string) string {
	return strings.ToLower(strings.TrimSpace(kind))
}

// unsupportedRequestedCapabilities reports which capabilities a request asks
// for that the provider does not declare.
//
// Only request-shaping capabilities are checked: a caller cannot "ask for"
// cache reporting or streaming support, it either gets them or does not.
func unsupportedRequestedCapabilities(kind string, opts ProviderOptions) []Capability {
	requested := make([]Capability, 0, 4)
	if strings.TrimSpace(opts.ReasoningEffort) != "" && normalizeReasoningEffort(opts.ReasoningEffort) != "" {
		requested = append(requested, CapReasoningEffort)
	}
	if strings.TrimSpace(opts.ServiceTier) != "" && normalizeServiceTier(opts.ServiceTier) != "" {
		requested = append(requested, CapServiceTier)
	}

	unsupported := make([]Capability, 0, len(requested))
	for _, capability := range requested {
		if !SupportsCapability(kind, capability) {
			unsupported = append(unsupported, capability)
		}
	}
	return unsupported
}

// reportUnsupportedCapabilities logs one warning per capability a request
// asks for that the provider cannot honor.
//
// It fires at construction rather than per request: a tier's settings are
// fixed for the life of its client, so one line at startup says what will be
// ignored for every subsequent turn without repeating it on each one.
//
// This is the whole point of the capability matrix. Before it, a setting that
// a provider did not implement was accepted, validated, shown in the console,
// and silently dropped — which is strictly worse than being rejected.
func reportUnsupportedCapabilities(kind string, opts ProviderOptions) {
	for _, capability := range unsupportedRequestedCapabilities(kind, opts) {
		zlog.Warn().
			Str("provider", kind).
			Str("capability", string(capability)).
			Str("model", strings.TrimSpace(opts.Model)).
			Msg("requested setting is not supported by this provider and will be ignored")
	}
}
