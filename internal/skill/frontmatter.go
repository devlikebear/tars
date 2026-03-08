package skill

import (
	"fmt"
	"strconv"
	"strings"
)

type Frontmatter struct {
	Name                    string
	Description             string
	UserInvocable           *bool
	RecommendedTools        []string
	RecommendedProjectFiles []string
	WakePhases              []string
}

func ParseFrontmatter(raw string) (Frontmatter, string, error) {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return Frontmatter{}, raw, nil
	}

	rest := normalized[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		if strings.HasSuffix(rest, "\n---") {
			end = len(rest) - len("\n---")
			rest = rest[:end]
			meta, err := parseFrontmatterBlock(rest)
			if err != nil {
				return Frontmatter{}, "", err
			}
			return meta, "", nil
		}
		return Frontmatter{}, "", fmt.Errorf("unterminated frontmatter block")
	}

	metaBlock := rest[:end]
	body := rest[end+len("\n---\n"):]
	meta, err := parseFrontmatterBlock(metaBlock)
	if err != nil {
		return Frontmatter{}, "", err
	}
	return meta, body, nil
}

func parseFrontmatterBlock(raw string) (Frontmatter, error) {
	meta := Frontmatter{}
	lines := strings.Split(raw, "\n")
	currentListKey := ""
	currentList := []string(nil)
	flushList := func() {
		if currentListKey == "" {
			return
		}
		switch currentListKey {
		case "recommended_tools":
			meta.RecommendedTools = normalizeFrontmatterList(currentList)
		case "recommended_project_files":
			meta.RecommendedProjectFiles = normalizeFrontmatterList(currentList)
		case "wake_phases":
			meta.WakePhases = normalizeFrontmatterList(currentList)
		}
		currentListKey = ""
		currentList = nil
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if currentListKey != "" {
			if strings.HasPrefix(trimmed, "- ") {
				currentList = append(currentList, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
				continue
			}
			flushList()
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			return Frontmatter{}, fmt.Errorf("invalid frontmatter line: %q", line)
		}
		k := strings.ToLower(strings.TrimSpace(key))
		v := strings.TrimSpace(value)
		if v == "" && isFrontmatterListKey(k) {
			currentListKey = k
			currentList = nil
			continue
		}
		v = strings.Trim(v, `"'`)
		switch k {
		case "name":
			meta.Name = v
		case "description":
			meta.Description = v
		case "user-invocable", "user_invocable":
			parsed, err := strconv.ParseBool(v)
			if err != nil {
				return Frontmatter{}, fmt.Errorf("invalid user-invocable value %q", v)
			}
			meta.UserInvocable = &parsed
		case "recommended_tools":
			meta.RecommendedTools = parseFrontmatterListValue(v)
		case "recommended_project_files":
			meta.RecommendedProjectFiles = parseFrontmatterListValue(v)
		case "wake_phases":
			meta.WakePhases = parseFrontmatterListValue(v)
		}
	}
	flushList()
	return meta, nil
}

func isFrontmatterListKey(key string) bool {
	switch key {
	case "recommended_tools", "recommended_project_files", "wake_phases":
		return true
	default:
		return false
	}
}

func parseFrontmatterListValue(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, "[")
	trimmed = strings.TrimSuffix(trimmed, "]")
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, ",")
	return normalizeFrontmatterList(parts)
}

func normalizeFrontmatterList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.Trim(strings.TrimSpace(value), `"'`)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
