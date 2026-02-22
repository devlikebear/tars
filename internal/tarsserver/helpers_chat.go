package tarsserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/llm"
	"github.com/devlikebear/tarsncase/internal/memory"
	"github.com/devlikebear/tarsncase/internal/session"
	"github.com/rs/zerolog"
)

func maybeAutoCompactSession(workspaceDir, transcriptPath, sessionID string, client llm.Client, logger zerolog.Logger) error {
	messages, err := session.ReadMessages(transcriptPath)
	if err != nil {
		return err
	}
	estimated := session.EstimateTokens(messages)
	if estimated < autoCompactTriggerTokens {
		return nil
	}

	now := time.Now().UTC()
	result, err := compactWithMemoryFlush(workspaceDir, transcriptPath, sessionID, autoCompactKeepRecent, autoCompactKeepTokens, client, now)
	if err != nil {
		return err
	}
	logger.Debug().
		Str("session_id", sessionID).
		Int("estimated_tokens", estimated).
		Bool("compacted", result.Compacted).
		Int("original_count", result.OriginalCount).
		Int("final_count", result.FinalCount).
		Msg("auto compaction evaluated")
	return nil
}

func compactWithMemoryFlush(workspaceDir, transcriptPath, sessionID string, keepRecent int, keepRecentTokens int, client llm.Client, now time.Time) (session.CompactResult, error) {
	return session.CompactTranscriptWithOptions(transcriptPath, keepRecent, now, session.CompactOptions{
		KeepRecentTokens: keepRecentTokens,
		SummaryBuilder: func(messages []session.Message) (string, error) {
			if client == nil {
				return session.BuildCompactionSummary(messages), nil
			}
			return buildLLMCompactionSummary(messages, client, now)
		},
		BeforeRewrite: func(summary string, compactedCount int, originalCount int) error {
			note := fmt.Sprintf("session %s compacted %d/%d messages", sessionID, compactedCount, originalCount)
			if err := memory.AppendMemoryNote(workspaceDir, now, note); err != nil {
				return err
			}

			preview := strings.ReplaceAll(strings.TrimSpace(summary), "\n", " ")
			if len(preview) > 240 {
				preview = preview[:240] + "..."
			}
			return memory.AppendDailyLog(workspaceDir, now, fmt.Sprintf("compaction flush: %s | %s", note, preview))
		},
	})
}

func buildLLMCompactionSummary(messages []session.Message, client llm.Client, now time.Time) (string, error) {
	const maxMessages = 80
	msgs := messages
	if len(msgs) > maxMessages {
		msgs = msgs[len(msgs)-maxMessages:]
	}

	var b strings.Builder
	for _, m := range msgs {
		content := strings.TrimSpace(strings.ReplaceAll(m.Content, "\n", " "))
		if len(content) > 240 {
			content = content[:240] + "..."
		}
		_, _ = fmt.Fprintf(&b, "- [%s] %s\n", m.Role, content)
	}

	userPrompt := fmt.Sprintf(
		"Create a compact context summary for old chat messages.\n"+
			"Keep concrete facts, goals, decisions, user preferences, unresolved tasks.\n"+
			"Return plain markdown under 900 characters.\n"+
			"Current UTC: %s\n\nMessages:\n%s",
		now.UTC().Format(time.RFC3339),
		b.String(),
	)

	resp, err := client.Chat(context.Background(), []llm.ChatMessage{
		{
			Role:    "system",
			Content: "You are a precise summarizer for context compaction. Output only the summary text.",
		},
		{
			Role:    "user",
			Content: userPrompt,
		},
	}, llm.ChatOptions{})
	if err != nil {
		return session.BuildCompactionSummary(messages), nil
	}

	summary := strings.TrimSpace(resp.Message.Content)
	if summary == "" {
		return session.BuildCompactionSummary(messages), nil
	}
	return "[COMPACTION SUMMARY]\n" + summary, nil
}

func writeChatMemory(workspaceDir, sessionID, projectID, userMessage, assistantMessage string, now time.Time) error {
	dailyEntry := fmt.Sprintf(
		"chat session=%s user=%q assistant=%q",
		sessionID,
		trimForMemory(userMessage, 120),
		trimForMemory(assistantMessage, 160),
	)
	if err := memory.AppendDailyLog(workspaceDir, now, dailyEntry); err != nil {
		return err
	}

	if shouldPromoteToMemory(userMessage) {
		note := fmt.Sprintf("session %s user preference/fact: %s", sessionID, strings.TrimSpace(userMessage))
		if err := memory.AppendMemoryNote(workspaceDir, now, note); err != nil {
			return err
		}
		_ = appendExperienceIfNew(workspaceDir, memory.Experience{
			Timestamp:     now.UTC(),
			Category:      "preference",
			Summary:       trimForMemory(strings.TrimSpace(userMessage), 220),
			Tags:          []string{"manual-memory"},
			SourceSession: sessionID,
			ProjectID:     strings.TrimSpace(projectID),
			Importance:    8,
			Auto:          true,
		})
	}

	if exp, ok := deriveAutoExperience(sessionID, strings.TrimSpace(projectID), userMessage, assistantMessage, now); ok {
		_ = appendExperienceIfNew(workspaceDir, exp)
	}
	return nil
}

func shouldPromoteToMemory(userMessage string) bool {
	lower := strings.ToLower(strings.TrimSpace(userMessage))
	return strings.HasPrefix(lower, "remember ") ||
		strings.HasPrefix(lower, "remember:") ||
		strings.HasPrefix(lower, "기억해") ||
		strings.HasPrefix(lower, "메모해")
}

func shouldForceMemoryToolCall(userMessage string) bool {
	v := strings.ToLower(strings.TrimSpace(userMessage))
	if v == "" {
		return false
	}
	keywords := []string{
		"memory_search",
		"memory_get",
		"memory",
		"remember",
		"recall",
		"history",
		"previous",
		"earlier",
		"what did i",
		"what do you remember",
		"preference",
		"기억",
		"메모리",
		"기록",
		"이전",
		"지난",
		"취향",
	}
	for _, kw := range keywords {
		if strings.Contains(v, kw) {
			return true
		}
	}
	return false
}

func trimForMemory(s string, max int) string {
	v := strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if max <= 0 || len(v) <= max {
		return v
	}
	return v[:max] + "..."
}

func deriveAutoExperience(sessionID, projectID, userMessage, assistantMessage string, now time.Time) (memory.Experience, bool) {
	lowerUser := strings.ToLower(strings.TrimSpace(userMessage))
	lowerAssistant := strings.ToLower(strings.TrimSpace(assistantMessage))

	exp := memory.Experience{
		Timestamp:     now.UTC(),
		SourceSession: strings.TrimSpace(sessionID),
		ProjectID:     strings.TrimSpace(projectID),
		Importance:    6,
		Auto:          true,
	}

	switch {
	case strings.Contains(lowerUser, "prefer") || strings.Contains(lowerUser, "선호") || strings.Contains(lowerUser, "취향"):
		exp.Category = "preference"
		exp.Summary = trimForMemory(strings.TrimSpace(userMessage), 220)
		exp.Tags = []string{"auto", "user-preference"}
		return exp, exp.Summary != ""
	case strings.Contains(lowerAssistant, "completed") || strings.Contains(lowerAssistant, "완료"):
		exp.Category = "task_completed"
		exp.Summary = trimForMemory(strings.TrimSpace(assistantMessage), 220)
		exp.Tags = []string{"auto", "task"}
		exp.Importance = 7
		return exp, exp.Summary != ""
	case strings.Contains(lowerAssistant, "fixed") || strings.Contains(lowerAssistant, "resolved") || strings.Contains(lowerAssistant, "해결"):
		exp.Category = "error_resolved"
		exp.Summary = trimForMemory(strings.TrimSpace(assistantMessage), 220)
		exp.Tags = []string{"auto", "error"}
		exp.Importance = 7
		return exp, exp.Summary != ""
	default:
		return memory.Experience{}, false
	}
}

func appendExperienceIfNew(workspaceDir string, exp memory.Experience) error {
	if strings.TrimSpace(exp.Summary) == "" {
		return nil
	}
	existing, err := memory.SearchExperiences(workspaceDir, memory.SearchOptions{
		Query:     strings.TrimSpace(exp.Summary),
		ProjectID: strings.TrimSpace(exp.ProjectID),
		Limit:     6,
	})
	if err == nil {
		normalizedSummary := strings.ToLower(strings.TrimSpace(exp.Summary))
		for _, item := range existing {
			if strings.ToLower(strings.TrimSpace(item.Summary)) == normalizedSummary && strings.TrimSpace(item.Category) == strings.TrimSpace(exp.Category) {
				return nil
			}
		}
	}
	return memory.AppendExperience(workspaceDir, exp)
}
