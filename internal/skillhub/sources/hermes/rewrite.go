package hermes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/devlikebear/tars/internal/skillhub"
)

// RewriteInput collects everything RewriteSkillMD needs from the installer.
type RewriteInput struct {
	Raw        []byte
	Entry      *skillhub.RegistryEntry
	OriginURL  string
	CommitSHA  string
	ImportedAt time.Time
}

// RewriteResult is the rewriter's output.
type RewriteResult struct {
	Converted []byte
	Warnings  []string
}

// tarsFrontmatter mirrors the subset of internal/skill.Frontmatter we emit.
// metadata values are JSON-stringified for the same reason openclaw does
// it: TARS' frontmatter parser is line-based and chokes on nested YAML
// list items inside the metadata block.
type tarsFrontmatter struct {
	Name          string            `yaml:"name"`
	Description   string            `yaml:"description"`
	UserInvocable bool              `yaml:"user_invocable,omitempty"`
	Tags          []string          `yaml:"tags,omitempty"`
	Metadata      map[string]string `yaml:"metadata,omitempty"`
}

// RewriteSkillMD parses the hermes SKILL.md and emits a TARS-shaped one.
// Returns no install-block warnings (hermes does not ship those).
func RewriteSkillMD(in RewriteInput) (RewriteResult, error) {
	fm, err := ParseFrontmatter(in.Raw)
	if err != nil {
		return RewriteResult{}, err
	}
	body, err := SplitBody(in.Raw)
	if err != nil {
		return RewriteResult{}, err
	}
	if strings.TrimSpace(fm.Name) == "" {
		return RewriteResult{}, fmt.Errorf("hermes: frontmatter is missing 'name'")
	}
	if strings.TrimSpace(fm.Description) == "" {
		return RewriteResult{}, fmt.Errorf("hermes: frontmatter is missing 'description'")
	}

	when := in.ImportedAt
	if when.IsZero() {
		when = time.Now().UTC()
	}

	originPayload := map[string]any{
		"source":      SourceID,
		"url":         in.OriginURL,
		"commit_sha":  in.CommitSHA,
		"imported_at": when.UTC().Format(time.RFC3339),
		"hermes":      cloneHermesMeta(fm),
	}
	if v := strings.TrimSpace(fm.Version); v != "" {
		originPayload["version"] = v
	}
	if a := strings.TrimSpace(fm.Author); a != "" {
		originPayload["author"] = a
	}
	if l := strings.TrimSpace(fm.License); l != "" {
		originPayload["license"] = l
	}

	originJSON, err := jsonStringify(originPayload)
	if err != nil {
		return RewriteResult{}, fmt.Errorf("hermes: stringify adapter_origin: %w", err)
	}

	tars := tarsFrontmatter{
		Name:          fm.Name,
		Description:   fm.Description,
		UserInvocable: true,
		Tags:          fm.Tags,
		Metadata:      map[string]string{"adapter_origin": originJSON},
	}

	yamlBytes, err := marshalFrontmatter(tars)
	if err != nil {
		return RewriteResult{}, fmt.Errorf("hermes: marshal frontmatter: %w", err)
	}

	var out bytes.Buffer
	out.WriteString("---\n")
	out.Write(yamlBytes)
	if !bytes.HasSuffix(yamlBytes, []byte("\n")) {
		out.WriteByte('\n')
	}
	out.WriteString("---\n")
	out.WriteString(body)

	return RewriteResult{Converted: out.Bytes()}, nil
}

// ConvertSkillContent implements skillhub.SkillContentConverter.
func (s *Source) ConvertSkillContent(entry *skillhub.RegistryEntry, raw []byte) ([]byte, []string, error) {
	if entry == nil {
		return nil, nil, fmt.Errorf("hermes: entry is nil")
	}
	originURL := fmt.Sprintf("https://github.com/%s/%s/blob/%s/%s/SKILL.md",
		s.Owner, s.Repo, s.Branch, strings.TrimRight(entry.Path, "/"))
	res, err := RewriteSkillMD(RewriteInput{
		Raw:       raw,
		Entry:     entry,
		OriginURL: originURL,
	})
	if err != nil {
		return nil, nil, err
	}
	return res.Converted, res.Warnings, nil
}

func cloneHermesMeta(fm Frontmatter) map[string]any {
	out := map[string]any{}
	if len(fm.Tags) > 0 {
		out["tags"] = append([]string{}, fm.Tags...)
	}
	if len(fm.RelatedSkills) > 0 {
		out["related_skills"] = append([]string{}, fm.RelatedSkills...)
	}
	// Pass through any other metadata.hermes.* keys (e.g. future fields).
	if hm := hermesMeta(fm.RawMetadata); hm != nil {
		keys := make([]string, 0, len(hm))
		for k := range hm {
			if k != "tags" && k != "related_skills" {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			out[k] = hm[k]
		}
	}
	return out
}

func jsonStringify(v any) (string, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func marshalFrontmatter(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		_ = enc.Close()
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
