package tool

import (
	"regexp"
	"sort"
	"strings"
)

var canonicalToolGroups = []string{"memory", "files", "shell", "web"}

var toolGroupAliases = map[string]string{
	"memory":   "memory",
	"file":     "files",
	"files":    "files",
	"exec":     "shell",
	"shell":    "shell",
	"terminal": "shell",
	"web":      "web",
}

func KnownToolGroupNames() []string {
	out := make([]string, len(canonicalToolGroups))
	copy(out, canonicalToolGroups)
	return out
}

func NormalizeToolGroupName(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "" {
		return ""
	}
	if canonical, ok := toolGroupAliases[normalized]; ok {
		return canonical
	}
	return ""
}

func ToolGroupForName(name string) string {
	canonical := CanonicalToolName(name)
	if canonical == "" {
		return ""
	}
	switch {
	case canonical == "memory" || canonical == "knowledge" || strings.HasPrefix(canonical, "memory_"):
		return "memory"
	case canonical == "exec" || canonical == "process":
		return "shell"
	case canonical == "web_search" || canonical == "web_fetch":
		return "web"
	case strings.HasPrefix(canonical, "read") ||
		strings.HasPrefix(canonical, "write") ||
		strings.HasPrefix(canonical, "edit") ||
		canonical == "list_dir" ||
		canonical == "glob" ||
		canonical == "apply_patch":
		return "files"
	default:
		return ""
	}
}

// KnownToolGroups categorizes known tool names into predefined groups.
func KnownToolGroups(known map[string]struct{}) map[string][]string {
	groups := map[string][]string{}
	for _, group := range canonicalToolGroups {
		groups[group] = []string{}
	}
	for name := range known {
		if group := ToolGroupForName(name); group != "" {
			groups[group] = append(groups[group], name)
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
	seenGroups := map[string]struct{}{}
	seenTools := map[string]struct{}{}
	seenUnknown := map[string]struct{}{}
	for _, g := range groupNames {
		normalized := NormalizeToolGroupName(g)
		if normalized == "" {
			unknown := strings.ToLower(strings.TrimSpace(g))
			if unknown == "" {
				continue
			}
			if _, exists := seenUnknown[unknown]; exists {
				continue
			}
			seenUnknown[unknown] = struct{}{}
			unknownGroups = append(unknownGroups, unknown)
			continue
		}
		tools, ok := groups[normalized]
		if !ok {
			if _, exists := seenUnknown[normalized]; exists {
				continue
			}
			seenUnknown[normalized] = struct{}{}
			unknownGroups = append(unknownGroups, normalized)
			continue
		}
		if _, exists := seenGroups[normalized]; !exists {
			seenGroups[normalized] = struct{}{}
			validGroups = append(validGroups, normalized)
		}
		for _, toolName := range tools {
			if _, exists := seenTools[toolName]; exists {
				continue
			}
			seenTools[toolName] = struct{}{}
			expandedTools = append(expandedTools, toolName)
		}
	}
	sort.Strings(validGroups)
	sort.Strings(expandedTools)
	sort.Strings(unknownGroups)
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
