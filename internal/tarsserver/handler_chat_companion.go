package tarsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/devlikebear/tars/internal/llm"
)

const companionFeedbackTimeout = 12 * time.Second

type companionFeedbackRequest struct {
	Stimulus        string `json:"stimulus"`
	RouteView       string `json:"route_view,omitempty"`
	Locale          string `json:"locale,omitempty"`
	FallbackMessage string `json:"fallback_message,omitempty"`
	FallbackDetail  string `json:"fallback_detail,omitempty"`
}

type companionFeedbackResponse struct {
	Mood    string `json:"mood"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
	Source  string `json:"source"`
}

type companionFeedbackLLMPayload struct {
	Stimulus        string `json:"stimulus"`
	RouteView       string `json:"route_view"`
	Locale          string `json:"locale"`
	FallbackMessage string `json:"fallback_message,omitempty"`
	FallbackDetail  string `json:"fallback_detail,omitempty"`
}

type companionFeedbackLLMResponse struct {
	Mood    string `json:"mood"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

func handleCompanionFeedbackRequest(w http.ResponseWriter, r *http.Request, deps chatHandlerDeps) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req companionFeedbackRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	req.Stimulus = strings.ToLower(strings.TrimSpace(req.Stimulus))
	if !validCompanionStimulus(req.Stimulus) {
		writeError(w, http.StatusBadRequest, "invalid_companion_stimulus", "stimulus must be one of poke, suggest, feedback")
		return
	}

	client, _, err := resolveCompanionFeedbackClient(deps)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "companion_llm_unavailable", err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), companionFeedbackTimeout)
	defer cancel()
	messages, err := buildCompanionFeedbackMessages(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_companion_request", err.Error())
		return
	}
	resp, err := client.Chat(ctx, messages, llm.ChatOptions{
		ToolChoice:               llm.ToolChoiceNone(),
		ResponseFormat:           &llm.ResponseFormat{Type: llm.ResponseFormatJSONObject},
		ReasoningEffort:          "low",
		ClaudeCodePermissionMode: "plan",
		ClaudeCodePermissionDeny: []string{
			"Bash(*)",
			"Edit(*)",
			"Glob(*)",
			"Grep(*)",
			"LS(*)",
			"Read(*)",
			"WebFetch",
			"WebSearch",
			"Write(*)",
		},
	})
	if err != nil {
		deps.logger.Warn().Err(err).Str("stimulus", req.Stimulus).Msg("companion feedback llm call failed")
		writeError(w, http.StatusBadGateway, "companion_llm_failed", "companion LLM feedback failed")
		return
	}

	out, err := parseCompanionFeedbackResponse(resp.Message.Content, req)
	if err != nil {
		deps.logger.Warn().Err(err).Str("stimulus", req.Stimulus).Msg("companion feedback llm response invalid")
		writeError(w, http.StatusBadGateway, "companion_llm_invalid", "companion LLM feedback was empty")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func resolveCompanionFeedbackClient(deps chatHandlerDeps) (llm.Client, llm.TierResolution, error) {
	if deps.router != nil {
		client, resolution, err := deps.router.ClientFor(llm.RoleCompanionFeedback)
		if err != nil {
			return nil, llm.TierResolution{}, err
		}
		if resolution.Source != "default" {
			return client, resolution, nil
		}
		lightClient, lightResolution, lightErr := deps.router.ClientForTier(llm.TierLight)
		if lightErr == nil {
			lightResolution.Role = llm.RoleCompanionFeedback
			lightResolution.Source = "role_default"
			return lightClient, lightResolution, nil
		}
		return client, resolution, nil
	}
	if deps.client != nil {
		return deps.client, llm.TierResolution{Role: llm.RoleCompanionFeedback, Source: "legacy"}, nil
	}
	return nil, llm.TierResolution{}, fmt.Errorf("llm router is not configured")
}

func buildCompanionFeedbackMessages(req companionFeedbackRequest) ([]llm.ChatMessage, error) {
	locale := normalizeCompanionFeedbackLocale(req.Locale)
	payload := companionFeedbackLLMPayload{
		Stimulus:        req.Stimulus,
		RouteView:       clipCompanionFeedbackText(req.RouteView, 80),
		Locale:          locale,
		FallbackMessage: clipCompanionFeedbackText(req.FallbackMessage, 180),
		FallbackDetail:  clipCompanionFeedbackText(req.FallbackDetail, 260),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode prompt payload: %w", err)
	}
	return []llm.ChatMessage{
		{
			Role: "system",
			Content: "You are the floating TARS Console companion. The user just pressed a companion action. " +
				"Give one short, context-aware micro-reaction that feels alive and useful. " +
				"Do not claim to inspect files, run tools, use camera/microphone, or know facts outside the supplied UI context. " +
				"Return strict JSON only with keys mood, message, and optional detail. " +
				"Allowed moods: spark, focus, warn, error, success. " +
				"If locale is ko, write natural Korean; otherwise write English. " +
				"Keep message under 90 characters and detail under 150 characters.",
		},
		{
			Role:    "user",
			Content: "Return JSON for this companion context:\n" + string(raw),
		},
	}, nil
}

func parseCompanionFeedbackResponse(raw string, req companionFeedbackRequest) (companionFeedbackResponse, error) {
	content := strings.TrimSpace(raw)
	if content == "" {
		return companionFeedbackResponse{}, fmt.Errorf("empty companion response")
	}
	var parsed companionFeedbackLLMResponse
	if err := json.Unmarshal([]byte(content), &parsed); err == nil {
		message := clipCompanionFeedbackText(parsed.Message, 180)
		if message == "" {
			message = clipCompanionFeedbackText(content, 180)
		}
		if message == "" {
			return companionFeedbackResponse{}, fmt.Errorf("empty companion message")
		}
		return companionFeedbackResponse{
			Mood:    normalizeCompanionFeedbackMood(parsed.Mood, req.Stimulus),
			Message: message,
			Detail:  clipCompanionFeedbackText(parsed.Detail, 260),
			Source:  "llm",
		}, nil
	}
	message := clipCompanionFeedbackText(content, 180)
	if message == "" {
		return companionFeedbackResponse{}, fmt.Errorf("empty companion text")
	}
	return companionFeedbackResponse{
		Mood:    defaultCompanionFeedbackMood(req.Stimulus),
		Message: message,
		Source:  "llm",
	}, nil
}

func validCompanionStimulus(stimulus string) bool {
	switch stimulus {
	case "poke", "suggest", "feedback":
		return true
	default:
		return false
	}
}

func normalizeCompanionFeedbackLocale(locale string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "ko") {
		return "ko"
	}
	return "en"
}

func normalizeCompanionFeedbackMood(mood, stimulus string) string {
	switch strings.ToLower(strings.TrimSpace(mood)) {
	case "spark", "focus", "warn", "error", "success":
		return strings.ToLower(strings.TrimSpace(mood))
	default:
		return defaultCompanionFeedbackMood(stimulus)
	}
}

func defaultCompanionFeedbackMood(stimulus string) string {
	switch strings.ToLower(strings.TrimSpace(stimulus)) {
	case "poke":
		return "spark"
	case "feedback":
		return "success"
	default:
		return "focus"
	}
}

func clipCompanionFeedbackText(value string, maxRunes int) string {
	text := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if maxRunes <= 0 || utf8.RuneCountInString(text) <= maxRunes {
		return text
	}
	runes := []rune(text)
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return strings.TrimSpace(string(runes[:maxRunes-3])) + "..."
}
