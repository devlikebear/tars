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

type chatCompactionInfo struct {
	Applied               bool
	Mode                  string
	OriginalCount         int
	FinalCount            int
	CompactedCount        int
	TriggerTokens         int
	EstimatedTokensBefore int
	KeepRecentTokens      int
	KeepRecentFraction    float64
}

// compactionClient resolves the Router to a concrete client for the
// context_compactor role. Returns nil when router is nil or resolution
// fails, letting callers fall back to the deterministic non-LLM summary.
func compactionClient(router llm.Router, mode string) llm.Client {
	if router == nil || strings.EqualFold(strings.TrimSpace(mode), "deterministic") {
		return nil
	}
	client, _, err := router.ClientFor(llm.RoleContextCompactor)
	if err != nil {
		return nil
	}
	return client
}

func maybeAutoCompactSession(workspaceDir, transcriptPath, sessionID string, router llm.Router, logger zerolog.Logger, compaction chatCompactionOptions, semanticCfg ...memory.SemanticConfig) (chatCompactionInfo, error) {
	compaction = normalizeChatCompactionOptions(compaction)
	messages, err := session.ReadMessages(transcriptPath)
	if err != nil {
		return chatCompactionInfo{}, err
	}
	estimated := session.EstimateTokens(messages)
	info := chatCompactionInfo{
		TriggerTokens:         compaction.TriggerTokens,
		EstimatedTokensBefore: estimated,
		KeepRecentTokens:      compaction.KeepRecentTokens,
		KeepRecentFraction:    compaction.KeepRecentFraction,
	}
	if estimated < compaction.TriggerTokens {
		return info, nil
	}

	now := time.Now().UTC()
	result, summaryMode, err := compactWithMemoryFlush(
		workspaceDir,
		transcriptPath,
		sessionID,
		0,
		compaction,
		"",
		router,
		now,
		buildSemanticMemoryService(workspaceDir, firstSemanticConfig(semanticCfg...)),
	)
	if err != nil {
		return info, err
	}
	info.Applied = result.Compacted
	info.Mode = summaryMode
	info.OriginalCount = result.OriginalCount
	info.FinalCount = result.FinalCount
	info.CompactedCount = result.CompactedCount
	logger.Debug().
		Str("session_id", sessionID).
		Int("estimated_tokens", estimated).
		Bool("compacted", result.Compacted).
		Int("original_count", result.OriginalCount).
		Int("final_count", result.FinalCount).
		Str("summary_mode", summaryMode).
		Msg("auto compaction evaluated")
	return info, nil
}

func compactWithMemoryFlush(workspaceDir, transcriptPath, sessionID string, keepRecent int, compaction chatCompactionOptions, instructions string, router llm.Router, now time.Time, semantic ...*memory.Service) (session.CompactResult, string, error) {
	memService := firstSemanticService(semantic...)
	client := compactionClient(router, compaction.LLMMode)
	summaryMode := "deterministic"
	result, err := session.CompactTranscriptWithOptions(transcriptPath, keepRecent, now, session.CompactOptions{
		KeepRecentTokens:    compaction.KeepRecentTokens,
		KeepRecentFraction:  compaction.KeepRecentFraction,
		SummaryInstructions: instructions,
		SummaryBuilder: func(messages []session.Message) (string, error) {
			if client == nil {
				summaryMode = "deterministic"
				return session.BuildCompactionSummaryWithOptions(messages, session.CompactionSummaryOptions{
					FocusInstructions: instructions,
				}), nil
			}
			ctx := context.Background()
			if compaction.LLMTimeoutSeconds > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(context.Background(), time.Duration(compaction.LLMTimeoutSeconds)*time.Second)
				defer cancel()
			}
			built, usedLLM, err := buildLLMCompactionSummaryWithContext(ctx, messages, client, now, instructions)
			if usedLLM {
				summaryMode = "llm"
			} else {
				summaryMode = "deterministic"
			}
			return built, err
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
	if err != nil {
		return session.CompactResult{}, summaryMode, err
	}
	if !result.Compacted {
		summaryMode = ""
	}
	return result, summaryMode, nil
}

// buildLLMCompactionSummary continues to take a resolved llm.Client
// because its caller (compactWithMemoryFlush) has already resolved the
// router to a concrete client; passing the already-resolved client keeps
// this pure-function boundary intact and testable.
func buildLLMCompactionSummary(messages []session.Message, client llm.Client, now time.Time, instructions string) (string, error) {
	summary, _, err := buildLLMCompactionSummaryWithContext(context.Background(), messages, client, now, instructions)
	return summary, err
}

func buildLLMCompactionSummaryWithContext(ctx context.Context, messages []session.Message, client llm.Client, now time.Time, instructions string) (string, bool, error) {
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

	resp, err := client.Chat(ctx, []llm.ChatMessage{
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
		}), false, nil
	}

	summary := strings.TrimSpace(resp.Message.Content)
	if summary == "" {
		return session.BuildCompactionSummaryWithOptions(messages, session.CompactionSummaryOptions{
			FocusInstructions: instructions,
		}), false, nil
	}
	if strings.Contains(summary, "[COMPACTION SUMMARY]") {
		return summary, true, nil
	}
	return "[COMPACTION SUMMARY]\n" + summary, true, nil
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

func normalizeChatCompactionOptions(opts chatCompactionOptions) chatCompactionOptions {
	defaults := defaultChatToolingOptions().Compaction
	if opts.TriggerTokens <= 0 {
		opts.TriggerTokens = defaults.TriggerTokens
	}
	if opts.KeepRecentTokens <= 0 {
		opts.KeepRecentTokens = defaults.KeepRecentTokens
	}
	if opts.KeepRecentFraction <= 0 {
		opts.KeepRecentFraction = defaults.KeepRecentFraction
	}
	switch strings.TrimSpace(strings.ToLower(opts.LLMMode)) {
	case "", "auto":
		opts.LLMMode = defaults.LLMMode
	case "deterministic":
		opts.LLMMode = "deterministic"
	default:
		opts.LLMMode = defaults.LLMMode
	}
	if opts.LLMTimeoutSeconds <= 0 {
		opts.LLMTimeoutSeconds = defaults.LLMTimeoutSeconds
	}
	return opts
}

func shouldForceMemoryToolCall(userMessage string) bool {
	v := strings.ToLower(strings.TrimSpace(userMessage))
	if v == "" {
		return false
	}
	keywords := []string{
		// Explicit memory tool references
		"memory_search",
		"memory_get",
		"memory",
		// English memory keywords
		"remember",
		"recall",
		"history",
		"previous",
		"earlier",
		"what did i",
		"what do you remember",
		"preference",
		// English conversational continuity
		"last time",
		"continue where",
		"more about",
		"that thing",
		"we discussed",
		"we talked",
		"you said",
		"you told me",
		"you mentioned",
		// Korean memory keywords
		"기억",
		"메모리",
		"기록",
		"이전",
		"지난",
		"취향",
		// Korean conversational continuity
		"그거",
		"그때",
		"아까",
		"전에 말한",
		"지난번",
		"더 알려줘",
		"자세히",
		"그 얘기",
		"말했던",
		"얘기했던",
		"알려줬던",
	}
	for _, kw := range keywords {
		if strings.Contains(v, kw) {
			return true
		}
	}
	return false
}
