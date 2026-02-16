package skill

import (
	"fmt"
	"strconv"
	"strings"
)

type Frontmatter struct {
	Name          string
	Description   string
	UserInvocable *bool
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
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			return Frontmatter{}, fmt.Errorf("invalid frontmatter line: %q", line)
		}
		k := strings.ToLower(strings.TrimSpace(key))
		v := strings.TrimSpace(value)
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
		}
	}
	return meta, nil
}
