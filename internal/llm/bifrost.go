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
	reqBody := map[string]any{
		"model":    c.model,
		"messages": messages,
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("request %s: %w", c.label, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return ChatResponse{}, fmt.Errorf("read response: %w", err)
		}
		return ChatResponse{}, fmt.Errorf("%s status %d: %s", c.label, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	if opts.OnDelta != nil {
		var (
			builder    strings.Builder
			stopReason string
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
						Content string `json:"content"`
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

			content := parsed.Choices[0].Delta.Content
			builder.WriteString(content)
			if parsed.Choices[0].FinishReason != "" {
				stopReason = parsed.Choices[0].FinishReason
			}
			opts.OnDelta(content)
		}
		if err := scanner.Err(); err != nil {
			return ChatResponse{}, fmt.Errorf("read stream response: %w", err)
		}

		return ChatResponse{
			Message: ChatMessage{
				Role:    "assistant",
				Content: builder.String(),
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
				Content string `json:"content"`
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

	return ChatResponse{
		Message: ChatMessage{
			Role:    "assistant",
			Content: parsed.Choices[0].Message.Content,
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
