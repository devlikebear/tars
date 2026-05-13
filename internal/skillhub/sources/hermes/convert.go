package hermes

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Frontmatter captures the hermes frontmatter fields we care about.
// hermes uses standard YAML (no JSON-in-YAML embedding) so direct
// struct unmarshal works.
type Frontmatter struct {
	Name           string
	Description    string
	Version        string
	Author         string
	License        string
	Tags           []string
	RelatedSkills  []string
	RawMetadata    map[string]any
}

// rawFrontmatter mirrors the on-disk YAML shape.
type rawFrontmatter struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Version     string         `yaml:"version"`
	Author      string         `yaml:"author"`
	License     string         `yaml:"license"`
	Metadata    map[string]any `yaml:"metadata"`
}

// ParseFrontmatter extracts the SKILL.md frontmatter block and returns the
// hermes-specific view used by the rewriter.
func ParseFrontmatter(raw []byte) (Frontmatter, error) {
	block, _, err := splitFrontmatter(raw)
	if err != nil {
		return Frontmatter{}, err
	}
	var rf rawFrontmatter
	if err := yaml.Unmarshal([]byte(block), &rf); err != nil {
		return Frontmatter{}, fmt.Errorf("yaml unmarshal: %w", err)
	}
	fm := Frontmatter{
		Name:        strings.TrimSpace(rf.Name),
		Description: strings.TrimSpace(rf.Description),
		Version:     strings.TrimSpace(rf.Version),
		Author:      strings.TrimSpace(rf.Author),
		License:     strings.TrimSpace(rf.License),
		RawMetadata: rf.Metadata,
	}
	if hm := hermesMeta(rf.Metadata); hm != nil {
		fm.Tags = stringSlice(hm["tags"])
		fm.RelatedSkills = stringSlice(hm["related_skills"])
	}
	return fm, nil
}

// SplitBody returns the markdown body after the closing "---" so the
// rewriter can re-emit it verbatim.
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

func hermesMeta(meta map[string]any) map[string]any {
	if meta == nil {
		return nil
	}
	v, ok := meta["hermes"]
	if !ok {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	if mi, ok := v.(map[any]any); ok {
		out := make(map[string]any, len(mi))
		for k, val := range mi {
			if ks, ok := k.(string); ok && ks != "" {
				out[ks] = val
			}
		}
		return out
	}
	return nil
}

func stringSlice(v any) []string {
	if v == nil {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}
