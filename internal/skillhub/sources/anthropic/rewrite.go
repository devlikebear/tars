package anthropic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/devlikebear/tars/internal/skillhub"
)

// RewriteInput collects everything RewriteSkillMD needs.
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
type tarsFrontmatter struct {
	Name          string            `yaml:"name"`
	Description   string            `yaml:"description"`
	UserInvocable bool              `yaml:"user_invocable,omitempty"`
	Metadata      map[string]string `yaml:"metadata,omitempty"`
}

// RewriteSkillMD parses the Anthropic SKILL.md and emits the TARS form.
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
		return RewriteResult{}, fmt.Errorf("anthropic: frontmatter is missing 'name'")
	}
	if strings.TrimSpace(fm.Description) == "" {
		return RewriteResult{}, fmt.Errorf("anthropic: frontmatter is missing 'description'")
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
	}
	if fm.LicenseHint != "" {
		originPayload["license_hint"] = fm.LicenseHint
	}
	originJSON, err := jsonStringify(originPayload)
	if err != nil {
		return RewriteResult{}, fmt.Errorf("anthropic: stringify adapter_origin: %w", err)
	}

	tars := tarsFrontmatter{
		Name:          fm.Name,
		Description:   fm.Description,
		UserInvocable: true,
		Metadata:      map[string]string{"adapter_origin": originJSON},
	}

	yamlBytes, err := marshalFrontmatter(tars)
	if err != nil {
		return RewriteResult{}, fmt.Errorf("anthropic: marshal frontmatter: %w", err)
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
		return nil, nil, fmt.Errorf("anthropic: entry is nil")
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
