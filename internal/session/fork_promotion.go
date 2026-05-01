package session

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

const (
	defaultForkPromotionMaxCandidates   = 12
	defaultForkPromotionMaxSummaryRunes = 220
)

// ForkPromotionOptions controls deterministic candidate extraction from a fork.
type ForkPromotionOptions struct {
	Now             time.Time
	MaxCandidates   int
	MaxSummaryRunes int
}

// ForkPromotionCandidate is a reviewable insight from a forked session that can
// be queued into Memory Inbox for explicit user approval.
type ForkPromotionCandidate struct {
	ID                  string    `json:"id"`
	SessionID           string    `json:"session_id"`
	ParentSessionID     string    `json:"parent_session_id"`
	RootSessionID       string    `json:"root_session_id,omitempty"`
	ForkedFromMessageID string    `json:"forked_from_message_id,omitempty"`
	ForkedFromIndex     *int      `json:"forked_from_index,omitempty"`
	MessageID           string    `json:"message_id"`
	MessageIndex        int       `json:"message_index"`
	Role                string    `json:"role"`
	Category            string    `json:"category"`
	Summary             string    `json:"summary"`
	CreatedAt           time.Time `json:"created_at"`
}

// DetectForkPromotionCandidates extracts reusable post-fork insights without
// mutating any session transcript. It is intentionally deterministic so the UI
// can refresh and submit stable candidate IDs.
func DetectForkPromotionCandidates(sess Session, messages []Message, opts ForkPromotionOptions) []ForkPromotionCandidate {
	sessionID := strings.TrimSpace(sess.ID)
	parentID := strings.TrimSpace(sess.ParentSessionID)
	if sessionID == "" || parentID == "" || len(messages) == 0 {
		return []ForkPromotionCandidate{}
	}
	startIndex := forkPromotionStartIndex(sess, messages)
	if startIndex < 0 || startIndex >= len(messages) {
		return []ForkPromotionCandidate{}
	}
	now := opts.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	limit := opts.MaxCandidates
	if limit <= 0 {
		limit = defaultForkPromotionMaxCandidates
	}
	maxRunes := opts.MaxSummaryRunes
	if maxRunes < 8 {
		maxRunes = defaultForkPromotionMaxSummaryRunes
	}

	out := make([]ForkPromotionCandidate, 0, limit)
	for i := startIndex; i < len(messages); i++ {
		msg := messages[i]
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		if IsTasksInjectionMessage(msg) || strings.TrimSpace(msg.ToolName) != "" {
			continue
		}
		summary := compactPromotionSummary(msg.Content, maxRunes)
		if !looksPromotableSummary(summary) {
			continue
		}
		createdAt := now
		if createdAt.IsZero() && !msg.Timestamp.IsZero() {
			createdAt = msg.Timestamp.UTC()
		}
		candidate := ForkPromotionCandidate{
			SessionID:           sessionID,
			ParentSessionID:     parentID,
			RootSessionID:       strings.TrimSpace(sess.RootSessionID),
			ForkedFromMessageID: strings.TrimSpace(sess.ForkedFromMessageID),
			ForkedFromIndex:     cloneIntPtr(sess.ForkedFromIndex),
			MessageID:           strings.TrimSpace(msg.ID),
			MessageIndex:        i,
			Role:                role,
			Category:            promotionCategory(summary),
			Summary:             summary,
			CreatedAt:           createdAt,
		}
		candidate.ID = stableForkPromotionID(candidate)
		out = append(out, candidate)
		if len(out) >= limit {
			break
		}
	}
	if out == nil {
		return []ForkPromotionCandidate{}
	}
	return out
}

func forkPromotionStartIndex(sess Session, messages []Message) int {
	if sess.ForkedFromIndex != nil {
		return *sess.ForkedFromIndex + 1
	}
	messageID := strings.TrimSpace(sess.ForkedFromMessageID)
	if messageID == "" {
		return -1
	}
	for i, msg := range messages {
		if strings.TrimSpace(msg.ID) == messageID {
			return i + 1
		}
	}
	return -1
}

func compactPromotionSummary(value string, maxRunes int) string {
	text := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if text == "" {
		return ""
	}
	runes := []rune(text)
	if maxRunes > 0 && len(runes) > maxRunes {
		text = strings.TrimSpace(string(runes[:maxRunes-3])) + "..."
	}
	return text
}

func looksPromotableSummary(summary string) bool {
	if len([]rune(strings.TrimSpace(summary))) < 24 {
		return false
	}
	lower := strings.ToLower(summary)
	for _, marker := range []string{
		"decision", "decided", "choose", "chose", "keep ", "prefer", "preference",
		"remember", "always", "never", "should", "must", "workflow", "process",
		"run ", "command", "verify", "test", "scope",
		"결정", "선택", "선호", "기억", "항상", "절대", "해야", "워크플로우",
		"프로세스", "명령", "검증", "테스트", "범위",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func promotionCategory(summary string) string {
	lower := strings.ToLower(summary)
	for _, marker := range []string{"prefer", "preference", "i want", "i like"} {
		if strings.Contains(lower, marker) {
			return "preference"
		}
	}
	for _, marker := range []string{"선호", "원해", "좋아"} {
		if strings.Contains(summary, marker) {
			return "preference"
		}
	}
	for _, marker := range []string{"decision", "decided", "choose", "chose"} {
		if strings.Contains(lower, marker) {
			return "decision"
		}
	}
	for _, marker := range []string{"결정", "선택"} {
		if strings.Contains(summary, marker) {
			return "decision"
		}
	}
	for _, marker := range []string{"workflow", "process", "run ", "command", "verify", "test"} {
		if strings.Contains(lower, marker) {
			return "procedure"
		}
	}
	for _, marker := range []string{"워크플로우", "프로세스", "명령", "검증", "테스트"} {
		if strings.Contains(summary, marker) {
			return "procedure"
		}
	}
	return "fact"
}

func stableForkPromotionID(candidate ForkPromotionCandidate) string {
	parts := []string{
		strings.TrimSpace(candidate.SessionID),
		strings.TrimSpace(candidate.ParentSessionID),
		strings.TrimSpace(candidate.MessageID),
		strings.ToLower(strings.TrimSpace(candidate.Category)),
		strings.ToLower(strings.TrimSpace(candidate.Summary)),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "fork_prom_" + hex.EncodeToString(sum[:])[:16]
}

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
