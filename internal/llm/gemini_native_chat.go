package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	zlog "github.com/rs/zerolog/log"
)

func (c *GeminiNativeClient) ensureModelSupportsGenerateContent(ctx context.Context) error {
	c.preflightMu.Lock()
	if c.preflightChecked {
		err := c.preflightErr
		c.preflightMu.Unlock()
		return err
	}
	c.preflightMu.Unlock()

	checkCtx := ctx
	cancel := func() {}
	if c.config.HTTPTimeout > 0 {
		checkCtx, cancel = context.WithTimeout(ctx, c.config.HTTPTimeout)
	}
	defer cancel()

	modelURL := fmt.Sprintf("%s/%s/%s", strings.TrimRight(c.apiBaseURL, "/"), c.apiVersion, geminiNativeModelPath(c.model))
	model, err := c.fetchModelInfo(checkCtx, modelURL)
	if err != nil {
		err = wrapGeminiHTTPError("preflight", err)
		c.preflightMu.Lock()
		c.preflightErr = err
		c.preflightChecked = true
		c.preflightMu.Unlock()
		return err
	}
	if err := validateGeminiSupportedActions(model); err != nil {
		err = newProviderError("gemini-native", "preflight", err)
		c.preflightMu.Lock()
		c.preflightErr = err
		c.preflightChecked = true
		c.preflightMu.Unlock()
		return err
	}

	c.preflightMu.Lock()
	c.preflightErr = nil
	c.preflightChecked = true
	c.preflightMu.Unlock()
	return nil
}

func (c *GeminiNativeClient) fetchModelInfo(ctx context.Context, modelURL string) (*geminiModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build model info request: %w", err)
	}
	req.Header.Set("x-goog-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch model info: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read model info: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp geminiErrorResponse
		_ = json.Unmarshal(body, &errResp)
		return nil, &ProviderError{
			Provider:   "gemini-native",
			Operation:  "preflight",
			StatusCode: resp.StatusCode,
			Message:    strings.TrimSpace(errResp.Error.Message),
		}
	}

	var model geminiModelInfo
	if err := json.Unmarshal(body, &model); err != nil {
		return nil, fmt.Errorf("parse model info: %w", err)
	}
	return &model, nil
}

func (c *GeminiNativeClient) chatNonStreamingResponse(ctx context.Context, contents []*geminiContent, config *geminiRequestConfig) (ChatResponse, error) {
	reqBody := c.buildHTTPRequestBody(contents, config)
	encoded, err := json.Marshal(reqBody)
	if err != nil {
		return ChatResponse{}, newProviderError("gemini-native", "request", fmt.Errorf("marshal request: %w", err))
	}

	reqURL := c.requestURL(false)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(encoded))
	if err != nil {
		return ChatResponse{}, newProviderError("gemini-native", "request", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return ChatResponse{}, wrapGeminiHTTPError("request", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return ChatResponse{}, newProviderError("gemini-native", "request", fmt.Errorf("read response: %w", err))
	}

	if resp.StatusCode != http.StatusOK {
		var errResp geminiErrorResponse
		_ = json.Unmarshal(body, &errResp)
		return ChatResponse{}, &ProviderError{
			Provider:   "gemini-native",
			Operation:  "request",
			StatusCode: resp.StatusCode,
			Message:    strings.TrimSpace(errResp.Error.Message),
		}
	}

	var parsed geminiGenerateResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ChatResponse{}, newProviderError("gemini-native", "parse", fmt.Errorf("unmarshal response: %w", err))
	}
	logLLMResponsePayload("gemini-native", resp.StatusCode, string(body))

	if len(parsed.Candidates) == 0 {
		return ChatResponse{}, newProviderError("gemini-native", "parse", fmt.Errorf("gemini-native response has no candidates"))
	}

	candidate := parsed.Candidates[0]
	content, toolCalls := parseGeminiNativeParts(candidate.Content)
	stopReason := normalizeGeminiNativeStopReason(candidate.FinishReason, len(toolCalls) > 0)
	usage := extractGeminiNativeUsage(parsed.UsageMetadata)

	zlog.Debug().
		Str("provider", "gemini-native").
		Int("assistant_len", len(content)).
		Int("tool_call_count", len(toolCalls)).
		Int("input_tokens", usage.InputTokens).
		Int("output_tokens", usage.OutputTokens).
		Str("stop_reason", stopReason).
		Msg("llm response parsed")

	return ChatResponse{
		Message: ChatMessage{
			Role:      "assistant",
			Content:   content,
			ToolCalls: toolCalls,
		},
		Usage:      usage,
		StopReason: stopReason,
	}, nil
}

func (c *GeminiNativeClient) chatStreamingResponse(ctx context.Context, contents []*geminiContent, config *geminiRequestConfig, onDelta func(text string)) (ChatResponse, error) {
	reqBody := c.buildHTTPRequestBody(contents, config)
	encoded, err := json.Marshal(reqBody)
	if err != nil {
		return ChatResponse{}, newProviderError("gemini-native", "stream", fmt.Errorf("marshal request: %w", err))
	}

	reqURL := c.requestURL(true)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(encoded))
	if err != nil {
		return ChatResponse{}, newProviderError("gemini-native", "stream", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return ChatResponse{}, wrapGeminiHTTPError("stream", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
		var errResp geminiErrorResponse
		_ = json.Unmarshal(body, &errResp)
		return ChatResponse{}, &ProviderError{
			Provider:   "gemini-native",
			Operation:  "stream",
			StatusCode: resp.StatusCode,
			Message:    strings.TrimSpace(errResp.Error.Message),
		}
	}

	var (
		builder       strings.Builder
		stopReasonRaw string
		usage         Usage
		toolCalls     []ToolCall
		seenToolCalls = map[string]struct{}{}
	)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 256*1024), 1*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var chunk geminiGenerateResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunkJSON, err := json.Marshal(chunk); err == nil {
			logLLMStreamPayload("gemini-native", string(chunkJSON))
		}

		if chunk.UsageMetadata != nil {
			if chunk.UsageMetadata.PromptTokenCount > 0 {
				usage.InputTokens = int(chunk.UsageMetadata.PromptTokenCount)
			}
			if chunk.UsageMetadata.CandidatesTokenCount > 0 {
				usage.OutputTokens = int(chunk.UsageMetadata.CandidatesTokenCount)
			}
		}

		if len(chunk.Candidates) == 0 {
			continue
		}
		candidate := chunk.Candidates[0]
		if candidate.FinishReason != "" {
			stopReasonRaw = candidate.FinishReason
		}
		if candidate.Content == nil {
			continue
		}

		for idx, part := range candidate.Content.Parts {
			if part == nil {
				continue
			}
			if part.Text != "" && !part.Thought {
				builder.WriteString(part.Text)
				zlog.Debug().Str("provider", "gemini-native").Int("delta_len", len(part.Text)).Str("delta", truncateForLog(part.Text, 4000)).Msg("llm stream delta")
				onDelta(part.Text)
			}
			if part.FunctionCall == nil || strings.TrimSpace(part.FunctionCall.Name) == "" {
				continue
			}
			call := geminiNativeFunctionCallToToolCall(part, idx)
			key := strings.TrimSpace(call.ID)
			if key == "" {
				key = call.Name + "|" + call.Arguments
			}
			if _, exists := seenToolCalls[key]; exists {
				continue
			}
			seenToolCalls[key] = struct{}{}
			toolCalls = append(toolCalls, call)
		}
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

func (c *GeminiNativeClient) buildGenerateContentConfig(messages []ChatMessage, opts ChatOptions) *geminiRequestConfig {
	config := &geminiRequestConfig{}
	if c.config.MaxTokens > 0 {
		config.MaxOutputTokens = int32(c.config.MaxTokens)
	}
	if thinkingConfig := buildGeminiThinkingConfig(c.config, opts); thinkingConfig != nil {
		config.ThinkingConfig = thinkingConfig
	}

	systemParts := make([]string, 0)
	for _, msg := range messages {
		if strings.EqualFold(strings.TrimSpace(msg.Role), "system") && strings.TrimSpace(msg.Content) != "" {
			systemParts = append(systemParts, strings.TrimSpace(msg.Content))
		}
	}
	if len(systemParts) > 0 {
		config.SystemInstruction = &geminiContent{Parts: []*geminiPart{{Text: strings.Join(systemParts, "\n")}}}
	}

	if tools := toGeminiNativeTools(opts.Tools); len(tools) > 0 {
		config.Tools = tools
		if toolConfig := toGeminiNativeToolConfig(opts.ToolChoice); toolConfig != nil {
			config.ToolConfig = toolConfig
		}
	}

	return config
}

func (c *GeminiNativeClient) buildHTTPRequestBody(contents []*geminiContent, config *geminiRequestConfig) *geminiGenerateRequest {
	req := &geminiGenerateRequest{
		Contents:          contents,
		SystemInstruction: config.SystemInstruction,
		Tools:             config.Tools,
		ToolConfig:        config.ToolConfig,
	}
	if config.MaxOutputTokens > 0 || config.ThinkingConfig != nil {
		req.GenerationConfig = &geminiGenConfig{
			MaxOutputTokens: config.MaxOutputTokens,
			ThinkingConfig:  config.ThinkingConfig,
		}
	}
	return req
}

func buildGeminiThinkingConfig(config ClientConfig, opts ChatOptions) *geminiThinkingConfig {
	effort := effectiveReasoningEffort(config, opts)
	budget := effectiveThinkingBudget(config, opts)
	if effort == "" && budget <= 0 {
		return nil
	}

	tc := &geminiThinkingConfig{}
	if budget > 0 {
		value := int32(budget)
		tc.ThinkingBudget = &value
	}
	switch effort {
	case "minimal":
		tc.ThinkingLevel = geminiThinkingMinimal
	case "low":
		tc.ThinkingLevel = geminiThinkingLow
	case "medium":
		tc.ThinkingLevel = geminiThinkingMedium
	case "high":
		tc.ThinkingLevel = geminiThinkingHigh
	}
	return tc
}

func (c *GeminiNativeClient) requestURL(streaming bool) string {
	path := fmt.Sprintf("%s/%s:generateContent", c.apiVersion, geminiNativeModelPath(c.model))
	if streaming {
		path = fmt.Sprintf("%s/%s:streamGenerateContent?alt=sse", c.apiVersion, geminiNativeModelPath(c.model))
	}
	return strings.TrimRight(c.apiBaseURL, "/") + "/" + path
}

func wrapGeminiHTTPError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return newProviderError("gemini-native", operation, err)
}

func splitGeminiNativeEndpoint(raw string) (string, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", "", fmt.Errorf("gemini-native base url is invalid: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", "", fmt.Errorf("gemini-native base url is invalid")
	}

	apiVersion := "v1beta"
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(segments) > 0 && segments[0] != "" {
		last := segments[len(segments)-1]
		if isGeminiAPIVersion(last) {
			apiVersion = last
			segments = segments[:len(segments)-1]
		}
	}

	if len(segments) == 0 || (len(segments) == 1 && segments[0] == "") {
		parsed.Path = ""
	} else {
		parsed.Path = "/" + strings.Join(segments, "/")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""

	return strings.TrimRight(parsed.String(), "/"), apiVersion, nil
}

func isGeminiAPIVersion(value string) bool {
	v := strings.TrimSpace(strings.ToLower(value))
	if len(v) < 2 || v[0] != 'v' {
		return false
	}

	hasDigit := false
	for _, ch := range v[1:] {
		switch {
		case ch >= '0' && ch <= '9':
			hasDigit = true
		case ch >= 'a' && ch <= 'z':
			continue
		default:
			return false
		}
	}
	return hasDigit
}

func geminiNativeModelPath(model string) string {
	trimmed := strings.TrimSpace(model)
	if strings.HasPrefix(trimmed, "models/") {
		return trimmed
	}
	return "models/" + trimmed
}
