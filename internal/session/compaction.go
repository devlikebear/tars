package session

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
)

const (
	DefaultKeepRecentMessages = 20
	MinKeepRecentMessages     = 5
	MaxKeepRecentMessages     = 200
	DefaultKeepRecentTokens   = 12000
	MinKeepRecentTokens       = 1
	MaxKeepRecentTokens       = 64000
	DefaultKeepRecentFraction = 0.30
	MinKeepRecentFraction     = 0.05
	MaxKeepRecentFraction     = 0.90
)

var (
	backtickIdentifierPattern = regexp.MustCompile("`[^`\n]+`")
	pathIdentifierPattern     = regexp.MustCompile(`(?:[A-Za-z0-9_.-]+/)+[A-Za-z0-9_.-]+`)
	slashCommandPattern       = regexp.MustCompile(`/[A-Za-z][A-Za-z0-9_-]*`)
	referencePattern          = regexp.MustCompile(`#\d+|[a-f0-9]{7,40}|[A-Z][A-Z0-9_]{2,}`)
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
	BeforeRewrite       func(summary string, compactedCount int, originalCount int) error
	SummaryBuilder      func(messages []Message) (string, error)
	KeepRecentTokens    int
	KeepRecentFraction  float64
	SummaryInstructions string
}

type CompactionSummaryOptions struct {
	FocusInstructions string
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
	keepRecentFraction := normalizeKeepRecentFraction(opts.KeepRecentFraction)
	var head []Message
	var tail []Message

	if keepRecentFraction > 0 {
		cutoff := cutoffIndexByRecentTokenShare(messages, keepRecentFraction, keepRecentTokens, keepRecent)
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
	} else if keepRecentTokens > 0 {
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
		summary = BuildCompactionSummaryWithOptions(head, CompactionSummaryOptions{
			FocusInstructions: opts.SummaryInstructions,
		})
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
	return BuildCompactionSummaryWithOptions(messages, CompactionSummaryOptions{})
}

func BuildCompactionSummaryWithOptions(messages []Message, opts CompactionSummaryOptions) string {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "[COMPACTION SUMMARY]\nCompacted %d messages.\n", len(messages))
	if len(messages) == 0 {
		return b.String()
	}

	if span := compactionTimeSpan(messages); span != "" {
		_, _ = fmt.Fprintf(&b, "Span: %s\n", span)
	}

	if focus := normalizeSummaryLine(opts.FocusInstructions, 240); focus != "" {
		appendCompactionSection(&b, "Requested Focus", []string{focus})
	}

	appendCompactionSection(&b, "Current Goal", currentGoalLines(messages))
	appendCompactionSection(&b, "Constraints And Preferences", constraintLines(messages))
	appendCompactionSection(&b, "Key Facts", keyFactLines(messages))
	appendCompactionSection(&b, "Identifiers To Preserve", identifierLines(messages))
	appendCompactionSection(&b, "Recent Context", recentContextLines(messages))
	appendCompactionSection(&b, "Open State", openStateLines(messages))

	if strings.Count(b.String(), "\n\n") == 0 {
		appendCompactionSection(&b, "Recent Context", recentContextLines(messages))
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

func normalizeKeepRecentFraction(v float64) float64 {
	if v <= 0 {
		return 0
	}
	if v < MinKeepRecentFraction {
		return MinKeepRecentFraction
	}
	if v > MaxKeepRecentFraction {
		return MaxKeepRecentFraction
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
	if kept < 2 && len(messages) >= 2 {
		kept = 2
	}
	cutoff := len(messages) - kept
	if cutoff < 0 {
		cutoff = 0
	}
	return cutoff
}

func cutoffIndexByRecentTokenShare(messages []Message, fraction float64, minTokenBudget int, minMessages int) int {
	if len(messages) <= 1 {
		return 0
	}
	fraction = normalizeKeepRecentFraction(fraction)
	if fraction == 0 {
		return 0
	}
	budget := int(math.Ceil(float64(EstimateTokens(messages)) * fraction))
	if budget < minTokenBudget {
		budget = minTokenBudget
	}
	cutoff := cutoffIndexByTokenBudget(messages, budget)
	return adjustCompactionCutoff(messages, cutoff, minMessages)
}

func adjustCompactionCutoff(messages []Message, cutoff int, minMessages int) int {
	if len(messages) <= 1 {
		return 0
	}
	if minMessages < 1 {
		minMessages = 1
	}
	maxCutoff := len(messages) - minMessages
	if maxCutoff < 1 {
		maxCutoff = 1
	}
	if cutoff > maxCutoff {
		cutoff = maxCutoff
	}
	if cutoff <= 0 {
		return 0
	}
	for i := cutoff; i < len(messages); i++ {
		if strings.EqualFold(strings.TrimSpace(messages[i].Role), "user") {
			return i
		}
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

func appendCompactionSection(b *strings.Builder, title string, lines []string) {
	lines = uniqueNonEmptyLines(lines)
	if len(lines) == 0 {
		return
	}
	_, _ = fmt.Fprintf(b, "\n%s:\n", title)
	for _, line := range lines {
		_, _ = fmt.Fprintf(b, "- %s\n", line)
	}
}

func currentGoalLines(messages []Message) []string {
	firstUser := firstMessageForRole(messages, "user")
	lastUser := lastMessageForRole(messages, "user")
	lines := make([]string, 0, 2)
	if firstUser != nil {
		lines = append(lines, normalizeSummaryLine(firstUser.Content, 220))
	}
	if lastUser != nil && (firstUser == nil || normalizeSummaryLine(lastUser.Content, 220) != normalizeSummaryLine(firstUser.Content, 220)) {
		lines = append(lines, normalizeSummaryLine(lastUser.Content, 220))
	}
	return lines
}

func constraintLines(messages []Message) []string {
	return keywordLines(messages, []string{
		"must", "should", "avoid", "do not", "don't", "keep", "use", "only", "never", "always",
		"required", "rule", "focus", "반드시", "하지 마", "유지", "사용", "규칙", "집중",
	}, 5)
}

func keyFactLines(messages []Message) []string {
	lines := keywordLines(messages, []string{
		"already", "current", "existing", "found", "supports", "because", "error", "failed",
		"path", "file", "issue", "branch", "현재", "기존", "이미", "발견", "오류", "파일", "브랜치",
	}, 5)
	if len(lines) > 0 {
		return lines
	}
	firstAssistant := firstMessageForRole(messages, "assistant")
	if firstAssistant == nil {
		firstAssistant = firstMessageForRole(messages, "system")
	}
	if firstAssistant == nil {
		return nil
	}
	return []string{normalizeSummaryLine(firstAssistant.Content, 220)}
}

func identifierLines(messages []Message) []string {
	lines := make([]string, 0, 12)
	seen := map[string]struct{}{}
	for _, msg := range messages {
		for _, token := range collectIdentifiers(msg.Content) {
			if _, ok := seen[token]; ok {
				continue
			}
			seen[token] = struct{}{}
			lines = append(lines, token)
			if len(lines) == 12 {
				return lines
			}
		}
	}
	return lines
}

func recentContextLines(messages []Message) []string {
	start := len(messages) - 6
	if start < 0 {
		start = 0
	}
	lines := make([]string, 0, len(messages)-start)
	for _, msg := range messages[start:] {
		lines = append(lines, formatCompactionMessageLine(msg, 180))
	}
	return lines
}

func openStateLines(messages []Message) []string {
	lines := make([]string, 0, 2)
	if lastUser := lastMessageForRole(messages, "user"); lastUser != nil {
		lines = append(lines, formatCompactionMessageLine(*lastUser, 180))
	}
	if lastAssistant := lastMessageForRole(messages, "assistant"); lastAssistant != nil {
		lines = append(lines, formatCompactionMessageLine(*lastAssistant, 180))
	}
	if len(lines) == 0 && len(messages) > 0 {
		lines = append(lines, formatCompactionMessageLine(messages[len(messages)-1], 180))
	}
	return lines
}

func keywordLines(messages []Message, keywords []string, limit int) []string {
	lines := make([]string, 0, limit)
	seen := map[string]struct{}{}
	for _, msg := range messages {
		content := strings.ToLower(strings.TrimSpace(msg.Content))
		if content == "" {
			continue
		}
		matched := false
		for _, kw := range keywords {
			if strings.Contains(content, kw) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		line := formatCompactionMessageLine(msg, 180)
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		lines = append(lines, line)
		if len(lines) == limit {
			return lines
		}
	}
	return lines
}

func collectIdentifiers(content string) []string {
	ordered := make([]string, 0, 12)
	seen := map[string]struct{}{}
	for _, pattern := range []*regexp.Regexp{
		backtickIdentifierPattern,
		pathIdentifierPattern,
		slashCommandPattern,
		referencePattern,
	} {
		for _, match := range pattern.FindAllString(content, -1) {
			token := strings.TrimSpace(match)
			if token == "" {
				continue
			}
			if _, ok := seen[token]; ok {
				continue
			}
			seen[token] = struct{}{}
			ordered = append(ordered, token)
		}
	}
	return ordered
}

func uniqueNonEmptyLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	seen := map[string]struct{}{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		out = append(out, line)
	}
	return out
}

func firstMessageForRole(messages []Message, role string) *Message {
	for i := range messages {
		if strings.EqualFold(strings.TrimSpace(messages[i].Role), role) {
			return &messages[i]
		}
	}
	return nil
}

func lastMessageForRole(messages []Message, role string) *Message {
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.EqualFold(strings.TrimSpace(messages[i].Role), role) {
			return &messages[i]
		}
	}
	return nil
}

func formatCompactionMessageLine(msg Message, maxLen int) string {
	return fmt.Sprintf("[%s] %s", strings.TrimSpace(msg.Role), normalizeSummaryLine(msg.Content, maxLen))
}

func normalizeSummaryLine(content string, maxLen int) string {
	content = strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
	if content == "" {
		return "(empty)"
	}
	if maxLen > 0 && len(content) > maxLen {
		return content[:maxLen] + "..."
	}
	return content
}

func compactionTimeSpan(messages []Message) string {
	if len(messages) == 0 {
		return ""
	}
	first := messages[0].Timestamp.UTC()
	last := messages[len(messages)-1].Timestamp.UTC()
	if first.IsZero() || last.IsZero() {
		return ""
	}
	return first.Format(time.RFC3339) + " to " + last.Format(time.RFC3339)
}
