package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	zlog "github.com/rs/zerolog/log"
)

type AnthropicClient struct {
	baseURL    string
	apiKey     string
	model      string
	maxTokens  int
	httpClient *http.Client
}

func NewAnthropicClient(baseURL, apiKey, model string, maxTokens int) (*AnthropicClient, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("anthropic base url is required")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("anthropic api key is required")
	}
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("anthropic model is required")
	}

	if maxTokens <= 0 {
		maxTokens = 4096
	}

	return &AnthropicClient{
		baseURL:   strings.TrimRight(baseURL, "/"),
		apiKey:    apiKey,
		model:     model,
		maxTokens: maxTokens,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

func (c *AnthropicClient) Chat(ctx context.Context, messages []ChatMessage, opts ChatOptions) (ChatResponse, error) {
	streaming := opts.OnDelta != nil
	if len(opts.Tools) > 0 || strings.TrimSpace(opts.ToolChoice) != "" {
		zlog.Debug().
			Str("provider", "anthropic").
			Int("tool_count", len(opts.Tools)).
			Str("tool_choice", strings.TrimSpace(opts.ToolChoice)).
			Msg("tool-calls unsupported path; ignoring tools")
	}
	zlog.Debug().
		Str("provider", "anthropic").
		Str("model", c.model).
		Str("url", c.baseURL+"/v1/messages").
		Int("message_count", len(messages)).
		Bool("stream", streaming).
		Msg("llm request start")

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

	reqBody := map[string]any{
		"model":      c.model,
		"max_tokens": c.maxTokens,
		"messages":   nonSystemMessages,
	}
	if len(systemMessages) > 0 {
		reqBody["system"] = strings.Join(systemMessages, "\n")
	}

	if opts.OnDelta != nil {
		reqBody["stream"] = true
		return c.chatStreaming(ctx, reqBody, opts.OnDelta)
	}
	return c.chatNonStreaming(ctx, reqBody)
}

func (c *AnthropicClient) Ask(ctx context.Context, prompt string) (string, error) {
	resp, err := c.Chat(ctx, []ChatMessage{
		{
			Role:    "user",
			Content: prompt,
		},
	}, ChatOptions{})
	if err != nil {
		return "", err
	}
	return resp.Message.Content, nil
}

func (c *AnthropicClient) chatNonStreaming(ctx context.Context, reqBody map[string]any) (ChatResponse, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/v1/messages",
		bytes.NewReader(body),
	)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("request anthropic: %w", err)
	}
	defer resp.Body.Close()
	zlog.Debug().Str("provider", "anthropic").Int("status", resp.StatusCode).Msg("llm response received")

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return ChatResponse{}, fmt.Errorf("read response: %w", err)
		}
		return ChatResponse{}, fmt.Errorf("anthropic status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("read response: %w", err)
	}

	var parsed struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		StopReason string `json:"stop_reason"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return ChatResponse{}, fmt.Errorf("decode response: %w", err)
	}
	var b strings.Builder
	for _, c := range parsed.Content {
		if c.Type == "text" {
			b.WriteString(c.Text)
		}
	}
	zlog.Debug().
		Str("provider", "anthropic").
		Int("assistant_len", len(b.String())).
		Int("input_tokens", parsed.Usage.InputTokens).
		Int("output_tokens", parsed.Usage.OutputTokens).
		Str("stop_reason", parsed.StopReason).
		Msg("llm response parsed")

	return ChatResponse{
		Message: ChatMessage{
			Role:    "assistant",
			Content: b.String(),
		},
		Usage: Usage{
			InputTokens:  parsed.Usage.InputTokens,
			OutputTokens: parsed.Usage.OutputTokens,
		},
		StopReason: parsed.StopReason,
	}, nil
}

func (c *AnthropicClient) chatStreaming(ctx context.Context, reqBody map[string]any, onDelta func(text string)) (ChatResponse, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/v1/messages",
		bytes.NewReader(body),
	)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("request anthropic: %w", err)
	}
	defer resp.Body.Close()
	zlog.Debug().Str("provider", "anthropic").Int("status", resp.StatusCode).Msg("llm response received")

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return ChatResponse{}, fmt.Errorf("read response: %w", err)
		}
		return ChatResponse{}, fmt.Errorf("anthropic status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var (
		response   ChatResponse
		eventType  string
		builder    strings.Builder
		stopReason string
	)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
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

		switch eventType {
		case "content_block_delta":
			var parsed struct {
				Delta struct {
					Text string `json:"text"`
				} `json:"delta"`
			}
			if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
				return ChatResponse{}, fmt.Errorf("decode stream content delta: %w", err)
			}
			if parsed.Delta.Text == "" {
				continue
			}
			builder.WriteString(parsed.Delta.Text)
			zlog.Debug().Str("provider", "anthropic").Int("delta_len", len(parsed.Delta.Text)).Msg("llm stream delta")
			onDelta(parsed.Delta.Text)
		case "message_start":
			var parsed struct {
				Message struct {
					Usage struct {
						InputTokens int `json:"input_tokens"`
					} `json:"usage"`
				} `json:"message"`
			}
			if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
				return ChatResponse{}, fmt.Errorf("decode stream message start: %w", err)
			}
			response.Usage.InputTokens = parsed.Message.Usage.InputTokens
		case "message_delta":
			var parsed struct {
				Delta struct {
					StopReason string `json:"stop_reason"`
				} `json:"delta"`
				Usage struct {
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
				return ChatResponse{}, fmt.Errorf("decode stream message delta: %w", err)
			}
			response.Usage.OutputTokens = parsed.Usage.OutputTokens
			if parsed.Delta.StopReason != "" {
				stopReason = parsed.Delta.StopReason
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return ChatResponse{}, fmt.Errorf("read stream response: %w", err)
	}

	response.Message = ChatMessage{
		Role:    "assistant",
		Content: builder.String(),
	}
	response.StopReason = stopReason
	zlog.Debug().
		Str("provider", "anthropic").
		Int("assistant_len", len(response.Message.Content)).
		Int("input_tokens", response.Usage.InputTokens).
		Int("output_tokens", response.Usage.OutputTokens).
		Str("stop_reason", response.StopReason).
		Msg("llm stream complete")
	return response, nil
}
