package pulse

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/pulse/autofix"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/tool"
	zlog "github.com/rs/zerolog/log"
)

// FailedChatCandidate describes a chat session whose most recent activity is
// a halted turn — either a tool error with no follow-up assistant message, or
// a user message that the LLM never finished responding to within the
// configured timeout. The session must still have active work; otherwise it
// is treated as completed and ignored.
//
// FailedChatCandidate is distinct from StalledChatCandidate: stalled chats
// are blocked on user input (the assistant asked a question), failed chats
// are blocked on a recoverable error or an aborted run. They are detected
// independently and resumed by separate autofixes.
type FailedChatCandidate struct {
	SessionID         string    `json:"session_id"`
	Title             string    `json:"title,omitempty"`
	LastMessageID     string    `json:"last_message_id,omitempty"`
	LastActivityAt    time.Time `json:"last_activity_at"`
	AgeMinutes        int       `json:"age_minutes"`
	FailureKind       string    `json:"failure_kind"`
	FailingToolName   string    `json:"failing_tool_name,omitempty"`
	FailurePreview    string    `json:"failure_preview,omitempty"`
	AutoResumeEnabled bool      `json:"auto_resume_enabled"`
	CanAutoResume     bool      `json:"can_auto_resume"`
	BlockReason       string    `json:"block_reason,omitempty"`
}

const (
	// FailedChatKindToolError indicates the last persisted message is a tool
	// response with ToolIsError=true and no assistant follow-up.
	FailedChatKindToolError = "tool_error"
	// FailedChatKindNoResponse indicates the last conversational message is a
	// user message with no assistant follow-up after the auto-resume window.
	FailedChatKindNoResponse = "no_response"
)

// DetectFailedChatCandidates returns sessions whose last activity indicates
// a halted turn. The detection is intentionally conservative — only the two
// shapes above are recognized, and any failure whose last action was a
// high-risk tool is marked unsafe to auto-resume so we never double-apply a
// mutation.
func DetectFailedChatCandidates(ctx context.Context, source ChatSessionSource, now time.Time) ([]FailedChatCandidate, error) {
	if source == nil {
		return nil, nil
	}
	sessions, err := source.List()
	if err != nil {
		return nil, err
	}
	var candidates []FailedChatCandidate
	for _, sess := range sessions {
		if err := ctx.Err(); err != nil {
			return candidates, err
		}
		tasks, err := source.GetTasks(sess.ID)
		if err != nil {
			zlog.Logger.Debug().Err(err).Str("session_id", sess.ID).Msg("pulse: skip failed-chat task read failure")
			continue
		}
		if !hasActiveSessionWork(tasks) {
			continue
		}
		messages, err := session.ReadMessages(source.TranscriptPath(sess.ID))
		if err != nil {
			zlog.Logger.Debug().Err(err).Str("session_id", sess.ID).Msg("pulse: skip failed-chat transcript read failure")
			continue
		}
		candidate, ok := classifyFailedChatTail(sess, messages, now)
		if !ok {
			continue
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

func classifyFailedChatTail(sess session.Session, messages []session.Message, now time.Time) (FailedChatCandidate, bool) {
	last, ok := lastNonSystemMessage(messages)
	if !ok {
		return FailedChatCandidate{}, false
	}

	consent := sess.AutomationConsent
	afterMinutes := session.DefaultAutoResumeAfterMinutes
	autoResumeEnabled := false
	if consent != nil {
		afterMinutes = consent.EffectiveAutoResumeAfterMinutes()
		autoResumeEnabled = consent.AllowsAutoResume()
	}

	candidate := FailedChatCandidate{
		SessionID:         sess.ID,
		Title:             sess.Title,
		LastMessageID:     last.ID,
		AutoResumeEnabled: autoResumeEnabled,
	}

	switch strings.TrimSpace(last.Role) {
	case "tool":
		if !last.ToolIsError {
			return FailedChatCandidate{}, false
		}
		candidate.FailureKind = FailedChatKindToolError
		candidate.FailingToolName = last.ToolName
		candidate.FailurePreview = trimPulsePreview(last.Content, 240)
	case "user":
		// Only flag when there is no assistant or tool response after this
		// user turn — i.e. the user spoke and nothing came back.
		if hasResponseAfter(messages, last) {
			return FailedChatCandidate{}, false
		}
		candidate.FailureKind = FailedChatKindNoResponse
		candidate.FailurePreview = trimPulsePreview(last.Content, 240)
	default:
		return FailedChatCandidate{}, false
	}

	candidate.LastActivityAt = last.Timestamp.UTC()
	if candidate.LastActivityAt.IsZero() {
		candidate.LastActivityAt = sess.UpdatedAt.UTC()
	}
	if now.Sub(candidate.LastActivityAt) < time.Duration(afterMinutes)*time.Minute {
		return FailedChatCandidate{}, false
	}
	candidate.AgeMinutes = int(now.Sub(candidate.LastActivityAt).Minutes())

	if candidate.FailureKind == FailedChatKindToolError && tool.IsHighRiskToolName(candidate.FailingToolName) {
		candidate.BlockReason = "high_risk_failure"
	}
	candidate.CanAutoResume = autoResumeEnabled && candidate.BlockReason == ""

	return candidate, true
}

func lastNonSystemMessage(messages []session.Message) (session.Message, bool) {
	for i := len(messages) - 1; i >= 0; i-- {
		role := strings.TrimSpace(messages[i].Role)
		if role == "" || role == "system" {
			continue
		}
		return messages[i], true
	}
	return session.Message{}, false
}

func hasResponseAfter(messages []session.Message, target session.Message) bool {
	seen := false
	for _, msg := range messages {
		if !seen {
			if msg.Timestamp.Equal(target.Timestamp) && strings.TrimSpace(msg.Role) == strings.TrimSpace(target.Role) && msg.Content == target.Content {
				seen = true
			}
			continue
		}
		switch strings.TrimSpace(msg.Role) {
		case "assistant", "tool":
			return true
		}
	}
	return false
}

// scanFailedChats produces a SignalKindFailedChat when at least one session
// has a halted turn that meets the auto-resume window.
func (s *Scanner) scanFailedChats(ctx context.Context, now time.Time) *Signal {
	if s.sources.ChatSessions == nil {
		return nil
	}
	candidates, err := DetectFailedChatCandidates(ctx, s.sources.ChatSessions, now)
	if err != nil {
		zlog.Logger.Warn().Err(err).Msg("pulse: failed-chat scan failed; skipping this tick")
		return nil
	}
	if len(candidates) == 0 {
		return nil
	}
	primary := candidates[0]
	details := newChatSignalDetails("failed_count", candidates, autofix.AutoResumeFailedChatName, map[string]any{
		"failure_kind": primary.FailureKind,
		"failing_tool": primary.FailingToolName,
	})
	return &Signal{
		Kind:     SignalKindFailedChat,
		Severity: SeverityWarn,
		Summary:  fmt.Sprintf("%d chat session(s) halted with a recoverable failure", len(candidates)),
		Details:  details,
		At:       now,
	}
}
