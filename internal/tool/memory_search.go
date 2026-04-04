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
    "include_sessions":{"type":"boolean","default":false,"description":"Search past session transcripts for conversational continuity. Always set to true when called."}
  },
  "required":["query"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			var input struct {
				Query           string `json:"query"`
				Limit           *int   `json:"limit,omitempty"`
				IncludeMemory   *bool  `json:"include_memory,omitempty"`
				IncludeDaily    *bool  `json:"include_daily,omitempty"`
				IncludeSessions *bool  `json:"include_sessions,omitempty"`
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
			includeSessions := false
			if input.IncludeSessions != nil {
				includeSessions = *input.IncludeSessions
			}

			matches, message := runMemorySearch(context.Background(), workspaceDir, query, limit, includeMemory, includeDaily, includeSessions, semantic)
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

func runMemorySearch(ctx context.Context, workspaceDir, query string, limit int, includeMemory, includeDaily, includeSessions bool, semantic *memory.Service) ([]memorySearchMatch, string) {
	results := make([]memorySearchMatch, 0, limit)
	seen := map[string]struct{}{}
	appendMatch := func(match memorySearchMatch) bool {
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
				if appendMatch(memorySearchMatch{
					Source:  hit.Source,
					Date:    hit.Date.UTC().Format("2006-01-02"),
					Line:    0,
					Snippet: hit.Snippet,
				}) {
					return results, ""
				}
			}
		}
	}

	for _, match := range searchKnowledgeNotes(workspaceDir, query, limit-len(results)) {
		if appendMatch(match) {
			return results, ""
		}
	}

	files := collectMemorySearchFiles(workspaceDir, includeMemory, includeDaily)
	if len(files) == 0 && !includeSessions && len(results) == 0 {
		return nil, "no memory files found"
	}

	lowerQuery := strings.ToLower(query)
	for _, file := range files {
		raw, err := os.ReadFile(file.Path)
		if err != nil {
			continue
		}
		lines := strings.Split(string(raw), "\n")
		for i, line := range lines {
			if !strings.Contains(strings.ToLower(line), lowerQuery) {
				continue
			}
			if appendMatch(memorySearchMatch{
				Source:  file.Source,
				Date:    file.Date,
				Line:    i + 1,
				Snippet: strings.TrimSpace(line),
			}) {
				return results, ""
			}
		}
	}

	if includeSessions && len(results) < limit {
		sessionResults := searchSessionTranscripts(workspaceDir, query, limit-len(results))
		for _, match := range sessionResults {
			if appendMatch(match) {
				return results, ""
			}
		}
	}

	if len(results) == 0 {
		return nil, "no matches found"
	}
	return results, ""
}

func searchKnowledgeNotes(workspaceDir, query string, limit int) []memorySearchMatch {
	if limit <= 0 {
		return nil
	}
	items, err := memory.NewKnowledgeStore(workspaceDir, nil).List(memory.KnowledgeListOptions{
		Query: query,
		Limit: limit,
	})
	if err != nil {
		return nil
	}
	results := make([]memorySearchMatch, 0, len(items))
	for _, item := range items {
		snippet := strings.TrimSpace(item.Summary)
		if snippet == "" {
			snippet = strings.TrimSpace(item.Body)
		}
		if snippet == "" {
			snippet = strings.TrimSpace(item.Title)
		}
		if len(snippet) > 200 {
			snippet = snippet[:200] + "..."
		}
		results = append(results, memorySearchMatch{
			Source:  item.Path,
			Date:    item.UpdatedAt.UTC().Format("2006-01-02"),
			Line:    0,
			Snippet: snippet,
		})
	}
	return results
}

func searchSessionTranscripts(workspaceDir, query string, limit int) []memorySearchMatch {
	store := session.NewStore(workspaceDir)
	sessions, err := store.List()
	if err != nil || len(sessions) == 0 {
		return nil
	}
	sort.SliceStable(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	if len(sessions) > maxSessionsToSearch {
		sessions = sessions[:maxSessionsToSearch]
	}

	lowerQuery := strings.ToLower(query)
	var results []memorySearchMatch
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
			if content == "" || !strings.Contains(strings.ToLower(content), lowerQuery) {
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
			results = append(results, memorySearchMatch{
				Source:  fmt.Sprintf("session:%s", item.ID),
				Date:    date,
				Line:    0,
				Snippet: fmt.Sprintf("[%s] %s", msg.Role, snippet),
			})
			if len(results) >= limit {
				return results
			}
		}
	}
	return results
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

func memorySearchErrorResult(message string) Result {
	return Result{
		Content: []ContentBlock{
			{Type: "text", Text: message},
		},
		IsError: true,
	}
}
