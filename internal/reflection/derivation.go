package reflection

import (
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/memory"
)

// deriveTurnExperiences runs keyword-based rules to extract auto
// experiences from a single user→assistant turn. It mirrors the logic
// that used to live in internal/tarsserver/chat_memory_hook.go, which
// ran per-turn; reflection now runs it in a nightly batch.
//
// Only the user's message is read. The assistant's reply used to be mined
// too: any reply containing "completed" or "resolved" became a
// task_completed or error_resolved candidate holding its first 220
// characters. An agent ends almost every turn with such a report, so the
// review queue filled with status lines ("Verification complete", "Plan and
// contract ready") and nothing a later chat should recall. What an agent
// decides is worth keeping it saves itself with the memory tool.
//
// The function is cheap (string matching only) and does not touch the
// filesystem or the LLM. Persistence happens in the caller via the
// memory inbox review queue.
func deriveTurnExperiences(sessionID string, t turn, now time.Time) []memory.Experience {
	out := make([]memory.Experience, 0, 1)
	if exp, ok := deriveUserExperience(sessionID, t.UserMessage, now); ok {
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
