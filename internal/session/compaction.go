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
	BeforeRewrite func(summary string, compactedCount int, originalCount int) error
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
	head := messages[:cutoff]
	tail := messages[cutoff:]

	summary := buildCompactionSummary(head)
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

func buildCompactionSummary(messages []Message) string {
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
