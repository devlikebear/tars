package session

import (
	"fmt"
	"strings"
	"time"
)

const (
	DefaultKeepRecentMessages = 20
	MinKeepRecentMessages     = 5
	MaxKeepRecentMessages     = 200
	DefaultKeepRecentTokens   = 12000
	MinKeepRecentTokens       = 512
	MaxKeepRecentTokens       = 64000
)

type CompactResult struct {
	Compacted      bool
	OriginalCount  int
	FinalCount     int
	CompactedCount int
	Summary        string
}

func CompactTranscript(path string, keepRecent int, now time.Time) (CompactResult, error) {
	return CompactTranscriptWithOptions(path, keepRecent, now, CompactOptions{})
}

type CompactOptions struct {
	BeforeRewrite    func(summary string, compactedCount int, originalCount int) error
	SummaryBuilder   func(messages []Message) (string, error)
	KeepRecentTokens int
}

func CompactTranscriptWithOptions(path string, keepRecent int, now time.Time, opts CompactOptions) (CompactResult, error) {
	if keepRecent <= 0 {
		keepRecent = DefaultKeepRecentMessages
	}
	if keepRecent < MinKeepRecentMessages {
		keepRecent = MinKeepRecentMessages
	}
	if keepRecent > MaxKeepRecentMessages {
		keepRecent = MaxKeepRecentMessages
	}

	messages, err := ReadMessages(path)
	if err != nil {
		return CompactResult{}, err
	}
	if len(messages) == 0 {
		return CompactResult{Compacted: false}, nil
	}
	keepRecentTokens := normalizeKeepRecentTokens(opts.KeepRecentTokens)
	var head []Message
	var tail []Message

	if keepRecentTokens > 0 {
		cutoff := cutoffIndexByTokenBudget(messages, keepRecentTokens)
		if cutoff <= 0 {
			return CompactResult{
				Compacted:      false,
				OriginalCount:  len(messages),
				FinalCount:     len(messages),
				CompactedCount: 0,
				Summary:        "",
			}, nil
		}
		head = messages[:cutoff]
		tail = messages[cutoff:]
	} else {
		if len(messages) <= keepRecent+1 {
			return CompactResult{
				Compacted:      false,
				OriginalCount:  len(messages),
				FinalCount:     len(messages),
				CompactedCount: 0,
				Summary:        "",
			}, nil
		}
		cutoff := len(messages) - keepRecent
		head = messages[:cutoff]
		tail = messages[cutoff:]
	}

	var summary string
	if opts.SummaryBuilder != nil {
		built, err := opts.SummaryBuilder(head)
		if err != nil {
			return CompactResult{}, err
		}
		summary = strings.TrimSpace(built)
	}
	if summary == "" {
		summary = BuildCompactionSummary(head)
	}
	compactionMessage := Message{
		Role:      "system",
		Content:   summary,
		Timestamp: now.UTC(),
	}
	if opts.BeforeRewrite != nil {
		if err := opts.BeforeRewrite(summary, len(head), len(messages)); err != nil {
			return CompactResult{}, err
		}
	}

	replaced := make([]Message, 0, 1+len(tail))
	replaced = append(replaced, compactionMessage)
	replaced = append(replaced, tail...)
	if err := RewriteMessages(path, replaced); err != nil {
		return CompactResult{}, err
	}

	return CompactResult{
		Compacted:      true,
		OriginalCount:  len(messages),
		FinalCount:     len(replaced),
		CompactedCount: len(head),
		Summary:        summary,
	}, nil
}

func BuildCompactionSummary(messages []Message) string {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "[COMPACTION SUMMARY]\nCompacted %d messages.\n", len(messages))

	previewCount := len(messages)
	if previewCount > 24 {
		previewCount = 24
	}
	if previewCount > 0 {
		_, _ = fmt.Fprintf(&b, "\nHighlights:\n")
		for i := 0; i < previewCount; i++ {
			msg := messages[i]
			content := strings.TrimSpace(msg.Content)
			if content == "" {
				content = "(empty)"
			}
			if len(content) > 160 {
				content = content[:160] + "..."
			}
			_, _ = fmt.Fprintf(&b, "- [%s] %s\n", msg.Role, content)
		}
	}
	if len(messages) > previewCount {
		_, _ = fmt.Fprintf(&b, "- ... and %d more messages\n", len(messages)-previewCount)
	}
	return b.String()
}

func normalizeKeepRecentTokens(v int) int {
	if v <= 0 {
		return 0
	}
	if v < MinKeepRecentTokens {
		return MinKeepRecentTokens
	}
	if v > MaxKeepRecentTokens {
		return MaxKeepRecentTokens
	}
	return v
}

func cutoffIndexByTokenBudget(messages []Message, budget int) int {
	if len(messages) <= 1 {
		return 0
	}

	kept := 0
	tokens := 0
	for i := len(messages) - 1; i >= 0; i-- {
		cost := estimateMessageTokenCost(messages[i])
		if tokens+cost > budget {
			break
		}
		tokens += cost
		kept++
	}
	if kept == 0 {
		kept = 1
	}
	cutoff := len(messages) - kept
	if cutoff < 0 {
		cutoff = 0
	}
	return cutoff
}

func estimateMessageTokenCost(msg Message) int {
	cost := len(msg.Content) / 4
	if cost < 1 {
		cost = 1
	}
	return cost
}

func EstimateTokens(messages []Message) int {
	total := 0
	for _, msg := range messages {
		total += estimateMessageTokenCost(msg)
	}
	return total
}
