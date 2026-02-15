package llm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"

	zlog "github.com/rs/zerolog/log"
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
	resp, err := c.Chat(ctx, []ChatMessage{{Role: "user", Content: prompt}}, ChatOptions{})
	if err != nil {
		return "", err
	}
	return resp.Message.Content, nil
}

func (c *GeminiNativeClient) Chat(ctx context.Context, messages []ChatMessage, opts ChatOptions) (ChatResponse, error) {
	if err := c.ensureModelSupportsGenerateContent(ctx); err != nil {
		return ChatResponse{}, err
	}

	streaming := opts.OnDelta != nil
	zlog.Debug().
		Str("provider", "gemini-native").
		Str("model", c.model).
		Str("url", c.requestURL(streaming)).
		Int("message_count", len(messages)).
		Bool("stream", streaming).
		Int("tool_count", len(opts.Tools)).
		Str("tool_choice", strings.TrimSpace(opts.ToolChoice)).
		Msg("llm request start")

	contents := toGeminiNativeContents(messages)
	config := c.buildGenerateContentConfig(messages, opts)
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
				zlog.Debug().Str("provider", "gemini-native").Int("delta_len", len(part.Text)).Msg("llm stream delta")
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

func parseGeminiNativeParts(content *genai.Content) (string, []ToolCall) {
	if content == nil {
		return "", nil
	}

	var (
		builder   strings.Builder
		toolCalls []ToolCall
	)
	for idx, part := range content.Parts {
		if part == nil {
			continue
		}
		if part.Text != "" && !part.Thought {
			builder.WriteString(part.Text)
		}
		if part.FunctionCall == nil || strings.TrimSpace(part.FunctionCall.Name) == "" {
			continue
		}
		toolCalls = append(toolCalls, geminiNativeFunctionCallToToolCall(part, idx))
	}
	return builder.String(), toolCalls
}

func geminiNativeFunctionCallToToolCall(part *genai.Part, idx int) ToolCall {
	if part == nil || part.FunctionCall == nil {
		return ToolCall{ID: fmt.Sprintf("tool_call_%d", idx), Name: "", Arguments: "{}"}
	}
	call := part.FunctionCall

	id := strings.TrimSpace(call.ID)
	if id == "" {
		id = fmt.Sprintf("tool_call_%d", idx)
	}

	signature := ""
	if len(part.ThoughtSignature) > 0 {
		signature = base64.StdEncoding.EncodeToString(part.ThoughtSignature)
	}

	return ToolCall{
		ID:               id,
		Name:             strings.TrimSpace(call.Name),
		Arguments:        normalizeGeminiNativeArguments(call.Args),
		ThoughtSignature: signature,
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

func extractGeminiNativeUsage(metadata *genai.GenerateContentResponseUsageMetadata) Usage {
	if metadata == nil {
		return Usage{}
	}
	return Usage{
		InputTokens:  int(metadata.PromptTokenCount),
		OutputTokens: int(metadata.CandidatesTokenCount),
	}
}

func toGeminiNativeContents(messages []ChatMessage) []*genai.Content {
	if len(messages) == 0 {
		return nil
	}

	out := make([]*genai.Content, 0, len(messages))
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
			out = append(out, &genai.Content{
				Role: string(genai.RoleUser),
				Parts: []*genai.Part{{
					Text: msg.Content,
				}},
			})
		case "assistant":
			parts := make([]*genai.Part, 0, len(msg.ToolCalls)+1)
			if strings.TrimSpace(msg.Content) != "" {
				parts = append(parts, &genai.Part{Text: msg.Content})
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
				part := &genai.Part{
					FunctionCall: &genai.FunctionCall{
						ID:   callID,
						Name: name,
						Args: parseToolArgumentsObject(tc.Arguments),
					},
				}
				if decoded, ok := decodeGeminiThoughtSignature(tc.ThoughtSignature); ok {
					part.ThoughtSignature = decoded
				}
				parts = append(parts, part)
			}
			if len(parts) == 0 {
				continue
			}
			out = append(out, &genai.Content{Role: string(genai.RoleModel), Parts: parts})
		case "tool":
			toolName := strings.TrimSpace(toolNameByID[strings.TrimSpace(msg.ToolCallID)])
			if toolName == "" {
				toolName = "tool_call"
			}
			out = append(out, &genai.Content{
				Role: string(genai.RoleUser),
				Parts: []*genai.Part{{
					FunctionResponse: &genai.FunctionResponse{
						Name:     toolName,
						Response: parseGeminiNativeToolResponse(msg.Content),
					},
				}},
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

func toGeminiNativeTools(tools []ToolSchema) []*genai.Tool {
	if len(tools) == 0 {
		return nil
	}

	declarations := make([]*genai.FunctionDeclaration, 0, len(tools))
	for _, tl := range tools {
		name := strings.TrimSpace(tl.Function.Name)
		if name == "" {
			continue
		}

		decl := &genai.FunctionDeclaration{
			Name:        name,
			Description: strings.TrimSpace(tl.Function.Description),
		}

		if len(tl.Function.Parameters) > 0 {
			var params any
			if err := json.Unmarshal(tl.Function.Parameters, &params); err == nil && params != nil {
				decl.ParametersJsonSchema = params
			}
		}

		declarations = append(declarations, decl)
	}

	if len(declarations) == 0 {
		return nil
	}

	return []*genai.Tool{{FunctionDeclarations: declarations}}
}

func toGeminiNativeToolConfig(choice string) *genai.ToolConfig {
	mode := genai.FunctionCallingConfigModeAuto
	switch strings.ToLower(strings.TrimSpace(choice)) {
	case "required":
		mode = genai.FunctionCallingConfigModeAny
	case "none":
		mode = genai.FunctionCallingConfigModeNone
	case "", "auto":
		mode = genai.FunctionCallingConfigModeAuto
	default:
		return nil
	}

	return &genai.ToolConfig{
		FunctionCallingConfig: &genai.FunctionCallingConfig{Mode: mode},
	}
}

func validateGeminiSupportedActions(model *genai.Model) error {
	if model == nil || len(model.SupportedActions) == 0 {
		return nil
	}
	for _, action := range model.SupportedActions {
		normalized := strings.ToLower(strings.TrimSpace(action))
		if normalized == "generatecontent" || normalized == "generate_content" || strings.HasSuffix(normalized, ".generatecontent") {
			return nil
		}
	}
	actions := append([]string(nil), model.SupportedActions...)
	slices.Sort(actions)
	return fmt.Errorf("model %q does not support generateContent (supported actions: %s)", strings.TrimSpace(model.Name), strings.Join(actions, ", "))
}

func decodeGeminiThoughtSignature(encoded string) ([]byte, bool) {
	trimmed := strings.TrimSpace(encoded)
	if trimmed == "" {
		return nil, false
	}
	decoded, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil || len(decoded) == 0 {
		return nil, false
	}
	return decoded, true
}
