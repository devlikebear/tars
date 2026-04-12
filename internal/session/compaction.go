package session

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode"
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
	SummaryBuilder      func(messages []Message, previousContext string) (string, error)
	KeepRecentTokens    int
	KeepRecentFraction  float64
	SummaryInstructions string
	// PreloadedMessages supplies already-read messages to avoid a second ReadMessages
	// call on the same path. When set, ReadMessages is skipped, preventing a
	// reentrant-lock deadlock when the caller already holds the path lock.
	PreloadedMessages []Message
}

type CompactionSummaryOptions struct {
	FocusInstructions string
	PreviousContext   string
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

	var messages []Message
	if len(opts.PreloadedMessages) > 0 {
		messages = opts.PreloadedMessages
	} else {
		var err error
		messages, err = ReadMessages(path)
		if err != nil {
			return CompactResult{}, err
		}
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

	// Stacking carry-forward: extract previous compaction summary before pruning
	var previousContext string
	if len(head) > 0 && head[0].Role == "system" && strings.Contains(head[0].Content, "[COMPACTION SUMMARY]") {
		previousContext = extractCompactionBody(head[0].Content)
		head = head[1:]
	}

	// Pre-compaction: prune long tool results to prevent code dumps in summary
	prunedHead := pruneToolResults(head, defaultToolPruneMaxLen)

	var summary string
	if opts.SummaryBuilder != nil {
		built, err := opts.SummaryBuilder(prunedHead, previousContext)
		if err != nil {
			return CompactResult{}, err
		}
		summary = strings.TrimSpace(built)
	}
	if summary == "" {
		summary = BuildCompactionSummaryWithOptions(prunedHead, CompactionSummaryOptions{
			FocusInstructions: opts.SummaryInstructions,
			PreviousContext:   previousContext,
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

	// Cross-section deduplication
	globalSeen := map[string]struct{}{}
	appendDedup := func(title string, lines []string) {
		fresh := make([]string, 0, len(lines))
		for _, line := range uniqueNonEmptyLines(lines) {
			if _, dup := globalSeen[line]; dup {
				continue
			}
			globalSeen[line] = struct{}{}
			fresh = append(fresh, line)
		}
		appendCompactionSection(&b, title, fresh)
	}

	// Prior context from previous compaction (populated by stacking carry-forward)
	if opts.PreviousContext != "" {
		appendDedup("Prior Context", []string{opts.PreviousContext})
	}

	if focus := normalizeSummaryLine(opts.FocusInstructions, 240); focus != "" {
		appendDedup("Requested Focus", []string{focus})
	}

	appendDedup("Topic", currentGoalLines(messages))
	appendDedup("Key Decisions", keyDecisionLines(messages))
	appendDedup("Active Identifiers", identifierLines(messages))
	appendDedup("Current State", currentStateLines(messages))

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
	return estimateStringTokens(msg.Content)
}

// EstimateMessageTokenCost is the exported wrapper for single-message token estimation.
func EstimateMessageTokenCost(msg Message) int {
	return estimateMessageTokenCost(msg)
}

func estimateStringTokens(s string) int {
	if len(s) == 0 {
		return 1
	}
	units := 0
	for _, r := range s {
		if isCJKRune(r) {
			units += 6 // CJK rune: ~1.5 tokens → weight as 6 quarter-tokens
		} else {
			units += 1 // ASCII/Latin byte: ~0.25 tokens
		}
	}
	cost := units / 4
	if cost < 1 {
		cost = 1
	}
	return cost
}

func isCJKRune(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hangul, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hiragana, r)
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

func keyDecisionLines(messages []Message) []string {
	return keywordLines(messages, []string{
		"decided", "decision", "agreed", "chosen", "approach", "strategy", "plan",
		"priority", "confirmed", "approved", "rejected",
		"결정", "합의", "방향", "전략", "우선순위", "확인", "승인",
	}, 5)
}

func identifierLines(messages []Message) []string {
	lines := make([]string, 0, 12)
	seen := map[string]struct{}{}
	for _, msg := range messages {
		if strings.EqualFold(strings.TrimSpace(msg.Role), "tool") {
			continue
		}
		for _, token := range collectIdentifiers(msg.Content) {
			// Normalize: strip backticks for dedup comparison
			normalized := strings.Trim(token, "`")
			if _, ok := seen[normalized]; ok {
				continue
			}
			seen[normalized] = struct{}{}
			lines = append(lines, token)
			if len(lines) == 12 {
				return lines
			}
		}
	}
	return lines
}

func currentStateLines(messages []Message) []string {
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
		if strings.EqualFold(strings.TrimSpace(msg.Role), "tool") {
			continue
		}
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
		return ""
	}
	if maxLen > 0 && len(content) > maxLen {
		return content[:maxLen] + "..."
	}
	return content
}

const defaultToolPruneMaxLen = 200

// pruneToolResults replaces long tool message content with a short placeholder.
// This prevents code dumps from polluting the compaction summary.
// Returns a new slice; the input is not mutated.
func pruneToolResults(messages []Message, maxContentLen int) []Message {
	out := make([]Message, len(messages))
	copy(out, messages)
	for i := range out {
		if !strings.EqualFold(strings.TrimSpace(out[i].Role), "tool") {
			continue
		}
		if len(out[i].Content) <= maxContentLen {
			continue
		}
		name := out[i].ToolName
		if name == "" {
			name = "unknown"
		}
		out[i].Content = fmt.Sprintf("[tool:%s] output cleared (%d chars)", name, len(out[i].Content))
	}
	return out
}

// extractCompactionBody strips the [COMPACTION SUMMARY] header and message
// count line, returning only the body sections for carry-forward.
func extractCompactionBody(summary string) string {
	// Skip lines: "[COMPACTION SUMMARY]", "Compacted N messages.", "Span: ..."
	lines := strings.Split(summary, "\n")
	bodyStart := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "[COMPACTION SUMMARY]") ||
			strings.HasPrefix(trimmed, "Compacted ") || strings.HasPrefix(trimmed, "Span:") {
			bodyStart = i + 1
			continue
		}
		break
	}
	if bodyStart >= len(lines) {
		return ""
	}
	body := strings.TrimSpace(strings.Join(lines[bodyStart:], "\n"))
	// Truncate to keep carry-forward compact
	if len(body) > 800 {
		body = body[:800] + "..."
	}
	return body
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
