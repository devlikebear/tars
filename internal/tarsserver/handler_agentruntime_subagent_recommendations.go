package tarsserver

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/config"
)

type agentRuntimeSubagentRecommendationRequest struct {
	Limit         int  `json:"limit"`
	MinRuns       int  `json:"min_runs"`
	IncludeFailed bool `json:"include_failed"`
}

type agentRuntimeSubagentRecommendationResponse struct {
	Count            int                                  `json:"count"`
	AnalyzedRunCount int                                  `json:"analyzed_run_count"`
	Recommendations  []agentRuntimeSubagentRecommendation `json:"recommendations"`
	Tiers            []agentRuntimeTierOption             `json:"tiers"`
	Warnings         []string                             `json:"warnings,omitempty"`
}

type agentRuntimeSubagentRecommendation struct {
	ID           string                    `json:"id"`
	Title        string                    `json:"title"`
	Reason       string                    `json:"reason"`
	Confidence   float64                   `json:"confidence"`
	RunCount     int                       `json:"run_count"`
	RecentRunIDs []string                  `json:"recent_run_ids"`
	Keywords     []string                  `json:"keywords"`
	Draft        agentRuntimeSubagentDraft `json:"draft"`
	ResolvedTier *agentRuntimeTierOption   `json:"resolved_tier,omitempty"`
}

type agentRuntimeRecommendationPattern struct {
	Name        string
	Description string
	Focus       string
	Reason      string
	Terms       []string
	ToolsAllow  []string
}

type agentRuntimeRecommendationGroup struct {
	Pattern    agentRuntimeRecommendationPattern
	Runs       []agentruntime.Run
	Keywords   map[string]int
	TierCounts map[string]int
	Latest     time.Time
}

func handleAgentRuntimeSubagentRecommendations(w http.ResponseWriter, r *http.Request, runtime *agentruntime.Runtime, cfg config.Config) {
	var req agentRuntimeSubagentRecommendationRequest
	if !decodeOptionalJSONBody(w, r, &req) {
		return
	}
	writeJSON(w, http.StatusOK, buildAgentRuntimeSubagentRecommendationResponse(runtime, cfg, req))
}

func buildAgentRuntimeSubagentRecommendationResponse(
	runtime *agentruntime.Runtime,
	cfg config.Config,
	req agentRuntimeSubagentRecommendationRequest,
) agentRuntimeSubagentRecommendationResponse {
	tiers, tierMap := agentRuntimeTierOptions(cfg)
	resp := agentRuntimeSubagentRecommendationResponse{
		Tiers:           tiers,
		Recommendations: []agentRuntimeSubagentRecommendation{},
	}
	if runtime == nil {
		resp.Warnings = append(resp.Warnings, "Agent Runtime is not configured.")
		return resp
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	minRuns := req.MinRuns
	if minRuns <= 0 {
		minRuns = 2
	}

	existing := map[string]struct{}{}
	for _, raw := range runtime.Agents() {
		if name := strings.ToLower(strings.TrimSpace(stringFromMap(raw, "name"))); name != "" {
			existing[name] = struct{}{}
		}
	}

	groups := map[string]*agentRuntimeRecommendationGroup{}
	assignedNames := map[string]string{}
	for _, run := range runtime.List(limit) {
		if !agentRuntimeRunEligibleForRecommendation(run, req.IncludeFailed) {
			continue
		}
		pattern := classifyAgentRuntimeRecommendationPattern(run)
		baseName := pattern.Name
		name, ok := assignedNames[baseName]
		if !ok {
			name = availableAgentRuntimeRecommendationName(baseName, existing)
			assignedNames[baseName] = name
		}
		if name != pattern.Name {
			pattern.Name = name
			pattern.Description = deriveAgentRuntimeSubagentDescription(pattern.Name, pattern.Name)
		}
		group := groups[pattern.Name]
		if group == nil {
			group = &agentRuntimeRecommendationGroup{
				Pattern:    pattern,
				Keywords:   map[string]int{},
				TierCounts: map[string]int{},
			}
			groups[pattern.Name] = group
		}
		group.Runs = append(group.Runs, run)
		for _, keyword := range agentRuntimeSubagentKeywords(run.Prompt) {
			group.Keywords[keyword]++
		}
		if tier := strings.ToLower(strings.TrimSpace(run.Tier)); tier != "" {
			group.TierCounts[tier]++
		}
		if ts := agentRuntimeRecommendationTimestamp(run); ts.After(group.Latest) {
			group.Latest = ts
		}
		resp.AnalyzedRunCount++
	}

	recommendations := make([]agentRuntimeSubagentRecommendation, 0, len(groups))
	for _, group := range groups {
		if len(group.Runs) < minRuns {
			continue
		}
		rec := buildAgentRuntimeSubagentRecommendation(group, cfg, tierMap)
		recommendations = append(recommendations, rec)
	}
	sort.Slice(recommendations, func(i, j int) bool {
		left := recommendations[i]
		right := recommendations[j]
		if left.RunCount != right.RunCount {
			return left.RunCount > right.RunCount
		}
		if left.Confidence != right.Confidence {
			return left.Confidence > right.Confidence
		}
		return left.Draft.Name < right.Draft.Name
	})
	if len(recommendations) > 5 {
		recommendations = recommendations[:5]
	}
	resp.Recommendations = recommendations
	resp.Count = len(recommendations)
	if resp.AnalyzedRunCount == 0 {
		resp.Warnings = append(resp.Warnings, "No completed Agent Runtime runs were available for recommendation.")
	} else if resp.Count == 0 {
		resp.Warnings = append(resp.Warnings, fmt.Sprintf("No repeated run pattern reached the minimum of %d runs.", minRuns))
	}
	return resp
}

func agentRuntimeRunEligibleForRecommendation(run agentruntime.Run, includeFailed bool) bool {
	if strings.TrimSpace(run.Prompt) == "" {
		return false
	}
	switch run.Status {
	case agentruntime.RunStatusCompleted:
		return true
	case agentruntime.RunStatusFailed, agentruntime.RunStatusCanceled:
		return includeFailed
	default:
		return false
	}
}

func classifyAgentRuntimeRecommendationPattern(run agentruntime.Run) agentRuntimeRecommendationPattern {
	text := strings.ToLower(strings.Join([]string{
		run.Agent,
		run.Prompt,
		run.Response,
		strings.Join(agentRuntimeRunFileAttentionPaths(run), " "),
	}, " "))
	patterns := []agentRuntimeRecommendationPattern{
		{
			Name:        "frontend-checker",
			Description: "Frontend verification subagent",
			Focus:       "frontend UI, browser smoke checks, Svelte/CSS regressions, and accessibility-sensitive changes",
			Reason:      "Recent runs repeatedly touched frontend review or UI verification work.",
			Terms:       []string{"frontend", "svelte", "ui", "console", "css", "browser", "playwright", "accessibility", "responsive"},
			ToolsAllow:  []string{"glob", "list_dir", "read_file", "exec"},
		},
		{
			Name:        "release-manager",
			Description: "Release and PR delivery subagent",
			Focus:       "version bumps, changelog hygiene, PR checks, merge readiness, release artifacts, and Homebrew tap verification",
			Reason:      "Recent runs repeatedly followed release, PR, or CI delivery workflows.",
			Terms:       []string{"release", "changelog", "version", "homebrew", "tag", "asset", "pr", "pull request", "merge", "ci"},
			ToolsAllow:  []string{"glob", "list_dir", "read_file", "edit_file", "write_file", "exec"},
		},
		{
			Name:        "docs-maintainer",
			Description: "Documentation maintenance subagent",
			Focus:       "README, changelog, tutorial, and Markdown documentation updates",
			Reason:      "Recent runs repeatedly worked on documentation or Markdown artifacts.",
			Terms:       []string{"docs", "documentation", "readme", "markdown", "tutorial", "guide"},
			ToolsAllow:  []string{"glob", "list_dir", "read_file", "edit_file", "write_file"},
		},
		{
			Name:        "code-reviewer",
			Description: "Code review and risk analysis subagent",
			Focus:       "code review, regression risk, test gaps, and file-grounded findings",
			Reason:      "Recent runs repeatedly performed review, inspection, or risk analysis.",
			Terms:       []string{"review", "audit", "inspect", "analyze", "analysis", "risk", "regression", "bug"},
			ToolsAllow:  []string{"glob", "list_dir", "read_file"},
		},
	}
	for _, pattern := range patterns {
		if agentRuntimeRecommendationMatches(text, pattern.Terms) {
			return pattern
		}
	}
	keywords := agentRuntimeSubagentKeywords(run.Prompt)
	if len(keywords) == 0 {
		keywords = []string{"custom"}
	}
	if len(keywords) > 2 {
		keywords = keywords[:2]
	}
	name := strings.Join(keywords, "-") + "-specialist"
	return agentRuntimeRecommendationPattern{
		Name:        name,
		Description: deriveAgentRuntimeSubagentDescription(name, run.Prompt),
		Focus:       strings.Join(keywords, " ") + " tasks that appeared repeatedly in recent Agent Runtime runs",
		Reason:      "Recent runs shared a repeated prompt pattern.",
		Terms:       keywords,
		ToolsAllow:  []string{"glob", "list_dir", "read_file"},
	}
}

func agentRuntimeRecommendationMatches(text string, terms []string) bool {
	for _, term := range terms {
		term = strings.ToLower(strings.TrimSpace(term))
		if term != "" && strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func availableAgentRuntimeRecommendationName(base string, existing map[string]struct{}) string {
	base = strings.ToLower(strings.TrimSpace(base))
	if base == "" {
		base = "custom-agent"
	}
	if _, ok := existing[base]; !ok {
		existing[base] = struct{}{}
		return base
	}
	for i := 2; i <= 20; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if _, ok := existing[candidate]; !ok {
			existing[candidate] = struct{}{}
			return candidate
		}
	}
	return fmt.Sprintf("%s-%d", base, time.Now().UTC().Unix())
}

func buildAgentRuntimeSubagentRecommendation(
	group *agentRuntimeRecommendationGroup,
	cfg config.Config,
	tierMap map[string]agentRuntimeTierOption,
) agentRuntimeSubagentRecommendation {
	pattern := group.Pattern
	keywords := topAgentRuntimeRecommendationKeywords(group.Keywords, 6)
	tier := preferredAgentRuntimeRecommendationTier(group.TierCounts, cfg)
	provenance := agentRuntimeRecommendationProvenance(group.Runs, 5)
	draft := normalizeAgentRuntimeSubagentDraft(agentRuntimeSubagentDraft{
		Action:      "create",
		Name:        pattern.Name,
		Description: pattern.Description,
		DefaultTier: tier,
		Prompt:      composeAgentRuntimeRecommendationPrompt(pattern, group.Runs, keywords),
		ToolsAllow:  recommendedAgentRuntimeTools(pattern.ToolsAllow, cfg),
		ToolsDeny:   []string{},
		Provenance:  provenance,
	}, cfg, nil)
	rec := agentRuntimeSubagentRecommendation{
		ID:           "rec_" + strings.ReplaceAll(draft.Name, "-", "_"),
		Title:        draft.Description,
		Reason:       pattern.Reason,
		Confidence:   agentRuntimeRecommendationConfidence(len(group.Runs)),
		RunCount:     len(group.Runs),
		RecentRunIDs: agentRuntimeRecommendationRunIDs(group.Runs, 5),
		Keywords:     keywords,
		Draft:        draft,
	}
	if option, ok := tierMap[strings.ToLower(strings.TrimSpace(draft.DefaultTier))]; ok {
		rec.ResolvedTier = &option
	}
	return rec
}

func preferredAgentRuntimeRecommendationTier(tierCounts map[string]int, cfg config.Config) string {
	type tierCount struct {
		Name  string
		Count int
	}
	counts := make([]tierCount, 0, len(tierCounts))
	for tier, count := range tierCounts {
		name := strings.ToLower(strings.TrimSpace(tier))
		if name == "" {
			continue
		}
		if _, ok := cfg.LLMTiers[name]; !ok {
			continue
		}
		counts = append(counts, tierCount{Name: name, Count: count})
	}
	sort.Slice(counts, func(i, j int) bool {
		if counts[i].Count != counts[j].Count {
			return counts[i].Count > counts[j].Count
		}
		return counts[i].Name < counts[j].Name
	})
	if len(counts) > 0 {
		return counts[0].Name
	}
	return preferredAgentRuntimeBuilderTier("", cfg)
}

func recommendedAgentRuntimeTools(raw []string, cfg config.Config) []string {
	known := knownAgentRuntimePromptTools(cfg.WorkspaceDir)
	tools := normalizeToolNames(raw)
	out := make([]string, 0, len(tools))
	for _, tool := range tools {
		if _, ok := known[tool]; ok {
			out = append(out, tool)
		}
	}
	if len(out) == 0 {
		return []string{"glob", "list_dir", "read_file"}
	}
	return out
}

func composeAgentRuntimeRecommendationPrompt(pattern agentRuntimeRecommendationPattern, runs []agentruntime.Run, keywords []string) string {
	var b strings.Builder
	b.WriteString("You are the ")
	b.WriteString(pattern.Name)
	b.WriteString(" subagent.\n\n")
	b.WriteString("Focus: ")
	b.WriteString(pattern.Focus)
	b.WriteString(".\n\n")
	if len(keywords) > 0 {
		b.WriteString("Recurring signals: ")
		b.WriteString(strings.Join(keywords, ", "))
		b.WriteString(".\n\n")
	}
	b.WriteString("Observed provenance:\n")
	for _, run := range runs {
		b.WriteString("- Run ")
		b.WriteString(run.ID)
		if strings.TrimSpace(run.Agent) != "" {
			b.WriteString(" via ")
			b.WriteString(strings.TrimSpace(run.Agent))
		}
		if strings.TrimSpace(run.Tier) != "" {
			b.WriteString(" on tier ")
			b.WriteString(strings.TrimSpace(run.Tier))
		}
		b.WriteString(": ")
		b.WriteString(trimAgentRuntimeRecommendationText(run.Prompt, 140))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString("- Start by checking the concrete files, commands, or artifacts related to the task.\n")
	b.WriteString("- Keep findings tied to evidence from the workspace.\n")
	b.WriteString("- Call out assumptions when the repeated pattern does not fully match the current request.")
	return strings.TrimSpace(b.String())
}

func agentRuntimeRecommendationProvenance(runs []agentruntime.Run, limit int) []agentRuntimeSubagentDraftProvenance {
	if limit <= 0 {
		limit = 5
	}
	out := make([]agentRuntimeSubagentDraftProvenance, 0, min(limit, len(runs)))
	for _, run := range runs {
		out = append(out, agentRuntimeSubagentDraftProvenance{
			RunID:       run.ID,
			Agent:       run.Agent,
			Status:      string(run.Status),
			Tier:        run.Tier,
			Prompt:      run.Prompt,
			CreatedAt:   run.CreatedAt,
			CompletedAt: run.CompletedAt,
		})
		if len(out) >= limit {
			break
		}
	}
	return out
}

func agentRuntimeRecommendationRunIDs(runs []agentruntime.Run, limit int) []string {
	if limit <= 0 {
		limit = 5
	}
	out := make([]string, 0, min(limit, len(runs)))
	for _, run := range runs {
		if strings.TrimSpace(run.ID) == "" {
			continue
		}
		out = append(out, run.ID)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func topAgentRuntimeRecommendationKeywords(counts map[string]int, limit int) []string {
	if limit <= 0 {
		limit = 6
	}
	type keywordCount struct {
		Keyword string
		Count   int
	}
	items := make([]keywordCount, 0, len(counts))
	for keyword, count := range counts {
		keyword = strings.ToLower(strings.TrimSpace(keyword))
		if keyword == "" {
			continue
		}
		items = append(items, keywordCount{Keyword: keyword, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count != items[j].Count {
			return items[i].Count > items[j].Count
		}
		return items[i].Keyword < items[j].Keyword
	})
	out := make([]string, 0, min(limit, len(items)))
	for _, item := range items {
		out = append(out, item.Keyword)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func agentRuntimeRecommendationConfidence(runCount int) float64 {
	if runCount <= 1 {
		return 0.55
	}
	score := 0.6 + float64(runCount-2)*0.08
	if score > 0.92 {
		return 0.92
	}
	return score
}

func agentRuntimeRunFileAttentionPaths(run agentruntime.Run) []string {
	out := make([]string, 0, len(run.FileAttention))
	for _, row := range run.FileAttention {
		if path := strings.TrimSpace(row.Path); path != "" {
			out = append(out, path)
		}
	}
	return out
}

func agentRuntimeRecommendationTimestamp(run agentruntime.Run) time.Time {
	for _, raw := range []string{run.CompletedAt, run.UpdatedAt, run.StartedAt, run.CreatedAt} {
		if raw = strings.TrimSpace(raw); raw != "" {
			if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
				return parsed.UTC()
			}
		}
	}
	return time.Time{}
}

func trimAgentRuntimeRecommendationText(raw string, limit int) string {
	text := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if limit <= 0 || len([]rune(text)) <= limit {
		return text
	}
	runes := []rune(text)
	return strings.TrimSpace(string(runes[:limit-1])) + "..."
}
