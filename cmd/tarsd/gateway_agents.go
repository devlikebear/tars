package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/devlikebear/tarsncase/internal/gateway"
	"github.com/devlikebear/tarsncase/internal/tool"
)

type workspaceGatewayAgent struct {
	Name               string
	Description        string
	Prompt             string
	FilePath           string
	PolicyMode         string
	ToolsAllow         []string
	ToolsDeny          []string
	ToolsRiskMax       string
	ToolsAllowGroups   []string
	ToolsAllowPatterns []string
	SessionRoutingMode string
	SessionFixedID     string
}

type workspaceGatewayAgentFrontmatter struct {
	Name                     string
	Description              string
	ToolsAllow               []string
	ToolsAllowExists         bool
	ToolsDeny                []string
	ToolsDenyExists          bool
	ToolsRiskMax             string
	ToolsAllowGroups         []string
	ToolsAllowGroupsExists   bool
	ToolsAllowPatterns       []string
	ToolsAllowPatternsExists bool
	SessionRoutingMode       string
	SessionFixedID           string
}

func loadWorkspaceGatewayAgents(workspaceDir string) ([]workspaceGatewayAgent, []string, error) {
	base := strings.TrimSpace(workspaceDir)
	if base == "" {
		return []workspaceGatewayAgent{}, []string{}, nil
	}
	root := filepath.Join(base, "agents")
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []workspaceGatewayAgent{}, []string{}, nil
		}
		return nil, nil, fmt.Errorf("stat agents dir %q: %w", root, err)
	}
	if !info.IsDir() {
		return []workspaceGatewayAgent{}, []string{}, nil
	}

	knownTools := knownGatewayPromptTools(base)
	knownGroups := knownGatewayPromptToolGroups(knownTools)
	loaded := make([]workspaceGatewayAgent, 0)
	diagnostics := make([]string, 0)
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Base(path), "AGENT.md") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		meta, body, err := parseWorkspaceGatewayAgentDocument(string(raw))
		if err != nil {
			diagnostics = append(diagnostics, fmt.Sprintf("skip %s: invalid frontmatter: %v", path, err))
			return nil
		}
		name := strings.TrimSpace(meta.Name)
		if name == "" {
			name = strings.TrimSpace(filepath.Base(filepath.Dir(path)))
		}
		if !isValidGatewayAgentName(name) {
			diagnostics = append(diagnostics, fmt.Sprintf("skip %s: invalid agent name %q", path, name))
			return nil
		}
		prompt := strings.TrimSpace(body)
		if prompt == "" {
			diagnostics = append(diagnostics, fmt.Sprintf("skip %s: empty prompt body", path))
			return nil
		}
		description := strings.TrimSpace(meta.Description)
		if description == "" {
			description = inferGatewayAgentDescription(prompt)
		}
		if description == "" {
			description = "Workspace markdown sub-agent"
		}
		policyMode := "full"
		toolsAllow := []string{}
		toolsDeny := []string{}
		toolsRiskMax := ""
		toolsAllowGroups := []string{}
		toolsAllowPatterns := []string{}
		policyRequested := meta.ToolsAllowExists ||
			meta.ToolsAllowGroupsExists ||
			meta.ToolsAllowPatternsExists ||
			meta.ToolsDenyExists ||
			strings.TrimSpace(meta.ToolsRiskMax) != ""
		if policyRequested {
			policyMode = "allowlist"
			union := make([]string, 0)
			unionSeen := map[string]struct{}{}
			appendUnion := func(items []string) {
				for _, item := range items {
					if _, exists := unionSeen[item]; exists {
						continue
					}
					unionSeen[item] = struct{}{}
					union = append(union, item)
				}
			}
			hasAllowSource := meta.ToolsAllowExists || meta.ToolsAllowGroupsExists || meta.ToolsAllowPatternsExists
			if !hasAllowSource {
				appendUnion(listGatewayKnownToolNames(knownTools))
			}

			normalizedNames, unknownTools := normalizeGatewayToolsAllow(meta.ToolsAllow, knownTools)
			if len(unknownTools) > 0 {
				diagnostics = append(
					diagnostics,
					fmt.Sprintf("agent %s tools_allow ignored unknown tools: %s", name, strings.Join(unknownTools, ", ")),
				)
			}
			appendUnion(normalizedNames)

			normalizedGroups, groupTools, unknownGroups := normalizeGatewayToolsAllowGroups(meta.ToolsAllowGroups, knownGroups)
			if len(unknownGroups) > 0 {
				diagnostics = append(
					diagnostics,
					fmt.Sprintf("agent %s tools_allow_groups ignored unknown groups: %s", name, strings.Join(unknownGroups, ", ")),
				)
			}
			toolsAllowGroups = normalizedGroups
			appendUnion(groupTools)

			normalizedPatterns, patternTools, invalidPatterns := normalizeGatewayToolsAllowPatterns(meta.ToolsAllowPatterns, knownTools)
			if len(invalidPatterns) > 0 {
				diagnostics = append(
					diagnostics,
					fmt.Sprintf("agent %s tools_allow_patterns ignored invalid patterns: %s", name, strings.Join(invalidPatterns, ", ")),
				)
			}
			toolsAllowPatterns = normalizedPatterns
			appendUnion(patternTools)

			normalizedDeny, unknownDeny := normalizeGatewayToolsAllow(meta.ToolsDeny, knownTools)
			if len(unknownDeny) > 0 {
				diagnostics = append(
					diagnostics,
					fmt.Sprintf("agent %s tools_deny ignored unknown tools: %s", name, strings.Join(unknownDeny, ", ")),
				)
			}
			toolsDeny = normalizedDeny
			if len(toolsDeny) > 0 {
				union = removeDeniedGatewayTools(union, toolsDeny)
			}

			normalizedRiskMax, riskOK := normalizeGatewayToolRiskMax(meta.ToolsRiskMax)
			if strings.TrimSpace(meta.ToolsRiskMax) != "" && !riskOK {
				diagnostics = append(
					diagnostics,
					fmt.Sprintf("agent %s tools_risk_max ignored invalid value: %q", name, strings.TrimSpace(meta.ToolsRiskMax)),
				)
			}
			toolsRiskMax = normalizedRiskMax
			if toolsRiskMax != "" {
				union = filterGatewayToolsByRisk(union, toolsRiskMax)
			}

			if len(union) == 0 {
				diagnostics = append(diagnostics, fmt.Sprintf("skip agent %s: tools_allow has no valid tools", name))
				return nil
			}
			sort.Strings(union)
			toolsAllow = union
		}
		sessionRoutingMode := normalizeGatewaySessionRoutingMode(meta.SessionRoutingMode)
		sessionFixedID := strings.TrimSpace(meta.SessionFixedID)
		if sessionRoutingMode == "fixed" && sessionFixedID == "" {
			diagnostics = append(diagnostics, fmt.Sprintf("skip agent %s: session_routing_mode fixed requires session_fixed_id", name))
			return nil
		}
		loaded = append(loaded, workspaceGatewayAgent{
			Name:               name,
			Description:        description,
			Prompt:             prompt,
			FilePath:           path,
			PolicyMode:         policyMode,
			ToolsAllow:         toolsAllow,
			ToolsDeny:          toolsDeny,
			ToolsRiskMax:       toolsRiskMax,
			ToolsAllowGroups:   toolsAllowGroups,
			ToolsAllowPatterns: toolsAllowPatterns,
			SessionRoutingMode: sessionRoutingMode,
			SessionFixedID:     sessionFixedID,
		})
		return nil
	})

	sort.Slice(loaded, func(i, j int) bool {
		left := strings.ToLower(loaded[i].FilePath)
		right := strings.ToLower(loaded[j].FilePath)
		return left < right
	})

	seen := map[string]struct{}{}
	out := make([]workspaceGatewayAgent, 0, len(loaded))
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

func isValidGatewayAgentName(name string) bool {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
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

func inferGatewayAgentDescription(prompt string) string {
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

func newWorkspacePromptExecutor(
	def workspaceGatewayAgent,
	runPrompt func(ctx context.Context, runLabel string, prompt string, allowedTools []string) (string, error),
) (gateway.AgentExecutor, error) {
	if runPrompt == nil {
		return nil, fmt.Errorf("run prompt is required")
	}
	name := strings.TrimSpace(def.Name)
	description := strings.TrimSpace(def.Description)
	if description == "" {
		description = "Workspace markdown sub-agent"
	}
	instructions := strings.TrimSpace(def.Prompt)
	return gateway.NewPromptExecutorWithOptions(gateway.PromptExecutorOptions{
		Name:               name,
		Description:        description,
		Source:             "workspace",
		Entry:              strings.TrimSpace(def.FilePath),
		PolicyMode:         normalizeGatewayPolicyMode(def.PolicyMode),
		ToolsAllow:         append([]string(nil), def.ToolsAllow...),
		ToolsDeny:          append([]string(nil), def.ToolsDeny...),
		ToolsRiskMax:       strings.TrimSpace(def.ToolsRiskMax),
		ToolsAllowGroups:   append([]string(nil), def.ToolsAllowGroups...),
		ToolsAllowPatterns: append([]string(nil), def.ToolsAllowPatterns...),
		SessionRoutingMode: normalizeGatewaySessionRoutingMode(def.SessionRoutingMode),
		SessionFixedID:     strings.TrimSpace(def.SessionFixedID),
		RunPrompt: func(ctx context.Context, runLabel string, prompt string, allowedTools []string) (string, error) {
			label := strings.TrimSpace(runLabel)
			if label == "" {
				label = "spawn"
			}
			label += ":" + name
			return runPrompt(ctx, label, composeWorkspaceAgentPrompt(name, instructions, prompt), allowedTools)
		},
	})
}

func composeWorkspaceAgentPrompt(name, instructions, userPrompt string) string {
	task := strings.TrimSpace(userPrompt)
	profile := strings.TrimSpace(instructions)
	if profile == "" {
		return task
	}
	var b strings.Builder
	b.WriteString("Sub-agent profile: ")
	b.WriteString(strings.TrimSpace(name))
	b.WriteString("\n\n")
	b.WriteString(profile)
	if task != "" {
		b.WriteString("\n\nUser task:\n")
		b.WriteString(task)
	}
	return b.String()
}

func normalizeGatewayPolicyMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		return "full"
	}
	if mode == "allowlist" {
		return mode
	}
	return "full"
}

func normalizeGatewaySessionRoutingMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case "", "caller":
		return "caller"
	case "new", "fixed":
		return mode
	default:
		return "caller"
	}
}

func knownGatewayPromptTools(workspaceDir string) map[string]struct{} {
	out := map[string]struct{}{}
	registry := newBaseToolRegistry(workspaceDir)
	for _, schema := range registry.Schemas() {
		name := tool.CanonicalToolName(schema.Function.Name)
		if name == "" {
			continue
		}
		out[name] = struct{}{}
	}
	return out
}

func knownGatewayPromptToolGroups(known map[string]struct{}) map[string][]string {
	groups := map[string][]string{
		"memory": {},
		"files":  {},
		"shell":  {},
		"web":    {},
	}
	for name := range known {
		switch {
		case strings.HasPrefix(name, "memory_"):
			groups["memory"] = append(groups["memory"], name)
		case name == "exec" || name == "process":
			groups["shell"] = append(groups["shell"], name)
		case name == "web_search" || name == "web_fetch":
			groups["web"] = append(groups["web"], name)
		case strings.HasPrefix(name, "read") ||
			strings.HasPrefix(name, "write") ||
			strings.HasPrefix(name, "edit") ||
			name == "list_dir" ||
			name == "glob" ||
			name == "apply_patch":
			groups["files"] = append(groups["files"], name)
		}
	}
	for key := range groups {
		sort.Strings(groups[key])
	}
	return groups
}

func normalizeGatewayToolsAllow(raw []string, known map[string]struct{}) ([]string, []string) {
	normalized := make([]string, 0, len(raw))
	unknown := make([]string, 0)
	seen := map[string]struct{}{}
	for _, item := range raw {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		canonical := tool.CanonicalToolName(trimmed)
		if canonical == "" {
			continue
		}
		if _, ok := known[canonical]; !ok {
			unknown = append(unknown, trimmed)
			continue
		}
		if _, exists := seen[canonical]; exists {
			continue
		}
		seen[canonical] = struct{}{}
		normalized = append(normalized, canonical)
	}
	sort.Strings(normalized)
	sort.Strings(unknown)
	return normalized, unknown
}

func normalizeGatewayToolsAllowGroups(raw []string, groups map[string][]string) ([]string, []string, []string) {
	normalizedGroups := make([]string, 0, len(raw))
	outTools := make([]string, 0)
	unknownGroups := make([]string, 0)
	groupSeen := map[string]struct{}{}
	toolSeen := map[string]struct{}{}
	for _, item := range raw {
		group := strings.ToLower(strings.TrimSpace(item))
		if group == "" {
			continue
		}
		tools, ok := groups[group]
		if !ok {
			unknownGroups = append(unknownGroups, group)
			continue
		}
		if _, exists := groupSeen[group]; !exists {
			groupSeen[group] = struct{}{}
			normalizedGroups = append(normalizedGroups, group)
		}
		for _, toolName := range tools {
			if _, exists := toolSeen[toolName]; exists {
				continue
			}
			toolSeen[toolName] = struct{}{}
			outTools = append(outTools, toolName)
		}
	}
	sort.Strings(normalizedGroups)
	sort.Strings(outTools)
	sort.Strings(unknownGroups)
	return normalizedGroups, outTools, unknownGroups
}

func normalizeGatewayToolsAllowPatterns(raw []string, known map[string]struct{}) ([]string, []string, []string) {
	knownNames := make([]string, 0, len(known))
	for name := range known {
		knownNames = append(knownNames, name)
	}
	sort.Strings(knownNames)

	normalizedPatterns := make([]string, 0, len(raw))
	outTools := make([]string, 0)
	invalidPatterns := make([]string, 0)
	patternSeen := map[string]struct{}{}
	toolSeen := map[string]struct{}{}
	for _, item := range raw {
		pattern := strings.TrimSpace(item)
		if pattern == "" {
			continue
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			invalidPatterns = append(invalidPatterns, pattern)
			continue
		}
		if _, exists := patternSeen[pattern]; !exists {
			patternSeen[pattern] = struct{}{}
			normalizedPatterns = append(normalizedPatterns, pattern)
		}
		for _, toolName := range knownNames {
			if !re.MatchString(toolName) {
				continue
			}
			if _, exists := toolSeen[toolName]; exists {
				continue
			}
			toolSeen[toolName] = struct{}{}
			outTools = append(outTools, toolName)
		}
	}
	sort.Strings(normalizedPatterns)
	sort.Strings(outTools)
	sort.Strings(invalidPatterns)
	return normalizedPatterns, outTools, invalidPatterns
}

func listGatewayKnownToolNames(known map[string]struct{}) []string {
	out := make([]string, 0, len(known))
	for name := range known {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func removeDeniedGatewayTools(allowed []string, denied []string) []string {
	if len(allowed) == 0 || len(denied) == 0 {
		return append([]string(nil), allowed...)
	}
	deniedSet := map[string]struct{}{}
	for _, item := range denied {
		name := strings.TrimSpace(item)
		if name == "" {
			continue
		}
		deniedSet[name] = struct{}{}
	}
	if len(deniedSet) == 0 {
		return append([]string(nil), allowed...)
	}
	out := make([]string, 0, len(allowed))
	for _, item := range allowed {
		if _, blocked := deniedSet[item]; blocked {
			continue
		}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func normalizeGatewayToolRiskMax(raw string) (string, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return "", true
	}
	switch value {
	case "low", "medium", "high":
		return value, true
	default:
		return "", false
	}
}

func filterGatewayToolsByRisk(allowed []string, riskMax string) []string {
	maxRank := gatewayToolRiskRank(riskMax)
	if maxRank == 0 || len(allowed) == 0 {
		return append([]string(nil), allowed...)
	}
	out := make([]string, 0, len(allowed))
	for _, item := range allowed {
		if gatewayToolRiskRank(gatewayToolRiskLevel(item)) <= maxRank {
			out = append(out, item)
		}
	}
	sort.Strings(out)
	return out
}

func gatewayToolRiskLevel(toolName string) string {
	switch strings.TrimSpace(toolName) {
	case "read", "read_file", "list_dir", "memory_search", "memory_get", "session_status":
		return "low"
	case "glob", "web_search", "web_fetch":
		return "medium"
	case "write", "write_file", "edit", "edit_file", "exec", "process", "apply_patch":
		return "high"
	default:
		return "high"
	}
}

func gatewayToolRiskRank(level string) int {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "low":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	default:
		return 0
	}
}

func parseWorkspaceGatewayAgentDocument(raw string) (workspaceGatewayAgentFrontmatter, string, error) {
	metaBlock, body, hasFrontmatter, err := splitYAMLFrontmatter(raw)
	if err != nil {
		return workspaceGatewayAgentFrontmatter{}, "", err
	}
	if !hasFrontmatter {
		return workspaceGatewayAgentFrontmatter{}, body, nil
	}
	meta, err := parseWorkspaceGatewayAgentFrontmatter(metaBlock)
	if err != nil {
		return workspaceGatewayAgentFrontmatter{}, "", err
	}
	return meta, body, nil
}

func splitYAMLFrontmatter(raw string) (meta string, body string, hasFrontmatter bool, err error) {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return "", raw, false, nil
	}
	rest := normalized[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		if strings.HasSuffix(rest, "\n---") {
			end = len(rest) - len("\n---")
			return rest[:end], "", true, nil
		}
		return "", "", false, fmt.Errorf("unterminated frontmatter block")
	}
	return rest[:end], rest[end+len("\n---\n"):], true, nil
}

func parseWorkspaceGatewayAgentFrontmatter(raw string) (workspaceGatewayAgentFrontmatter, error) {
	meta := workspaceGatewayAgentFrontmatter{}
	lines := strings.Split(raw, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "- ") {
			return workspaceGatewayAgentFrontmatter{}, fmt.Errorf("unexpected list item: %q", line)
		}
		keyRaw, valueRaw, ok := strings.Cut(line, ":")
		if !ok {
			return workspaceGatewayAgentFrontmatter{}, fmt.Errorf("invalid frontmatter line: %q", lines[i])
		}
		key := strings.ToLower(strings.TrimSpace(keyRaw))
		value := strings.TrimSpace(valueRaw)

		switch key {
		case "name":
			meta.Name = trimYAMLScalar(value)
		case "description":
			meta.Description = trimYAMLScalar(value)
		case "tools_allow", "tools-allow":
			meta.ToolsAllowExists = true
			items, next := parseYAMLListValue(lines, i, value)
			meta.ToolsAllow = append(meta.ToolsAllow, items...)
			i = next
		case "tools_deny", "tools-deny":
			meta.ToolsDenyExists = true
			items, next := parseYAMLListValue(lines, i, value)
			meta.ToolsDeny = append(meta.ToolsDeny, items...)
			i = next
		case "tools_risk_max", "tools-risk-max":
			meta.ToolsRiskMax = trimYAMLScalar(value)
		case "tools_allow_groups", "tools-allow-groups":
			meta.ToolsAllowGroupsExists = true
			items, next := parseYAMLListValue(lines, i, value)
			meta.ToolsAllowGroups = append(meta.ToolsAllowGroups, items...)
			i = next
		case "tools_allow_patterns", "tools-allow-patterns":
			meta.ToolsAllowPatternsExists = true
			items, next := parseYAMLListValue(lines, i, value)
			meta.ToolsAllowPatterns = append(meta.ToolsAllowPatterns, items...)
			i = next
		case "session_routing_mode", "session-routing-mode":
			meta.SessionRoutingMode = trimYAMLScalar(value)
		case "session_fixed_id", "session-fixed-id":
			meta.SessionFixedID = trimYAMLScalar(value)
		}
	}
	return meta, nil
}

func parseYAMLListValue(lines []string, index int, rawValue string) ([]string, int) {
	value := strings.TrimSpace(rawValue)
	if value == "" {
		out := make([]string, 0)
		i := index
		for i+1 < len(lines) {
			next := strings.TrimSpace(lines[i+1])
			if next == "" || strings.HasPrefix(next, "#") {
				i++
				continue
			}
			if !strings.HasPrefix(next, "-") {
				break
			}
			item := strings.TrimSpace(strings.TrimPrefix(next, "-"))
			item = trimYAMLScalar(item)
			if item != "" {
				out = append(out, item)
			}
			i++
		}
		return out, i
	}
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		return parseInlineYAMLList(value), index
	}
	item := trimYAMLScalar(value)
	if item == "" {
		return []string{}, index
	}
	return []string{item}, index
}

func parseInlineYAMLList(raw string) []string {
	inner := strings.TrimSpace(raw)
	inner = strings.TrimPrefix(inner, "[")
	inner = strings.TrimSuffix(inner, "]")
	if strings.TrimSpace(inner) == "" {
		return []string{}
	}
	parts := strings.Split(inner, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		item := trimYAMLScalar(part)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func trimYAMLScalar(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) >= 2 {
		if (trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"') ||
			(trimmed[0] == '\'' && trimmed[len(trimmed)-1] == '\'') {
			trimmed = trimmed[1 : len(trimmed)-1]
		}
	}
	return strings.TrimSpace(trimmed)
}
