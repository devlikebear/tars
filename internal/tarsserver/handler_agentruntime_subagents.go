package tarsserver

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/llm"
	"gopkg.in/yaml.v3"
)

type agentRuntimeTierOption struct {
	Name            string   `json:"name"`
	ProviderAlias   string   `json:"provider_alias,omitempty"`
	Kind            string   `json:"kind,omitempty"`
	Model           string   `json:"model,omitempty"`
	ReasoningEffort string   `json:"reasoning_effort,omitempty"`
	ThinkingBudget  int      `json:"thinking_budget,omitempty"`
	ServiceTier     string   `json:"service_tier,omitempty"`
	MaxTokens       int      `json:"max_tokens,omitempty"`
	BetaFeatures    []string `json:"beta_features,omitempty"`
	Error           string   `json:"error,omitempty"`
}

type agentRuntimeSubagentRunSummary struct {
	RunID       string `json:"run_id"`
	Status      string `json:"status"`
	Tier        string `json:"tier,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
}

type agentRuntimeSubagentView struct {
	Name               string                           `json:"name"`
	Description        string                           `json:"description,omitempty"`
	Enabled            bool                             `json:"enabled"`
	Kind               string                           `json:"kind,omitempty"`
	Source             string                           `json:"source,omitempty"`
	Entry              string                           `json:"entry,omitempty"`
	Default            bool                             `json:"default"`
	PolicyMode         string                           `json:"policy_mode,omitempty"`
	ToolsAllow         []string                         `json:"tools_allow"`
	ToolsAllowCount    int                              `json:"tools_allow_count"`
	ToolsDeny          []string                         `json:"tools_deny"`
	ToolsDenyCount     int                              `json:"tools_deny_count"`
	ToolsRiskMax       string                           `json:"tools_risk_max,omitempty"`
	ToolsAllowGroups   []string                         `json:"tools_allow_groups"`
	ToolsDenyGroups    []string                         `json:"tools_deny_groups"`
	ToolsAllowPatterns []string                         `json:"tools_allow_patterns"`
	SessionRoutingMode string                           `json:"session_routing_mode,omitempty"`
	SessionFixedID     string                           `json:"session_fixed_id,omitempty"`
	DefaultTier        string                           `json:"default_tier,omitempty"`
	EffectiveTier      string                           `json:"effective_tier,omitempty"`
	TierSource         string                           `json:"tier_source,omitempty"`
	TierMissing        bool                             `json:"tier_missing"`
	TierError          string                           `json:"tier_error,omitempty"`
	TierEditable       bool                             `json:"tier_editable"`
	ProviderOverride   *agentruntime.ProviderOverride   `json:"provider_override,omitempty"`
	ResolvedAlias      string                           `json:"resolved_alias,omitempty"`
	ResolvedKind       string                           `json:"resolved_kind,omitempty"`
	ResolvedModel      string                           `json:"resolved_model,omitempty"`
	RunCount           int                              `json:"run_count"`
	LastRun            *agentRuntimeSubagentRunSummary  `json:"last_run,omitempty"`
	RecentRuns         []agentRuntimeSubagentRunSummary `json:"recent_runs"`
}

type agentRuntimeSubagentsResponse struct {
	Count                  int                        `json:"count"`
	Agents                 []agentRuntimeSubagentView `json:"agents"`
	Tiers                  []agentRuntimeTierOption   `json:"tiers"`
	DefaultTier            string                     `json:"default_tier,omitempty"`
	AgentRuntimeTier       string                     `json:"agentruntime_default_tier,omitempty"`
	AgentRuntimeTierSource string                     `json:"agentruntime_default_tier_source,omitempty"`
}

func newAgentRuntimeSubagentsAPIHandler(runtime *agentruntime.Runtime, cfg config.Config, reloadHook func(), routers ...llm.Router) http.Handler {
	var router llm.Router
	if len(routers) > 0 {
		router = routers[0]
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/agentruntime/subagents/builder/draft", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		handleAgentRuntimeSubagentBuilderDraft(w, r, runtime, cfg, router)
	})
	mux.HandleFunc("/v1/agentruntime/subagents/builder/apply", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		handleAgentRuntimeSubagentBuilderApply(w, r, runtime, cfg, reloadHook)
	})
	mux.HandleFunc("/v1/agentruntime/subagents/recommendations", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		handleAgentRuntimeSubagentRecommendations(w, r, runtime, cfg)
	})
	mux.HandleFunc("/v1/agentruntime/subagents", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		resp := buildAgentRuntimeSubagentsResponse(runtime, cfg)
		writeJSON(w, http.StatusOK, resp)
	})
	mux.HandleFunc("/v1/agentruntime/subagents/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(strings.TrimSpace(r.URL.Path), "/archive") {
			if !requireMethod(w, r, http.MethodPost) {
				return
			}
			handleAgentRuntimeSubagentArchive(w, r, runtime, cfg, reloadHook)
			return
		}
		if !requireMethod(w, r, http.MethodGet, http.MethodPatch) {
			return
		}
		name, ok := parseAgentRuntimeSubagentName(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPatch {
			handleAgentRuntimeSubagentUpdate(w, r, runtime, cfg, name, reloadHook)
			return
		}
		resp := buildAgentRuntimeSubagentsResponse(runtime, cfg)
		for _, agent := range resp.Agents {
			if agent.Name == name {
				writeJSON(w, http.StatusOK, agent)
				return
			}
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "subagent not found"})
	})
	return mux
}

type agentRuntimeSubagentUpdateRequest struct {
	DefaultTier *string `json:"default_tier"`
}

func handleAgentRuntimeSubagentUpdate(
	w http.ResponseWriter,
	r *http.Request,
	runtime *agentruntime.Runtime,
	cfg config.Config,
	name string,
	reloadHook func(),
) {
	var req agentRuntimeSubagentUpdateRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.DefaultTier == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "default_tier is required"})
		return
	}
	nextTier := strings.ToLower(strings.TrimSpace(*req.DefaultTier))
	if nextTier != "" {
		if _, ok := cfg.LLMTiers[nextTier]; !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("tier %q is not configured in llm_tiers", nextTier)})
			return
		}
		if _, err := config.ResolveLLMTier(&cfg, nextTier); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}
	resp := buildAgentRuntimeSubagentsResponse(runtime, cfg)
	var target *agentRuntimeSubagentView
	for i := range resp.Agents {
		if resp.Agents[i].Name == name {
			target = &resp.Agents[i]
			break
		}
	}
	if target == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "subagent not found"})
		return
	}
	if !target.TierEditable {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "subagent tier is only editable for workspace AGENT.md profiles"})
		return
	}
	entry, ok := workspaceAgentRuntimeEntryPath(target.Entry, cfg.WorkspaceDir)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "subagent entry is not editable"})
		return
	}
	if err := updateWorkspaceAgentRuntimeAgentTier(entry, nextTier); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if reloadHook != nil {
		reloadHook()
	}
	updated := buildAgentRuntimeSubagentsResponse(runtime, cfg)
	for _, agent := range updated.Agents {
		if agent.Name == name {
			writeJSON(w, http.StatusOK, agent)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func parseAgentRuntimeSubagentName(path string) (string, bool) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(path), "/v1/agentruntime/subagents/")
	if strings.TrimSpace(trimmed) == "" || strings.Contains(trimmed, "/") {
		return "", false
	}
	name, err := url.PathUnescape(trimmed)
	if err != nil {
		return "", false
	}
	name = strings.TrimSpace(name)
	return name, name != ""
}

func buildAgentRuntimeSubagentsResponse(runtime *agentruntime.Runtime, cfg config.Config) agentRuntimeSubagentsResponse {
	tiers, tierMap := agentRuntimeTierOptions(cfg)
	defaultTier := strings.ToLower(strings.TrimSpace(cfg.LLMDefaultTier))
	agentRuntimeTier, agentRuntimeTierSource := configuredAgentRuntimeDefaultTier(cfg)
	resp := agentRuntimeSubagentsResponse{
		Tiers:                  tiers,
		DefaultTier:            defaultTier,
		AgentRuntimeTier:       agentRuntimeTier,
		AgentRuntimeTierSource: agentRuntimeTierSource,
		Agents:                 []agentRuntimeSubagentView{},
	}
	if runtime == nil {
		return resp
	}
	runs := runtime.List(200)
	for _, raw := range runtime.Agents() {
		agent := agentRuntimeSubagentFromMap(raw, cfg, tierMap, agentRuntimeTier, agentRuntimeTierSource, runs)
		resp.Agents = append(resp.Agents, agent)
	}
	resp.Count = len(resp.Agents)
	return resp
}

func agentRuntimeTierOptions(cfg config.Config) ([]agentRuntimeTierOption, map[string]agentRuntimeTierOption) {
	names := make([]string, 0, len(cfg.LLMTiers))
	for name := range cfg.LLMTiers {
		tier := strings.ToLower(strings.TrimSpace(name))
		if tier != "" {
			names = append(names, tier)
		}
	}
	sort.Strings(names)
	out := make([]agentRuntimeTierOption, 0, len(names))
	byName := make(map[string]agentRuntimeTierOption, len(names))
	for _, name := range names {
		option := agentRuntimeTierOption{Name: name}
		resolved, err := config.ResolveLLMTier(&cfg, name)
		if err != nil {
			option.Error = err.Error()
		} else {
			option.ProviderAlias = resolved.ProviderAlias
			option.Kind = resolved.Kind
			option.Model = resolved.Model
			option.ReasoningEffort = resolved.ReasoningEffort
			option.ThinkingBudget = resolved.ThinkingBudget
			option.ServiceTier = resolved.ServiceTier
			option.MaxTokens = resolved.MaxTokens
			option.BetaFeatures = resolved.BetaFeatures
		}
		out = append(out, option)
		byName[name] = option
	}
	return out, byName
}

func configuredAgentRuntimeDefaultTier(cfg config.Config) (string, string) {
	if tier := strings.ToLower(strings.TrimSpace(cfg.LLMRoleDefaults[string(llm.RoleAgentRuntimeDefault)])); tier != "" {
		return tier, "role_default"
	}
	if tier := strings.ToLower(strings.TrimSpace(cfg.LLMDefaultTier)); tier != "" {
		return tier, "default"
	}
	return "", ""
}

func agentRuntimeSubagentFromMap(
	raw map[string]any,
	cfg config.Config,
	tierMap map[string]agentRuntimeTierOption,
	fallbackTier string,
	fallbackSource string,
	runs []agentruntime.Run,
) agentRuntimeSubagentView {
	name := stringFromMap(raw, "name")
	defaultTier := strings.ToLower(strings.TrimSpace(stringFromMap(raw, "tier")))
	effectiveTier := defaultTier
	tierSource := "agent"
	if effectiveTier == "" {
		effectiveTier = fallbackTier
		tierSource = fallbackSource
	}
	if effectiveTier == "" {
		tierSource = ""
	}

	view := agentRuntimeSubagentView{
		Name:               name,
		Description:        stringFromMap(raw, "description"),
		Enabled:            boolFromMap(raw, "enabled"),
		Kind:               stringFromMap(raw, "kind"),
		Source:             stringFromMap(raw, "source"),
		Entry:              stringFromMap(raw, "entry"),
		Default:            boolFromMap(raw, "default"),
		PolicyMode:         stringFromMap(raw, "policy_mode"),
		ToolsAllow:         stringSliceFromMap(raw, "tools_allow"),
		ToolsAllowCount:    intFromMap(raw, "tools_allow_count"),
		ToolsDeny:          stringSliceFromMap(raw, "tools_deny"),
		ToolsDenyCount:     intFromMap(raw, "tools_deny_count"),
		ToolsRiskMax:       stringFromMap(raw, "tools_risk_max"),
		ToolsAllowGroups:   stringSliceFromMap(raw, "tools_allow_groups"),
		ToolsDenyGroups:    stringSliceFromMap(raw, "tools_deny_groups"),
		ToolsAllowPatterns: stringSliceFromMap(raw, "tools_allow_patterns"),
		SessionRoutingMode: stringFromMap(raw, "session_routing_mode"),
		SessionFixedID:     stringFromMap(raw, "session_fixed_id"),
		DefaultTier:        defaultTier,
		EffectiveTier:      effectiveTier,
		TierSource:         tierSource,
		TierEditable:       isWorkspaceAgentRuntimeEntry(stringFromMap(raw, "entry"), cfg.WorkspaceDir),
		ProviderOverride:   providerOverrideFromMap(raw, "provider_override"),
		RecentRuns:         []agentRuntimeSubagentRunSummary{},
	}
	resolveAgentRuntimeSubagentTier(&view, cfg, tierMap)
	view.RecentRuns, view.RunCount = recentRunsForAgent(runs, name, 5)
	if len(view.RecentRuns) > 0 {
		last := view.RecentRuns[0]
		view.LastRun = &last
	}
	return view
}

func resolveAgentRuntimeSubagentTier(view *agentRuntimeSubagentView, cfg config.Config, tierMap map[string]agentRuntimeTierOption) {
	if view == nil || strings.TrimSpace(view.EffectiveTier) == "" {
		return
	}
	tier := strings.ToLower(strings.TrimSpace(view.EffectiveTier))
	option, ok := tierMap[tier]
	if !ok {
		view.TierMissing = true
		view.TierError = fmt.Sprintf("tier %q is not configured in llm_tiers", tier)
		return
	}
	if option.Error != "" {
		view.TierError = option.Error
		return
	}
	view.ResolvedAlias = option.ProviderAlias
	view.ResolvedKind = option.Kind
	view.ResolvedModel = option.Model

	if view.ProviderOverride == nil {
		return
	}
	resolved, meta, err := agentruntime.ResolveOverride(&cfg, tier, view.ProviderOverride, "agent")
	if err != nil {
		view.TierError = err.Error()
		return
	}
	view.ResolvedAlias = meta.ResolvedAlias
	if view.ResolvedAlias == "" {
		view.ResolvedAlias = resolved.ProviderAlias
	}
	view.ResolvedKind = meta.ResolvedKind
	if view.ResolvedKind == "" {
		view.ResolvedKind = resolved.Kind
	}
	view.ResolvedModel = meta.ResolvedModel
	if view.ResolvedModel == "" {
		view.ResolvedModel = resolved.Model
	}
}

func recentRunsForAgent(runs []agentruntime.Run, agent string, maxRecent int) ([]agentRuntimeSubagentRunSummary, int) {
	if maxRecent <= 0 {
		maxRecent = 5
	}
	recent := make([]agentRuntimeSubagentRunSummary, 0, maxRecent)
	count := 0
	for _, run := range runs {
		if strings.TrimSpace(run.Agent) != strings.TrimSpace(agent) {
			continue
		}
		count++
		if len(recent) >= maxRecent {
			continue
		}
		recent = append(recent, agentRuntimeSubagentRunSummary{
			RunID:       run.ID,
			Status:      string(run.Status),
			Tier:        strings.TrimSpace(run.Tier),
			CreatedAt:   run.CreatedAt,
			UpdatedAt:   run.UpdatedAt,
			CompletedAt: run.CompletedAt,
		})
	}
	return recent, count
}

func stringFromMap(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	if v, ok := values[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func boolFromMap(values map[string]any, key string) bool {
	if values == nil {
		return false
	}
	if v, ok := values[key].(bool); ok {
		return v
	}
	return false
}

func intFromMap(values map[string]any, key string) int {
	if values == nil {
		return 0
	}
	switch v := values[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func stringSliceFromMap(values map[string]any, key string) []string {
	if values == nil {
		return []string{}
	}
	switch raw := values[key].(type) {
	case []string:
		return append([]string{}, raw...)
	case []any:
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	default:
		return []string{}
	}
}

func providerOverrideFromMap(values map[string]any, key string) *agentruntime.ProviderOverride {
	if values == nil {
		return nil
	}
	if override, ok := values[key].(*agentruntime.ProviderOverride); ok {
		return agentruntime.CloneProviderOverride(override)
	}
	if raw, ok := values[key].(map[string]any); ok {
		return agentruntime.CloneProviderOverride(&agentruntime.ProviderOverride{
			Alias: stringFromMap(raw, "alias"),
			Model: stringFromMap(raw, "model"),
		})
	}
	return nil
}

func isWorkspaceAgentRuntimeEntry(entry string, workspaceDir string) bool {
	_, ok := workspaceAgentRuntimeEntryPath(entry, workspaceDir)
	return ok
}

func workspaceAgentRuntimeEntryPath(entry string, workspaceDir string) (string, bool) {
	path := strings.TrimSpace(entry)
	root := strings.TrimSpace(workspaceDir)
	if path == "" || root == "" {
		return "", false
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path = filepath.Clean(path)
	agentsRoot := filepath.Join(filepath.Clean(root), "agents")
	rel, err := filepath.Rel(agentsRoot, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", false
	}
	if !strings.EqualFold(filepath.Base(path), "AGENT.md") {
		return "", false
	}
	return path, true
}

func updateWorkspaceAgentRuntimeAgentTier(path string, tier string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read agent profile: %w", err)
	}
	metaBlock, body, hasFrontmatter, err := splitYAMLFrontmatter(string(raw))
	if err != nil {
		return fmt.Errorf("parse agent profile frontmatter: %w", err)
	}
	if !hasFrontmatter && strings.TrimSpace(tier) == "" {
		return nil
	}
	meta := map[string]any{}
	if hasFrontmatter && strings.TrimSpace(metaBlock) != "" {
		if err := yaml.Unmarshal([]byte(metaBlock), &meta); err != nil {
			return fmt.Errorf("decode agent profile frontmatter: %w", err)
		}
	}
	if strings.TrimSpace(tier) == "" {
		delete(meta, "tier")
	} else {
		meta["tier"] = strings.ToLower(strings.TrimSpace(tier))
	}
	if len(meta) == 0 {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return fmt.Errorf("write agent profile: %w", err)
		}
		return nil
	}
	encoded, err := yaml.Marshal(meta)
	if err != nil {
		return fmt.Errorf("encode agent profile frontmatter: %w", err)
	}
	var out strings.Builder
	out.WriteString("---\n")
	out.Write(encoded)
	out.WriteString("---\n")
	out.WriteString(body)
	if err := os.WriteFile(path, []byte(out.String()), 0o644); err != nil {
		return fmt.Errorf("write agent profile: %w", err)
	}
	return nil
}
