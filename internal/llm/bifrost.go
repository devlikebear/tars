package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	zlog "github.com/rs/zerolog/log"
)

// OpenAICompatibleClient works with any OpenAI-compatible /chat/completions API
// (Bifrost, OpenAI, Azure OpenAI, etc.).
type OpenAICompatibleClient struct {
	label      string
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

func newOpenAICompatibleClient(label, baseURL, apiKey, model string) (*OpenAICompatibleClient, error) {
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
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func NewBifrostClient(baseURL, apiKey, model string) (*OpenAICompatibleClient, error) {
	return newOpenAICompatibleClient("bifrost", baseURL, apiKey, model)
}

func NewOpenAIClient(baseURL, apiKey, model string) (*OpenAICompatibleClient, error) {
	return newOpenAICompatibleClient("openai", baseURL, apiKey, model)
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

	body, err := json.Marshal(reqBody)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	httpClient := c.httpClient
	if streaming {
		// Do not apply a hard client timeout to streaming responses.
		httpClient = &http.Client{
			Transport: c.httpClient.Transport,
		}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("request %s: %w", c.label, err)
	}
	defer resp.Body.Close()
	zlog.Debug().Str("provider", c.label).Int("status", resp.StatusCode).Msg("llm response received")

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return ChatResponse{}, fmt.Errorf("read response: %w", err)
		}
		return ChatResponse{}, fmt.Errorf("%s status %d: %s", c.label, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	if opts.OnDelta != nil {
		var (
			builder          strings.Builder
			stopReason       string
			toolCallsByIndex = map[int]ToolCall{}
		)
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 1024), 1024*1024)
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
				return ChatResponse{}, fmt.Errorf("decode stream response: %w", err)
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
			return ChatResponse{}, fmt.Errorf("read stream response: %w", err)
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

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("read response: %w", err)
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
		return ChatResponse{}, fmt.Errorf("decode response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return ChatResponse{}, fmt.Errorf("%s response has no choices", c.label)
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

func sanitizeToolArgumentsJSON(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "{}"
	}
	if json.Valid([]byte(v)) {
		return v
	}
	return "{}"
}
