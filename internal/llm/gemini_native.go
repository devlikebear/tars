package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	zlog "github.com/rs/zerolog/log"
)

type GeminiNativeClient struct {
	baseURL    string
	apiKey     string
	model      string
	config     ClientConfig
	httpClient *http.Client
}

func NewGeminiNativeClient(baseURL, apiKey, model string) (*GeminiNativeClient, error) {
	return newGeminiNativeClientWithConfig(baseURL, apiKey, model, DefaultClientConfig())
}

func newGeminiNativeClientWithConfig(baseURL, apiKey, model string, config ClientConfig) (*GeminiNativeClient, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("gemini-native base url is required")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("gemini-native api key is required")
	}
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("gemini-native model is required")
	}
	return &GeminiNativeClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
		config:     config,
		httpClient: newHTTPClient(config.HTTPTimeout),
	}, nil
}

func (c *GeminiNativeClient) Ask(ctx context.Context, prompt string) (string, error) {
	resp, err := c.Chat(ctx, []ChatMessage{
		{Role: "user", Content: prompt},
	}, ChatOptions{})
	if err != nil {
		return "", err
	}
	return resp.Message.Content, nil
}

func (c *GeminiNativeClient) Chat(ctx context.Context, messages []ChatMessage, opts ChatOptions) (ChatResponse, error) {
	streaming := opts.OnDelta != nil
	url := c.requestURL(streaming)
	zlog.Debug().
		Str("provider", "gemini-native").
		Str("model", c.model).
		Str("url", url).
		Int("message_count", len(messages)).
		Bool("stream", streaming).
		Int("tool_count", len(opts.Tools)).
		Str("tool_choice", strings.TrimSpace(opts.ToolChoice)).
		Msg("llm request start")

	reqBody, err := c.buildGenerateContentRequest(messages, opts)
	if err != nil {
		return ChatResponse{}, err
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return ChatResponse{}, newProviderError("gemini-native", "parse", fmt.Errorf("marshal request: %w", err))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, newProviderError("gemini-native", "request", fmt.Errorf("create request: %w", err))
	}
	req.Header.Set("x-goog-api-key", c.apiKey)
	req.Header.Set("content-type", "application/json")

	httpClient := c.httpClient
	if streaming {
		httpClient = &http.Client{Transport: c.httpClient.Transport}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return ChatResponse{}, newProviderError("gemini-native", "request", fmt.Errorf("request gemini-native: %w", err))
	}
	defer resp.Body.Close()
	zlog.Debug().Str("provider", "gemini-native").Int("status", resp.StatusCode).Msg("llm response received")

	if err := checkHTTPStatus(resp, "gemini-native"); err != nil {
		return ChatResponse{}, err
	}

	if streaming {
		return c.chatStreamingResponse(resp.Body, opts.OnDelta)
	}
	return c.chatNonStreamingResponse(resp.Body)
}

func (c *GeminiNativeClient) requestURL(streaming bool) string {
	if streaming {
		return fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse", c.baseURL, c.model)
	}
	return fmt.Sprintf("%s/models/%s:generateContent", c.baseURL, c.model)
}

func (c *GeminiNativeClient) buildGenerateContentRequest(messages []ChatMessage, opts ChatOptions) (map[string]any, error) {
	systemParts := make([]string, 0)
	for _, msg := range messages {
		if strings.EqualFold(strings.TrimSpace(msg.Role), "system") && strings.TrimSpace(msg.Content) != "" {
			systemParts = append(systemParts, strings.TrimSpace(msg.Content))
		}
	}

	req := map[string]any{
		"contents": toGeminiNativeContents(messages),
	}
	if c.config.MaxTokens > 0 {
		req["generationConfig"] = map[string]any{"maxOutputTokens": c.config.MaxTokens}
	}
	if len(systemParts) > 0 {
		req["systemInstruction"] = map[string]any{
			"parts": []map[string]any{{"text": strings.Join(systemParts, "\n")}},
		}
	}
	if tools := toGeminiNativeTools(opts.Tools); len(tools) > 0 {
		req["tools"] = tools
		if toolConfig := toGeminiNativeToolConfig(opts.ToolChoice); len(toolConfig) > 0 {
			req["toolConfig"] = toolConfig
		}
	}
	return req, nil
}

func (c *GeminiNativeClient) chatNonStreamingResponse(body io.Reader) (ChatResponse, error) {
	respBody, err := io.ReadAll(body)
	if err != nil {
		return ChatResponse{}, newProviderError("gemini-native", "request", fmt.Errorf("read response: %w", err))
	}

	var parsed geminiNativeResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return ChatResponse{}, newProviderError("gemini-native", "parse", fmt.Errorf("decode response: %w", err))
	}
	if len(parsed.Candidates) == 0 {
		return ChatResponse{}, newProviderError("gemini-native", "parse", fmt.Errorf("gemini-native response has no candidates"))
	}
	content, toolCalls := parseGeminiNativeParts(parsed.Candidates[0].Content.Parts)
	stopReason := normalizeGeminiNativeStopReason(parsed.Candidates[0].FinishReason, len(toolCalls) > 0)
	zlog.Debug().
		Str("provider", "gemini-native").
		Int("assistant_len", len(content)).
		Int("tool_call_count", len(toolCalls)).
		Int("input_tokens", parsed.UsageMetadata.PromptTokenCount).
		Int("output_tokens", parsed.UsageMetadata.CandidatesTokenCount).
		Str("stop_reason", stopReason).
		Msg("llm response parsed")

	return ChatResponse{
		Message: ChatMessage{
			Role:      "assistant",
			Content:   content,
			ToolCalls: toolCalls,
		},
		Usage: Usage{
			InputTokens:  parsed.UsageMetadata.PromptTokenCount,
			OutputTokens: parsed.UsageMetadata.CandidatesTokenCount,
		},
		StopReason: stopReason,
	}, nil
}

func (c *GeminiNativeClient) chatStreamingResponse(body io.Reader, onDelta func(text string)) (ChatResponse, error) {
	var (
		builder       strings.Builder
		stopReasonRaw string
		usage         Usage
		toolCalls     []ToolCall
		seenToolCalls = map[string]struct{}{}
	)

	scanner := createSSEScanner(body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}

		var parsed geminiNativeResponse
		if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
			return ChatResponse{}, newProviderError("gemini-native", "parse", fmt.Errorf("decode stream response: %w", err))
		}
		if parsed.UsageMetadata.PromptTokenCount > 0 {
			usage.InputTokens = parsed.UsageMetadata.PromptTokenCount
		}
		if parsed.UsageMetadata.CandidatesTokenCount > 0 {
			usage.OutputTokens = parsed.UsageMetadata.CandidatesTokenCount
		}
		if len(parsed.Candidates) == 0 {
			continue
		}

		candidate := parsed.Candidates[0]
		if candidate.FinishReason != "" {
			stopReasonRaw = candidate.FinishReason
		}
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				builder.WriteString(part.Text)
				zlog.Debug().Str("provider", "gemini-native").Int("delta_len", len(part.Text)).Msg("llm stream delta")
				onDelta(part.Text)
			}
			if part.FunctionCall == nil || strings.TrimSpace(part.FunctionCall.Name) == "" {
				continue
			}
			call := geminiNativeFunctionCallToToolCall(*part.FunctionCall, len(toolCalls))
			key := call.Name + "|" + call.Arguments
			if _, exists := seenToolCalls[key]; exists {
				continue
			}
			seenToolCalls[key] = struct{}{}
			toolCalls = append(toolCalls, call)
		}
	}
	if err := scanner.Err(); err != nil {
		return ChatResponse{}, newProviderError("gemini-native", "stream", fmt.Errorf("read stream response: %w", err))
	}
	stopReason := normalizeGeminiNativeStopReason(stopReasonRaw, len(toolCalls) > 0)
	zlog.Debug().
		Str("provider", "gemini-native").
		Int("assistant_len", len(builder.String())).
		Int("tool_call_count", len(toolCalls)).
		Str("stop_reason", stopReason).
		Msg("llm stream complete")

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

type geminiNativeResponse struct {
	Candidates []struct {
		Content struct {
			Parts []geminiNativePart `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
	} `json:"usageMetadata"`
}

type geminiNativePart struct {
	Text             string                    `json:"text"`
	FunctionCall     *geminiNativeFunctionCall `json:"functionCall"`
	FunctionResponse *struct {
		Name     string `json:"name"`
		Response any    `json:"response"`
	} `json:"functionResponse"`
}

type geminiNativeFunctionCall struct {
	Name string `json:"name"`
	Args any    `json:"args"`
}

func parseGeminiNativeParts(parts []geminiNativePart) (string, []ToolCall) {
	var (
		builder   strings.Builder
		toolCalls []ToolCall
	)
	for idx, part := range parts {
		if part.Text != "" {
			builder.WriteString(part.Text)
		}
		if part.FunctionCall == nil || strings.TrimSpace(part.FunctionCall.Name) == "" {
			continue
		}
		toolCalls = append(toolCalls, geminiNativeFunctionCallToToolCall(*part.FunctionCall, idx))
	}
	return builder.String(), toolCalls
}

func geminiNativeFunctionCallToToolCall(call geminiNativeFunctionCall, idx int) ToolCall {
	return ToolCall{
		ID:        fmt.Sprintf("tool_call_%d", idx),
		Name:      strings.TrimSpace(call.Name),
		Arguments: normalizeGeminiNativeArguments(call.Args),
	}
}

func normalizeGeminiNativeArguments(raw any) string {
	if raw == nil {
		return "{}"
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return "{}"
	}
	return sanitizeToolArgumentsJSON(string(encoded))
}

func normalizeGeminiNativeStopReason(raw string, hasToolCalls bool) string {
	if hasToolCalls {
		return "tool_calls"
	}
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "":
		return ""
	case "STOP":
		return "stop"
	case "MAX_TOKENS":
		return "max_tokens"
	case "SAFETY":
		return "safety"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func toGeminiNativeContents(messages []ChatMessage) []map[string]any {
	if len(messages) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(messages))
	toolNameByID := map[string]string{}

	for _, msg := range messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		switch role {
		case "system":
			continue
		case "user":
			if strings.TrimSpace(msg.Content) == "" {
				continue
			}
			out = append(out, map[string]any{
				"role":  "user",
				"parts": []map[string]any{{"text": msg.Content}},
			})
		case "assistant":
			parts := make([]map[string]any, 0, len(msg.ToolCalls)+1)
			if strings.TrimSpace(msg.Content) != "" {
				parts = append(parts, map[string]any{"text": msg.Content})
			}
			for idx, tc := range msg.ToolCalls {
				name := strings.TrimSpace(tc.Name)
				if name == "" {
					continue
				}
				callID := strings.TrimSpace(tc.ID)
				if callID == "" {
					callID = fmt.Sprintf("tool_call_%d", idx)
				}
				toolNameByID[callID] = name
				parts = append(parts, map[string]any{
					"functionCall": map[string]any{
						"name": name,
						"args": parseToolArgumentsObject(tc.Arguments),
					},
				})
			}
			if len(parts) == 0 {
				continue
			}
			out = append(out, map[string]any{
				"role":  "model",
				"parts": parts,
			})
		case "tool":
			toolName := strings.TrimSpace(toolNameByID[strings.TrimSpace(msg.ToolCallID)])
			if toolName == "" {
				toolName = "tool_call"
			}
			out = append(out, map[string]any{
				"role": "user",
				"parts": []map[string]any{
					{
						"functionResponse": map[string]any{
							"name":     toolName,
							"response": parseGeminiNativeToolResponse(msg.Content),
						},
					},
				},
			})
		}
	}
	return out
}

func parseGeminiNativeToolResponse(raw string) map[string]any {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string]any{}
	}
	var parsed any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return map[string]any{"text": trimmed}
	}
	switch v := parsed.(type) {
	case map[string]any:
		return v
	default:
		return map[string]any{"value": v}
	}
}

func toGeminiNativeTools(tools []ToolSchema) []map[string]any {
	if len(tools) == 0 {
		return nil
	}
	declarations := make([]map[string]any, 0, len(tools))
	for _, tl := range tools {
		name := strings.TrimSpace(tl.Function.Name)
		if name == "" {
			continue
		}
		decl := map[string]any{
			"name":        name,
			"description": strings.TrimSpace(tl.Function.Description),
		}
		if len(tl.Function.Parameters) > 0 {
			var params map[string]any
			if err := json.Unmarshal(tl.Function.Parameters, &params); err == nil && len(params) > 0 {
				decl["parameters"] = params
			}
		}
		declarations = append(declarations, decl)
	}
	if len(declarations) == 0 {
		return nil
	}
	return []map[string]any{
		{"functionDeclarations": declarations},
	}
}

func toGeminiNativeToolConfig(choice string) map[string]any {
	mode := "AUTO"
	switch strings.ToLower(strings.TrimSpace(choice)) {
	case "required":
		mode = "ANY"
	case "none":
		mode = "NONE"
	case "", "auto":
		mode = "AUTO"
	default:
		return nil
	}
	return map[string]any{
		"functionCallingConfig": map[string]any{"mode": mode},
	}
}
