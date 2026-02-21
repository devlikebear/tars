package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	zlog "github.com/rs/zerolog/log"
	"google.golang.org/genai"
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

	model, err := c.client.Models.Get(checkCtx, c.model, nil)
	if err != nil {
		err = wrapGeminiNativeSDKError("preflight", err)
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

func (c *GeminiNativeClient) chatNonStreamingResponse(ctx context.Context, contents []*genai.Content, config *genai.GenerateContentConfig) (ChatResponse, error) {
	parsed, err := c.client.Models.GenerateContent(ctx, c.model, contents, config)
	if err != nil {
		return ChatResponse{}, wrapGeminiNativeSDKError("request", err)
	}
	if respJSON, err := json.Marshal(parsed); err == nil {
		logLLMResponsePayload("gemini-native", http.StatusOK, string(respJSON))
	}

	if len(parsed.Candidates) == 0 {
		return ChatResponse{}, newProviderError("gemini-native", "parse", fmt.Errorf("gemini-native response has no candidates"))
	}

	candidate := parsed.Candidates[0]
	content, toolCalls := parseGeminiNativeParts(candidate.Content)
	stopReason := normalizeGeminiNativeStopReason(string(candidate.FinishReason), len(toolCalls) > 0)
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

func (c *GeminiNativeClient) chatStreamingResponse(ctx context.Context, contents []*genai.Content, config *genai.GenerateContentConfig, onDelta func(text string)) (ChatResponse, error) {
	var (
		builder       strings.Builder
		stopReasonRaw string
		usage         Usage
		toolCalls     []ToolCall
		seenToolCalls = map[string]struct{}{}
	)

	for parsed, err := range c.client.Models.GenerateContentStream(ctx, c.model, contents, config) {
		if err != nil {
			return ChatResponse{}, wrapGeminiNativeSDKError("stream", err)
		}
		if parsed == nil {
			continue
		}
		if chunkJSON, err := json.Marshal(parsed); err == nil {
			logLLMStreamPayload("gemini-native", string(chunkJSON))
		}

		if parsed.UsageMetadata != nil {
			if parsed.UsageMetadata.PromptTokenCount > 0 {
				usage.InputTokens = int(parsed.UsageMetadata.PromptTokenCount)
			}
			if parsed.UsageMetadata.CandidatesTokenCount > 0 {
				usage.OutputTokens = int(parsed.UsageMetadata.CandidatesTokenCount)
			}
		}

		if len(parsed.Candidates) == 0 {
			continue
		}
		candidate := parsed.Candidates[0]
		if candidate.FinishReason != "" {
			stopReasonRaw = string(candidate.FinishReason)
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

func (c *GeminiNativeClient) buildGenerateContentConfig(messages []ChatMessage, opts ChatOptions) *genai.GenerateContentConfig {
	config := &genai.GenerateContentConfig{}
	if c.config.MaxTokens > 0 {
		config.MaxOutputTokens = int32(c.config.MaxTokens)
	}

	systemParts := make([]string, 0)
	for _, msg := range messages {
		if strings.EqualFold(strings.TrimSpace(msg.Role), "system") && strings.TrimSpace(msg.Content) != "" {
			systemParts = append(systemParts, strings.TrimSpace(msg.Content))
		}
	}
	if len(systemParts) > 0 {
		config.SystemInstruction = &genai.Content{Parts: []*genai.Part{{Text: strings.Join(systemParts, "\n")}}}
	}

	if tools := toGeminiNativeTools(opts.Tools); len(tools) > 0 {
		config.Tools = tools
		if toolConfig := toGeminiNativeToolConfig(opts.ToolChoice); toolConfig != nil {
			config.ToolConfig = toolConfig
		}
	}

	return config
}

func (c *GeminiNativeClient) requestURL(streaming bool) string {
	path := fmt.Sprintf("%s/%s:generateContent", c.apiVersion, geminiNativeModelPath(c.model))
	if streaming {
		path = fmt.Sprintf("%s/%s:streamGenerateContent?alt=sse", c.apiVersion, geminiNativeModelPath(c.model))
	}
	return strings.TrimRight(c.apiBaseURL, "/") + "/" + path
}

func wrapGeminiNativeSDKError(operation string, err error) error {
	if err == nil {
		return nil
	}

	var apiErr genai.APIError
	if errors.As(err, &apiErr) {
		return &ProviderError{
			Provider:   "gemini-native",
			Operation:  operation,
			StatusCode: apiErr.Code,
			Message:    strings.TrimSpace(apiErr.Message),
		}
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
