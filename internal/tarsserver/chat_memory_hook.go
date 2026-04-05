package tarsserver

import (
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
)

// chatMemoryHookInput carries the minimal context the per-turn memory
// hook needs. LLMClient is retained on the struct to keep callsites
// stable during the reflection migration; the hook itself no longer
// uses it because knowledge compilation moved to internal/reflection.
type chatMemoryHookInput struct {
	WorkspaceDir     string
	SessionID        string
	UserMessage      string
	AssistantMessage string
	AssistantTime    time.Time
	LLMClient        llm.Client
}

// applyPostChatMemoryHooks runs the cheap, user-visible work that must
// happen synchronously at the end of every chat turn:
//
//  1. Append a one-line summary to today's daily log so the user can
//     scan "what did I talk about today" immediately.
//  2. On the explicit "remember …" hot path, promote the user message
//     to a memory note + single experience so the next chat turn sees
//     it without waiting for nightly reflection.
//
// Everything else that used to live here — auto experience derivation
// from keywords, knowledge-base compilation via LLM — was moved to the
// nightly reflection runner (internal/reflection.MemoryJob). Per-turn
// latency is therefore bounded by cheap file I/O.
func applyPostChatMemoryHooks(input chatMemoryHookInput) error {
	dailyEntry := fmt.Sprintf(
		"chat session=%s user=%q assistant=%q",
		strings.TrimSpace(input.SessionID),
		trimForMemory(input.UserMessage, 120),
		trimForMemory(input.AssistantMessage, 160),
	)
	if err := memory.AppendDailyLog(input.WorkspaceDir, input.AssistantTime, dailyEntry); err != nil {
		return err
	}

	if shouldPromoteToMemory(input.UserMessage) {
		note := fmt.Sprintf("session %s user preference/fact: %s",
			strings.TrimSpace(input.SessionID),
			strings.TrimSpace(input.UserMessage),
		)
		if err := memory.AppendMemoryNote(input.WorkspaceDir, input.AssistantTime, note); err != nil {
			return err
		}
		exp := memory.Experience{
			Timestamp:     input.AssistantTime.UTC(),
			Category:      "preference",
			Summary:       trimForMemory(strings.TrimSpace(input.UserMessage), 220),
			Tags:          []string{"manual-memory"},
			SourceSession: strings.TrimSpace(input.SessionID),
			Importance:    8,
			Auto:          true,
		}
		if err := appendRememberExperience(input.WorkspaceDir, exp); err != nil {
			return err
		}
	}
	return nil
}

// appendRememberExperience writes a "remember" hot-path experience,
// skipping the write when an identical (category, summary) tuple is
// already recorded. This is an intentionally small dedup check —
// broader auto-derived experiences are handled in the nightly
// reflection job's appendExperienceIfNew helper.
func appendRememberExperience(workspaceDir string, exp memory.Experience) error {
	if strings.TrimSpace(exp.Summary) == "" {
		return nil
	}
	existing, err := memory.SearchExperiences(workspaceDir, memory.SearchOptions{
		Query: strings.TrimSpace(exp.Summary),
		Limit: 6,
	})
	if err == nil {
		normalized := strings.ToLower(strings.TrimSpace(exp.Summary))
		for _, item := range existing {
			if strings.ToLower(strings.TrimSpace(item.Summary)) == normalized &&
				strings.TrimSpace(item.Category) == strings.TrimSpace(exp.Category) {
				return nil
			}
		}
	}
	return memory.AppendExperience(workspaceDir, exp)
}

// shouldPromoteToMemory detects the explicit "remember …" hot path.
// Matching keyword families (EN + KR) so the user can trigger recall
// in either language.
func shouldPromoteToMemory(userMessage string) bool {
	lower := strings.ToLower(strings.TrimSpace(userMessage))
	return strings.HasPrefix(lower, "remember ") ||
		strings.HasPrefix(lower, "remember:") ||
		strings.Contains(lower, "remember this") ||
		strings.Contains(lower, "please remember") ||
		strings.HasPrefix(lower, "기억해") ||
		strings.Contains(lower, "기억해줘") ||
		strings.Contains(lower, "기억해 줘") ||
		strings.Contains(lower, "기억해두") ||
		strings.HasPrefix(lower, "메모해")
}

// trimForMemory normalizes whitespace and caps length so memory entries
// stay readable in the UI. Kept as a local helper because the per-turn
// hook is the only caller left in this package.
func trimForMemory(s string, max int) string {
	v := strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if max <= 0 || len(v) <= max {
		return v
	}
	return v[:max] + "..."
}
