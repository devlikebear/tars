package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"google.golang.org/genai"
)

type GeminiNativeClient struct {
	baseURL    string
	apiBaseURL string
	apiVersion string
	apiKey     string
	model      string
	config     ClientConfig
	httpClient *http.Client
	client     *genai.Client

	preflightMu      sync.Mutex
	preflightChecked bool
	preflightErr     error
}

func NewGeminiNativeClient(baseURL, apiKey, model string) (*GeminiNativeClient, error) {
	return newGeminiNativeClientWithConfig(baseURL, apiKey, model, DefaultClientConfig())
}

func newGeminiNativeClientWithConfig(baseURL, apiKey, model string, config ClientConfig) (*GeminiNativeClient, error) {
	trimmedBaseURL := strings.TrimSpace(baseURL)
	if trimmedBaseURL == "" {
		return nil, fmt.Errorf("gemini-native base url is required")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("gemini-native api key is required")
	}
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("gemini-native model is required")
	}

	apiBaseURL, apiVersion, err := splitGeminiNativeEndpoint(trimmedBaseURL)
	if err != nil {
		return nil, err
	}

	sdkClient, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
		HTTPClient: &http.Client{
			Transport: http.DefaultTransport,
		},
		HTTPOptions: genai.HTTPOptions{
			BaseURL:    apiBaseURL,
			APIVersion: apiVersion,
		},
	})
	if err != nil {
		return nil, newProviderError("gemini-native", "init", err)
	}

	return &GeminiNativeClient{
		baseURL:    strings.TrimRight(trimmedBaseURL, "/"),
		apiBaseURL: apiBaseURL,
		apiVersion: apiVersion,
		apiKey:     apiKey,
		model:      model,
		config:     config,
		httpClient: newHTTPClient(config.HTTPTimeout),
		client:     sdkClient,
	}, nil
}

func (c *GeminiNativeClient) Ask(ctx context.Context, prompt string) (string, error) {
	return askFromSinglePrompt(ctx, c.Chat, prompt)
}

func (c *GeminiNativeClient) Chat(ctx context.Context, messages []ChatMessage, opts ChatOptions) (ChatResponse, error) {
	if err := c.ensureModelSupportsGenerateContent(ctx); err != nil {
		return ChatResponse{}, err
	}

	streaming := opts.OnDelta != nil
	logChatRequestStart("gemini-native", c.model, c.requestURL(streaming), len(messages), streaming, len(opts.Tools), opts.ToolChoice)

	contents := toGeminiNativeContents(messages)
	config := c.buildGenerateContentConfig(messages, opts)
	if reqJSON, err := json.Marshal(map[string]any{
		"model":    c.model,
		"contents": contents,
		"config":   config,
	}); err == nil {
		logLLMRequestPayload("gemini-native", reqJSON)
	}
	if streaming {
		return c.chatStreamingResponse(ctx, contents, config, opts.OnDelta)
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, c.config.HTTPTimeout)
	if c.config.HTTPTimeout <= 0 {
		ctxWithTimeout = ctx
		cancel = func() {}
	}
	defer cancel()

	return c.chatNonStreamingResponse(ctxWithTimeout, contents, config)
}
