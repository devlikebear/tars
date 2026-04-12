package reflection

import (
	"context"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/memory"
)

// deriveTurnExperiences runs keyword-based rules to extract auto
// experiences from a single user→assistant turn. It mirrors the logic
// that used to live in internal/tarsserver/chat_memory_hook.go, which
// ran per-turn; reflection now runs it in a nightly batch.
//
// The function is cheap (string matching only) and does not touch the
// filesystem or the LLM. Persistence happens in the caller via
// appendExperienceIfNew.
func deriveTurnExperiences(sessionID string, t turn, now time.Time) []memory.Experience {
	out := make([]memory.Experience, 0, 2)
	if exp, ok := deriveUserExperience(sessionID, t.UserMessage, now); ok {
		out = append(out, exp)
	}
	if exp, ok := deriveAssistantExperience(sessionID, t.AssistantMessage, now); ok {
		out = append(out, exp)
	}
	return out
}

func deriveUserExperience(sessionID, userMessage string, now time.Time) (memory.Experience, bool) {
	lower := strings.ToLower(strings.TrimSpace(userMessage))
	exp := memory.Experience{
		Timestamp:     now.UTC(),
		SourceSession: strings.TrimSpace(sessionID),
		Importance:    6,
		Auto:          true,
	}
	switch {
	case strings.Contains(lower, "prefer") || strings.Contains(lower, "선호") || strings.Contains(lower, "취향"):
		exp.Category = "preference"
		exp.Summary = trimText(strings.TrimSpace(userMessage), 220)
		exp.Tags = []string{"auto", "user-preference"}
		return exp, exp.Summary != ""
	default:
		return memory.Experience{}, false
	}
}

func deriveAssistantExperience(sessionID, assistantMessage string, now time.Time) (memory.Experience, bool) {
	lower := strings.ToLower(strings.TrimSpace(assistantMessage))
	exp := memory.Experience{
		Timestamp:     now.UTC(),
		SourceSession: strings.TrimSpace(sessionID),
		Importance:    7,
		Auto:          true,
	}
	switch {
	case strings.Contains(lower, "completed") || strings.Contains(lower, "완료"):
		exp.Category = "task_completed"
		exp.Summary = trimText(strings.TrimSpace(assistantMessage), 220)
		exp.Tags = []string{"auto", "task"}
		return exp, exp.Summary != ""
	case strings.Contains(lower, "fixed") || strings.Contains(lower, "resolved") || strings.Contains(lower, "해결"):
		exp.Category = "error_resolved"
		exp.Summary = trimText(strings.TrimSpace(assistantMessage), 220)
		exp.Tags = []string{"auto", "error"}
		return exp, exp.Summary != ""
	default:
		return memory.Experience{}, false
	}
}

// shouldCompileKnowledge decides whether a single turn is worth sending
// to the LLM for knowledge-base compilation. The gate is intentionally
// loose — the LLM itself is responsible for returning empty arrays when
// nothing durable is present.
func shouldCompileKnowledge(t turn) bool {
	combined := strings.ToLower(strings.TrimSpace(t.UserMessage + "\n" + t.AssistantMessage))
	if combined == "" {
		return false
	}
	hints := []string{
		"prefer", "preference", "habit", "workflow", "policy", "decision", "owns",
		"선호", "취향", "습관", "워크플로", "규칙", "정책", "결정", "보유",
	}
	for _, hint := range hints {
		if strings.Contains(combined, hint) {
			return true
		}
	}
	return false
}

// appendExperienceIfNew persists an experience only if an identical
// summary+category pair does not already exist. Returns true when a new
// entry was written. Errors are swallowed — reflection jobs aggregate
// errors via their JobResult.Details rather than propagating.
func appendExperienceIfNew(ctx context.Context, backend memory.Backend, exp memory.Experience) bool {
	if strings.TrimSpace(exp.Summary) == "" {
		return false
	}
	existing, err := backend.SearchExperiences(ctx, memory.SearchOptions{
		Query: strings.TrimSpace(exp.Summary),
		Limit: 6,
	})
	if err == nil {
		normalized := strings.ToLower(strings.TrimSpace(exp.Summary))
		for _, item := range existing {
			if strings.ToLower(strings.TrimSpace(item.Summary)) == normalized &&
				strings.TrimSpace(item.Category) == strings.TrimSpace(exp.Category) {
				return false
			}
		}
	}
	if err := backend.AppendExperience(ctx, exp); err != nil {
		return false
	}
	return true
}

// trimText is a local copy of the tarsserver trimForMemory helper.
// Duplicated rather than imported to avoid a tarsserver→reflection
// cycle.
func trimText(s string, max int) string {
	v := strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if max <= 0 || len(v) <= max {
		return v
	}
	return v[:max] + "..."
}
