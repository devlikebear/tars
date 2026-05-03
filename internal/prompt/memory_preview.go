package prompt

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/devlikebear/tars/internal/memory"
)

// PreviewMode distinguishes how the prior-context preview was constructed.
//   - PreviewModeDefault: ranked recall against opts.Query (same logic as the
//     live LLM prompt path)
//   - PreviewModeRecent: opts.Query was empty, so the preview shows recent
//     experiences/memory entries as a discoverability fallback
type PreviewMode string

const (
	PreviewModeDefault PreviewMode = "default"
	PreviewModeRecent  PreviewMode = "recent"
)

// PriorContextPreview is the structured payload backing the console's "Prior"
// panel. Unlike BuildResult, it never feeds the LLM prompt — the extra fields
// (BelowThreshold, RecentFallback) exist to help users understand *why* the
// live recall produced no results.
type PriorContextPreview struct {
	Mode                 PreviewMode          `json:"mode"`
	Section              string               `json:"section"`
	Items                []RelevantMemoryItem `json:"items"`
	BelowThreshold       []RelevantMemoryItem `json:"below_threshold_items"`
	RecentFallback       []RelevantMemoryItem `json:"recent_fallback_items"`
	RelevantTokens       int                  `json:"relevant_tokens"`
	RelevantBudgetTokens int                  `json:"relevant_budget_tokens"`
}

// BuildPriorContextPreview produces the preview for the console panel. When
// opts.Query is empty it falls back to the most recent experiences /
// MEMORY.md entries so the panel has something to display; the live LLM
// prompt path explicitly does not do this fallback.
func BuildPriorContextPreview(opts BuildOptions, budgetTokens int) PriorContextPreview {
	if budgetTokens <= 0 {
		budgetTokens = defaultRelevantBudgetTokens
	}
	preview := PriorContextPreview{
		Mode:                 PreviewModeDefault,
		Items:                []RelevantMemoryItem{},
		BelowThreshold:       []RelevantMemoryItem{},
		RecentFallback:       []RelevantMemoryItem{},
		RelevantBudgetTokens: budgetTokens,
	}

	query := strings.TrimSpace(opts.Query)
	if query == "" {
		preview.Mode = PreviewModeRecent
		preview.RecentFallback = collectRecentFallbackItems(opts, defaultRelevantResultLimit)
		return preview
	}

	above, below := collectRelevantMemoryWithSplit(opts)
	section, items, used := renderRelevantMemorySection(above, budgetTokens)
	preview.Section = section
	if items != nil {
		preview.Items = items
	}
	preview.RelevantTokens = used
	preview.BelowThreshold = matchesToItems(below)
	return preview
}

// collectRecentFallbackItems returns the most recent experience entries and
// MEMORY.md highlights as un-scored items. Used when the user has not typed
// anything yet, so the panel can still show "what would be available."
func collectRecentFallbackItems(opts BuildOptions, limit int) []RelevantMemoryItem {
	if limit <= 0 {
		limit = defaultRelevantResultLimit
	}
	out := make([]RelevantMemoryItem, 0, limit)

	// Recent experiences: SearchExperiences already returns rows ordered by
	// timestamp desc.
	if rows, err := memory.SearchExperiences(opts.WorkspaceDir, memory.SearchOptions{Limit: limit}); err == nil {
		for _, row := range rows {
			snippet := strings.TrimSpace(row.Summary)
			if snippet == "" {
				continue
			}
			source := "experience"
			if cat := strings.TrimSpace(row.Category); cat != "" {
				source += ":" + cat
			}
			snippet = trimToBudget(snippet, 360, 220)
			out = append(out, RelevantMemoryItem{
				Source:    source,
				SourceTag: classifySourceTag(source),
				Snippet:   snippet,
				Tokens:    estimateTokens(snippet),
			})
			if len(out) >= limit {
				return out
			}
		}
	}

	// Top non-empty lines from MEMORY.md (skip headings)
	if path := filepath.Join(opts.WorkspaceDir, "MEMORY.md"); fileExists(path) {
		raw, err := os.ReadFile(path)
		if err == nil {
			for _, line := range strings.Split(string(raw), "\n") {
				snippet := strings.TrimSpace(line)
				if snippet == "" || strings.HasPrefix(snippet, "#") {
					continue
				}
				snippet = trimToBudget(snippet, 360, 220)
				out = append(out, RelevantMemoryItem{
					Source:    "MEMORY.md",
					SourceTag: classifySourceTag("MEMORY.md"),
					Snippet:   snippet,
					Tokens:    estimateTokens(snippet),
				})
				if len(out) >= limit {
					return out
				}
			}
		}
	}

	// Fall back to the most recent daily log lines (memory/*.md) when neither
	// experience entries nor MEMORY.md filled the slot.
	if len(out) == 0 {
		if paths, _ := filepath.Glob(filepath.Join(opts.WorkspaceDir, "memory", "*.md")); len(paths) > 0 {
			sort.SliceStable(paths, func(i, j int) bool {
				return filepath.Base(paths[i]) > filepath.Base(paths[j])
			})
			for _, p := range paths {
				raw, err := os.ReadFile(p)
				if err != nil {
					continue
				}
				source := filepath.ToSlash(filepath.Join("memory", filepath.Base(p)))
				for _, line := range strings.Split(string(raw), "\n") {
					snippet := strings.TrimSpace(line)
					if snippet == "" || strings.HasPrefix(snippet, "#") {
						continue
					}
					snippet = trimToBudget(snippet, 360, 220)
					out = append(out, RelevantMemoryItem{
						Source:    source,
						SourceTag: classifySourceTag(source),
						Snippet:   snippet,
						Tokens:    estimateTokens(snippet),
					})
					if len(out) >= limit {
						return out
					}
				}
				if len(out) >= limit {
					break
				}
			}
		}
	}

	return out
}

func matchesToItems(matches []relevantMemoryMatch) []RelevantMemoryItem {
	if len(matches) == 0 {
		return []RelevantMemoryItem{}
	}
	out := make([]RelevantMemoryItem, 0, len(matches))
	for _, m := range matches {
		source := strings.TrimSpace(m.Source)
		if source == "" {
			source = "memory"
		}
		snippet := trimToBudget(m.Snippet, 360, 220)
		if snippet == "" {
			continue
		}
		out = append(out, RelevantMemoryItem{
			Source:    source,
			SourceTag: classifySourceTag(source),
			Snippet:   snippet,
			Tokens:    estimateTokens(snippet),
		})
	}
	return out
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
