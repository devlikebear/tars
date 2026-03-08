package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/devlikebear/tars/internal/auth"
)

const (
	openAICodexProviderLabel = "openai-codex"
	codexResponsesBetaHeader = "responses=experimental"
	codexOriginatorHeader    = "tars"
)

type codexCredentialResolver func() (auth.CodexCredential, error)
type codexCredentialRefresher func(ctx context.Context, cred auth.CodexCredential) (auth.CodexCredential, error)

type openAICodexToolNameMap struct {
	outboundByOriginal map[string]string
	originalByOutbound map[string]string
}

type OpenAICodexClient struct {
	baseURL       string
	model         string
	authMode      string
	oauthProvider string
	apiKey        string
	config        ClientConfig
	httpClient    *http.Client

	resolveCredential codexCredentialResolver
	refreshCredential codexCredentialRefresher

	mu               sync.RWMutex
	overrideCred     *auth.CodexCredential
	validatedOnStart bool
}

func NewOpenAICodexClient(baseURL, model, authMode, oauthProvider, apiKey string) (*OpenAICodexClient, error) {
	return newOpenAICodexClientWithConfig(
		baseURL,
		model,
		authMode,
		oauthProvider,
		apiKey,
		DefaultClientConfig(),
		nil,
		nil,
	)
}

func newOpenAICodexClientWithConfig(
	baseURL, model, authMode, oauthProvider, apiKey string,
	config ClientConfig,
	resolver codexCredentialResolver,
	refresher codexCredentialRefresher,
) (*OpenAICodexClient, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("%s base url is required", openAICodexProviderLabel)
	}
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("%s model is required", openAICodexProviderLabel)
	}

	mode := strings.TrimSpace(strings.ToLower(authMode))
	if mode == "" {
		mode = "oauth"
	}
	if mode != "oauth" && mode != "api-key" {
		return nil, fmt.Errorf("%s unsupported auth mode: %s", openAICodexProviderLabel, authMode)
	}

	client := &OpenAICodexClient{
		baseURL:           strings.TrimRight(baseURL, "/"),
		model:             strings.TrimSpace(model),
		authMode:          mode,
		oauthProvider:     strings.TrimSpace(strings.ToLower(oauthProvider)),
		apiKey:            strings.TrimSpace(apiKey),
		config:            config,
		httpClient:        newHTTPClient(config.HTTPTimeout),
		resolveCredential: resolver,
		refreshCredential: refresher,
	}
	if client.resolveCredential == nil {
		client.resolveCredential = client.defaultResolveCredential
	}
	if client.refreshCredential == nil {
		client.refreshCredential = client.defaultRefreshCredential
	}

	cred, err := client.getCredential()
	if err != nil {
		return nil, err
	}
	client.setOverrideCredential(cred)
	client.validatedOnStart = true
	return client, nil
}

func (c *OpenAICodexClient) Ask(ctx context.Context, prompt string) (string, error) {
	resp, err := c.Chat(ctx, []ChatMessage{{Role: "user", Content: prompt}}, ChatOptions{})
	if err != nil {
		return "", err
	}
	return resp.Message.Content, nil
}

func (c *OpenAICodexClient) Chat(ctx context.Context, messages []ChatMessage, opts ChatOptions) (ChatResponse, error) {
	cred, err := c.getCredential()
	if err != nil {
		return ChatResponse{}, newProviderError(openAICodexProviderLabel, "auth", err)
	}
	streaming := opts.OnDelta != nil
	return c.chatWithCredential(ctx, cred, messages, opts, streaming, true, true, true)
}

func (c *OpenAICodexClient) chatWithCredential(
	ctx context.Context,
	cred auth.CodexCredential,
	messages []ChatMessage,
	opts ChatOptions,
	streaming bool,
	allowRefreshRetry bool,
	allowStreamFallback bool,
	allowTransientRetry bool,
) (ChatResponse, error) {
	toolNameMap := newOpenAICodexToolNameMap(opts.Tools)
	body, err := buildOpenAICodexRequestBody(messages, opts, c.model, streaming, toolNameMap)
	if err != nil {
		return ChatResponse{}, newProviderError(openAICodexProviderLabel, "parse", err)
	}
	rawBody, err := json.Marshal(body)
	if err != nil {
		return ChatResponse{}, newProviderError(openAICodexProviderLabel, "parse", fmt.Errorf("marshal request: %w", err))
	}
	logLLMRequestPayload(openAICodexProviderLabel, rawBody)

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		resolveOpenAICodexResponsesURL(c.baseURL),
		bytes.NewReader(rawBody),
	)
	if err != nil {
		return ChatResponse{}, newProviderError(openAICodexProviderLabel, "request", fmt.Errorf("create request: %w", err))
	}
	req.Header.Set("Authorization", "Bearer "+cred.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OpenAI-Beta", codexResponsesBetaHeader)
	req.Header.Set("originator", codexOriginatorHeader)
	if strings.TrimSpace(cred.AccountID) != "" {
		req.Header.Set("chatgpt-account-id", strings.TrimSpace(cred.AccountID))
	}
	if streaming {
		req.Header.Set("Accept", "text/event-stream")
	}

	resp, err := doRawPreparedRequest(req, openAICodexProviderLabel, transportHTTPClient(c.httpClient, streaming))
	if err != nil {
		return ChatResponse{}, err
	}

	if (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) &&
		allowRefreshRetry &&
		c.authMode == "oauth" &&
		strings.TrimSpace(cred.RefreshToken) != "" {
		resp.Body.Close()
		refreshed, refreshErr := c.refreshCredential(ctx, cred)
		if refreshErr != nil {
			return ChatResponse{}, newProviderError(openAICodexProviderLabel, "auth", refreshErr)
		}
		if strings.TrimSpace(refreshed.AccountID) == "" {
			refreshed.AccountID = auth.ParseCodexAccountIDFromJWT(refreshed.AccessToken)
		}
		c.setOverrideCredential(refreshed)
		return c.chatWithCredential(ctx, refreshed, messages, opts, streaming, false, allowStreamFallback, allowTransientRetry)
	}

	defer resp.Body.Close()
	if err := checkHTTPStatus(resp, openAICodexProviderLabel); err != nil {
		if allowStreamFallback && !streaming && isOpenAICodexStreamRequiredError(err) {
			return c.chatWithCredential(ctx, cred, messages, opts, true, allowRefreshRetry, false, allowTransientRetry)
		}
		return ChatResponse{}, err
	}

	var parsedResp ChatResponse
	if streaming {
		parsedResp, err = parseOpenAICodexSSE(resp.Body, opts, toolNameMap)
	} else {
		parsedResp, err = parseOpenAICodexJSON(resp.Body, toolNameMap)
	}
	if err != nil {
		if streaming && allowTransientRetry && isOpenAICodexRetryableStreamError(err) {
			return c.chatWithCredential(ctx, cred, messages, opts, streaming, allowRefreshRetry, allowStreamFallback, false)
		}
		return ChatResponse{}, err
	}
	return parsedResp, nil
}

func isOpenAICodexStreamRequiredError(err error) bool {
	providerErr, ok := err.(*ProviderError)
	if !ok || providerErr == nil {
		return false
	}
	if providerErr.StatusCode != http.StatusBadRequest {
		return false
	}
	message := strings.TrimSpace(strings.ToLower(providerErr.Message))
	return strings.Contains(message, "stream must be set to true")
}

func isOpenAICodexRetryableStreamError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.TrimSpace(strings.ToLower(err.Error()))
	if message == "" {
		return false
	}
	return strings.Contains(message, "internal_error") ||
		strings.Contains(message, "stream error: stream id") ||
		strings.Contains(message, "received from peer")
}

func (c *OpenAICodexClient) getCredential() (auth.CodexCredential, error) {
	c.mu.RLock()
	if c.overrideCred != nil {
		cred := *c.overrideCred
		c.mu.RUnlock()
		return cred, nil
	}
	c.mu.RUnlock()

	cred, err := c.resolveCredential()
	if err != nil {
		return auth.CodexCredential{}, err
	}
	if strings.TrimSpace(cred.AccountID) == "" {
		cred.AccountID = auth.ParseCodexAccountIDFromJWT(cred.AccessToken)
	}
	return cred, nil
}

func (c *OpenAICodexClient) setOverrideCredential(cred auth.CodexCredential) {
	c.mu.Lock()
	defer c.mu.Unlock()
	copy := cred
	c.overrideCred = &copy
}

func (c *OpenAICodexClient) defaultResolveCredential() (auth.CodexCredential, error) {
	if c.authMode == "api-key" {
		if strings.TrimSpace(c.apiKey) == "" {
			return auth.CodexCredential{}, fmt.Errorf("%s api key is required for auth mode api-key", openAICodexProviderLabel)
		}
		return auth.CodexCredential{
			AccessToken: strings.TrimSpace(c.apiKey),
			AccountID:   auth.ParseCodexAccountIDFromJWT(strings.TrimSpace(c.apiKey)),
			Source:      auth.CodexCredentialSourceEnv,
		}, nil
	}
	return auth.ResolveCodexCredential(auth.CodexResolveOptions{})
}

func (c *OpenAICodexClient) defaultRefreshCredential(ctx context.Context, cred auth.CodexCredential) (auth.CodexCredential, error) {
	return auth.RefreshCodexCredential(ctx, cred, auth.CodexRefreshOptions{PersistFile: true})
}

func resolveOpenAICodexResponsesURL(baseURL string) string {
	normalized := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(normalized, "/codex/responses") {
		return normalized
	}
	if strings.HasSuffix(normalized, "/codex") {
		return normalized + "/responses"
	}
	return normalized + "/codex/responses"
}

func buildOpenAICodexRequestBody(messages []ChatMessage, opts ChatOptions, model string, stream bool, nameMap openAICodexToolNameMap) (map[string]any, error) {
	instructions := make([]string, 0, len(messages))
	input := make([]any, 0, len(messages))
	callSeq := 0

	for _, msg := range messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		switch role {
		case "system":
			if s := strings.TrimSpace(msg.Content); s != "" {
				instructions = append(instructions, s)
			}
		case "user":
			input = append(input, map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": msg.Content},
				},
			})
		case "assistant":
			if strings.TrimSpace(msg.Content) != "" {
				input = append(input, map[string]any{
					"type":   "message",
					"role":   "assistant",
					"status": "completed",
					"content": []any{
						map[string]any{"type": "output_text", "text": msg.Content},
					},
				})
			}
			for _, tc := range msg.ToolCalls {
				name := nameMap.outbound(tc.Name)
				if name == "" {
					continue
				}
				callID := strings.TrimSpace(tc.ID)
				if callID == "" {
					callSeq++
					callID = fmt.Sprintf("call_%d", callSeq)
				}
				args := strings.TrimSpace(tc.Arguments)
				if args == "" {
					args = "{}"
				}
				input = append(input, map[string]any{
					"type":      "function_call",
					"call_id":   callID,
					"name":      name,
					"arguments": args,
				})
			}
		case "tool":
			callID := strings.TrimSpace(msg.ToolCallID)
			if callID == "" {
				continue
			}
			output := msg.Content
			if strings.TrimSpace(output) == "" {
				output = "(no output)"
			}
			input = append(input, map[string]any{
				"type":    "function_call_output",
				"call_id": callID,
				"output":  output,
			})
		}
	}

	body := map[string]any{
		"model":   strings.TrimSpace(model),
		"store":   false,
		"stream":  stream,
		"input":   input,
		"include": []string{"reasoning.encrypted_content"},
	}
	if len(instructions) > 0 {
		body["instructions"] = strings.Join(instructions, "\n\n")
	}

	if len(opts.Tools) > 0 {
		tools, err := convertOpenAICodexTools(opts.Tools, nameMap)
		if err != nil {
			return nil, err
		}
		body["tools"] = tools
		body["tool_choice"] = "auto"
		body["parallel_tool_calls"] = true
	}
	return body, nil
}

func convertOpenAICodexTools(tools []ToolSchema, nameMap openAICodexToolNameMap) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		name := nameMap.outbound(tool.Function.Name)
		if name == "" {
			continue
		}
		item := map[string]any{
			"type":        "function",
			"name":        name,
			"description": strings.TrimSpace(tool.Function.Description),
		}
		if len(tool.Function.Parameters) > 0 {
			var params any
			if err := json.Unmarshal(tool.Function.Parameters, &params); err != nil {
				return nil, fmt.Errorf("decode tool parameters for %s: %w", name, err)
			}
			item["parameters"] = params
		}
		out = append(out, item)
	}
	return out, nil
}

func parseOpenAICodexSSE(body io.Reader, opts ChatOptions, nameMap openAICodexToolNameMap) (ChatResponse, error) {
	var (
		builder       strings.Builder
		usage         Usage
		stopReason    string
		hasTextDelta  bool
		toolCallsByID = map[string]ToolCall{}
		toolOrder     []string
	)

	scanner := createSSEScanner(body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
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
		logLLMStreamPayload(openAICodexProviderLabel, payload)

		var event map[string]any
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return ChatResponse{}, newProviderError(openAICodexProviderLabel, "parse", fmt.Errorf("decode stream event: %w", err))
		}
		eventType := strings.TrimSpace(getStringAny(event["type"]))
		switch eventType {
		case "response.output_text.delta":
			delta := getStringAny(event["delta"])
			if delta == "" {
				continue
			}
			hasTextDelta = true
			builder.WriteString(delta)
			if opts.OnDelta != nil {
				opts.OnDelta(delta)
			}
		case "response.output_item.added":
			parseOpenAICodexToolCallFromItem(castMapAny(event["item"]), toolCallsByID, &toolOrder, nameMap)
		case "response.content_part.added":
		// no-op in v1: content_part lifecycle only
		case "response.output_item.done":
			item := castMapAny(event["item"])
			parseOpenAICodexToolCallFromItem(item, toolCallsByID, &toolOrder, nameMap)
			if !hasTextDelta {
				text := extractOpenAICodexOutputText(item)
				if text != "" {
					hasTextDelta = true
					builder.WriteString(text)
					if opts.OnDelta != nil {
						opts.OnDelta(text)
					}
				}
			}
		case "response.completed", "response.done":
			response := castMapAny(event["response"])
			usage = parseOpenAICodexUsage(castMapAny(response["usage"]))
			if status := strings.TrimSpace(getStringAny(response["status"])); status != "" {
				stopReason = status
			}
		case "response.failed", "error":
			return ChatResponse{}, newProviderError(openAICodexProviderLabel, "request", fmt.Errorf("%s", extractOpenAICodexErrorMessage(event)))
		}
	}
	if err := scanner.Err(); err != nil {
		return ChatResponse{}, newProviderError(openAICodexProviderLabel, "stream", fmt.Errorf("read stream response: %w", err))
	}

	toolCalls := orderedOpenAICodexToolCalls(toolCallsByID, toolOrder)
	if strings.TrimSpace(stopReason) == "" {
		if len(toolCalls) > 0 {
			stopReason = "tool_calls"
		} else {
			stopReason = "stop"
		}
	}
	return ChatResponse{
		Message: ChatMessage{
			Role:      "assistant",
			Content:   builder.String(),
			ToolCalls: toolCalls,
		},
		Usage:      usage,
		StopReason: stopReason,
	}, nil
}

func parseOpenAICodexJSON(body io.Reader, nameMap openAICodexToolNameMap) (ChatResponse, error) {
	respBody, err := io.ReadAll(body)
	if err != nil {
		return ChatResponse{}, newProviderError(openAICodexProviderLabel, "request", fmt.Errorf("read response: %w", err))
	}
	logLLMResponsePayload(openAICodexProviderLabel, http.StatusOK, string(respBody))

	var parsed map[string]any
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return ChatResponse{}, newProviderError(openAICodexProviderLabel, "parse", fmt.Errorf("decode response: %w", err))
	}
	if status := strings.TrimSpace(getStringAny(parsed["status"])); status == "failed" {
		return ChatResponse{}, newProviderError(openAICodexProviderLabel, "request", fmt.Errorf("%s", extractOpenAICodexErrorMessage(parsed)))
	}

	var (
		builder       strings.Builder
		toolCallsByID = map[string]ToolCall{}
		toolOrder     []string
	)
	for _, raw := range castSliceAny(parsed["output"]) {
		item := castMapAny(raw)
		parseOpenAICodexToolCallFromItem(item, toolCallsByID, &toolOrder, nameMap)
		if text := extractOpenAICodexOutputText(item); text != "" {
			builder.WriteString(text)
		}
	}
	toolCalls := orderedOpenAICodexToolCalls(toolCallsByID, toolOrder)

	stopReason := strings.TrimSpace(getStringAny(parsed["status"]))
	if stopReason == "" {
		if len(toolCalls) > 0 {
			stopReason = "tool_calls"
		} else {
			stopReason = "stop"
		}
	}
	return ChatResponse{
		Message: ChatMessage{
			Role:      "assistant",
			Content:   builder.String(),
			ToolCalls: toolCalls,
		},
		Usage:      parseOpenAICodexUsage(castMapAny(parsed["usage"])),
		StopReason: stopReason,
	}, nil
}

func parseOpenAICodexUsage(raw map[string]any) Usage {
	cached := getIntAny(raw["cached_tokens"])
	if cached <= 0 {
		cached = getIntAny(raw["input_cached_tokens"])
	}
	cacheRead := getIntAny(raw["cache_read_tokens"])
	cacheWrite := getIntAny(raw["cache_write_tokens"])
	if details := castMapAny(raw["input_tokens_details"]); len(details) > 0 {
		if cached <= 0 {
			cached = getIntAny(details["cached_tokens"])
		}
		if cacheRead <= 0 {
			cacheRead = getIntAny(details["cache_read_tokens"])
		}
		if cacheWrite <= 0 {
			cacheWrite = getIntAny(details["cache_write_tokens"])
		}
	}
	return Usage{
		InputTokens:      getIntAny(raw["input_tokens"]),
		OutputTokens:     getIntAny(raw["output_tokens"]),
		CachedTokens:     cached,
		CacheReadTokens:  cacheRead,
		CacheWriteTokens: cacheWrite,
	}
}

func parseOpenAICodexToolCallFromItem(item map[string]any, calls map[string]ToolCall, order *[]string, nameMap openAICodexToolNameMap) {
	if strings.TrimSpace(getStringAny(item["type"])) != "function_call" {
		return
	}
	callID := strings.TrimSpace(firstNonEmptyAny(item["call_id"], item["id"]))
	if callID == "" {
		return
	}
	existing, ok := calls[callID]
	if !ok {
		existing = ToolCall{ID: callID}
		*order = append(*order, callID)
	}
	if name := strings.TrimSpace(getStringAny(item["name"])); name != "" {
		name = nameMap.original(name)
		existing.Name = name
	}
	if args := getStringAny(item["arguments"]); args != "" {
		existing.Arguments = args
	}
	calls[callID] = existing
}

func newOpenAICodexToolNameMap(tools []ToolSchema) openAICodexToolNameMap {
	m := openAICodexToolNameMap{
		outboundByOriginal: map[string]string{},
		originalByOutbound: map[string]string{},
	}
	for _, schema := range tools {
		original := strings.TrimSpace(schema.Function.Name)
		if original == "" {
			continue
		}
		if _, exists := m.outboundByOriginal[original]; exists {
			continue
		}
		base := sanitizeOpenAICodexToolName(original)
		if base == "" {
			continue
		}
		candidate := base
		if prev, exists := m.originalByOutbound[candidate]; exists && prev != original {
			for idx := 2; ; idx++ {
				candidate = fmt.Sprintf("%s_%d", base, idx)
				if prev2, exists2 := m.originalByOutbound[candidate]; !exists2 || prev2 == original {
					break
				}
			}
		}
		m.outboundByOriginal[original] = candidate
		m.originalByOutbound[candidate] = original
	}
	return m
}

func sanitizeOpenAICodexToolName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_-")
	if out == "" {
		return "tool"
	}
	return out
}

func (m openAICodexToolNameMap) outbound(name string) string {
	original := strings.TrimSpace(name)
	if original == "" {
		return ""
	}
	if mapped, ok := m.outboundByOriginal[original]; ok && strings.TrimSpace(mapped) != "" {
		return mapped
	}
	return sanitizeOpenAICodexToolName(original)
}

func (m openAICodexToolNameMap) original(name string) string {
	outbound := strings.TrimSpace(name)
	if outbound == "" {
		return ""
	}
	if mapped, ok := m.originalByOutbound[outbound]; ok && strings.TrimSpace(mapped) != "" {
		return mapped
	}
	return outbound
}

func extractOpenAICodexOutputText(item map[string]any) string {
	if strings.TrimSpace(getStringAny(item["type"])) != "message" {
		return ""
	}
	var builder strings.Builder
	for _, raw := range castSliceAny(item["content"]) {
		part := castMapAny(raw)
		partType := strings.TrimSpace(getStringAny(part["type"]))
		if partType != "output_text" && partType != "text" {
			continue
		}
		builder.WriteString(getStringAny(part["text"]))
	}
	return builder.String()
}

func orderedOpenAICodexToolCalls(calls map[string]ToolCall, order []string) []ToolCall {
	if len(order) == 0 {
		return nil
	}
	out := make([]ToolCall, 0, len(order))
	for _, id := range order {
		tc := calls[id]
		if strings.TrimSpace(tc.Name) == "" {
			continue
		}
		out = append(out, tc)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func extractOpenAICodexErrorMessage(event map[string]any) string {
	if msg := strings.TrimSpace(getStringAny(event["message"])); msg != "" {
		return msg
	}
	if code := strings.TrimSpace(getStringAny(event["code"])); code != "" {
		return code
	}
	response := castMapAny(event["response"])
	errObj := castMapAny(response["error"])
	if msg := strings.TrimSpace(getStringAny(errObj["message"])); msg != "" {
		return msg
	}
	if len(event) == 0 {
		return "openai-codex request failed"
	}
	raw, _ := json.Marshal(event)
	if len(raw) == 0 {
		return "openai-codex request failed"
	}
	return string(raw)
}

func castMapAny(v any) map[string]any {
	m, _ := v.(map[string]any)
	if m == nil {
		return map[string]any{}
	}
	return m
}

func castSliceAny(v any) []any {
	s, _ := v.([]any)
	if s == nil {
		return []any{}
	}
	return s
}

func getStringAny(v any) string {
	switch typed := v.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func getIntAny(v any) int {
	switch typed := v.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		i, _ := typed.Int64()
		return int(i)
	default:
		return 0
	}
}

func firstNonEmptyAny(values ...any) string {
	for _, v := range values {
		if s := strings.TrimSpace(getStringAny(v)); s != "" {
			return s
		}
	}
	return ""
}
