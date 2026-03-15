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
)

const (
	defaultMemorySearchLimit = 8
	maxMemorySearchLimit     = 30
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

func NewMemorySearchTool(workspaceDir string) Tool {
	return Tool{
		Name:        "memory_search",
		Description: "Search MEMORY.md and daily memory logs for text snippets with source metadata.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "query":{"type":"string","description":"Search query text."},
    "limit":{"type":"integer","minimum":1,"maximum":30,"default":8},
    "include_memory":{"type":"boolean","default":true},
    "include_daily":{"type":"boolean","default":true}
  },
  "required":["query"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			var input struct {
				Query         string `json:"query"`
				Limit         *int   `json:"limit,omitempty"`
				IncludeMemory *bool  `json:"include_memory,omitempty"`
				IncludeDaily  *bool  `json:"include_daily,omitempty"`
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

			matches, message := runMemorySearch(workspaceDir, query, limit, includeMemory, includeDaily)
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

func runMemorySearch(workspaceDir, query string, limit int, includeMemory, includeDaily bool) ([]memorySearchMatch, string) {
	files := collectMemorySearchFiles(workspaceDir, includeMemory, includeDaily)
	if len(files) == 0 {
		return nil, "no memory files found"
	}

	lowerQuery := strings.ToLower(query)
	results := make([]memorySearchMatch, 0, limit)
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
			results = append(results, memorySearchMatch{
				Source:  file.Source,
				Date:    file.Date,
				Line:    i + 1,
				Snippet: strings.TrimSpace(line),
			})
			if len(results) >= limit {
				return results, ""
			}
		}
	}

	if len(results) == 0 {
		return nil, "no matches found"
	}
	return results, ""
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
