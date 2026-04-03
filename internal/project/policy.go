package project

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type ToolPolicySpec struct {
	ToolsAllow               []string
	ToolsAllowExists         bool
	ToolsAllowGroups         []string
	ToolsAllowGroupsExists   bool
	ToolsAllowPatterns       []string
	ToolsAllowPatternsExists bool
	ToolsDeny                []string
	ToolsDenyExists          bool
	ToolsRiskMax             string
	ToolsRiskMaxExists       bool
}

type ToolPolicyOptions struct {
	ExpandAllKnownWhenPolicyWithoutAllowSource bool
}

type NormalizedToolPolicy struct {
	ToolsAllow         []string
	ToolsAllowGroups   []string
	ToolsAllowPatterns []string
	ToolsDeny          []string
	ToolsRiskMax       string
	AllowedTools       []string
	UnknownTools       []string
	UnknownGroups      []string
	InvalidPatterns    []string
	UnknownDeny        []string
	InvalidRiskMax     bool
	HasAllowSource     bool
	HasPolicy          bool
}

type PromptContextOptions struct {
	Header            string
	FieldPrefix       string
	ArtifactsDir      string
	IncludeObjective  bool
	IncludeToolsAllow bool
	IncludeBody       bool
	BodyHeader        string
}

func (s ToolPolicySpec) HasAllowSource() bool {
	return s.ToolsAllowExists || s.ToolsAllowGroupsExists || s.ToolsAllowPatternsExists
}

func (s ToolPolicySpec) HasPolicy() bool {
	return s.HasAllowSource() || s.ToolsDenyExists || s.ToolsRiskMaxExists
}

func KnownToolGroups(known map[string]struct{}) map[string][]string {
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

func NormalizeToolPolicy(spec ToolPolicySpec, known map[string]struct{}, opts ToolPolicyOptions) NormalizedToolPolicy {
	policy := NormalizedToolPolicy{
		HasAllowSource: spec.HasAllowSource(),
		HasPolicy:      spec.HasPolicy(),
	}
	policy.ToolsAllow, policy.UnknownTools = normalizeKnownTools(spec.ToolsAllow, known)
	groups := KnownToolGroups(known)
	allowedFromGroups := []string(nil)
	allowedFromPatterns := []string(nil)
	policy.ToolsAllowGroups, allowedFromGroups, policy.UnknownGroups = normalizeToolGroups(spec.ToolsAllowGroups, groups)
	policy.ToolsAllowPatterns, allowedFromPatterns, policy.InvalidPatterns = normalizeToolPatterns(spec.ToolsAllowPatterns, known)
	policy.ToolsDeny, policy.UnknownDeny = normalizeKnownTools(spec.ToolsDeny, known)
	policy.ToolsRiskMax, policy.InvalidRiskMax = normalizeToolRiskMax(spec.ToolsRiskMax, spec.ToolsRiskMaxExists)

	union := make([]string, 0)
	seen := map[string]struct{}{}
	appendUnion := func(items []string) {
		for _, item := range items {
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			union = append(union, item)
		}
	}
	if opts.ExpandAllKnownWhenPolicyWithoutAllowSource && policy.HasPolicy && !policy.HasAllowSource {
		appendUnion(listKnownToolNames(known))
	}
	appendUnion(policy.ToolsAllow)
	appendUnion(allowedFromGroups)
	appendUnion(allowedFromPatterns)
	union = applyToolConstraints(union, policy.ToolsDeny, policy.ToolsRiskMax)
	policy.AllowedTools = union
	return policy
}

var toolNameAliases = map[string]string{
	"shell_execute":   "exec",
	"shell_exec":      "exec",
	"run_command":     "exec",
	"terminal_exec":   "exec",
	"execute_shell":   "exec",
	"session_list":    "sessions_list",
	"session_history": "sessions_history",
	"session_send":    "sessions_send",
	"session_spawn":   "sessions_spawn",
	"session_runs":    "sessions_runs",
	"subagent_run":    "subagents_run",
	"agent_runs":      "sessions_runs",
	"gateway_status":  "gateway",
}

func canonicalToolName(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "" {
		return ""
	}
	if canonical, ok := toolNameAliases[normalized]; ok {
		return canonical
	}
	return normalized
}

func ApplyToolPolicy(base []string, policy NormalizedToolPolicy) []string {
	allowed := sanitizeToolNames(base)
	if policy.HasAllowSource {
		if len(policy.AllowedTools) == 0 {
			return nil
		}
		if len(allowed) == 0 {
			return append([]string(nil), policy.AllowedTools...)
		}
		allowSet := map[string]struct{}{}
		for _, name := range policy.AllowedTools {
			allowSet[name] = struct{}{}
		}
		filtered := make([]string, 0, len(allowed))
		for _, name := range allowed {
			if _, ok := allowSet[name]; ok {
				filtered = append(filtered, name)
			}
		}
		sort.Strings(filtered)
		return filtered
	}
	return applyToolConstraints(allowed, policy.ToolsDeny, policy.ToolsRiskMax)
}

func ApplyToolConstraints(names []string, policy NormalizedToolPolicy) []string {
	return applyToolConstraints(names, policy.ToolsDeny, policy.ToolsRiskMax)
}

func RenderPromptContext(item Project, opts PromptContextOptions) string {
	var b strings.Builder
	header := strings.TrimSpace(opts.Header)
	if header != "" {
		b.WriteString(header)
		b.WriteString("\n")
	}
	prefix := strings.TrimSpace(opts.FieldPrefix)
	writeField := func(name, value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		_, _ = fmt.Fprintf(&b, "- %s%s: %s\n", prefix, name, trimmed)
	}

	writeField("id", item.ID)
	writeField("name", item.Name)
	writeField("type", item.Type)
	writeField("status", item.Status)
	if opts.IncludeObjective {
		writeField("objective", item.Objective)
	}
	if sp := strings.TrimSpace(item.SourcePath); sp != "" {
		writeField("source_path", sp)
	}
	if strings.TrimSpace(opts.ArtifactsDir) != "" {
		_, _ = fmt.Fprintf(&b, "- artifacts_dir: %s\n", strings.TrimSpace(opts.ArtifactsDir))
	}
	if opts.IncludeToolsAllow && len(item.ToolsAllow) > 0 {
		_, _ = fmt.Fprintf(&b, "- tools_allow: %s\n", strings.Join(item.ToolsAllow, ", "))
	}
	if opts.IncludeBody {
		if body := strings.TrimSpace(item.Body); body != "" {
			b.WriteString("\n")
			if bodyHeader := strings.TrimSpace(opts.BodyHeader); bodyHeader != "" {
				b.WriteString(bodyHeader)
				b.WriteString("\n")
			}
			b.WriteString(body)
			if !strings.HasSuffix(body, "\n") {
				b.WriteString("\n")
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func ProjectPromptContext(item Project) string {
	opts := PromptContextOptions{
		Header:            "## Active Project",
		IncludeObjective:  true,
		IncludeToolsAllow: true,
		IncludeBody:       true,
	}
	if wd := item.WorkingDir(); wd != item.Path {
		opts.ArtifactsDir = wd
	}
	return RenderPromptContext(item, opts)
}

func CronPromptContext(workspaceDir string, item Project) string {
	artifactsDir := item.WorkingDir()
	if artifactsDir == "" {
		artifactsDir = filepath.Join(strings.TrimSpace(workspaceDir), "projects", strings.TrimSpace(item.ID))
	}
	return RenderPromptContext(item, PromptContextOptions{
		Header:       "CRON_PROJECT_CONTEXT:",
		FieldPrefix:  "project_",
		ArtifactsDir: artifactsDir,
		IncludeBody:  true,
		BodyHeader:   "PROJECT_INSTRUCTIONS:",
	})
}

func normalizeKnownTools(raw []string, known map[string]struct{}) ([]string, []string) {
	normalized := make([]string, 0, len(raw))
	unknown := make([]string, 0)
	seen := map[string]struct{}{}
	for _, item := range raw {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		canonical := canonicalToolName(trimmed)
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

func normalizeToolGroups(raw []string, groups map[string][]string) ([]string, []string, []string) {
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

func normalizeToolPatterns(raw []string, known map[string]struct{}) ([]string, []string, []string) {
	knownNames := listKnownToolNames(known)
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

func normalizeToolRiskMax(raw string, exists bool) (string, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return "", false
	}
	switch value {
	case "low", "medium", "high":
		return value, false
	default:
		return "", exists
	}
}

func listKnownToolNames(known map[string]struct{}) []string {
	out := make([]string, 0, len(known))
	for name := range known {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func sanitizeToolNames(raw []string) []string {
	out := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, item := range raw {
		name := canonicalToolName(item)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func applyToolConstraints(allowed []string, denied []string, riskMax string) []string {
	out := append([]string(nil), sanitizeToolNames(allowed)...)
	if len(out) == 0 {
		return out
	}
	if len(denied) > 0 {
		deniedSet := map[string]struct{}{}
		for _, item := range denied {
			name := canonicalToolName(item)
			if name == "" {
				continue
			}
			deniedSet[name] = struct{}{}
		}
		filtered := make([]string, 0, len(out))
		for _, item := range out {
			if _, blocked := deniedSet[item]; blocked {
				continue
			}
			filtered = append(filtered, item)
		}
		out = filtered
	}
	if rank := toolRiskRank(riskMax); rank > 0 {
		filtered := make([]string, 0, len(out))
		for _, item := range out {
			if toolRiskRank(toolRiskLevel(item)) <= rank {
				filtered = append(filtered, item)
			}
		}
		out = filtered
	}
	sort.Strings(out)
	return out
}

func toolRiskLevel(toolName string) string {
	switch strings.TrimSpace(toolName) {
	case "read", "read_file", "list_dir", "memory_search", "memory_get", "memory_save", "session_status":
		return "low"
	case "glob", "web_search", "web_fetch":
		return "medium"
	case "write", "write_file", "edit", "edit_file", "exec", "process", "apply_patch":
		return "high"
	default:
		return "high"
	}
}

func toolRiskRank(level string) int {
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
