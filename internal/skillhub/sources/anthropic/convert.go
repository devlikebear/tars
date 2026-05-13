package anthropic

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Frontmatter captures the (minimal) Anthropic SKILL.md frontmatter. The
// `license` field is a hint string ("Complete terms in LICENSE.txt") —
// the actual license body is fetched separately from the per-skill
// LICENSE.txt file.
type Frontmatter struct {
	Name        string
	Description string
	LicenseHint string
	RawMetadata map[string]any
}

type rawFrontmatter struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	License     string         `yaml:"license"`
	Metadata    map[string]any `yaml:"metadata"`
}

// ParseFrontmatter extracts the SKILL.md frontmatter block.
func ParseFrontmatter(raw []byte) (Frontmatter, error) {
	block, _, err := splitFrontmatter(raw)
	if err != nil {
		return Frontmatter{}, err
	}
	var rf rawFrontmatter
	if err := yaml.Unmarshal([]byte(block), &rf); err != nil {
		return Frontmatter{}, fmt.Errorf("yaml unmarshal: %w", err)
	}
	return Frontmatter{
		Name:        strings.TrimSpace(rf.Name),
		Description: strings.TrimSpace(rf.Description),
		LicenseHint: strings.TrimSpace(rf.License),
		RawMetadata: rf.Metadata,
	}, nil
}

// SplitBody returns the markdown body after the closing "---".
func SplitBody(raw []byte) (string, error) {
	_, body, err := splitFrontmatter(raw)
	return body, err
}

func splitFrontmatter(raw []byte) (block string, body string, err error) {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return "", "", fmt.Errorf("missing opening frontmatter '---' delimiter")
	}
	rest := text[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		if strings.HasSuffix(rest, "\n---") {
			return rest[:len(rest)-len("\n---")], "", nil
		}
		return "", "", fmt.Errorf("missing closing frontmatter '---' delimiter")
	}
	return rest[:end], rest[end+len("\n---\n"):], nil
}
