package tarsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func maybeAutoCompactSession(workspaceDir, transcriptPath, sessionID string, client llm.Client, logger zerolog.Logger, semanticCfg ...memory.SemanticConfig) error {
	messages, err := session.ReadMessages(transcriptPath)
	if err != nil {
		return err
	}
	estimated := session.EstimateTokens(messages)
	if estimated < autoCompactTriggerTokens {
		return nil
	}

	now := time.Now().UTC()
	result, err := compactWithMemoryFlush(
		workspaceDir,
		transcriptPath,
		sessionID,
		autoCompactKeepRecent,
		autoCompactKeepTokens,
		autoCompactKeepShare,
		"",
		client,
		now,
		buildSemanticMemoryService(workspaceDir, firstSemanticConfig(semanticCfg...)),
	)
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

func compactWithMemoryFlush(workspaceDir, transcriptPath, sessionID string, keepRecent int, keepRecentTokens int, keepRecentFraction float64, instructions string, client llm.Client, now time.Time, semantic ...*memory.Service) (session.CompactResult, error) {
	memService := firstSemanticService(semantic...)
	return session.CompactTranscriptWithOptions(transcriptPath, keepRecent, now, session.CompactOptions{
		KeepRecentTokens:    keepRecentTokens,
		KeepRecentFraction:  keepRecentFraction,
		SummaryInstructions: instructions,
		SummaryBuilder: func(messages []session.Message) (string, error) {
			if client == nil {
				return session.BuildCompactionSummaryWithOptions(messages, session.CompactionSummaryOptions{
					FocusInstructions: instructions,
				}), nil
			}
			return buildLLMCompactionSummary(messages, client, now, instructions)
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
			if err := memory.AppendDailyLog(workspaceDir, now, fmt.Sprintf("compaction flush: %s | %s", note, preview)); err != nil {
				return err
			}
			if memService == nil {
				return nil
			}
			if err := memService.IndexCompactionSummary(context.Background(), sessionID, summary, now); err != nil {
				return nil
			}
			if client == nil {
				return nil
			}
			items, err := buildLLMCompactionMemories(summary, client, now)
			if err != nil {
				return nil
			}
			return memService.IndexCompactionMemories(context.Background(), sessionID, items, now)
		},
	})
}

func buildLLMCompactionSummary(messages []session.Message, client llm.Client, now time.Time, instructions string) (string, error) {
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

	focusBlock := ""
	if focus := strings.TrimSpace(instructions); focus != "" {
		focusBlock = "Requested focus:\n" + focus + "\n\n"
	}

	userPrompt := fmt.Sprintf(
		"Create a compact context summary for old chat messages.\n"+
			"%s"+
			"Keep concrete facts, goals, decisions, user preferences, unresolved tasks.\n"+
			"Return plain markdown under 900 characters.\n"+
			"Current UTC: %s\n\nMessages:\n%s",
		focusBlock,
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
		return session.BuildCompactionSummaryWithOptions(messages, session.CompactionSummaryOptions{
			FocusInstructions: instructions,
		}), nil
	}

	summary := strings.TrimSpace(resp.Message.Content)
	if summary == "" {
		return session.BuildCompactionSummaryWithOptions(messages, session.CompactionSummaryOptions{
			FocusInstructions: instructions,
		}), nil
	}
	if strings.Contains(summary, "[COMPACTION SUMMARY]") {
		return summary, nil
	}
	return "[COMPACTION SUMMARY]\n" + summary, nil
}

func buildLLMCompactionMemories(summary string, client llm.Client, now time.Time) ([]memory.CompactionMemory, error) {
	if client == nil {
		return nil, nil
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil, nil
	}
	userPrompt := fmt.Sprintf(
		"Extract durable memories from this compaction summary.\n"+
			"Return strict JSON with shape {\"memories\":[{\"category\":\"preference|fact|decision|task_pattern|project_note\",\"summary\":\"...\",\"importance\":1-10,\"tags\":[\"...\"]}]}.\n"+
			"Only keep durable items worth reusing in future sessions.\n"+
			"Current UTC: %s\n\nSummary:\n%s",
		now.UTC().Format(time.RFC3339),
		summary,
	)
	resp, err := client.Chat(context.Background(), []llm.ChatMessage{
		{
			Role:    "system",
			Content: "You extract durable memory candidates from summaries. Return strict JSON only.",
		},
		{
			Role:    "user",
			Content: userPrompt,
		},
	}, llm.ChatOptions{})
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(resp.Message.Content)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	var payload struct {
		Memories []struct {
			Category   string   `json:"category"`
			Summary    string   `json:"summary"`
			Importance int      `json:"importance"`
			Tags       []string `json:"tags"`
		} `json:"memories"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, err
	}
	items := make([]memory.CompactionMemory, 0, len(payload.Memories))
	for _, item := range payload.Memories {
		if strings.TrimSpace(item.Summary) == "" {
			continue
		}
		items = append(items, memory.CompactionMemory{
			Category:   strings.TrimSpace(item.Category),
			Summary:    strings.TrimSpace(item.Summary),
			Importance: item.Importance,
			Tags:       item.Tags,
		})
	}
	return items, nil
}

func firstSemanticService(values ...*memory.Service) *memory.Service {
	if len(values) == 0 {
		return nil
	}
	return values[0]
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
