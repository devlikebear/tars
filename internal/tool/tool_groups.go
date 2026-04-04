package tool

import (
	"regexp"
	"sort"
	"strings"
)

// KnownToolGroups categorizes known tool names into predefined groups.
func KnownToolGroups(known map[string]struct{}) map[string][]string {
	groups := map[string][]string{
		"memory": {},
		"files":  {},
		"shell":  {},
		"web":    {},
	}
	for name := range known {
		switch {
		case name == "memory" || name == "knowledge" || strings.HasPrefix(name, "memory_"):
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

// ExpandToolGroups expands group names into tool names using known tools.
func ExpandToolGroups(groupNames []string, known map[string]struct{}) (validGroups []string, expandedTools []string, unknownGroups []string) {
	groups := KnownToolGroups(known)
	for _, g := range groupNames {
		normalized := strings.ToLower(strings.TrimSpace(g))
		if normalized == "" {
			continue
		}
		tools, ok := groups[normalized]
		if !ok {
			unknownGroups = append(unknownGroups, normalized)
			continue
		}
		validGroups = append(validGroups, normalized)
		expandedTools = append(expandedTools, tools...)
	}
	return
}

// ExpandToolPatterns matches regex patterns against known tool names.
func ExpandToolPatterns(patterns []string, known map[string]struct{}) (validPatterns []string, matchedTools []string, invalidPatterns []string) {
	for _, p := range patterns {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		re, err := regexp.Compile(trimmed)
		if err != nil {
			invalidPatterns = append(invalidPatterns, trimmed)
			continue
		}
		validPatterns = append(validPatterns, trimmed)
		for name := range known {
			if re.MatchString(name) {
				matchedTools = append(matchedTools, name)
			}
		}
	}
	return
}
