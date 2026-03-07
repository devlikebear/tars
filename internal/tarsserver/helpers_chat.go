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
