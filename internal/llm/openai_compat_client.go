package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	zlog "github.com/rs/zerolog/log"
)

// OpenAICompatibleClient works with any OpenAI-compatible /chat/completions API
// (OpenAI, Azure OpenAI, etc.).
type OpenAICompatibleClient struct {
	label      string
	baseURL    string
	apiKey     string
	model      string
	config     ClientConfig
	httpClient *http.Client
}

type openAICompatibleResponseContextKey struct{}

func newOpenAICompatibleClientWithConfig(label, baseURL, apiKey, model string, config ClientConfig) (*OpenAICompatibleClient, error) {
	if _, err := requireConfiguredValue(label, "base url", baseURL); err != nil {
		return nil, err
	}
	if _, err := requireConfiguredValue(label, "api key", apiKey); err != nil {
		return nil, err
	}
	if _, err := requireConfiguredValue(label, "model", model); err != nil {
		return nil, err
	}

	return &OpenAICompatibleClient{
		label:      label,
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
		config:     config,
		httpClient: newHTTPClient(config.HTTPTimeout),
	}, nil
}

func NewOpenAIClient(baseURL, apiKey, model string) (*OpenAICompatibleClient, error) {
	return newOpenAICompatibleClientWithConfig("openai", baseURL, apiKey, model, DefaultClientConfig())
}

func NewGeminiClient(baseURL, apiKey, model string) (*OpenAICompatibleClient, error) {
	return newOpenAICompatibleClientWithConfig("gemini", baseURL, apiKey, model, DefaultClientConfig())
}

func (c *OpenAICompatibleClient) Chat(ctx context.Context, messages []ChatMessage, opts ChatOptions) (ChatResponse, error) {
	streaming := opts.OnDelta != nil
	url := c.baseURL + "/chat/completions"
	logChatRequestStart(c.label, c.model, url, len(messages), streaming, len(opts.Tools), opts.ToolChoice)

	reqBody, err := c.buildChatRequest(messages, opts)
	if err != nil {
		return ChatResponse{}, err
	}

	req, resp, err := executeJSONChatRequest(ctx, jsonRequestSpec{
		Provider: c.label,
		URL:      url,
		Headers: map[string]string{
			"Authorization": "Bearer " + c.apiKey,
			"Content-Type":  "application/json",
		},
		Body: reqBody,
	}, c.httpClient, streaming)
	if err != nil {
		return ChatResponse{}, err
	}
	defer resp.Body.Close()

	req = req.WithContext(context.WithValue(req.Context(), openAICompatibleResponseContextKey{}, resp))
	if opts.OnDelta != nil {
		return c.chatStreaming(ctx, req, opts)
	}

	return c.chatNonStreaming(ctx, req)
}

func (c *OpenAICompatibleClient) buildChatRequest(messages []ChatMessage, opts ChatOptions) (map[string]any, error) {
	reqBody := map[string]any{
		"model":    c.model,
		"messages": toOpenAIWireMessages(messages),
	}
	if c.label != "gemini" {
		if effort := effectiveReasoningEffort(c.config, opts); effort != "" && effort != "none" {
			reqBody["reasoning_effort"] = effort
		}
		if tier := effectiveServiceTier(c.config, opts); tier != "" {
			reqBody["service_tier"] = tier
		}
	}
	if len(opts.Tools) > 0 {
		reqBody["tools"] = opts.Tools
		if choice := strings.TrimSpace(opts.ToolChoice); choice != "" {
			reqBody["tool_choice"] = choice
		}
	}
	if opts.OnDelta != nil {
		reqBody["stream"] = true
	}
	return reqBody, nil
}

func (c *OpenAICompatibleClient) chatStreaming(ctx context.Context, req *http.Request, opts ChatOptions) (ChatResponse, error) {
	_ = ctx
	resp := req.Context().Value(openAICompatibleResponseContextKey{}).(*http.Response)

	var (
		builder          strings.Builder
		stopReason       string
		toolCallsByIndex = map[int]ToolCall{}
	)
	scanner := createSSEScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		if payload == "" {
			continue
		}
		logLLMStreamPayload(c.label, payload)

		var parsed struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
			return ChatResponse{}, newProviderError(c.label, "parse", fmt.Errorf("decode stream response: %w", err))
		}
		if len(parsed.Choices) == 0 {
			continue
		}

		choice := parsed.Choices[0]
		content := choice.Delta.Content
		builder.WriteString(content)
		if choice.FinishReason != "" {
			stopReason = choice.FinishReason
		}
		for _, tc := range choice.Delta.ToolCalls {
			prev := toolCallsByIndex[tc.Index]
			if tc.ID != "" {
				prev.ID = tc.ID
			}
			if tc.Function.Name != "" {
				prev.Name = tc.Function.Name
			}
			if tc.Function.Arguments != "" {
				prev.Arguments += tc.Function.Arguments
			}
			toolCallsByIndex[tc.Index] = prev
		}
		if content != "" {
			zlog.Debug().Str("provider", c.label).Int("delta_len", len(content)).Str("delta", truncateForLog(content, 4000)).Msg("llm stream delta")
			opts.OnDelta(content)
		}
	}
	if err := scanner.Err(); err != nil {
		return ChatResponse{}, newProviderError(c.label, "stream", fmt.Errorf("read stream response: %w", err))
	}
	toolCalls := orderedToolCalls(toolCallsByIndex)
	zlog.Debug().
		Str("provider", c.label).
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
		StopReason: stopReason,
	}, nil
}

func (c *OpenAICompatibleClient) chatNonStreaming(ctx context.Context, req *http.Request) (ChatResponse, error) {
	_ = ctx
	resp := req.Context().Value(openAICompatibleResponseContextKey{}).(*http.Response)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChatResponse{}, newProviderError(c.label, "request", fmt.Errorf("read response: %w", err))
	}
	logLLMResponsePayload(c.label, resp.StatusCode, string(respBody))

	var parsed struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens        int `json:"prompt_tokens"`
			CompletionTokens    int `json:"completion_tokens"`
			PromptTokensDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			CacheReadTokens  int `json:"cache_read_tokens"`
			CacheWriteTokens int `json:"cache_write_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return ChatResponse{}, newProviderError(c.label, "parse", fmt.Errorf("decode response: %w", err))
	}
	if len(parsed.Choices) == 0 {
		return ChatResponse{}, newProviderError(c.label, "parse", fmt.Errorf("%s response has no choices", c.label))
	}
	zlog.Debug().
		Str("provider", c.label).
		Int("assistant_len", len(parsed.Choices[0].Message.Content)).
		Int("tool_call_count", len(parsed.Choices[0].Message.ToolCalls)).
		Int("input_tokens", parsed.Usage.PromptTokens).
		Int("output_tokens", parsed.Usage.CompletionTokens).
		Str("stop_reason", parsed.Choices[0].FinishReason).
		Msg("llm response parsed")

	return ChatResponse{
		Message: ChatMessage{
			Role:      "assistant",
			Content:   parsed.Choices[0].Message.Content,
			ToolCalls: nonStreamingToolCalls(parsed.Choices[0].Message.ToolCalls),
		},
		Usage: Usage{
			InputTokens:      parsed.Usage.PromptTokens,
			OutputTokens:     parsed.Usage.CompletionTokens,
			CachedTokens:     parsed.Usage.PromptTokensDetails.CachedTokens,
			CacheReadTokens:  parsed.Usage.CacheReadTokens,
			CacheWriteTokens: parsed.Usage.CacheWriteTokens,
		},
		StopReason: parsed.Choices[0].FinishReason,
	}, nil
}

func (c *OpenAICompatibleClient) Ask(ctx context.Context, prompt string) (string, error) {
	return askFromSinglePrompt(ctx, c.Chat, prompt)
}

func orderedToolCalls(m map[int]ToolCall) []ToolCall {
	if len(m) == 0 {
		return nil
	}
	indices := make([]int, 0, len(m))
	for idx := range m {
		indices = append(indices, idx)
	}
	sort.Ints(indices)

	out := make([]ToolCall, 0, len(indices))
	for _, idx := range indices {
		call := m[idx]
		if strings.TrimSpace(call.Name) == "" {
			continue
		}
		out = append(out, call)
	}
	return out
}

func nonStreamingToolCalls(src []struct {
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}) []ToolCall {
	if len(src) == 0 {
		return nil
	}
	out := make([]ToolCall, 0, len(src))
	for _, tc := range src {
		if strings.TrimSpace(tc.Function.Name) == "" {
			continue
		}
		out = append(out, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}
	return out
}

type openAIWireToolCall struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAIWireMessage struct {
	Role       string               `json:"role"`
	Content    any                  `json:"content,omitempty"`
	ToolCalls  []openAIWireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string               `json:"tool_call_id,omitempty"`
}

// toOpenAIContent converts a ChatMessage to OpenAI Chat Completions content format.
// Returns string for text-only, or array for multimodal (text + image_url).
func toOpenAIContent(msg ChatMessage) any {
	if len(msg.ContentBlocks) == 0 {
		return msg.Content
	}

	blocks := make([]map[string]any, 0, len(msg.ContentBlocks)+1)
	if strings.TrimSpace(msg.Content) != "" {
		blocks = append(blocks, map[string]any{"type": "text", "text": msg.Content})
	}
	for _, b := range msg.ContentBlocks {
		switch b.Type {
		case "text":
			if strings.TrimSpace(b.Text) != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": b.Text})
			}
		case "image":
			dataURL := "data:" + b.MediaType + ";base64," + b.Data
			blocks = append(blocks, map[string]any{
				"type": "image_url",
				"image_url": map[string]string{
					"url": dataURL,
				},
			})
		case "document":
			blocks = append(blocks, map[string]any{
				"type": "text",
				"text": "[Attached PDF document]",
			})
		}
	}
	if len(blocks) == 0 {
		return msg.Content
	}
	return blocks
}

func toOpenAIWireMessages(messages []ChatMessage) []openAIWireMessage {
	if len(messages) == 0 {
		return nil
	}
	out := make([]openAIWireMessage, 0, len(messages))
	for _, m := range messages {
		wire := openAIWireMessage{
			Role:       m.Role,
			Content:    toOpenAIContent(m),
			ToolCallID: m.ToolCallID,
		}
		if len(m.ToolCalls) > 0 {
			wire.ToolCalls = make([]openAIWireToolCall, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				wc := openAIWireToolCall{
					ID:   tc.ID,
					Type: "function",
				}
				wc.Function.Name = tc.Name
				wc.Function.Arguments = sanitizeToolArgumentsJSON(tc.Arguments)
				wire.ToolCalls = append(wire.ToolCalls, wc)
			}
		}
		out = append(out, wire)
	}
	return out
}
