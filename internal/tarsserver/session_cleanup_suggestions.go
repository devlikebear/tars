package tarsserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
)

const (
	sessionCleanupModeArchive         = "archive"
	sessionCleanupModeDelete          = "delete"
	defaultSessionCleanupLimit        = 8
	maxSessionCleanupLimit            = 20
	maxSessionCleanupCandidatesForLLM = 30
	sessionCleanupRecentProtection    = 24 * time.Hour
	sessionCleanupTrivialProtection   = 30 * time.Minute
)

var errInvalidSessionCleanupMode = errors.New("mode must be archive or delete")

type sessionCleanupSuggestionRequest struct {
	Mode  string `json:"mode,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

type sessionCleanupSuggestionResponse struct {
	Mode          string                     `json:"mode"`
	Action        string                     `json:"action"`
	Source        string                     `json:"source"`
	LLMTier       string                     `json:"llm_tier,omitempty"`
	LLMProvider   string                     `json:"llm_provider,omitempty"`
	LLMModel      string                     `json:"llm_model,omitempty"`
	Suggestions   []sessionCleanupSuggestion `json:"suggestions"`
	Count         int                        `json:"count"`
	AnalyzedCount int                        `json:"analyzed_count"`
	ExcludedCount int                        `json:"excluded_count"`
	Warnings      []string                   `json:"warnings,omitempty"`
}

type sessionCleanupSuggestion struct {
	SessionID  string  `json:"session_id"`
	Title      string  `json:"title"`
	Action     string  `json:"action"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
	UpdatedAt  string  `json:"updated_at,omitempty"`
	ArchivedAt string  `json:"archived_at,omitempty"`
}

type sessionCleanupCandidate struct {
	SessionID       string  `json:"session_id"`
	Title           string  `json:"title"`
	Kind            string  `json:"kind"`
	Archived        bool    `json:"archived"`
	Pinned          bool    `json:"pinned"`
	AgeDays         float64 `json:"age_days"`
	ArchivedAgeDays float64 `json:"archived_age_days,omitempty"`
	MessageCount    int     `json:"message_count"`
	HasActivePlan   bool    `json:"has_active_plan"`
	PlanStatus      string  `json:"plan_status,omitempty"`
	FirstUser       string  `json:"first_user,omitempty"`
	LastUser        string  `json:"last_user,omitempty"`
	LastAssistant   string  `json:"last_assistant,omitempty"`
	UpdatedAt       string  `json:"updated_at"`
	ArchivedAt      string  `json:"archived_at,omitempty"`
}

type sessionCleanupLLMPayload struct {
	Suggestions []struct {
		SessionID  string  `json:"session_id"`
		Action     string  `json:"action"`
		Confidence float64 `json:"confidence"`
		Reason     string  `json:"reason"`
	} `json:"suggestions"`
}

func buildSessionCleanupSuggestions(ctx context.Context, store *session.Store, router llm.Router, req sessionCleanupSuggestionRequest, now time.Time) (sessionCleanupSuggestionResponse, error) {
	mode, err := normalizeSessionCleanupMode(req.Mode)
	if err != nil {
		return sessionCleanupSuggestionResponse{}, err
	}
	limit := normalizeSessionCleanupLimit(req.Limit)
	action := mode
	resp := sessionCleanupSuggestionResponse{
		Mode:        mode,
		Action:      action,
		Source:      "llm",
		Suggestions: []sessionCleanupSuggestion{},
	}
	candidates, excluded, warnings, err := collectSessionCleanupCandidates(store, mode, now)
	if err != nil {
		return sessionCleanupSuggestionResponse{}, err
	}
	resp.ExcludedCount = excluded
	resp.Warnings = append(resp.Warnings, warnings...)
	if len(candidates) == 0 {
		resp.Source = "none"
		return resp, nil
	}
	if len(candidates) > maxSessionCleanupCandidatesForLLM {
		candidates = candidates[:maxSessionCleanupCandidatesForLLM]
		resp.Warnings = append(resp.Warnings, fmt.Sprintf("analyzed first %d eligible sessions", maxSessionCleanupCandidatesForLLM))
	}
	resp.AnalyzedCount = len(candidates)

	client, resolution, err := router.ClientFor(llm.RoleSessionCleanup)
	if err != nil {
		return sessionCleanupSuggestionResponse{}, err
	}
	resp.LLMTier = string(resolution.Tier)
	resp.LLMProvider = resolution.Provider
	resp.LLMModel = resolution.Model

	raw, err := requestSessionCleanupLLM(ctx, client, mode, action, candidates, limit, now)
	if err != nil {
		return sessionCleanupSuggestionResponse{}, err
	}
	payload, err := parseSessionCleanupLLMPayload(raw)
	if err != nil {
		return sessionCleanupSuggestionResponse{}, err
	}
	byID := make(map[string]sessionCleanupCandidate, len(candidates))
	for _, candidate := range candidates {
		byID[candidate.SessionID] = candidate
	}
	ignored := 0
	for _, item := range payload.Suggestions {
		if len(resp.Suggestions) >= limit {
			break
		}
		sessionID := strings.TrimSpace(item.SessionID)
		if sessionID == "" || normalizeSessionCleanupAction(item.Action) != action {
			ignored++
			continue
		}
		candidate, ok := byID[sessionID]
		if !ok {
			ignored++
			continue
		}
		reason := strings.TrimSpace(item.Reason)
		if reason == "" {
			reason = "AI marked this session as a cleanup candidate."
		}
		resp.Suggestions = append(resp.Suggestions, sessionCleanupSuggestion{
			SessionID:  candidate.SessionID,
			Title:      candidate.Title,
			Action:     action,
			Confidence: clampConfidence(item.Confidence),
			Reason:     reason,
			UpdatedAt:  candidate.UpdatedAt,
			ArchivedAt: candidate.ArchivedAt,
		})
	}
	if ignored > 0 {
		resp.Warnings = append(resp.Warnings, fmt.Sprintf("ignored %d unsafe or invalid LLM suggestions", ignored))
	}
	resp.Count = len(resp.Suggestions)
	return resp, nil
}

func normalizeSessionCleanupMode(raw string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		mode = sessionCleanupModeArchive
	}
	switch mode {
	case sessionCleanupModeArchive, sessionCleanupModeDelete:
		return mode, nil
	default:
		return "", errInvalidSessionCleanupMode
	}
}

func normalizeSessionCleanupAction(raw string) string {
	action := strings.ToLower(strings.TrimSpace(raw))
	if action == "remove" {
		return sessionCleanupModeDelete
	}
	return action
}

func normalizeSessionCleanupLimit(limit int) int {
	if limit <= 0 {
		return defaultSessionCleanupLimit
	}
	if limit > maxSessionCleanupLimit {
		return maxSessionCleanupLimit
	}
	return limit
}

func collectSessionCleanupCandidates(store *session.Store, mode string, now time.Time) ([]sessionCleanupCandidate, int, []string, error) {
	if store == nil {
		return nil, 0, nil, fmt.Errorf("session store is not configured")
	}
	sessions, err := store.ListAll()
	if err != nil {
		return nil, 0, nil, err
	}
	candidates := make([]sessionCleanupCandidate, 0, len(sessions))
	warnings := []string{}
	excluded := 0
	for _, sess := range sessions {
		candidate, eligible, warning := buildSessionCleanupCandidate(store, sess, mode, now)
		if warning != "" {
			warnings = append(warnings, warning)
		}
		if !eligible {
			excluded++
			continue
		}
		candidates = append(candidates, candidate)
	}
	return candidates, excluded, warnings, nil
}

func buildSessionCleanupCandidate(store *session.Store, sess session.Session, mode string, now time.Time) (sessionCleanupCandidate, bool, string) {
	kind := sessionCleanupKind(sess)
	archived := sess.ArchivedAt != nil
	pinned := sess.PinnedAt != nil
	updatedAt := sess.UpdatedAt.UTC()
	age := now.Sub(updatedAt)
	if updatedAt.IsZero() {
		age = 0
	}
	if kind != "session" || pinned {
		return sessionCleanupCandidate{}, false, ""
	}

	tasks, err := store.GetTasks(sess.ID)
	if err != nil {
		return sessionCleanupCandidate{}, false, fmt.Sprintf("skipped %s tasks: %v", sess.ID, err)
	}
	planStatus := ""
	hasActivePlan := false
	if tasks.Plan != nil {
		planStatus = strings.TrimSpace(tasks.Plan.Status)
		if planStatus == "" {
			planStatus = session.PlanStatusExecuting
		}
		hasActivePlan = planStatus != session.PlanStatusCompleted && planStatus != session.PlanStatusAborted
	}
	if hasActivePlan {
		return sessionCleanupCandidate{}, false, ""
	}

	summary, warning := sessionCleanupTranscriptSummary(store, sess.ID)

	switch mode {
	case sessionCleanupModeArchive:
		if archived || age < sessionCleanupRecentProtection {
			return sessionCleanupCandidate{}, false, ""
		}
	case sessionCleanupModeDelete:
		if !archived || sess.ArchivedAt == nil {
			return sessionCleanupCandidate{}, false, ""
		}
		archivedAge := now.Sub(sess.ArchivedAt.UTC())
		if archivedAge < sessionCleanupRecentProtection {
			if archivedAge < sessionCleanupTrivialProtection || !isTrivialSessionCleanupTranscript(sess.Title, summary) {
				return sessionCleanupCandidate{}, false, warning
			}
		}
	}

	candidate := sessionCleanupCandidate{
		SessionID:     sess.ID,
		Title:         firstNonEmpty(strings.TrimSpace(sess.Title), sess.ID),
		Kind:          kind,
		Archived:      archived,
		Pinned:        pinned,
		AgeDays:       roundDays(age),
		MessageCount:  summary.messageCount,
		HasActivePlan: hasActivePlan,
		PlanStatus:    planStatus,
		FirstUser:     summary.firstUser,
		LastUser:      summary.lastUser,
		LastAssistant: summary.lastAssistant,
		UpdatedAt:     formatCleanupTime(sess.UpdatedAt),
	}
	if sess.ArchivedAt != nil {
		candidate.ArchivedAt = formatCleanupTime(*sess.ArchivedAt)
		candidate.ArchivedAgeDays = roundDays(now.Sub(sess.ArchivedAt.UTC()))
	}
	return candidate, true, warning
}

type sessionCleanupTranscript struct {
	messageCount  int
	firstUser     string
	lastUser      string
	lastAssistant string
}

func sessionCleanupTranscriptSummary(store *session.Store, sessionID string) (sessionCleanupTranscript, string) {
	messages, err := session.ReadMessages(store.TranscriptPath(sessionID))
	if err != nil {
		return sessionCleanupTranscript{}, fmt.Sprintf("skipped transcript snippets for %s: %v", sessionID, err)
	}
	out := sessionCleanupTranscript{messageCount: len(messages)}
	for _, msg := range messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		content := cleanupSnippet(msg.Content, 220)
		if content == "" {
			continue
		}
		switch role {
		case "user":
			if out.firstUser == "" {
				out.firstUser = content
			}
			out.lastUser = content
		case "assistant":
			out.lastAssistant = content
		}
	}
	return out, ""
}

func requestSessionCleanupLLM(ctx context.Context, client llm.Client, mode string, action string, candidates []sessionCleanupCandidate, limit int, now time.Time) (string, error) {
	candidateJSON, err := json.MarshalIndent(candidates, "", "  ")
	if err != nil {
		return "", err
	}
	userPrompt := fmt.Sprintf(
		"Current UTC: %s\nMode: %s\nTarget action: %s\n\n"+
			"Candidate sessions are pre-filtered by hard safety rules. Analyze them and return only the sessions that are good cleanup candidates.\n"+
			"For archive mode, choose conversations that look complete, temporary, generic, duplicated, or low-value to keep in the active list.\n"+
			"For delete mode, choose only archived conversations that look empty, accidental, test-only, duplicated, or safely represented elsewhere.\n"+
			"Prefer no suggestion over a risky suggestion. Return at most %d suggestions.\n\nCandidates:\n%s",
		now.UTC().Format(time.RFC3339),
		mode,
		action,
		limit,
		string(candidateJSON),
	)
	resp, err := client.Chat(ctx, []llm.ChatMessage{
		{
			Role: "system",
			Content: "You analyze TARS chat sessions for user-reviewed cleanup. Return strict JSON only. " +
				"Never suggest sessions absent from the candidate list. Never suggest delete unless the target action is delete.",
		},
		{
			Role:    "user",
			Content: userPrompt,
		},
	}, llm.ChatOptions{
		OnDelta:        func(string) {},
		ResponseFormat: sessionCleanupResponseFormat(),
	})
	if err != nil {
		return "", err
	}
	return resp.Message.Content, nil
}

func sessionCleanupResponseFormat() *llm.ResponseFormat {
	schema := json.RawMessage(`{
		"type": "object",
		"additionalProperties": false,
		"properties": {
			"suggestions": {
				"type": "array",
				"items": {
					"type": "object",
					"additionalProperties": false,
					"properties": {
						"session_id": {"type": "string"},
						"action": {"type": "string", "enum": ["archive", "delete"]},
						"confidence": {"type": "number"},
						"reason": {"type": "string"}
					},
					"required": ["session_id", "action", "confidence", "reason"]
				}
			}
		},
		"required": ["suggestions"]
	}`)
	return &llm.ResponseFormat{
		Type:   llm.ResponseFormatJSONSchema,
		Name:   "session_cleanup_suggestions",
		Schema: schema,
		Strict: true,
	}
}

func parseSessionCleanupLLMPayload(raw string) (sessionCleanupLLMPayload, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	var payload sessionCleanupLLMPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return sessionCleanupLLMPayload{}, err
	}
	return payload, nil
}

func sessionCleanupKind(sess session.Session) string {
	if sess.Kind == "main" {
		return "main"
	}
	if sess.Hidden {
		return "worker"
	}
	return "session"
}

func cleanupSnippet(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if limit <= 0 || len(runes) <= limit {
		return value
	}
	if limit <= 3 {
		return strings.TrimSpace(string(runes[:limit]))
	}
	return strings.TrimSpace(string(runes[:limit-3])) + "..."
}

func isTrivialSessionCleanupTranscript(title string, summary sessionCleanupTranscript) bool {
	if summary.messageCount == 0 {
		return true
	}
	if summary.messageCount > 2 {
		return false
	}
	if isCleanupGreeting(summary.firstUser) || isCleanupGreeting(summary.lastUser) {
		return true
	}
	if isGenericCleanupTitle(title) && summary.messageCount == 1 && cleanupTextRuneLen(summary.firstUser) <= 24 {
		return true
	}
	return false
}

func isGenericCleanupTitle(title string) bool {
	switch strings.ToLower(strings.TrimSpace(title)) {
	case "", "new chat", "untitled", "chat", "새 대화":
		return true
	default:
		return false
	}
}

func isCleanupGreeting(value string) bool {
	compact := compactCleanupText(value)
	switch compact {
	case "hi", "hello", "hey", "안녕", "안녕하세요", "하이", "ㅎㅇ":
		return true
	default:
		return false
	}
}

func compactCleanupText(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func cleanupTextRuneLen(value string) int {
	return len([]rune(strings.TrimSpace(value)))
}

func clampConfidence(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return math.Round(value*100) / 100
}

func roundDays(value time.Duration) float64 {
	if value <= 0 {
		return 0
	}
	return math.Round((value.Hours()/24)*10) / 10
}

func formatCleanupTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
