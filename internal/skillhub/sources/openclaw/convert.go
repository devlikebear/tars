package openclaw

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Frontmatter captures the openclaw frontmatter we care about during
// conversion. It deliberately accepts the JSON-in-YAML metadata block used
// by openclaw (gopkg.in/yaml.v3 parses both forms transparently).
type Frontmatter struct {
	Name        string
	Description string

	// Metadata is the entire metadata.openclaw map, retained for the
	// rewriter so it can preserve the origin under metadata.adapter_origin.
	Metadata map[string]any

	// RequiresBins is metadata.openclaw.requires.bins, pulled up for
	// convenience (becomes the TARS requires_bins field).
	RequiresBins []string

	// OS is metadata.openclaw.os, also pulled up.
	OS []string

	// InstallBlocks is metadata.openclaw.install, preserved verbatim.
	// Never executed. Surfaced as human-readable adapter_warnings.
	InstallBlocks []map[string]any
}

// rawFrontmatter is the structural shape we extract from a SKILL.md.
type rawFrontmatter struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Metadata    map[string]any `yaml:"metadata"`
}

// ParseFrontmatter pulls the YAML frontmatter block out of an openclaw
// SKILL.md and returns the structured view used by the rewriter. The body
// after the closing "---" is discarded — the rewriter re-emits a fresh body
// pass-through later.
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
		Metadata:    rf.Metadata,
	}
	if oc := openclawMeta(rf.Metadata); oc != nil {
		fm.RequiresBins = stringSlice(digMap(oc, "requires", "bins"))
		fm.OS = stringSlice(oc["os"])
		fm.InstallBlocks = mapSlice(oc["install"])
	}
	return fm, nil
}

// SplitBody returns the SKILL.md body (everything after the closing "---")
// so the rewriter can re-emit it untouched.
func SplitBody(raw []byte) (string, error) {
	_, body, err := splitFrontmatter(raw)
	return body, err
}

// splitFrontmatter separates the YAML frontmatter block from the markdown
// body. Returns an error if the document is missing a frontmatter pair.
func splitFrontmatter(raw []byte) (block string, body string, err error) {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return "", "", fmt.Errorf("missing opening frontmatter '---' delimiter")
	}
	rest := text[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		// Allow a trailing "---" with no newline (end of file).
		if strings.HasSuffix(rest, "\n---") {
			return rest[:len(rest)-len("\n---")], "", nil
		}
		return "", "", fmt.Errorf("missing closing frontmatter '---' delimiter")
	}
	return rest[:end], rest[end+len("\n---\n"):], nil
}

// openclawMeta returns metadata.openclaw as a map, or nil.
func openclawMeta(meta map[string]any) map[string]any {
	if meta == nil {
		return nil
	}
	v, ok := meta["openclaw"]
	if !ok {
		return nil
	}
	m, _ := v.(map[string]any)
	if m != nil {
		return m
	}
	// gopkg.in/yaml.v3 parses some shapes as map[interface{}]interface{};
	// normalize before returning.
	if mi, ok := v.(map[any]any); ok {
		out := make(map[string]any, len(mi))
		for k, val := range mi {
			ks, _ := k.(string)
			if ks != "" {
				out[ks] = val
			}
		}
		return out
	}
	return nil
}

func digMap(m map[string]any, keys ...string) any {
	var cur any = m
	for _, k := range keys {
		nested, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = nested[k]
	}
	return cur
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

func mapSlice(v any) []map[string]any {
	if v == nil {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if ok {
			out = append(out, m)
		}
	}
	return out
}

// SummarizeInstallBlocks renders openclaw `install[]` entries as one-line
// human-readable warnings. These are the messages surfaced to the user in
// the dry-run output (Phase 3) and stored in metadata.adapter_warnings.
func SummarizeInstallBlocks(blocks []map[string]any) []string {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]string, 0, len(blocks))
	for _, b := range blocks {
		kind, _ := b["kind"].(string)
		label, _ := b["label"].(string)
		switch kind {
		case "brew":
			formula, _ := b["formula"].(string)
			out = append(out, fmt.Sprintf("install block skipped: brew install %s (label: %s)", formula, label))
		case "apt":
			pkg, _ := b["package"].(string)
			out = append(out, fmt.Sprintf("install block skipped: apt install %s (label: %s)", pkg, label))
		case "node":
			pkg, _ := b["package"].(string)
			out = append(out, fmt.Sprintf("install block skipped: npm install -g %s (label: %s)", pkg, label))
		default:
			id, _ := b["id"].(string)
			out = append(out, fmt.Sprintf("install block skipped: kind=%s id=%s label=%s", kind, id, label))
		}
	}
	return out
}
