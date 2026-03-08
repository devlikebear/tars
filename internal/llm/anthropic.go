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

const anthropicAPIVersion = "2023-06-01"
const anthropicPromptCachingBeta = "prompt-caching-2024-07-31"

type AnthropicClient struct {
	baseURL    string
	apiKey     string
	model      string
	config     ClientConfig
	httpClient *http.Client
}

func NewAnthropicClient(baseURL, apiKey, model string, maxTokens int) (*AnthropicClient, error) {
	config := DefaultClientConfig()
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
	config.MaxTokens = maxTokens

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
	zlog.Debug().
		Str("provider", "anthropic").
		Str("model", c.model).
		Str("url", c.baseURL+"/v1/messages").
		Int("message_count", len(messages)).
		Bool("stream", streaming).
		Int("tool_count", len(opts.Tools)).
		Str("tool_choice", strings.TrimSpace(opts.ToolChoice)).
		Msg("llm request start")

	reqBody := c.buildChatRequest(messages, opts, streaming)
	req, err := jsonRequestSpec{
		Provider: "anthropic",
		URL:      c.baseURL + "/v1/messages",
		Headers: map[string]string{
			"x-api-key":         c.apiKey,
			"anthropic-version": anthropicAPIVersion,
			"anthropic-beta":    anthropicPromptCachingBeta,
			"content-type":      "application/json",
		},
		Body: reqBody,
	}.buildRequest(ctx)
	if err != nil {
		return ChatResponse{}, err
	}
	resp, err := doPreparedRequest(req, "anthropic", transportHTTPClient(c.httpClient, streaming))
	if err != nil {
		return ChatResponse{}, err
	}
	defer resp.Body.Close()
	zlog.Debug().Str("provider", "anthropic").Int("status", resp.StatusCode).Msg("llm response received")

	if streaming {
		return c.chatStreamingResponse(resp.Body, opts.OnDelta)
	}
	return c.chatNonStreamingResponse(resp.Body)
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

	reqBody := map[string]any{
		"model":      c.model,
		"max_tokens": c.config.MaxTokens,
		"messages":   toAnthropicWireMessages(nonSystemMessages),
	}
	if budget := effectiveThinkingBudget(c.config, opts); budget > 0 {
		reqBody["thinking"] = map[string]any{
			"type":          "enabled",
			"budget_tokens": budget,
		}
	}
	if len(systemMessages) > 0 {
		reqBody["system"] = []map[string]any{
			{
				"type": "text",
				"text": strings.Join(systemMessages, "\n"),
				"cache_control": map[string]any{
					"type": "ephemeral",
				},
			},
		}
	}
	if tools := toAnthropicTools(opts.Tools); len(tools) > 0 {
		tools[len(tools)-1].CacheControl = map[string]any{"type": "ephemeral"}
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
	content, toolCalls := parseAnthropicContentBlocks(parsed.Content)
	zlog.Debug().
		Str("provider", "anthropic").
		Int("assistant_len", len(content)).
		Int("tool_call_count", len(toolCalls)).
		Int("input_tokens", parsed.Usage.InputTokens).
		Int("output_tokens", parsed.Usage.OutputTokens).
		Str("stop_reason", parsed.StopReason).
		Msg("llm response parsed")

	return ChatResponse{
		Message: ChatMessage{
			Role:      "assistant",
			Content:   content,
			ToolCalls: toolCalls,
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

func (c *AnthropicClient) chatStreamingResponse(body io.Reader, onDelta func(text string)) (ChatResponse, error) {
	var (
		response         ChatResponse
		eventType        string
		builder          strings.Builder
		stopReason       string
		toolCallsByIndex = map[int]ToolCall{}
		toolInputByIndex = map[int]string{}
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
					Text        string `json:"text"`
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
			if parsed.Delta.PartialJSON != "" {
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
		Role:      "assistant",
		Content:   builder.String(),
		ToolCalls: toolCalls,
	}
	response.StopReason = stopReason
	zlog.Debug().
		Str("provider", "anthropic").
		Int("assistant_len", len(response.Message.Content)).
		Int("tool_call_count", len(toolCalls)).
		Int("input_tokens", response.Usage.InputTokens).
		Int("output_tokens", response.Usage.OutputTokens).
		Str("stop_reason", response.StopReason).
		Msg("llm stream complete")
	return response, nil
}

type anthropicContentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
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
			out = append(out, anthropicWireMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}
	return out
}

func toAnthropicAssistantMessage(msg ChatMessage) anthropicWireMessage {
	if len(msg.ToolCalls) == 0 {
		return anthropicWireMessage{
			Role:    "assistant",
			Content: msg.Content,
		}
	}

	blocks := make([]map[string]any, 0, len(msg.ToolCalls)+1)
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

func parseAnthropicContentBlocks(blocks []anthropicContentBlock) (string, []ToolCall) {
	var builder strings.Builder
	toolCalls := make([]ToolCall, 0)
	for idx, block := range blocks {
		switch block.Type {
		case "text":
			builder.WriteString(block.Text)
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
	if len(toolCalls) == 0 {
		return builder.String(), nil
	}
	return builder.String(), toolCalls
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

func toAnthropicToolChoice(choice string) map[string]any {
	trimmed := strings.TrimSpace(choice)
	switch trimmed {
	case "":
		return nil
	case "required", "any":
		return map[string]any{"type": "any"}
	case "auto":
		return map[string]any{"type": "auto"}
	case "none":
		return map[string]any{"type": "none"}
	default:
		return map[string]any{
			"type": "tool",
			"name": trimmed,
		}
	}
}
