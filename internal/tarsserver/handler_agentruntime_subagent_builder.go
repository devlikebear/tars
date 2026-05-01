package tarsserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/llm"
	"gopkg.in/yaml.v3"
)

const (
	agentRuntimeSubagentBuilderLLMSystemPrompt = "Create a TARS subagent profile draft. Return only a json object with action, name, description, default_tier, prompt, tools_allow, tools_deny, tools_risk_max, session_routing_mode, and session_fixed_id. Use only configured tiers and safe tool names."
	agentRuntimeSubagentBuilderLLMResponseHint = "json"
)

type agentRuntimeSubagentBuilderDraftRequest struct {
	Mode        string `json:"mode"`
	Request     string `json:"request"`
	BaseName    string `json:"base_name,omitempty"`
	DefaultTier string `json:"default_tier,omitempty"`
	UseLLM      *bool  `json:"use_llm,omitempty"`
}

type agentRuntimeSubagentDraft struct {
	Action             string                                `json:"action"`
	Name               string                                `json:"name"`
	Description        string                                `json:"description"`
	DefaultTier        string                                `json:"default_tier"`
	Prompt             string                                `json:"prompt"`
	ToolsAllow         []string                              `json:"tools_allow"`
	ToolsDeny          []string                              `json:"tools_deny"`
	ToolsRiskMax       string                                `json:"tools_risk_max,omitempty"`
	SessionRoutingMode string                                `json:"session_routing_mode,omitempty"`
	SessionFixedID     string                                `json:"session_fixed_id,omitempty"`
	Provenance         []agentRuntimeSubagentDraftProvenance `json:"provenance,omitempty"`
}

type agentRuntimeSubagentDraftProvenance struct {
	RunID       string `json:"run_id"`
	Agent       string `json:"agent,omitempty"`
	Status      string `json:"status,omitempty"`
	Tier        string `json:"tier,omitempty"`
	Prompt      string `json:"prompt,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
}

type agentRuntimeSubagentBuilderDraftResponse struct {
	Draft        agentRuntimeSubagentDraft `json:"draft"`
	DraftSource  string                    `json:"draft_source"`
	Warnings     []string                  `json:"warnings,omitempty"`
	Tiers        []agentRuntimeTierOption  `json:"tiers"`
	ResolvedTier *agentRuntimeTierOption   `json:"resolved_tier,omitempty"`
}

type agentRuntimeSubagentBuilderApplyRequest struct {
	Draft agentRuntimeSubagentDraft `json:"draft"`
}

type agentRuntimeSubagentArchiveRequest struct {
	Confirm bool   `json:"confirm"`
	Reason  string `json:"reason,omitempty"`
}

func handleAgentRuntimeSubagentBuilderDraft(
	w http.ResponseWriter,
	r *http.Request,
	runtime *agentruntime.Runtime,
	cfg config.Config,
	router llm.Router,
) {
	var req agentRuntimeSubagentBuilderDraftRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	request := strings.TrimSpace(req.Request)
	if request == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request is required"})
		return
	}
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "create"
	}
	if mode != "create" && mode != "edit" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "mode must be create or edit"})
		return
	}

	resp := buildAgentRuntimeSubagentsResponse(runtime, cfg)
	var base *agentRuntimeSubagentView
	if mode == "edit" {
		baseName := strings.TrimSpace(req.BaseName)
		for i := range resp.Agents {
			if strings.EqualFold(resp.Agents[i].Name, baseName) {
				base = &resp.Agents[i]
				break
			}
		}
		if base == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "subagent not found"})
			return
		}
		if !base.TierEditable {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "only workspace AGENT.md profiles can be edited with the builder"})
			return
		}
	}

	tiers, tierMap := agentRuntimeTierOptions(cfg)
	draft, source, warnings := agentRuntimeDraftWithOptionalLLM(r.Context(), router, req, request, cfg, resp, base)
	if errText := validateAgentRuntimeSubagentDraft(cfg, runtime, draft); errText != "" {
		warnings = append(warnings, errText)
	}
	response := agentRuntimeSubagentBuilderDraftResponse{
		Draft:       draft,
		DraftSource: source,
		Warnings:    warnings,
		Tiers:       tiers,
	}
	if option, ok := tierMap[strings.ToLower(strings.TrimSpace(draft.DefaultTier))]; ok {
		response.ResolvedTier = &option
	}
	writeJSON(w, http.StatusOK, response)
}

func handleAgentRuntimeSubagentBuilderApply(
	w http.ResponseWriter,
	r *http.Request,
	runtime *agentruntime.Runtime,
	cfg config.Config,
	reloadHook func(),
) {
	var req agentRuntimeSubagentBuilderApplyRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	draft := normalizeAgentRuntimeSubagentDraft(req.Draft, cfg, nil)
	if errText := validateAgentRuntimeSubagentDraft(cfg, runtime, draft); errText != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": errText})
		return
	}

	path, err := pathForAgentRuntimeSubagentDraft(runtime, cfg, draft)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	rendered, err := renderAgentRuntimeSubagentDraftDocument(draft)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("create agent directory: %v", err)})
		return
	}
	if err := os.WriteFile(path, []byte(rendered), 0o644); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("write agent profile: %v", err)})
		return
	}
	if reloadHook != nil {
		reloadHook()
	}
	updated := buildAgentRuntimeSubagentsResponse(runtime, cfg)
	for _, agent := range updated.Agents {
		if strings.EqualFold(agent.Name, draft.Name) {
			writeJSON(w, http.StatusOK, agent)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": draft.Name})
}

func handleAgentRuntimeSubagentArchive(
	w http.ResponseWriter,
	r *http.Request,
	runtime *agentruntime.Runtime,
	cfg config.Config,
	reloadHook func(),
) {
	var req agentRuntimeSubagentArchiveRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	name, ok := parseAgentRuntimeSubagentArchiveName(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	resp := buildAgentRuntimeSubagentsResponse(runtime, cfg)
	var target *agentRuntimeSubagentView
	for i := range resp.Agents {
		if strings.EqualFold(resp.Agents[i].Name, name) {
			target = &resp.Agents[i]
			break
		}
	}
	if target == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "subagent not found"})
		return
	}
	if !target.TierEditable {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "only workspace AGENT.md profiles can be archived"})
		return
	}
	if !req.Confirm {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":     "confirm is required before archiving a subagent",
			"run_count": target.RunCount,
		})
		return
	}
	entry, ok := workspaceAgentRuntimeEntryPath(target.Entry, cfg.WorkspaceDir)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "subagent entry is not editable"})
		return
	}
	archivePath := filepath.Join(filepath.Dir(entry), "AGENT.archived."+time.Now().UTC().Format("20060102T150405Z")+".md")
	if err := os.Rename(entry, archivePath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("archive agent profile: %v", err)})
		return
	}
	if reloadHook != nil {
		reloadHook()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"archived":      true,
		"name":          target.Name,
		"archived_path": archivePath,
		"run_count":     target.RunCount,
	})
}

func agentRuntimeDraftWithOptionalLLM(
	ctx context.Context,
	router llm.Router,
	req agentRuntimeSubagentBuilderDraftRequest,
	request string,
	cfg config.Config,
	resp agentRuntimeSubagentsResponse,
	base *agentRuntimeSubagentView,
) (agentRuntimeSubagentDraft, string, []string) {
	useLLM := req.UseLLM == nil || *req.UseLLM
	if useLLM && router != nil {
		if draft, err := draftAgentRuntimeSubagentWithLLM(ctx, router, req, request, cfg, resp, base); err == nil {
			return normalizeAgentRuntimeSubagentDraft(draft, cfg, base), "llm", nil
		} else {
			fallback := heuristicAgentRuntimeSubagentDraft(req, request, cfg, base)
			return fallback, "heuristic", []string{"LLM draft failed; generated a local draft instead: " + err.Error()}
		}
	}
	return heuristicAgentRuntimeSubagentDraft(req, request, cfg, base), "heuristic", nil
}

func draftAgentRuntimeSubagentWithLLM(
	ctx context.Context,
	router llm.Router,
	req agentRuntimeSubagentBuilderDraftRequest,
	request string,
	cfg config.Config,
	resp agentRuntimeSubagentsResponse,
	base *agentRuntimeSubagentView,
) (agentRuntimeSubagentDraft, error) {
	client, _, err := router.ClientFor(llm.RoleAgentRuntimePlanner)
	if err != nil {
		return agentRuntimeSubagentDraft{}, err
	}
	tierNames := make([]string, 0, len(resp.Tiers))
	for _, tier := range resp.Tiers {
		if tier.Error == "" {
			tierNames = append(tierNames, tier.Name)
		}
	}
	payload := map[string]any{
		"mode":            req.Mode,
		"request":         request,
		"default_tier":    preferredAgentRuntimeBuilderTier(req.DefaultTier, cfg),
		"allowed_tiers":   tierNames,
		"response_format": agentRuntimeSubagentBuilderLLMResponseHint,
	}
	if base != nil {
		payload["base"] = base
	}
	raw, _ := json.Marshal(payload)
	messages := []llm.ChatMessage{
		{Role: "system", Content: agentRuntimeSubagentBuilderLLMSystemPrompt},
		{Role: "user", Content: string(raw)},
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	respLLM, err := client.Chat(ctx, messages, llm.ChatOptions{
		ResponseFormat: &llm.ResponseFormat{Type: llm.ResponseFormatJSONObject},
	})
	if err != nil {
		return agentRuntimeSubagentDraft{}, err
	}
	var draft agentRuntimeSubagentDraft
	if err := json.Unmarshal([]byte(strings.TrimSpace(respLLM.Message.Content)), &draft); err != nil {
		return agentRuntimeSubagentDraft{}, fmt.Errorf("decode llm draft: %w", err)
	}
	return draft, nil
}

func heuristicAgentRuntimeSubagentDraft(req agentRuntimeSubagentBuilderDraftRequest, request string, cfg config.Config, base *agentRuntimeSubagentView) agentRuntimeSubagentDraft {
	action := "create"
	name := deriveAgentRuntimeSubagentName(request)
	description := deriveAgentRuntimeSubagentDescription(name, request)
	prompt := composeAgentRuntimeSubagentPrompt(name, request)
	toolsAllow := []string{"glob", "list_dir", "read_file"}
	if base != nil {
		action = "update"
		name = base.Name
		description = firstNonEmpty(base.Description, description)
		prompt = composeAgentRuntimeEditedSubagentPrompt(base, request)
		if len(base.ToolsAllow) > 0 {
			toolsAllow = append([]string(nil), base.ToolsAllow...)
		}
	}
	draft := agentRuntimeSubagentDraft{
		Action:             action,
		Name:               name,
		Description:        description,
		DefaultTier:        preferredAgentRuntimeBuilderTier(req.DefaultTier, cfg),
		Prompt:             prompt,
		ToolsAllow:         toolsAllow,
		ToolsDeny:          []string{},
		SessionRoutingMode: "caller",
	}
	return normalizeAgentRuntimeSubagentDraft(draft, cfg, base)
}

func normalizeAgentRuntimeSubagentDraft(draft agentRuntimeSubagentDraft, cfg config.Config, base *agentRuntimeSubagentView) agentRuntimeSubagentDraft {
	draft.Action = strings.ToLower(strings.TrimSpace(draft.Action))
	switch draft.Action {
	case "edit", "modify", "replace":
		draft.Action = "update"
	case "new":
		draft.Action = "create"
	}
	if draft.Action == "" {
		if base != nil {
			draft.Action = "update"
		} else {
			draft.Action = "create"
		}
	}
	draft.Name = strings.ToLower(strings.TrimSpace(draft.Name))
	if draft.Name == "" && base != nil {
		draft.Name = base.Name
	}
	draft.Description = strings.TrimSpace(draft.Description)
	draft.DefaultTier = preferredAgentRuntimeBuilderTier(draft.DefaultTier, cfg)
	draft.Prompt = strings.TrimSpace(draft.Prompt)
	draft.ToolsAllow = normalizeToolNames(draft.ToolsAllow)
	draft.ToolsDeny = normalizeToolNames(draft.ToolsDeny)
	draft.ToolsRiskMax = strings.TrimSpace(draft.ToolsRiskMax)
	draft.SessionRoutingMode = normalizeAgentRuntimeSessionRoutingMode(draft.SessionRoutingMode)
	draft.SessionFixedID = strings.TrimSpace(draft.SessionFixedID)
	draft.Provenance = normalizeAgentRuntimeSubagentDraftProvenance(draft.Provenance)
	if len(draft.ToolsAllow) == 0 {
		draft.ToolsAllow = []string{"glob", "list_dir", "read_file"}
	}
	return draft
}

func normalizeAgentRuntimeSubagentDraftProvenance(entries []agentRuntimeSubagentDraftProvenance) []agentRuntimeSubagentDraftProvenance {
	if len(entries) == 0 {
		return nil
	}
	out := make([]agentRuntimeSubagentDraftProvenance, 0, min(len(entries), 8))
	seen := map[string]struct{}{}
	for _, entry := range entries {
		runID := strings.TrimSpace(entry.RunID)
		if runID == "" {
			continue
		}
		if _, exists := seen[runID]; exists {
			continue
		}
		seen[runID] = struct{}{}
		out = append(out, agentRuntimeSubagentDraftProvenance{
			RunID:       runID,
			Agent:       strings.TrimSpace(entry.Agent),
			Status:      strings.TrimSpace(entry.Status),
			Tier:        strings.TrimSpace(entry.Tier),
			Prompt:      trimAgentRuntimeRecommendationText(entry.Prompt, 240),
			CreatedAt:   strings.TrimSpace(entry.CreatedAt),
			CompletedAt: strings.TrimSpace(entry.CompletedAt),
		})
		if len(out) >= 8 {
			break
		}
	}
	return out
}

func validateAgentRuntimeSubagentDraft(cfg config.Config, runtime *agentruntime.Runtime, draft agentRuntimeSubagentDraft) string {
	if draft.Action != "create" && draft.Action != "update" {
		return "draft action must be create or update"
	}
	if !isValidAgentRuntimeAgentName(draft.Name) {
		return "subagent name must use letters, numbers, dash, underscore, or dot"
	}
	if strings.TrimSpace(draft.Prompt) == "" {
		return "prompt is required"
	}
	if len(draft.Prompt) > 20000 {
		return "prompt is too large"
	}
	tier := strings.ToLower(strings.TrimSpace(draft.DefaultTier))
	if tier != "" {
		if _, ok := cfg.LLMTiers[tier]; !ok {
			return fmt.Sprintf("tier %q is not configured in llm_tiers", tier)
		}
		if _, err := config.ResolveLLMTier(&cfg, tier); err != nil {
			return err.Error()
		}
	}
	if draft.Action == "create" && runtime != nil {
		if _, ok := runtime.LookupAgent(draft.Name); ok {
			return fmt.Sprintf("subagent %q already exists", draft.Name)
		}
	}
	path, err := pathForAgentRuntimeSubagentDraft(runtime, cfg, draft)
	if err != nil {
		return err.Error()
	}
	rendered, err := renderAgentRuntimeSubagentDraftDocument(draft)
	if err != nil {
		return err.Error()
	}
	_, diagnostics, ok, err := buildWorkspaceAgentRuntimeAgent(path, rendered, knownAgentRuntimePromptTools(cfg.WorkspaceDir))
	if err != nil {
		return err.Error()
	}
	if !ok {
		return strings.Join(diagnostics, "; ")
	}
	return ""
}

func pathForAgentRuntimeSubagentDraft(runtime *agentruntime.Runtime, cfg config.Config, draft agentRuntimeSubagentDraft) (string, error) {
	root := strings.TrimSpace(cfg.WorkspaceDir)
	if root == "" {
		return "", fmt.Errorf("workspace_dir is required")
	}
	if draft.Action == "update" {
		resp := buildAgentRuntimeSubagentsResponse(runtime, cfg)
		for _, agent := range resp.Agents {
			if strings.EqualFold(agent.Name, draft.Name) {
				if !agent.TierEditable {
					return "", fmt.Errorf("only workspace AGENT.md profiles can be updated")
				}
				if path, ok := workspaceAgentRuntimeEntryPath(agent.Entry, cfg.WorkspaceDir); ok {
					return path, nil
				}
				return "", fmt.Errorf("subagent entry is not editable")
			}
		}
		return "", fmt.Errorf("subagent %q not found", draft.Name)
	}
	return filepath.Join(root, "agents", draft.Name, "AGENT.md"), nil
}

func renderAgentRuntimeSubagentDraftDocument(draft agentRuntimeSubagentDraft) (string, error) {
	meta := map[string]any{
		"name":        strings.TrimSpace(draft.Name),
		"description": strings.TrimSpace(draft.Description),
	}
	if strings.TrimSpace(draft.DefaultTier) != "" {
		meta["tier"] = strings.ToLower(strings.TrimSpace(draft.DefaultTier))
	}
	if len(draft.ToolsAllow) > 0 {
		meta["tools_allow"] = append([]string(nil), draft.ToolsAllow...)
	}
	if len(draft.ToolsDeny) > 0 {
		meta["tools_deny"] = append([]string(nil), draft.ToolsDeny...)
	}
	if strings.TrimSpace(draft.ToolsRiskMax) != "" {
		meta["tools_risk_max"] = strings.TrimSpace(draft.ToolsRiskMax)
	}
	if strings.TrimSpace(draft.SessionRoutingMode) != "" && strings.TrimSpace(draft.SessionRoutingMode) != "caller" {
		meta["session_routing_mode"] = strings.TrimSpace(draft.SessionRoutingMode)
	}
	if strings.TrimSpace(draft.SessionFixedID) != "" {
		meta["session_fixed_id"] = strings.TrimSpace(draft.SessionFixedID)
	}
	if len(draft.Provenance) > 0 {
		meta["provenance"] = agentRuntimeSubagentDraftProvenanceMeta(draft.Provenance)
	}
	encoded, err := yaml.Marshal(meta)
	if err != nil {
		return "", fmt.Errorf("encode agent profile: %w", err)
	}
	var out bytes.Buffer
	out.WriteString("---\n")
	out.Write(encoded)
	out.WriteString("---\n")
	out.WriteString(strings.TrimSpace(draft.Prompt))
	out.WriteString("\n")
	return out.String(), nil
}

func agentRuntimeSubagentDraftProvenanceMeta(entries []agentRuntimeSubagentDraftProvenance) map[string]any {
	runs := make([]map[string]string, 0, len(entries))
	for _, entry := range entries {
		row := map[string]string{"run_id": strings.TrimSpace(entry.RunID)}
		if strings.TrimSpace(entry.Agent) != "" {
			row["agent"] = strings.TrimSpace(entry.Agent)
		}
		if strings.TrimSpace(entry.Status) != "" {
			row["status"] = strings.TrimSpace(entry.Status)
		}
		if strings.TrimSpace(entry.Tier) != "" {
			row["tier"] = strings.TrimSpace(entry.Tier)
		}
		if strings.TrimSpace(entry.CreatedAt) != "" {
			row["created_at"] = strings.TrimSpace(entry.CreatedAt)
		}
		if strings.TrimSpace(entry.CompletedAt) != "" {
			row["completed_at"] = strings.TrimSpace(entry.CompletedAt)
		}
		runs = append(runs, row)
	}
	return map[string]any{
		"source": "agentruntime_recommendation",
		"runs":   runs,
	}
}

func preferredAgentRuntimeBuilderTier(raw string, cfg config.Config) string {
	tier := strings.ToLower(strings.TrimSpace(raw))
	if tier != "" {
		if _, ok := cfg.LLMTiers[tier]; ok {
			return tier
		}
	}
	if tier = strings.ToLower(strings.TrimSpace(cfg.LLMRoleDefaults[string(llm.RoleAgentRuntimePlanner)])); tier != "" {
		if _, ok := cfg.LLMTiers[tier]; ok {
			return tier
		}
	}
	if tier = strings.ToLower(strings.TrimSpace(cfg.LLMRoleDefaults[string(llm.RoleAgentRuntimeDefault)])); tier != "" {
		if _, ok := cfg.LLMTiers[tier]; ok {
			return tier
		}
	}
	if tier = strings.ToLower(strings.TrimSpace(cfg.LLMDefaultTier)); tier != "" {
		if _, ok := cfg.LLMTiers[tier]; ok {
			return tier
		}
	}
	names := make([]string, 0, len(cfg.LLMTiers))
	for name := range cfg.LLMTiers {
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func deriveAgentRuntimeSubagentName(request string) string {
	words := agentRuntimeSubagentKeywords(request)
	if len(words) == 0 {
		return "custom-agent"
	}
	if len(words) > 3 {
		words = words[:3]
	}
	return strings.Join(words, "-")
}

func deriveAgentRuntimeSubagentDescription(name string, request string) string {
	label := strings.ReplaceAll(strings.TrimSpace(name), "-", " ")
	if label == "" {
		label = "custom"
	}
	return strings.ToUpper(label[:1]) + label[1:] + " subagent"
}

func agentRuntimeSubagentKeywords(request string) []string {
	normalized := strings.ToLower(strings.TrimSpace(request))
	re := regexp.MustCompile(`[^a-z0-9]+`)
	parts := strings.Fields(re.ReplaceAllString(normalized, " "))
	stop := map[string]struct{}{
		"a": {}, "an": {}, "the": {}, "new": {}, "create": {}, "make": {}, "build": {}, "add": {},
		"agent": {}, "subagent": {}, "sub": {}, "for": {}, "to": {}, "please": {}, "worker": {},
	}
	words := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if len(part) < 2 {
			continue
		}
		if _, skip := stop[part]; skip {
			continue
		}
		words = append(words, part)
	}
	return words
}

func composeAgentRuntimeSubagentPrompt(name string, request string) string {
	focus := strings.TrimSpace(request)
	if focus == "" {
		focus = strings.ReplaceAll(name, "-", " ")
	}
	return strings.TrimSpace(fmt.Sprintf(`You are the %s subagent.

Focus: %s

- Work inside the current workspace.
- Gather concrete evidence before making recommendations.
- Prefer concise findings with file paths, commands, or artifacts when relevant.
- Call out uncertainty and next steps instead of guessing.`, strings.TrimSpace(name), focus))
}

func composeAgentRuntimeEditedSubagentPrompt(base *agentRuntimeSubagentView, request string) string {
	current := ""
	if base != nil {
		current = strings.TrimSpace(base.Description)
	}
	focus := strings.TrimSpace(request)
	if current != "" {
		return composeAgentRuntimeSubagentPrompt(base.Name, current+"\n\nUpdate request: "+focus)
	}
	return composeAgentRuntimeSubagentPrompt(base.Name, focus)
}

func parseAgentRuntimeSubagentArchiveName(path string) (string, bool) {
	trimmed := strings.TrimSpace(path)
	trimmed = strings.TrimSuffix(trimmed, "/archive")
	return parseAgentRuntimeSubagentName(trimmed)
}
