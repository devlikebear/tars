package prompt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
)

const (
	defaultStaticBudgetTokens   = 7000
	defaultRelevantBudgetTokens = 700
	defaultTotalBudgetTokens    = defaultStaticBudgetTokens + defaultRelevantBudgetTokens
	defaultRelevantResultLimit  = 6
	sessionFallbackMessageCount = 4
	sectionHeaderTokenCost      = 8
)

type relevantMemoryMatch struct {
	Source    string
	Snippet   string
	Score     int
	Timestamp time.Time
}

func buildRelevantMemorySection(opts BuildOptions, budgetTokens int) (string, int, int) {
	query := strings.TrimSpace(opts.Query)
	if query == "" {
		return "", 0, 0
	}
	matches := collectRelevantMemory(opts)
	if len(matches) == 0 {
		return "", 0, 0
	}

	remainingTokens := budgetTokens
	if remainingTokens <= 0 {
		remainingTokens = defaultRelevantBudgetTokens
	}
	if remainingTokens <= sectionHeaderTokenCost {
		return "", 0, 0
	}

	var section strings.Builder
	section.WriteString("## Prior Context\n\n")
	usedTokens := sectionHeaderTokenCost
	remainingTokens -= sectionHeaderTokenCost
	added := 0
	for _, match := range matches {
		if remainingTokens <= 0 {
			break
		}
		source := strings.TrimSpace(match.Source)
		if source == "" {
			source = "memory"
		}
		snippet := trimToBudget(match.Snippet, 360, max(1, remainingTokens-6))
		if snippet == "" {
			continue
		}
		tag := classifySourceTag(source)
		line := fmt.Sprintf("- [%s|%s] %s\n", tag, source, snippet)
		lineTokens := estimateTokens(line)
		if lineTokens > remainingTokens {
			snippet = trimToBudget(match.Snippet, 240, max(1, remainingTokens-8))
			if snippet == "" {
				continue
			}
			line = fmt.Sprintf("- [%s|%s] %s\n", tag, source, snippet)
			lineTokens = estimateTokens(line)
			if lineTokens > remainingTokens {
				break
			}
		}
		section.WriteString(line)
		remainingTokens -= lineTokens
		usedTokens += lineTokens
		added++
	}
	if added == 0 {
		return "", 0, 0
	}
	section.WriteString("\n")
	return section.String(), added, usedTokens
}

func collectRelevantMemory(opts BuildOptions) []relevantMemoryMatch {
	terms := normalizeRelevantTerms(opts.Query)
	if len(terms) == 0 && !opts.ForceRelevantMemory {
		return nil
	}
	if semantic := collectSemanticMatches(opts); len(semantic) > 0 {
		return semantic
	}

	matches := make([]relevantMemoryMatch, 0, 16)
	matches = append(matches, collectProjectDocumentMatches(opts, terms)...)
	matches = append(matches, collectBriefMatches(opts, terms)...)
	matches = append(matches, collectExperienceMatches(opts, terms)...)
	matches = append(matches, collectMemoryFileMatches(opts, terms)...)
	matches = append(matches, collectDailyLogMatches(opts, terms)...)
	matches = append(matches, collectSessionMatches(opts, terms)...)
	if len(matches) == 0 {
		return nil
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score == matches[j].Score {
			if matches[i].Timestamp.Equal(matches[j].Timestamp) {
				return matches[i].Source < matches[j].Source
			}
			return matches[i].Timestamp.After(matches[j].Timestamp)
		}
		return matches[i].Score > matches[j].Score
	})

	seen := map[string]struct{}{}
	filtered := make([]relevantMemoryMatch, 0, defaultRelevantResultLimit)
	for _, match := range matches {
		key := strings.ToLower(strings.TrimSpace(match.Snippet))
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		if match.Score < 100 && !opts.ForceRelevantMemory {
			continue
		}
		filtered = append(filtered, match)
		if len(filtered) >= defaultRelevantResultLimit {
			break
		}
	}
	return filtered
}

func collectSemanticMatches(opts BuildOptions) []relevantMemoryMatch {
	if opts.MemorySearcher == nil {
		return nil
	}
	hits, err := opts.MemorySearcher.Search(context.Background(), memory.SearchRequest{
		Query:     strings.TrimSpace(opts.Query),
		ProjectID: strings.TrimSpace(opts.ProjectID),
		SessionID: strings.TrimSpace(opts.SessionID),
		Limit:     defaultRelevantResultLimit,
	})
	if err != nil || len(hits) == 0 {
		return nil
	}
	matches := make([]relevantMemoryMatch, 0, len(hits))
	for _, hit := range hits {
		snippet := strings.TrimSpace(hit.Snippet)
		if snippet == "" {
			continue
		}
		score := int(hit.Score * 1000)
		matches = append(matches, relevantMemoryMatch{
			Source:    strings.TrimSpace(hit.Source),
			Snippet:   snippet,
			Score:     score,
			Timestamp: hit.Date,
		})
	}
	return matches
}

func collectProjectDocumentMatches(_ BuildOptions, _ []string) []relevantMemoryMatch {
	// Project documents are no longer available after project package removal.
	return nil
}

func collectBriefMatches(_ BuildOptions, _ []string) []relevantMemoryMatch {
	// Project briefs are no longer available after project package removal.
	return nil
}

func collectExperienceMatches(opts BuildOptions, terms []string) []relevantMemoryMatch {
	rows, err := memory.SearchExperiences(opts.WorkspaceDir, memory.SearchOptions{
		ProjectID: strings.TrimSpace(opts.ProjectID),
		Limit:     24,
	})
	if err != nil || len(rows) == 0 {
		rows, err = memory.SearchExperiences(opts.WorkspaceDir, memory.SearchOptions{Limit: 24})
		if err != nil || len(rows) == 0 {
			return nil
		}
	}
	out := make([]relevantMemoryMatch, 0, len(rows))
	for _, row := range rows {
		snippet := strings.TrimSpace(row.Summary)
		score := scoreRelevantText(snippet, terms)
		if score == 0 {
			continue
		}
		score += 140 + recencyScore(row.Timestamp)
		if strings.TrimSpace(row.ProjectID) != "" && strings.TrimSpace(row.ProjectID) == strings.TrimSpace(opts.ProjectID) {
			score += 40
		}
		if strings.TrimSpace(row.SourceSession) != "" && strings.TrimSpace(row.SourceSession) == strings.TrimSpace(opts.SessionID) {
			score += 25
		}
		source := "experience"
		if category := strings.TrimSpace(row.Category); category != "" {
			source += ":" + category
		}
		out = append(out, relevantMemoryMatch{
			Source:    source,
			Snippet:   snippet,
			Score:     score,
			Timestamp: row.Timestamp,
		})
	}
	return out
}

func collectMemoryFileMatches(opts BuildOptions, terms []string) []relevantMemoryMatch {
	path := filepath.Join(opts.WorkspaceDir, "MEMORY.md")
	stat, err := os.Stat(path)
	if err != nil {
		return nil
	}
	return collectFileLineMatches(path, "MEMORY.md", stat.ModTime().UTC(), opts, terms, 100)
}

func collectDailyLogMatches(opts BuildOptions, terms []string) []relevantMemoryMatch {
	paths, _ := filepath.Glob(filepath.Join(opts.WorkspaceDir, "memory", "*.md"))
	if len(paths) == 0 {
		return nil
	}
	sort.SliceStable(paths, func(i, j int) bool {
		return filepath.Base(paths[i]) > filepath.Base(paths[j])
	})
	out := make([]relevantMemoryMatch, 0, len(paths))
	for _, path := range paths {
		stat, err := os.Stat(path)
		if err != nil {
			continue
		}
		source := filepath.ToSlash(filepath.Join("memory", filepath.Base(path)))
		out = append(out, collectFileLineMatches(path, source, stat.ModTime().UTC(), opts, terms, 85)...)
		if len(out) >= 10 {
			break
		}
	}
	return out
}

func collectFileLineMatches(path, source string, timestamp time.Time, opts BuildOptions, terms []string, baseScore int) []relevantMemoryMatch {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(raw), "\n")
	out := make([]relevantMemoryMatch, 0, len(lines))
	for _, line := range lines {
		snippet := strings.TrimSpace(line)
		if snippet == "" || strings.HasPrefix(snippet, "#") {
			continue
		}
		score := scoreRelevantText(snippet, terms)
		if score == 0 {
			continue
		}
		score += baseScore + recencyScore(timestamp)
		if strings.TrimSpace(opts.ProjectID) != "" && strings.Contains(strings.ToLower(snippet), strings.ToLower(strings.TrimSpace(opts.ProjectID))) {
			score += 20
		}
		if strings.TrimSpace(opts.SessionID) != "" && strings.Contains(strings.ToLower(snippet), strings.ToLower(strings.TrimSpace(opts.SessionID))) {
			score += 10
		}
		out = append(out, relevantMemoryMatch{
			Source:    source,
			Snippet:   snippet,
			Score:     score,
			Timestamp: timestamp,
		})
	}
	return out
}

func collectSessionMatches(opts BuildOptions, terms []string) []relevantMemoryMatch {
	store := session.NewStore(opts.WorkspaceDir)
	sessions, err := store.List()
	if err != nil || len(sessions) == 0 {
		return nil
	}
	sort.SliceStable(sessions, func(i, j int) bool {
		if sessions[i].UpdatedAt.Equal(sessions[j].UpdatedAt) {
			return sessions[i].CreatedAt.After(sessions[j].CreatedAt)
		}
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	out := make([]relevantMemoryMatch, 0, len(sessions))
	for _, item := range sessions {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.ID) == strings.TrimSpace(opts.SessionID) {
			continue
		}
		msgs, err := session.ReadMessages(store.TranscriptPath(item.ID))
		if err != nil || len(msgs) == 0 {
			continue
		}
		snippet := latestCompactionSummary(msgs)
		source := "session:" + item.ID
		base := 110
		if snippet == "" {
			snippet = recentSessionExcerpt(msgs, sessionFallbackMessageCount)
			base = 75
		}
		if snippet == "" {
			snippet = searchSessionMessageContent(msgs, terms, 3)
			base = 90
		}
		snippet = strings.TrimSpace(snippet)
		if snippet == "" {
			continue
		}
		score := scoreRelevantText(snippet, terms)
		if score == 0 {
			continue
		}
		score += base + recencyScore(item.UpdatedAt)
		out = append(out, relevantMemoryMatch{
			Source:    source,
			Snippet:   snippet,
			Score:     score,
			Timestamp: item.UpdatedAt,
		})
	}
	return out
}

func latestCompactionSummary(messages []session.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "system" {
			continue
		}
		content := strings.TrimSpace(messages[i].Content)
		if strings.Contains(content, "[COMPACTION SUMMARY]") {
			return strings.TrimSpace(strings.Replace(content, "[COMPACTION SUMMARY]", "", 1))
		}
	}
	return ""
}

func recentSessionExcerpt(messages []session.Message, count int) string {
	if count <= 0 || len(messages) == 0 {
		return ""
	}
	start := len(messages) - count
	if start < 0 {
		start = 0
	}
	parts := make([]string, 0, len(messages)-start)
	for _, msg := range messages[start:] {
		content := strings.TrimSpace(strings.ReplaceAll(msg.Content, "\n", " "))
		if content == "" {
			continue
		}
		if len(content) > 160 {
			content = content[:160] + "..."
		}
		parts = append(parts, fmt.Sprintf("[%s] %s", msg.Role, content))
	}
	return strings.Join(parts, " | ")
}

func normalizeRelevantTerms(query string) []string {
	replacer := strings.NewReplacer(
		".", " ", ",", " ", "?", " ", "!", " ", ":", " ", ";", " ",
		"(", " ", ")", " ", "\"", " ", "'", " ", "\n", " ", "\t", " ",
	)
	cleaned := strings.ToLower(strings.TrimSpace(replacer.Replace(query)))
	if cleaned == "" {
		return nil
	}
	stopwords := map[string]struct{}{
		"the": {}, "a": {}, "an": {}, "and": {}, "or": {}, "to": {}, "of": {}, "in": {}, "on": {},
		"what": {}, "do": {}, "i": {}, "you": {}, "me": {}, "my": {}, "about": {}, "is": {}, "are": {},
		"did": {}, "was": {}, "were": {}, "that": {}, "this": {}, "it": {}, "remember": {},
		"prefer": {}, "preference": {}, "like": {}, "likes": {},
	}
	seen := map[string]struct{}{}
	terms := make([]string, 0, 8)
	for _, part := range strings.Fields(cleaned) {
		if len(part) < 2 {
			continue
		}
		if _, skip := stopwords[part]; skip {
			continue
		}
		if _, exists := seen[part]; exists {
			continue
		}
		seen[part] = struct{}{}
		terms = append(terms, part)
	}
	return terms
}

func scoreRelevantText(content string, terms []string) int {
	lower := strings.ToLower(strings.TrimSpace(content))
	if lower == "" {
		return 0
	}
	score := 0
	matches := 0
	for _, term := range terms {
		if strings.Contains(lower, term) {
			score += 25
			matches++
		}
	}
	if matches == 0 {
		return 0
	}
	if matches == len(terms) && len(terms) > 1 {
		score += 30
	}
	if strings.Contains(lower, strings.Join(terms, " ")) && len(terms) > 1 {
		score += 20
	}
	return score
}

func recencyScore(ts time.Time) int {
	if ts.IsZero() {
		return 0
	}
	age := time.Since(ts.UTC())
	switch {
	case age <= 48*time.Hour:
		return 20
	case age <= 7*24*time.Hour:
		return 12
	case age <= 30*24*time.Hour:
		return 6
	default:
		return 0
	}
}

func classifySourceTag(source string) string {
	switch {
	case strings.HasPrefix(source, "session:"):
		return "conversation"
	case strings.HasPrefix(source, "experience"):
		return "experience"
	case strings.HasPrefix(source, "projects/"):
		return "project"
	case strings.HasPrefix(source, "_shared/"):
		return "brief"
	case source == "MEMORY.md":
		return "memory"
	case strings.HasPrefix(source, "memory/"):
		return "daily"
	default:
		return "context"
	}
}

func searchSessionMessageContent(msgs []session.Message, terms []string, maxMatches int) string {
	var parts []string
	for _, msg := range msgs {
		if msg.Role == "system" || msg.Role == "tool" {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		score := scoreRelevantText(content, terms)
		if score > 0 {
			if len(content) > 160 {
				content = content[:160] + "..."
			}
			parts = append(parts, fmt.Sprintf("[%s] %s", msg.Role, content))
			if len(parts) >= maxMatches {
				break
			}
		}
	}
	return strings.Join(parts, " | ")
}
