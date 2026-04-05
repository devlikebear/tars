package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
)

const (
	defaultMemorySearchLimit = 8
	maxMemorySearchLimit     = 30
	maxSessionsToSearch      = 10
)

type memorySearchMatch struct {
	Source  string `json:"source"`
	Date    string `json:"date"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
}

type memorySearchResult struct {
	Query   string              `json:"query"`
	Limit   int                 `json:"limit"`
	Results []memorySearchMatch `json:"results"`
	Message string              `json:"message,omitempty"`
}

type memorySearchCandidate struct {
	Match     memorySearchMatch
	Score     int
	Timestamp time.Time
}

func NewMemorySearchTool(workspaceDir string, semantic *memory.Service) Tool {
	return Tool{
		Name:        "memory_search",
		Description: "Search knowledge-base notes, MEMORY.md, daily memory logs, and optionally past session transcripts for text snippets with source metadata.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "query":{"type":"string","description":"Search query text."},
    "limit":{"type":"integer","minimum":1,"maximum":30,"default":8},
    "include_memory":{"type":"boolean","default":true},
    "include_daily":{"type":"boolean","default":true},
    "include_knowledge":{"type":"boolean","default":false,"description":"Search knowledge-base notes only when explicitly requested."},
    "include_sessions":{"type":"boolean","default":false,"description":"Search past session transcripts for conversational continuity. Always set to true when called."}
  },
  "required":["query"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			var input struct {
				Query            string `json:"query"`
				Limit            *int   `json:"limit,omitempty"`
				IncludeMemory    *bool  `json:"include_memory,omitempty"`
				IncludeDaily     *bool  `json:"include_daily,omitempty"`
				IncludeKnowledge *bool  `json:"include_knowledge,omitempty"`
				IncludeSessions  *bool  `json:"include_sessions,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return memorySearchErrorResult(fmt.Sprintf("invalid arguments: %v", err)), nil
			}

			query := strings.TrimSpace(input.Query)
			if query == "" {
				return memorySearchErrorResult("query is required"), nil
			}

			limit := resolvePositiveBoundedInt(defaultMemorySearchLimit, maxMemorySearchLimit, input.Limit)

			includeMemory := true
			if input.IncludeMemory != nil {
				includeMemory = *input.IncludeMemory
			}
			includeDaily := true
			if input.IncludeDaily != nil {
				includeDaily = *input.IncludeDaily
			}
			includeKnowledge := false
			if input.IncludeKnowledge != nil {
				includeKnowledge = *input.IncludeKnowledge
			}
			includeSessions := false
			if input.IncludeSessions != nil {
				includeSessions = *input.IncludeSessions
			}

			matches, message := runMemorySearch(context.Background(), workspaceDir, query, limit, includeMemory, includeDaily, includeKnowledge, includeSessions, semantic)
			payload := memorySearchResult{
				Query:   query,
				Limit:   limit,
				Results: matches,
				Message: message,
			}
			raw, err := json.Marshal(payload)
			if err != nil {
				return Result{}, fmt.Errorf("marshal memory search result: %w", err)
			}

			return Result{
				Content: []ContentBlock{
					{Type: "text", Text: string(raw)},
				},
			}, nil
		},
	}
}

type memorySearchFile struct {
	Path   string
	Source string
	Date   string
	MTime  time.Time
}

func runMemorySearch(ctx context.Context, workspaceDir, query string, limit int, includeMemory, includeDaily, includeKnowledge, includeSessions bool, semantic *memory.Service) ([]memorySearchMatch, string) {
	results := make([]memorySearchMatch, 0, limit)
	terms := memorySearchTerms(query)
	seen := map[string]struct{}{}
	appendMatch := func(candidate memorySearchCandidate) bool {
		match := candidate.Match
		key := strings.ToLower(strings.TrimSpace(match.Source + "|" + match.Snippet))
		if match.Line > 0 {
			key = fmt.Sprintf("%s|%d", key, match.Line)
		}
		if key == "|" {
			return false
		}
		if _, exists := seen[key]; exists {
			return false
		}
		seen[key] = struct{}{}
		results = append(results, match)
		return len(results) >= limit
	}

	if semantic != nil {
		hits, err := semantic.Search(ctx, memory.SearchRequest{
			Query: query,
			Limit: limit,
		})
		if err == nil && len(hits) > 0 {
			for _, hit := range hits {
				if appendMatch(memorySearchCandidate{
					Match: memorySearchMatch{
						Source:  hit.Source,
						Date:    hit.Date.UTC().Format("2006-01-02"),
						Line:    0,
						Snippet: hit.Snippet,
					},
					Score:     1000,
					Timestamp: hit.Date.UTC(),
				}) {
					return results, ""
				}
			}
		}
	}

	candidates := make([]memorySearchCandidate, 0, limit*4)
	hasSearchableSource := false

	if includeKnowledge {
		knowledgeMatches, hasKnowledge := searchKnowledgeNotes(workspaceDir, query, terms, limit)
		hasSearchableSource = hasSearchableSource || hasKnowledge
		candidates = append(candidates, knowledgeMatches...)
	}

	experienceMatches, hasExperiences := searchExperienceLog(workspaceDir, query, terms, limit)
	hasSearchableSource = hasSearchableSource || hasExperiences
	candidates = append(candidates, experienceMatches...)

	files := collectMemorySearchFiles(workspaceDir, includeMemory, includeDaily)
	if len(files) > 0 {
		hasSearchableSource = true
		candidates = append(candidates, searchMemoryFiles(files, query, terms, limit)...)
	}

	if includeSessions && len(results) < limit {
		sessionResults, hasSessions := searchSessionTranscripts(workspaceDir, query, terms, limit-len(results))
		hasSearchableSource = hasSearchableSource || hasSessions
		candidates = append(candidates, sessionResults...)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			if candidates[i].Timestamp.Equal(candidates[j].Timestamp) {
				return candidates[i].Match.Source < candidates[j].Match.Source
			}
			return candidates[i].Timestamp.After(candidates[j].Timestamp)
		}
		return candidates[i].Score > candidates[j].Score
	})
	for _, candidate := range candidates {
		if appendMatch(candidate) {
			return results, ""
		}
	}

	if len(results) == 0 {
		if !hasSearchableSource {
			return nil, "no memory sources found"
		}
		return nil, "no matches found"
	}
	return results, ""
}

func searchKnowledgeNotes(workspaceDir, query string, terms []string, limit int) ([]memorySearchCandidate, bool) {
	if limit <= 0 {
		return nil, false
	}
	items, err := memory.NewKnowledgeStore(workspaceDir, nil).List(memory.KnowledgeListOptions{
		Limit: max(limit*6, 50),
	})
	if err != nil {
		return nil, false
	}
	results := make([]memorySearchCandidate, 0, len(items))
	for _, item := range items {
		snippet := strings.TrimSpace(item.Summary)
		if snippet == "" {
			snippet = strings.TrimSpace(item.Body)
		}
		if snippet == "" {
			snippet = strings.TrimSpace(item.Title)
		}
		score := scoreMemorySearchText(query, terms, strings.Join([]string{item.Title, item.Summary, item.Body, strings.Join(item.Tags, " ")}, "\n"))
		if score == 0 {
			continue
		}
		if len(snippet) > 200 {
			snippet = snippet[:200] + "..."
		}
		results = append(results, memorySearchCandidate{
			Match: memorySearchMatch{
				Source:  item.Path,
				Date:    item.UpdatedAt.UTC().Format("2006-01-02"),
				Line:    0,
				Snippet: snippet,
			},
			Score:     score + 120,
			Timestamp: item.UpdatedAt.UTC(),
		})
	}
	return results, len(items) > 0
}

func searchSessionTranscripts(workspaceDir, query string, terms []string, limit int) ([]memorySearchCandidate, bool) {
	store := session.NewStore(workspaceDir)
	sessions, err := store.List()
	if err != nil || len(sessions) == 0 {
		return nil, false
	}
	sort.SliceStable(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	if len(sessions) > maxSessionsToSearch {
		sessions = sessions[:maxSessionsToSearch]
	}

	var results []memorySearchCandidate
	for _, item := range sessions {
		if strings.TrimSpace(item.ID) == "" {
			continue
		}
		msgs, err := session.ReadMessages(store.TranscriptPath(item.ID))
		if err != nil || len(msgs) == 0 {
			continue
		}
		for _, msg := range msgs {
			if msg.Role == "system" || msg.Role == "tool" {
				continue
			}
			content := strings.TrimSpace(msg.Content)
			score := scoreMemorySearchText(query, terms, content)
			if content == "" || score == 0 {
				continue
			}
			snippet := content
			if len(snippet) > 200 {
				snippet = snippet[:200] + "..."
			}
			date := item.UpdatedAt.UTC().Format("2006-01-02")
			if !msg.Timestamp.IsZero() {
				date = msg.Timestamp.UTC().Format("2006-01-02")
			}
			timestamp := item.UpdatedAt.UTC()
			if !msg.Timestamp.IsZero() {
				timestamp = msg.Timestamp.UTC()
			}
			results = append(results, memorySearchCandidate{
				Match: memorySearchMatch{
					Source:  fmt.Sprintf("session:%s", item.ID),
					Date:    date,
					Line:    0,
					Snippet: fmt.Sprintf("[%s] %s", msg.Role, snippet),
				},
				Score:     score + 80,
				Timestamp: timestamp,
			})
			if len(results) >= limit {
				return results, true
			}
		}
	}
	return results, true
}

func collectMemorySearchFiles(workspaceDir string, includeMemory, includeDaily bool) []memorySearchFile {
	files := make([]memorySearchFile, 0, 8)
	if includeMemory {
		path := filepath.Join(workspaceDir, "MEMORY.md")
		if stat, err := os.Stat(path); err == nil {
			files = append(files, memorySearchFile{
				Path:   path,
				Source: "MEMORY.md",
				Date:   stat.ModTime().UTC().Format("2006-01-02"),
				MTime:  stat.ModTime().UTC(),
			})
		}
	}

	if includeDaily {
		paths, _ := filepath.Glob(filepath.Join(workspaceDir, "memory", "*.md"))
		for _, path := range paths {
			stat, err := os.Stat(path)
			if err != nil {
				continue
			}

			base := filepath.Base(path)
			name := strings.TrimSuffix(base, filepath.Ext(base))
			date := stat.ModTime().UTC().Format("2006-01-02")
			if _, err := time.Parse("2006-01-02", name); err == nil {
				date = name
			}

			files = append(files, memorySearchFile{
				Path:   path,
				Source: filepath.ToSlash(filepath.Join("memory", base)),
				Date:   date,
				MTime:  stat.ModTime().UTC(),
			})
		}
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].MTime.Equal(files[j].MTime) {
			return files[i].Source > files[j].Source
		}
		return files[i].MTime.After(files[j].MTime)
	})
	return files
}

func searchExperienceLog(workspaceDir, query string, terms []string, limit int) ([]memorySearchCandidate, bool) {
	rows, err := memory.SearchExperiences(workspaceDir, memory.SearchOptions{Limit: 100})
	if err != nil || len(rows) == 0 {
		return nil, false
	}
	results := make([]memorySearchCandidate, 0, min(limit, len(rows)))
	for _, row := range rows {
		haystack := strings.Join([]string{
			row.Summary,
			row.Category,
			strings.Join(row.Tags, " "),
			row.SourceSession,
		}, "\n")
		score := scoreMemorySearchText(query, terms, haystack)
		if score == 0 {
			continue
		}
		source := "experience"
		if category := strings.TrimSpace(row.Category); category != "" {
			source += ":" + category
		}
		results = append(results, memorySearchCandidate{
			Match: memorySearchMatch{
				Source:  source,
				Date:    row.Timestamp.UTC().Format("2006-01-02"),
				Line:    0,
				Snippet: strings.TrimSpace(row.Summary),
			},
			Score:     score + 160,
			Timestamp: row.Timestamp.UTC(),
		})
		if len(results) >= limit {
			break
		}
	}
	return results, true
}

func searchMemoryFiles(files []memorySearchFile, query string, terms []string, limit int) []memorySearchCandidate {
	results := make([]memorySearchCandidate, 0, limit)
	for _, file := range files {
		raw, err := os.ReadFile(file.Path)
		if err != nil {
			continue
		}
		lines := strings.Split(string(raw), "\n")
		for i, line := range lines {
			snippet := strings.TrimSpace(line)
			if snippet == "" {
				continue
			}
			score := scoreMemorySearchText(query, terms, snippet)
			if score == 0 {
				continue
			}
			results = append(results, memorySearchCandidate{
				Match: memorySearchMatch{
					Source:  file.Source,
					Date:    file.Date,
					Line:    i + 1,
					Snippet: snippet,
				},
				Score:     score + 100,
				Timestamp: file.MTime.UTC(),
			})
			if len(results) >= limit {
				return results
			}
		}
	}
	return results
}

func memorySearchTerms(query string) []string {
	normalized := strings.Map(func(r rune) rune {
		switch {
		case unicode.IsLetter(r), unicode.IsNumber(r):
			return unicode.ToLower(r)
		default:
			return ' '
		}
	}, query)
	fields := strings.Fields(normalized)
	seen := map[string]struct{}{}
	terms := make([]string, 0, len(fields))
	for _, field := range fields {
		if len([]rune(field)) < 2 {
			continue
		}
		if _, exists := seen[field]; exists {
			continue
		}
		seen[field] = struct{}{}
		terms = append(terms, field)
	}
	if len(terms) == 0 {
		query = strings.ToLower(strings.TrimSpace(query))
		if query != "" {
			return []string{query}
		}
	}
	return terms
}

func scoreMemorySearchText(query string, terms []string, text string) int {
	query = strings.ToLower(strings.TrimSpace(query))
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return 0
	}
	score := 0
	matchedTerms := 0
	if query != "" && strings.Contains(text, query) {
		score += 100
	}
	for _, term := range terms {
		if term == "" || !strings.Contains(text, term) {
			continue
		}
		matchedTerms++
		score += 18 + min(len([]rune(term)), 12)
	}
	if matchedTerms == 0 && score == 0 {
		return 0
	}
	score += matchedTerms * 8
	if matchedTerms > 1 {
		score += matchedTerms * 4
	}
	return score
}

func memorySearchErrorResult(message string) Result {
	return Result{
		Content: []ContentBlock{
			{Type: "text", Text: message},
		},
		IsError: true,
	}
}
