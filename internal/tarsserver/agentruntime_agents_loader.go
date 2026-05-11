package tarsserver

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/tool"
)

func loadWorkspaceAgentRuntimeAgents(workspaceDir string) ([]workspaceAgentRuntimeAgent, []string, error) {
	files, err := findWorkspaceAgentRuntimeAgentFiles(workspaceDir)
	if err != nil {
		return nil, nil, err
	}

	knownTools := knownAgentRuntimePromptTools(strings.TrimSpace(workspaceDir))
	loaded := make([]workspaceAgentRuntimeAgent, 0, len(files))
	diagnostics := make([]string, 0)
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		agent, agentDiagnostics, ok, err := buildWorkspaceAgentRuntimeAgent(path, string(raw), knownTools)
		diagnostics = append(diagnostics, agentDiagnostics...)
		if err != nil {
			return nil, nil, err
		}
		if ok {
			loaded = append(loaded, agent)
		}
	}

	seen := map[string]struct{}{}
	out := make([]workspaceAgentRuntimeAgent, 0, len(loaded))
	for _, item := range loaded {
		key := strings.ToLower(strings.TrimSpace(item.Name))
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			diagnostics = append(diagnostics, fmt.Sprintf("skip duplicate agent name: %s", item.Name))
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out, diagnostics, nil
}

func findWorkspaceAgentRuntimeAgentFiles(workspaceDir string) ([]string, error) {
	base := strings.TrimSpace(workspaceDir)
	if base == "" {
		return []string{}, nil
	}
	root := filepath.Join(base, "agents")
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("stat agents dir %q: %w", root, err)
	}
	if !info.IsDir() {
		return []string{}, nil
	}

	files := make([]string, 0)
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Base(path), "AGENT.md") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	sort.Slice(files, func(i, j int) bool {
		left := strings.ToLower(files[i])
		right := strings.ToLower(files[j])
		return left < right
	})
	return files, nil
}

func buildWorkspaceAgentRuntimeAgent(path, raw string, knownTools map[string]struct{}) (workspaceAgentRuntimeAgent, []string, bool, error) {
	meta, body, err := parseWorkspaceAgentRuntimeAgentDocument(raw)
	if err != nil {
		return workspaceAgentRuntimeAgent{}, []string{fmt.Sprintf("skip %s: invalid frontmatter: %v", path, err)}, false, nil
	}

	name := strings.TrimSpace(meta.Name)
	if name == "" {
		name = strings.TrimSpace(filepath.Base(filepath.Dir(path)))
	}
	if !isValidAgentRuntimeAgentName(name) {
		return workspaceAgentRuntimeAgent{}, []string{fmt.Sprintf("skip %s: invalid agent name %q", path, name)}, false, nil
	}

	prompt := strings.TrimSpace(body)
	if prompt == "" {
		return workspaceAgentRuntimeAgent{}, []string{fmt.Sprintf("skip %s: empty prompt body", path)}, false, nil
	}

	description := strings.TrimSpace(meta.Description)
	if description == "" {
		description = inferAgentRuntimeAgentDescription(prompt)
	}
	if description == "" {
		description = "Workspace markdown sub-agent"
	}

	policyMode, toolsAllow, toolsDeny, toolsRiskMax, toolsAllowGroups, toolsDenyGroups, toolsAllowPatterns, diagnostics, ok := buildWorkspaceAgentRuntimeAgentPolicy(name, meta, knownTools)
	if !ok {
		return workspaceAgentRuntimeAgent{}, diagnostics, false, nil
	}

	sessionRoutingMode := normalizeAgentRuntimeSessionRoutingMode(meta.SessionRoutingMode)
	sessionFixedID := strings.TrimSpace(meta.SessionFixedID)
	if sessionRoutingMode == "fixed" && sessionFixedID == "" {
		diagnostics = append(diagnostics, fmt.Sprintf("skip agent %s: session_routing_mode fixed requires session_fixed_id", name))
		return workspaceAgentRuntimeAgent{}, diagnostics, false, nil
	}

	tier := strings.ToLower(strings.TrimSpace(meta.Tier))

	return workspaceAgentRuntimeAgent{
		Name:               name,
		Description:        description,
		Prompt:             prompt,
		FilePath:           path,
		PolicyMode:         policyMode,
		ToolsAllow:         toolsAllow,
		ToolsDeny:          toolsDeny,
		ToolsRiskMax:       toolsRiskMax,
		ToolsAllowGroups:   toolsAllowGroups,
		ToolsDenyGroups:    toolsDenyGroups,
		ToolsAllowPatterns: toolsAllowPatterns,
		SessionRoutingMode: sessionRoutingMode,
		SessionFixedID:     sessionFixedID,
		Tier:               tier,
		ProviderOverride:   agentruntime.CloneProviderOverride(meta.ProviderOverride),
	}, diagnostics, true, nil
}

func buildWorkspaceAgentRuntimeAgentPolicy(
	name string,
	meta workspaceAgentRuntimeAgentFrontmatter,
	knownTools map[string]struct{},
) (string, []string, []string, string, []string, []string, []string, []string, bool) {
	policyMode := "full"
	toolsAllow := []string{}
	toolsDeny := []string{}
	toolsRiskMax := ""
	toolsAllowGroups := []string{}
	toolsDenyGroups := []string{}
	toolsAllowPatterns := []string{}
	diagnostics := make([]string, 0)

	policyRequested := meta.ToolsAllowExists ||
		meta.ToolsAllowGroupsExists ||
		meta.ToolsDenyGroupsExists ||
		meta.ToolsAllowPatternsExists ||
		meta.ToolsDenyExists ||
		strings.TrimSpace(meta.ToolsRiskMax) != ""
	if !policyRequested {
		return policyMode, toolsAllow, toolsDeny, toolsRiskMax, toolsAllowGroups, toolsDenyGroups, toolsAllowPatterns, diagnostics, true
	}

	policyMode = "allowlist"
	seen := map[string]struct{}{}
	addUnique := func(names []string) {
		for _, n := range names {
			if _, exists := seen[n]; !exists {
				seen[n] = struct{}{}
				toolsAllow = append(toolsAllow, n)
			}
		}
	}

	// Explicit allow list
	if meta.ToolsAllowExists {
		for _, t := range meta.ToolsAllow {
			normalized := tool.CanonicalToolName(t)
			if normalized == "" {
				continue
			}
			if _, ok := knownTools[normalized]; ok {
				addUnique([]string{normalized})
			} else {
				diagnostics = append(diagnostics, fmt.Sprintf("agent %s tools_allow ignored unknown tool: %s", name, normalized))
			}
		}
	}

	// Expand groups
	if meta.ToolsAllowGroupsExists {
		validGroups, expanded, unknowns := tool.ExpandToolGroups(meta.ToolsAllowGroups, knownTools)
		toolsAllowGroups = validGroups
		addUnique(expanded)
		for _, u := range unknowns {
			diagnostics = append(diagnostics, fmt.Sprintf("agent %s tools_allow_groups ignored unknown group: %s", name, u))
		}
	}

	if meta.ToolsDenyGroupsExists {
		validGroups, expanded, unknowns := tool.ExpandToolGroups(meta.ToolsDenyGroups, knownTools)
		toolsDenyGroups = validGroups
		for _, expandedTool := range expanded {
			toolsDeny = append(toolsDeny, expandedTool)
		}
		for _, u := range unknowns {
			diagnostics = append(diagnostics, fmt.Sprintf("agent %s tools_deny_groups ignored unknown group: %s", name, u))
		}
	}

	// Expand patterns
	if meta.ToolsAllowPatternsExists {
		validPatterns, matched, invalids := tool.ExpandToolPatterns(meta.ToolsAllowPatterns, knownTools)
		toolsAllowPatterns = validPatterns
		addUnique(matched)
		for _, inv := range invalids {
			diagnostics = append(diagnostics, fmt.Sprintf("agent %s tools_allow_patterns invalid regex: %s", name, inv))
		}
	}

	// If no allow sources specified but policy requested, allow all
	if !meta.ToolsAllowExists && !meta.ToolsAllowGroupsExists && !meta.ToolsAllowPatternsExists && len(knownTools) > 0 {
		for t := range knownTools {
			addUnique([]string{t})
		}
	}

	// Deny list
	if meta.ToolsDenyExists {
		for _, t := range meta.ToolsDeny {
			normalized := tool.CanonicalToolName(t)
			if normalized == "" {
				continue
			}
			toolsDeny = append(toolsDeny, normalized)
		}
	}
	if len(toolsDeny) > 0 {
		toolsDeny = normalizeToolNames(toolsDeny)
	}
	toolsRiskMax = strings.TrimSpace(meta.ToolsRiskMax)

	// Apply deny filter
	if len(toolsDeny) > 0 {
		denySet := map[string]struct{}{}
		for _, d := range toolsDeny {
			denySet[d] = struct{}{}
		}
		filtered := make([]string, 0, len(toolsAllow))
		for _, t := range toolsAllow {
			if _, denied := denySet[t]; !denied {
				filtered = append(filtered, t)
			}
		}
		toolsAllow = filtered
	}

	// Apply risk_max filter
	if toolsRiskMax != "" {
		filtered := make([]string, 0, len(toolsAllow))
		for _, t := range toolsAllow {
			if toolsRiskMax == "low" && isHighRiskToolName(t) {
				continue
			}
			filtered = append(filtered, t)
		}
		toolsAllow = filtered
	}

	sort.Strings(toolsAllow)

	if len(toolsAllow) == 0 && (meta.ToolsAllowExists || meta.ToolsAllowGroupsExists || meta.ToolsAllowPatternsExists) {
		diagnostics = append(diagnostics, fmt.Sprintf("skip agent %s: tools_allow has no valid tools", name))
		return policyMode, toolsAllow, toolsDeny, toolsRiskMax, toolsAllowGroups, toolsDenyGroups, toolsAllowPatterns, diagnostics, false
	}
	return policyMode, toolsAllow, toolsDeny, toolsRiskMax, toolsAllowGroups, toolsDenyGroups, toolsAllowPatterns, diagnostics, true
}

func isValidAgentRuntimeAgentName(name string) bool {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return false
	}
	// Reject names that are purely dots ("..", "...", etc.). `.` is allowed
	// as part of a name (e.g. "github.com"), but a name made of nothing but
	// dots would let filepath.Join collapse `<workspace>/agents/<name>/...`
	// outside the agents/ directory.
	if strings.Trim(trimmed, ".") == "" {
		return false
	}
	for _, ch := range trimmed {
		if (ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') ||
			ch == '-' ||
			ch == '_' ||
			ch == '.' {
			continue
		}
		return false
	}
	return true
}

func inferAgentRuntimeAgentDescription(prompt string) string {
	lines := strings.Split(strings.ReplaceAll(prompt, "\r\n", "\n"), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		trimmed = strings.TrimLeft(trimmed, "#")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed == "" {
			continue
		}
		if len(trimmed) > 140 {
			return trimmed[:140] + "..."
		}
		return trimmed
	}
	return ""
}
