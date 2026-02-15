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

	zlog "github.com/rs/zerolog/log"
)

// OpenAICompatibleClient works with any OpenAI-compatible /chat/completions API
// (Bifrost, OpenAI, Azure OpenAI, etc.).
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
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("%s base url is required", label)
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("%s api key is required", label)
	}
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("%s model is required", label)
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

func NewBifrostClient(baseURL, apiKey, model string) (*OpenAICompatibleClient, error) {
	return newOpenAICompatibleClientWithConfig("bifrost", baseURL, apiKey, model, DefaultClientConfig())
}

func NewOpenAIClient(baseURL, apiKey, model string) (*OpenAICompatibleClient, error) {
	return newOpenAICompatibleClientWithConfig("openai", baseURL, apiKey, model, DefaultClientConfig())
}

func NewOpenAICodexClient(baseURL, apiKey, model string) (*OpenAICompatibleClient, error) {
	return newOpenAICodexClientWithConfig(baseURL, apiKey, model, DefaultClientConfig())
}

func newOpenAICodexClientWithConfig(baseURL, apiKey, model string, config ClientConfig) (*OpenAICompatibleClient, error) {
	return newOpenAICompatibleClientWithConfig("openai-codex", baseURL, apiKey, model, config)
}

func (c *OpenAICompatibleClient) Chat(ctx context.Context, messages []ChatMessage, opts ChatOptions) (ChatResponse, error) {
	streaming := opts.OnDelta != nil
	zlog.Debug().
		Str("provider", c.label).
		Str("model", c.model).
		Str("url", c.baseURL+"/chat/completions").
		Int("message_count", len(messages)).
		Bool("stream", streaming).
		Int("tool_count", len(opts.Tools)).
		Str("tool_choice", strings.TrimSpace(opts.ToolChoice)).
		Msg("llm request start")

	reqBody, err := c.buildChatRequest(messages, opts)
	if err != nil {
		return ChatResponse{}, err
	}

	req, err := c.createChatHTTPRequest(ctx, reqBody)
	if err != nil {
		return ChatResponse{}, err
	}

	httpClient := c.httpClient
	if streaming {
		// Do not apply a hard client timeout to streaming responses.
		httpClient = &http.Client{
			Transport: c.httpClient.Transport,
		}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return ChatResponse{}, newProviderError(c.label, "request", fmt.Errorf("request %s: %w", c.label, err))
	}
	defer resp.Body.Close()
	zlog.Debug().Str("provider", c.label).Int("status", resp.StatusCode).Msg("llm response received")

	if err := checkHTTPStatus(resp, c.label); err != nil {
		return ChatResponse{}, err
	}

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

func (c *OpenAICompatibleClient) createChatHTTPRequest(ctx context.Context, reqBody map[string]any) (*http.Request, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, newProviderError(c.label, "parse", fmt.Errorf("marshal request: %w", err))
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, newProviderError(c.label, "request", fmt.Errorf("create request: %w", err))
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	return req, nil
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
			zlog.Debug().Str("provider", c.label).Int("delta_len", len(content)).Msg("llm stream delta")
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
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
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
			InputTokens:  parsed.Usage.PromptTokens,
			OutputTokens: parsed.Usage.CompletionTokens,
		},
		StopReason: parsed.Choices[0].FinishReason,
	}, nil
}

func (c *OpenAICompatibleClient) Ask(ctx context.Context, prompt string) (string, error) {
	resp, err := c.Chat(ctx, []ChatMessage{{Role: "user", Content: prompt}}, ChatOptions{})
	if err != nil {
		return "", err
	}
	return resp.Message.Content, nil
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
	Content    string               `json:"content,omitempty"`
	ToolCalls  []openAIWireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string               `json:"tool_call_id,omitempty"`
}

func toOpenAIWireMessages(messages []ChatMessage) []openAIWireMessage {
	if len(messages) == 0 {
		return nil
	}
	out := make([]openAIWireMessage, 0, len(messages))
	for _, m := range messages {
		wire := openAIWireMessage{
			Role:       m.Role,
			Content:    m.Content,
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
