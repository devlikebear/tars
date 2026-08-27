package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/devlikebear/tars/internal/llmdefaults"
	zlog "github.com/rs/zerolog/log"
)

const anthropicAPIVersion = "2023-06-01"

// anthropicFallbackMaxTokens is the last-resort output ceiling for a model
// llmdefaults does not recognize — gateway-hosted models reached over an
// Anthropic-compatible endpoint, mostly. It is deliberately conservative:
// a too-low ceiling truncates a response, but a too-high one is rejected
// outright by the provider.
const anthropicFallbackMaxTokens = 4096

// anthropicDefaultMaxTokensCeiling caps what a tier gets *by default*, as
// distinct from what it may ask for.
//
// The current families document a 128000 ceiling, but this client shares one
// 30-second HTTPTimeout (http_utils.go) across streaming and non-streaming
// calls, and Ask/askFromSinglePrompt — `tars --message`, the compactor, pulse,
// the memory hooks — never streams. Handing those paths a 128000 ceiling
// silently converts a clean truncation into a timeout on any genuinely long
// generation. Anthropic's own guidance puts non-streaming requests around
// 16000 for the same reason, against a far more generous SDK timeout.
//
// A tier that wants the model's full ceiling states max_tokens explicitly —
// which is the point of making the field configurable.
const anthropicDefaultMaxTokensCeiling = 16000

// resolveAnthropicMaxTokens picks the effective output ceiling for a request.
//
// An explicit setting always wins, uncapped: it is the only way to reach a
// limit TARS does not know about, and the operator asking for one has
// accepted its consequences. An unset tier takes the model's documented
// ceiling, capped for the reason above; a model with no documented ceiling
// falls back further still.
//
// Both construction paths (NewProvider and NewAnthropicClient) route through
// here so the two cannot drift: external consumers of pkg/llm get the same
// defaults as the TARS router.
func resolveAnthropicMaxTokens(model string, configured int) int {
	if configured > 0 {
		return configured
	}
	if documented := llmdefaults.MaxOutputTokens(model); documented > 0 {
		return min(documented, anthropicDefaultMaxTokensCeiling)
	}
	zlog.Debug().
		Str("provider", "anthropic").
		Str("model", model).
		Int("max_tokens", anthropicFallbackMaxTokens).
		Msg("no documented output ceiling for model; using fallback — set max_tokens on the tier to raise it")
	return anthropicFallbackMaxTokens
}

// anthropicBetaHeader joins the tier's beta flags into one anthropic-beta
// header value, or returns "" when the request should carry no header.
//
// The default set is empty on purpose. Prompt caching went GA, so the
// beta flag this client used to send unconditionally did nothing except
// occupy the only header slot — and it was sent to third-party
// Anthropic-compatible gateways that never asked for it.
func anthropicBetaHeader(features []string) string {
	cleaned := make([]string, 0, len(features))
	for _, feature := range features {
		if trimmed := strings.TrimSpace(feature); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return strings.Join(cleaned, ",")
}

const (
	// anthropicMaxCacheBreakpoints is the provider-wide limit on cache_control
	// markers per request, across system blocks, tool definitions, and
	// messages.
	anthropicMaxCacheBreakpoints = 4
	// anthropicMaxMessageCacheBreakpoints caps how many of the remaining
	// slots this client spends on the message array. Two are enough for a
	// rolling window: the newest completed turn plus one older fallback.
	anthropicMaxMessageCacheBreakpoints = 2
)

const (
	// anthropicMinThinkingBudget is the provider's floor for
	// thinking.budget_tokens. A request below it is rejected outright.
	anthropicMinThinkingBudget = 1024
	// anthropicThinkingOutputHeadroom is how much of max_tokens is held back
	// for the visible answer. budget_tokens must be strictly below
	// max_tokens, and a budget that eats all of it leaves the model no room
	// to reply, so derived budgets are clamped to max_tokens minus this.
	anthropicThinkingOutputHeadroom = 1024
)

// anthropicThinkingBudgetForEffort maps the provider-agnostic
// reasoning_effort levels onto Anthropic thinking budgets. The Messages API
// exposes no native effort control — extended thinking is budgeted in tokens —
// so the levels are rendered as budgets here:
//
//	effort   budget_tokens
//	none     0 (thinking stays off)
//	minimal  1024 (the provider floor)
//	low      2048
//	medium   8192
//	high     16384
//
// The result is a request, not a guarantee: buildAnthropicThinking clamps it
// against max_tokens, which is pinned at 4096 until LP-004 (#923) makes it
// tier-configurable. At that cap everything above minimal lands on 3072.
func anthropicThinkingBudgetForEffort(effort string) int {
	switch effort {
	case "minimal":
		return anthropicMinThinkingBudget
	case "low":
		return 2048
	case "medium":
		return 8192
	case "high":
		return 16384
	default: // "" and "none"
		return 0
	}
}

// anthropicEffortForLevel maps TARS's provider-agnostic reasoning_effort onto
// Anthropic's output_config.effort ladder.
//
// Anthropic's lowest band is "low", so TARS's "minimal" and "low" both land
// there — the distinction cannot be expressed. TARS normalizes "xhigh" and
// "veryhigh" down to "high" (provider.go), so the upper bands "xhigh" and
// "max" are unreachable from config today; widening that is a separate
// change to the shared effort vocabulary, not to this renderer.
func anthropicEffortForLevel(effort string) string {
	switch effort {
	case "minimal", "low":
		return "low"
	case "medium":
		return "medium"
	case "high":
		return "high"
	default:
		return ""
	}
}

// anthropicEffortForThinkingBudget approximates an effort band from a token
// budget, for callers that set thinking_budget against a model that has no
// budget knob. It is the inverse of anthropicThinkingBudgetForEffort, rounded
// up to the band whose budget covers the request.
//
// TARS configs never reach this: ResolveLLMTier rejects a thinking_budget on
// an adaptive-thinking model outright. It exists for callers that build a
// provider directly through pkg/llm and would otherwise ship a 400.
func anthropicEffortForThinkingBudget(budget int) string {
	switch {
	case budget <= 2048:
		return "low"
	case budget <= 8192:
		return "medium"
	default:
		return "high"
	}
}

// anthropicReasoning is the pair of request fields that carry reasoning
// configuration. Either may be nil; which one is populated depends on the
// model's ThinkingMode, and populating the wrong one is a 400.
type anthropicReasoning struct {
	Thinking     map[string]any
	OutputConfig map[string]any
}

// buildAnthropicReasoning renders reasoning_effort / thinking_budget into the
// shape the target model accepts.
//
// The two generations are mutually exclusive. Budget-mode models take a token
// budget and reject output_config.effort — including Haiku 4.5, this client's
// default model — so effort must never leak onto that path. Adaptive-mode
// models take the opposite: an effort band, with budget_tokens rejected.
//
// display: "summarized" is set on the adaptive path deliberately. Those models
// default to omitting reasoning text, which would silently blank the console's
// reasoning stream (handler_chat_execution.go consumes OnReasoningDelta) for
// anyone moving from a budget-mode model.
func buildAnthropicReasoning(model string, config ClientConfig, opts ChatOptions, maxTokens int) anthropicReasoning {
	behavior, _ := llmdefaults.ModelBehaviorFor(model)
	if behavior.Thinking != llmdefaults.ThinkingModeAdaptive {
		return anthropicReasoning{Thinking: buildAnthropicThinking(config, opts, maxTokens)}
	}

	effort := effectiveReasoningEffort(config, opts)
	budget := effectiveThinkingBudget(config, opts)

	if effort == "none" {
		if !behavior.CanDisableThinking {
			zlog.Warn().
				Str("provider", "anthropic").
				Str("model", model).
				Msg("reasoning_effort=none not honored: this model thinks unconditionally and rejects an explicit disable")
			return anthropicReasoning{}
		}
		// Honored as asked. Disabling thinking on these models has two
		// documented failure modes that bite hardest in a tool loop: the
		// model may write a tool call into its visible text — the turn
		// succeeds, the call never runs, nothing errors — and it may leak
		// <thinking> tags into the response. Lowering effort is the better
		// lever, but silently substituting it for the operator's explicit
		// "none" is the kind of non-honoring this client is being cured of.
		if len(opts.Tools) > 0 {
			zlog.Warn().
				Str("provider", "anthropic").
				Str("model", model).
				Msg("thinking disabled on a tool-calling request: this model may emit tool calls as plain text instead of tool_use blocks — prefer reasoning_effort=minimal")
		}
		// No output_config: disabling is accepted only at effort high or
		// below, and omitting the field defaults to high.
		return anthropicReasoning{Thinking: map[string]any{"type": "disabled"}}
	}

	if budget > 0 {
		zlog.Warn().
			Str("provider", "anthropic").
			Str("model", model).
			Int("thinking_budget", budget).
			Msg("thinking_budget not honored: this model sets reasoning depth by effort, not by token budget — approximating with an effort level")
	}

	level := anthropicEffortForLevel(effort)
	if level == "" && budget > 0 {
		level = anthropicEffortForThinkingBudget(budget)
	}
	if level == "" {
		// Neither knob set: leave both fields off and take the model's own
		// defaults rather than asserting one.
		return anthropicReasoning{}
	}

	zlog.Debug().
		Str("provider", "anthropic").
		Str("model", model).
		Str("reasoning_effort", effort).
		Str("anthropic_effort", level).
		Msg("anthropic adaptive thinking enabled")
	return anthropicReasoning{
		Thinking:     map[string]any{"type": "adaptive", "display": "summarized"},
		OutputConfig: map[string]any{"effort": level},
	}
}

// buildAnthropicThinking resolves the thinking block for a budget-mode model,
// or nil when extended thinking stays off. Adaptive-mode models do not come
// through here — see buildAnthropicReasoning.
//
// An explicit thinking_budget is the more specific knob and outranks
// reasoning_effort; only when none is set does the effort level derive one.
// Whatever the source, the budget is clamped into the range the provider will
// accept: at least anthropicMinThinkingBudget, and low enough to leave
// anthropicThinkingOutputHeadroom of max_tokens for the answer. When those two
// cannot both hold, thinking degrades off with a warning rather than shipping
// a request the provider rejects.
func buildAnthropicThinking(config ClientConfig, opts ChatOptions, maxTokens int) map[string]any {
	source := "thinking_budget"
	budget := effectiveThinkingBudget(config, opts)
	effort := effectiveReasoningEffort(config, opts)
	if budget <= 0 {
		source = "reasoning_effort"
		budget = anthropicThinkingBudgetForEffort(effort)
	}
	if budget <= 0 {
		return nil
	}

	ceiling := maxTokens - anthropicThinkingOutputHeadroom
	if ceiling < anthropicMinThinkingBudget {
		zlog.Warn().
			Str("provider", "anthropic").
			Str("reasoning_effort", effort).
			Int("requested_budget", budget).
			Int("max_tokens", maxTokens).
			Msg("thinking disabled: max_tokens leaves no room for a thinking budget")
		return nil
	}
	if budget > ceiling {
		budget = ceiling
	}
	if budget < anthropicMinThinkingBudget {
		budget = anthropicMinThinkingBudget
	}
	zlog.Debug().
		Str("provider", "anthropic").
		Str("budget_source", source).
		Str("reasoning_effort", effort).
		Int("thinking_budget", budget).
		Int("max_tokens", maxTokens).
		Msg("anthropic extended thinking enabled")
	return map[string]any{
		"type":          "enabled",
		"budget_tokens": budget,
	}
}

// anthropicEphemeralCacheControl builds one cache_control marker. Every
// breakpoint this client emits — system block, tool definition, message —
// goes through here so they cannot drift apart.
//
// It must return a fresh map per call: callers store the result into blocks
// they do not own, and a shared map would let one edit rewrite every
// breakpoint in the request.
func anthropicEphemeralCacheControl() map[string]any {
	return map[string]any{"type": "ephemeral"}
}

// anthropicMessageCacheBudget returns how many cache_control markers the
// message array may carry, given how many the request already spends
// elsewhere. The provider counts system blocks and tool definitions against
// the same per-request limit, so whatever they reserve is unavailable here.
//
// The message array is additionally capped at
// anthropicMaxMessageCacheBreakpoints: a rolling window needs two, and extra
// markers would just buy more cache writes at 1.25x without extending the
// cached prefix any further.
func anthropicMessageCacheBudget(reserved int) int {
	budget := anthropicMaxCacheBreakpoints - reserved
	if budget > anthropicMaxMessageCacheBreakpoints {
		budget = anthropicMaxMessageCacheBreakpoints
	}
	if budget < 0 {
		budget = 0
	}
	return budget
}

type AnthropicClient struct {
	baseURL    string
	apiKey     string
	model      string
	config     ClientConfig
	httpClient *http.Client
}

func NewAnthropicClient(baseURL, apiKey, model string, maxTokens int) (*AnthropicClient, error) {
	config := DefaultClientConfig()
	if _, err := requireConfiguredValue("anthropic", "base url", baseURL); err != nil {
		return nil, err
	}
	if _, err := requireConfiguredValue("anthropic", "api key", apiKey); err != nil {
		return nil, err
	}
	if _, err := requireConfiguredValue("anthropic", "model", model); err != nil {
		return nil, err
	}

	config.MaxTokens = resolveAnthropicMaxTokens(model, maxTokens)

	return newAnthropicClientWithConfig(baseURL, apiKey, model, config)
}

func newAnthropicClientWithConfig(baseURL, apiKey, model string, config ClientConfig) (*AnthropicClient, error) {
	return &AnthropicClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
		config:     config,
		httpClient: newHTTPClient(config.HTTPTimeout),
	}, nil
}

func (c *AnthropicClient) Chat(ctx context.Context, messages []ChatMessage, opts ChatOptions) (ChatResponse, error) {
	streaming := opts.OnDelta != nil
	url := c.baseURL + "/v1/messages"
	logChatRequestStart("anthropic", c.model, url, len(messages), streaming, len(opts.Tools), opts.ToolChoice.String())

	reqBody := c.buildChatRequest(messages, opts, streaming)
	headers := map[string]string{
		"x-api-key":         c.apiKey,
		"anthropic-version": anthropicAPIVersion,
		"content-type":      "application/json",
	}
	// Absent, not empty: an empty anthropic-beta is still a header the
	// gateway has to interpret.
	if beta := anthropicBetaHeader(c.config.BetaFeatures); beta != "" {
		headers["anthropic-beta"] = beta
	}
	_, resp, err := executeJSONChatRequest(ctx, jsonRequestSpec{
		Provider: "anthropic",
		URL:      url,
		Headers:  headers,
		Body:     reqBody,
	}, c.httpClient, streaming)
	if err != nil {
		return ChatResponse{}, err
	}
	defer resp.Body.Close()

	if streaming {
		return c.chatStreamingResponse(resp.Body, opts.OnDelta, opts.OnReasoningDelta)
	}
	return c.chatNonStreamingResponse(resp.Body)
}

func (c *AnthropicClient) Ask(ctx context.Context, prompt string) (string, error) {
	return askFromSinglePrompt(ctx, c.Chat, prompt)
}

func (c *AnthropicClient) buildChatRequest(messages []ChatMessage, opts ChatOptions, streaming bool) map[string]any {
	nonSystemMessages := make([]ChatMessage, 0, len(messages))
	systemMessages := make([]string, 0)
	for _, msg := range messages {
		if msg.Role == "system" {
			if strings.TrimSpace(msg.Content) != "" {
				systemMessages = append(systemMessages, strings.TrimSpace(msg.Content))
			}
			continue
		}
		nonSystemMessages = append(nonSystemMessages, msg)
	}

	tools := toAnthropicTools(opts.Tools)
	wireMessages := toAnthropicWireMessages(nonSystemMessages)
	// toAnthropicSystemBlocks marks one block and the tool array marks its
	// last entry, so each contributes at most one breakpoint to the
	// provider-wide limit.
	reservedBreakpoints := 0
	if len(systemMessages) > 0 {
		reservedBreakpoints++
	}
	if len(tools) > 0 {
		reservedBreakpoints++
	}
	applyAnthropicRollingCacheBreakpoints(wireMessages, nonSystemMessages, reservedBreakpoints)

	reqBody := map[string]any{
		"model":      c.model,
		"max_tokens": c.config.MaxTokens,
		"messages":   wireMessages,
	}
	reasoning := buildAnthropicReasoning(c.model, c.config, opts, c.config.MaxTokens)
	if reasoning.Thinking != nil {
		reqBody["thinking"] = reasoning.Thinking
	}
	if reasoning.OutputConfig != nil {
		reqBody["output_config"] = reasoning.OutputConfig
	}
	if len(systemMessages) > 0 {
		reqBody["system"] = toAnthropicSystemBlocks(systemMessages)
	}
	if len(tools) > 0 {
		tools[len(tools)-1].CacheControl = anthropicEphemeralCacheControl()
		reqBody["tools"] = tools
		if choice := toAnthropicToolChoice(opts.ToolChoice); len(choice) > 0 {
			reqBody["tool_choice"] = choice
		}
	}
	if streaming {
		reqBody["stream"] = true
	}
	return reqBody
}

// toAnthropicSystemBlocks renders the collected system messages as one text
// block each and marks the cacheable prefix.
//
// The breakpoint goes on the FIRST block. Callers order their system messages
// stable-first, volatile-last (see prompt.BuildResultFor's ordering
// invariant), so the first block is the turn-stable region and everything
// after it — per-turn recall, the clock, drained critic feedback — stays
// outside the cached prefix. Anthropic's cache is prefix-matched at the
// breakpoint, so a marker placed after volatile text would write a fresh entry
// every turn and never read one.
//
// A single system message keeps the previous behavior exactly: one block,
// cached in full.
func toAnthropicSystemBlocks(systemMessages []string) []map[string]any {
	blocks := make([]map[string]any, 0, len(systemMessages))
	for i, text := range systemMessages {
		block := map[string]any{
			"type": "text",
			"text": text,
		}
		if i == 0 {
			block["cache_control"] = anthropicEphemeralCacheControl()
		}
		blocks = append(blocks, block)
	}
	return blocks
}

// applyAnthropicRollingCacheBreakpoints places cache_control markers on the
// message array so a growing conversation reuses its prefix instead of paying
// full input rates every turn.
//
// Markers land only on completed turns: the newest sits on the last message of
// the most recent completed turn — making everything before the incoming turn
// one cacheable prefix — and the second sits on the turn before it. That
// second marker is the fallback: it is the position the previous request
// marked as its newest, so it is already warm, and it survives a change that
// invalidates only the newer prefix.
//
// The trailing group of messages (the incoming user message plus any in-flight
// tool exchanges) never gets one. Note this is a deliberately conservative
// choice, not a free one: agent.Loop appends each tool exchange to the same
// slice and re-sends it, so a marker on a completed in-flight tool_result
// WOULD be read by the next loop iteration. Placing one there is the obvious
// next step for cutting tool-loop cost; it is kept out of scope here so the
// placement rule stays "completed turns only".
//
// Budget comes from anthropicMessageCacheBudget, so a request that already
// spends breakpoints on system blocks and tools simply places fewer here.
// Short histories use fewer slots; an empty array gets none.
//
// Slots are filled newest-first and a slot is only consumed when a marker
// actually lands, so a turn that cannot carry one (see
// markAnthropicCacheBreakpoint) falls through to an older completed turn
// instead of being dropped.
func applyAnthropicRollingCacheBreakpoints(wire []anthropicWireMessage, messages []ChatMessage, reservedBreakpoints int) {
	if len(wire) == 0 || len(wire) != len(messages) {
		return
	}
	budget := anthropicMessageCacheBudget(reservedBreakpoints)
	if budget == 0 {
		return
	}
	ends := anthropicCompletedTurnEndIndexes(messages)
	placed := 0
	for i := len(ends) - 1; i >= 0 && placed < budget; i-- {
		if markAnthropicCacheBreakpoint(&wire[ends[i]]) {
			placed++
		}
	}
}

// anthropicCompletedTurnEndIndexes returns the index of the last message of
// each completed turn. A turn starts at a user-initiated message (role "user"
// that is not a tool result); assistant replies and tool results stay inside
// the turn that triggered them. The final group is the in-flight turn — the
// request exists to extend it — so it is excluded.
func anthropicCompletedTurnEndIndexes(messages []ChatMessage) []int {
	ends := make([]int, 0, len(messages))
	for i := range messages {
		// Every user turn start closes the group before it. i == 0 has no
		// preceding group; any later start closes either a previous turn or
		// (if the history opens with assistant/tool messages) the leading
		// prologue group.
		if i > 0 && anthropicIsUserTurnStart(messages[i]) {
			ends = append(ends, i-1)
		}
	}
	return ends
}

func anthropicIsUserTurnStart(msg ChatMessage) bool {
	return msg.Role == "user" && strings.TrimSpace(msg.ToolCallID) == ""
}

// markAnthropicCacheBreakpoint attaches cache_control to the last content
// block of one wire message, which makes Anthropic treat everything up to and
// including that block as the cached prefix. Plain string content is upgraded
// to a single text block; block arrays get the marker appended to their last
// entry. Messages that would split a tool-call/tool-result pairing (any
// tool_use-bearing content) or carry no markable content are skipped.
//
// It reports whether a marker was placed so the caller can spend the freed
// budget on an older turn rather than silently shipping fewer breakpoints.
func markAnthropicCacheBreakpoint(msg *anthropicWireMessage) bool {
	switch content := msg.Content.(type) {
	case string:
		if strings.TrimSpace(content) == "" {
			return false
		}
		msg.Content = []map[string]any{
			{
				"type":          "text",
				"text":          content,
				"cache_control": anthropicEphemeralCacheControl(),
			},
		}
		return true
	case []map[string]any:
		if len(content) == 0 {
			return false
		}
		last := content[len(content)-1]
		if blockType, _ := last["type"].(string); blockType == "tool_use" {
			return false
		}
		last["cache_control"] = anthropicEphemeralCacheControl()
		return true
	}
	return false
}

func (c *AnthropicClient) chatNonStreamingResponse(body io.Reader) (ChatResponse, error) {
	respBody, err := io.ReadAll(body)
	if err != nil {
		return ChatResponse{}, newProviderError("anthropic", "request", fmt.Errorf("read response: %w", err))
	}
	logLLMResponsePayload("anthropic", http.StatusOK, string(respBody))

	var parsed struct {
		Content []anthropicContentBlock `json:"content"`
		Usage   struct {
			InputTokens      int `json:"input_tokens"`
			OutputTokens     int `json:"output_tokens"`
			CacheReadTokens  int `json:"cache_read_input_tokens"`
			CacheWriteTokens int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
		StopReason string `json:"stop_reason"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return ChatResponse{}, newProviderError("anthropic", "parse", fmt.Errorf("decode response: %w", err))
	}
	content, toolCalls, reasoningBlocks := parseAnthropicContentBlocks(parsed.Content)
	zlog.Debug().
		Str("provider", "anthropic").
		Int("assistant_len", len(content)).
		Int("tool_call_count", len(toolCalls)).
		Int("reasoning_block_count", len(reasoningBlocks)).
		Int("input_tokens", parsed.Usage.InputTokens).
		Int("output_tokens", parsed.Usage.OutputTokens).
		Str("stop_reason", parsed.StopReason).
		Msg("llm response parsed")

	return ChatResponse{
		Message: ChatMessage{
			Role:             "assistant",
			Content:          content,
			ToolCalls:        toolCalls,
			ReasoningBlocks:  reasoningBlocks,
			ReasoningContent: flattenReasoningBlocks(reasoningBlocks),
		},
		Usage: Usage{
			InputTokens:      parsed.Usage.InputTokens,
			OutputTokens:     parsed.Usage.OutputTokens,
			CacheReadTokens:  parsed.Usage.CacheReadTokens,
			CacheWriteTokens: parsed.Usage.CacheWriteTokens,
		},
		StopReason: parsed.StopReason,
	}, nil
}

func (c *AnthropicClient) chatStreamingResponse(body io.Reader, onDelta func(text string), onReasoningDelta func(text string)) (ChatResponse, error) {
	var (
		response         ChatResponse
		eventType        string
		builder          strings.Builder
		reasoningBuilder strings.Builder
		stopReason       string
		toolCallsByIndex = map[int]ToolCall{}
		toolInputByIndex = map[int]string{}
		thinkingByIndex  = map[int]bool{}
		// Reasoning blocks are keyed by their stream index and re-ordered by
		// it at the end, because signature_delta for one block can arrive
		// after another block has already started.
		reasoningByIndex = map[int]*ReasoningBlock{}
	)

	scanner := createSSEScanner(body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			break
		}
		logLLMStreamPayload("anthropic", payload)

		switch eventType {
		case "content_block_start":
			var parsed struct {
				Index        int                   `json:"index"`
				ContentBlock anthropicContentBlock `json:"content_block"`
			}
			if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
				return ChatResponse{}, newProviderError("anthropic", "parse", fmt.Errorf("decode stream content block start: %w", err))
			}
			switch parsed.ContentBlock.Type {
			case "text":
				if parsed.ContentBlock.Text == "" {
					continue
				}
				builder.WriteString(parsed.ContentBlock.Text)
				zlog.Debug().Str("provider", "anthropic").Int("delta_len", len(parsed.ContentBlock.Text)).Str("delta", truncateForLog(parsed.ContentBlock.Text, 4000)).Msg("llm stream delta")
				onDelta(parsed.ContentBlock.Text)
			case ReasoningBlockThinking:
				thinkingByIndex[parsed.Index] = true
				reasoningByIndex[parsed.Index] = &ReasoningBlock{
					Type:      ReasoningBlockThinking,
					Text:      parsed.ContentBlock.Thinking,
					Signature: parsed.ContentBlock.Signature,
				}
				if parsed.ContentBlock.Thinking == "" {
					continue
				}
				reasoningBuilder.WriteString(parsed.ContentBlock.Thinking)
				zlog.Debug().Str("provider", "anthropic").Int("delta_len", len(parsed.ContentBlock.Thinking)).Str("delta", truncateForLog(parsed.ContentBlock.Thinking, 4000)).Msg("llm stream reasoning delta")
				if onReasoningDelta != nil {
					onReasoningDelta(parsed.ContentBlock.Thinking)
				}
			case ReasoningBlockRedacted:
				// Redacted blocks carry no readable text and never receive
				// deltas — the whole payload arrives here. Marking the index
				// as thinking keeps any stray partial_json off a tool call.
				thinkingByIndex[parsed.Index] = true
				reasoningByIndex[parsed.Index] = &ReasoningBlock{
					Type: ReasoningBlockRedacted,
					Data: parsed.ContentBlock.Data,
				}
			case "tool_use":
				prev := toolCallsByIndex[parsed.Index]
				if id := strings.TrimSpace(parsed.ContentBlock.ID); id != "" {
					prev.ID = id
				}
				if name := strings.TrimSpace(parsed.ContentBlock.Name); name != "" {
					prev.Name = name
				}
				if len(parsed.ContentBlock.Input) > 0 {
					toolInputByIndex[parsed.Index] = normalizeJSONRaw(parsed.ContentBlock.Input)
				}
				toolCallsByIndex[parsed.Index] = prev
			}
		case "content_block_delta":
			var parsed struct {
				Index int `json:"index"`
				Delta struct {
					Type        string `json:"type"`
					Text        string `json:"text"`
					Thinking    string `json:"thinking"`
					Signature   string `json:"signature"`
					PartialJSON string `json:"partial_json"`
				} `json:"delta"`
			}
			if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
				return ChatResponse{}, newProviderError("anthropic", "parse", fmt.Errorf("decode stream content delta: %w", err))
			}
			if parsed.Delta.Text != "" {
				builder.WriteString(parsed.Delta.Text)
				zlog.Debug().Str("provider", "anthropic").Int("delta_len", len(parsed.Delta.Text)).Str("delta", truncateForLog(parsed.Delta.Text, 4000)).Msg("llm stream delta")
				onDelta(parsed.Delta.Text)
			}
			if parsed.Delta.Thinking != "" || parsed.Delta.Type == "thinking_delta" {
				thinkingByIndex[parsed.Index] = true
				thinkingText := parsed.Delta.Thinking
				if thinkingText != "" {
					if block := reasoningByIndex[parsed.Index]; block != nil {
						block.Text += thinkingText
					} else {
						reasoningByIndex[parsed.Index] = &ReasoningBlock{Type: ReasoningBlockThinking, Text: thinkingText}
					}
					reasoningBuilder.WriteString(thinkingText)
					zlog.Debug().Str("provider", "anthropic").Int("delta_len", len(thinkingText)).Str("delta", truncateForLog(thinkingText, 4000)).Msg("llm stream reasoning delta")
					if onReasoningDelta != nil {
						onReasoningDelta(thinkingText)
					}
				}
			}
			// The signature closes a thinking block and is what makes it
			// replayable; without it the block has to be dropped on the way
			// back out.
			if parsed.Delta.Signature != "" {
				thinkingByIndex[parsed.Index] = true
				if block := reasoningByIndex[parsed.Index]; block != nil {
					block.Signature += parsed.Delta.Signature
				} else {
					reasoningByIndex[parsed.Index] = &ReasoningBlock{Type: ReasoningBlockThinking, Signature: parsed.Delta.Signature}
				}
			}
			if parsed.Delta.PartialJSON != "" && !thinkingByIndex[parsed.Index] {
				prev := toolCallsByIndex[parsed.Index]
				prev.Arguments += parsed.Delta.PartialJSON
				toolCallsByIndex[parsed.Index] = prev
			}
		case "message_start":
			var parsed struct {
				Message struct {
					Usage struct {
						InputTokens      int `json:"input_tokens"`
						CacheReadTokens  int `json:"cache_read_input_tokens"`
						CacheWriteTokens int `json:"cache_creation_input_tokens"`
					} `json:"usage"`
				} `json:"message"`
			}
			if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
				return ChatResponse{}, newProviderError("anthropic", "parse", fmt.Errorf("decode stream message start: %w", err))
			}
			response.Usage.InputTokens = parsed.Message.Usage.InputTokens
			response.Usage.CacheReadTokens = parsed.Message.Usage.CacheReadTokens
			response.Usage.CacheWriteTokens = parsed.Message.Usage.CacheWriteTokens
		case "message_delta":
			var parsed struct {
				Delta struct {
					StopReason string `json:"stop_reason"`
				} `json:"delta"`
				Usage struct {
					OutputTokens     int `json:"output_tokens"`
					CacheReadTokens  int `json:"cache_read_input_tokens"`
					CacheWriteTokens int `json:"cache_creation_input_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
				return ChatResponse{}, newProviderError("anthropic", "parse", fmt.Errorf("decode stream message delta: %w", err))
			}
			response.Usage.OutputTokens = parsed.Usage.OutputTokens
			if parsed.Usage.CacheReadTokens > 0 {
				response.Usage.CacheReadTokens = parsed.Usage.CacheReadTokens
			}
			if parsed.Usage.CacheWriteTokens > 0 {
				response.Usage.CacheWriteTokens = parsed.Usage.CacheWriteTokens
			}
			if parsed.Delta.StopReason != "" {
				stopReason = parsed.Delta.StopReason
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return ChatResponse{}, newProviderError("anthropic", "stream", fmt.Errorf("read stream response: %w", err))
	}
	for idx, tc := range toolCallsByIndex {
		if strings.TrimSpace(tc.Name) == "" {
			continue
		}
		if strings.TrimSpace(tc.ID) == "" {
			tc.ID = fmt.Sprintf("tool_call_%d", idx)
		}
		if strings.TrimSpace(tc.Arguments) == "" {
			tc.Arguments = toolInputByIndex[idx]
		}
		tc.Arguments = sanitizeToolArgumentsJSON(tc.Arguments)
		toolCallsByIndex[idx] = tc
	}
	toolCalls := orderedToolCalls(toolCallsByIndex)

	response.Message = ChatMessage{
		Role:             "assistant",
		Content:          builder.String(),
		ToolCalls:        toolCalls,
		ReasoningBlocks:  orderedReasoningBlocks(reasoningByIndex),
		ReasoningContent: reasoningBuilder.String(),
	}
	response.StopReason = stopReason
	zlog.Debug().
		Str("provider", "anthropic").
		Int("assistant_len", len(response.Message.Content)).
		Int("tool_call_count", len(toolCalls)).
		Int("reasoning_block_count", len(response.Message.ReasoningBlocks)).
		Int("input_tokens", response.Usage.InputTokens).
		Int("output_tokens", response.Usage.OutputTokens).
		Str("stop_reason", response.StopReason).
		Msg("llm stream complete")
	return response, nil
}

type anthropicContentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Thinking string `json:"thinking,omitempty"`
	// Signature authenticates a thinking block; Data is the opaque payload
	// of a redacted_thinking block. Both round-trip untouched.
	Signature string          `json:"signature,omitempty"`
	Data      string          `json:"data,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
}

type anthropicWireMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type anthropicWireTool struct {
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"input_schema,omitempty"`
	CacheControl map[string]any  `json:"cache_control,omitempty"`
}

func toAnthropicWireMessages(messages []ChatMessage) []anthropicWireMessage {
	if len(messages) == 0 {
		return nil
	}
	out := make([]anthropicWireMessage, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case "assistant":
			out = append(out, toAnthropicAssistantMessage(msg))
		case "tool":
			out = append(out, toAnthropicToolResultMessage(msg))
		default:
			if len(msg.ContentBlocks) > 0 {
				out = append(out, anthropicWireMessage{
					Role:    msg.Role,
					Content: toAnthropicContentBlocks(msg.Content, msg.ContentBlocks),
				})
			} else {
				out = append(out, anthropicWireMessage{
					Role:    msg.Role,
					Content: msg.Content,
				})
			}
		}
	}
	return out
}

// toAnthropicContentBlocks converts ContentBlocks to Anthropic wire format.
// Supports text, image (base64 source), and document (base64 source) blocks.
func toAnthropicContentBlocks(textContent string, blocks []ContentBlock) []map[string]any {
	out := make([]map[string]any, 0, len(blocks)+1)
	if strings.TrimSpace(textContent) != "" {
		out = append(out, map[string]any{
			"type": "text",
			"text": textContent,
		})
	}
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if strings.TrimSpace(b.Text) != "" {
				out = append(out, map[string]any{
					"type": "text",
					"text": b.Text,
				})
			}
		case "image":
			out = append(out, map[string]any{
				"type": "image",
				"source": map[string]string{
					"type":       "base64",
					"media_type": b.MediaType,
					"data":       b.Data,
				},
			})
		case "document":
			out = append(out, map[string]any{
				"type": "document",
				"source": map[string]string{
					"type":       "base64",
					"media_type": b.MediaType,
					"data":       b.Data,
				},
			})
		}
	}
	if len(out) == 0 {
		return []map[string]any{{"type": "text", "text": textContent}}
	}
	return out
}

// toAnthropicAssistantMessage renders one stored assistant turn back onto the
// wire.
//
// Thinking blocks are re-emitted only for turns that carry tool_use, and they
// lead the block array. That is exactly what the provider asks for: with
// extended thinking enabled it validates the signed thinking sequence of the
// assistant turn a tool_result answers, and it ignores thinking blocks on
// every other turn. Sending them anyway would grow the transcript for nothing.
func toAnthropicAssistantMessage(msg ChatMessage) anthropicWireMessage {
	if len(msg.ToolCalls) == 0 {
		return anthropicWireMessage{
			Role:    "assistant",
			Content: msg.Content,
		}
	}

	reasoning := toAnthropicReasoningBlocks(msg.ReasoningBlocks)
	blocks := make([]map[string]any, 0, len(msg.ToolCalls)+len(reasoning)+1)
	blocks = append(blocks, reasoning...)
	if strings.TrimSpace(msg.Content) != "" {
		blocks = append(blocks, map[string]any{
			"type": "text",
			"text": msg.Content,
		})
	}
	for idx, tc := range msg.ToolCalls {
		if strings.TrimSpace(tc.Name) == "" {
			continue
		}
		toolCallID := strings.TrimSpace(tc.ID)
		if toolCallID == "" {
			toolCallID = fmt.Sprintf("tool_call_%d", idx)
		}
		blocks = append(blocks, map[string]any{
			"type":  "tool_use",
			"id":    toolCallID,
			"name":  tc.Name,
			"input": parseToolArgumentsObject(tc.Arguments),
		})
	}
	if len(blocks) == 0 {
		return anthropicWireMessage{
			Role:    "assistant",
			Content: msg.Content,
		}
	}

	return anthropicWireMessage{
		Role:    "assistant",
		Content: blocks,
	}
}

func toAnthropicToolResultMessage(msg ChatMessage) anthropicWireMessage {
	toolUseID := strings.TrimSpace(msg.ToolCallID)
	if toolUseID == "" {
		toolUseID = "tool_call_missing"
	}
	return anthropicWireMessage{
		Role: "user",
		Content: []map[string]any{
			{
				"type":        "tool_result",
				"tool_use_id": toolUseID,
				"content":     msg.Content,
			},
		},
	}
}

// parseAnthropicContentBlocks splits one response content array into the
// visible text, the tool calls, and the reasoning blocks. Reasoning blocks are
// returned in wire order: Anthropic validates the sequence when it comes back,
// so it must not be reordered or deduplicated.
func parseAnthropicContentBlocks(blocks []anthropicContentBlock) (string, []ToolCall, []ReasoningBlock) {
	var builder strings.Builder
	toolCalls := make([]ToolCall, 0)
	reasoning := make([]ReasoningBlock, 0)
	for idx, block := range blocks {
		switch block.Type {
		case "text":
			builder.WriteString(block.Text)
		case ReasoningBlockThinking:
			reasoning = append(reasoning, ReasoningBlock{
				Type:      ReasoningBlockThinking,
				Text:      block.Thinking,
				Signature: block.Signature,
			})
		case ReasoningBlockRedacted:
			reasoning = append(reasoning, ReasoningBlock{
				Type: ReasoningBlockRedacted,
				Data: block.Data,
			})
		case "tool_use":
			if strings.TrimSpace(block.Name) == "" {
				continue
			}
			toolCallID := strings.TrimSpace(block.ID)
			if toolCallID == "" {
				toolCallID = fmt.Sprintf("tool_call_%d", idx)
			}
			toolCalls = append(toolCalls, ToolCall{
				ID:        toolCallID,
				Name:      block.Name,
				Arguments: normalizeJSONRaw(block.Input),
			})
		}
	}
	if len(reasoning) == 0 {
		reasoning = nil
	}
	if len(toolCalls) == 0 {
		return builder.String(), nil, reasoning
	}
	return builder.String(), toolCalls, reasoning
}

// toAnthropicReasoningBlocks renders stored reasoning blocks back onto the
// wire, preserving their original order.
//
// A thinking block with no signature is dropped: the provider signs what it
// emitted and rejects an unsigned block, so a transcript written before this
// client captured signatures degrades to "no thinking replayed" instead of a
// failed request. Redacted blocks are passed through unread.
func toAnthropicReasoningBlocks(blocks []ReasoningBlock) []map[string]any {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(blocks))
	for _, block := range blocks {
		switch block.Type {
		case ReasoningBlockThinking:
			if strings.TrimSpace(block.Signature) == "" {
				continue
			}
			out = append(out, map[string]any{
				"type":      ReasoningBlockThinking,
				"thinking":  block.Text,
				"signature": block.Signature,
			})
		case ReasoningBlockRedacted:
			if strings.TrimSpace(block.Data) == "" {
				continue
			}
			out = append(out, map[string]any{
				"type": ReasoningBlockRedacted,
				"data": block.Data,
			})
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// orderedReasoningBlocks flattens stream-indexed reasoning blocks back into
// the order the provider emitted them.
func orderedReasoningBlocks(m map[int]*ReasoningBlock) []ReasoningBlock {
	if len(m) == 0 {
		return nil
	}
	indices := make([]int, 0, len(m))
	for idx := range m {
		indices = append(indices, idx)
	}
	sort.Ints(indices)

	out := make([]ReasoningBlock, 0, len(indices))
	for _, idx := range indices {
		block := m[idx]
		if block == nil {
			continue
		}
		out = append(out, *block)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// flattenReasoningBlocks renders the readable part of a reasoning sequence as
// one string, which is what ReasoningContent and the console reasoning stream
// consume. Redacted blocks contribute nothing — there is nothing to show.
func flattenReasoningBlocks(blocks []ReasoningBlock) string {
	var builder strings.Builder
	for _, block := range blocks {
		if block.Type != ReasoningBlockThinking {
			continue
		}
		builder.WriteString(block.Text)
	}
	return builder.String()
}

func toAnthropicTools(tools []ToolSchema) []anthropicWireTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]anthropicWireTool, 0, len(tools))
	for _, tl := range tools {
		name := strings.TrimSpace(tl.Function.Name)
		if name == "" {
			continue
		}
		inputSchema := tl.Function.Parameters
		if len(bytes.TrimSpace(inputSchema)) == 0 {
			inputSchema = json.RawMessage(`{"type":"object"}`)
		}
		out = append(out, anthropicWireTool{
			Name:        name,
			Description: tl.Function.Description,
			InputSchema: inputSchema,
		})
	}
	return out
}

func toAnthropicToolChoice(choice *ToolChoice) map[string]any {
	if choice == nil {
		return nil
	}
	switch choice.Mode {
	case ToolChoiceModeRequired:
		return map[string]any{"type": "any"}
	case ToolChoiceModeAuto:
		return map[string]any{"type": "auto"}
	case ToolChoiceModeNone:
		return map[string]any{"type": "none"}
	case ToolChoiceModeSpecific:
		name := strings.TrimSpace(choice.Name)
		if name == "" {
			return nil
		}
		return map[string]any{"type": "tool", "name": name}
	}
	return nil
}
