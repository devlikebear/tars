package llm

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"google.golang.org/genai"
)

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
