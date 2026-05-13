package openclaw

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
	ImportedAt time.Time // optional; zero means now()
}

// RewriteResult is the rewriter's output.
type RewriteResult struct {
	Converted []byte
	Warnings  []string
}

// tarsFrontmatter mirrors the subset of internal/skill.Frontmatter we
// emit. The metadata block is encoded with JSON-stringified nested values
// so the line-based TARS frontmatter parser (which silently ignores
// unknown keys but chokes on nested YAML list items) accepts the output.
type tarsFrontmatter struct {
	Name          string            `yaml:"name"`
	Description   string            `yaml:"description"`
	UserInvocable bool              `yaml:"user_invocable,omitempty"`
	RequiresBins  []string          `yaml:"requires_bins,omitempty"`
	OS            []string          `yaml:"os,omitempty"`
	Metadata      map[string]string `yaml:"metadata,omitempty"`
}

// RewriteSkillMD parses the openclaw SKILL.md in input.Raw, builds a
// TARS-shaped frontmatter, and emits the converted SKILL.md (frontmatter +
// original body). Warnings list the openclaw `install[]` entries we refused
// to execute.
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
		return RewriteResult{}, fmt.Errorf("openclaw: frontmatter is missing 'name'")
	}
	if strings.TrimSpace(fm.Description) == "" {
		return RewriteResult{}, fmt.Errorf("openclaw: frontmatter is missing 'description'")
	}

	warnings := SummarizeInstallBlocks(fm.InstallBlocks)

	when := in.ImportedAt
	if when.IsZero() {
		when = time.Now().UTC()
	}

	originJSON, err := jsonStringify(map[string]any{
		"source":      SourceID,
		"url":         in.OriginURL,
		"commit_sha":  in.CommitSHA,
		"imported_at": when.UTC().Format(time.RFC3339),
		"openclaw":    cloneOpenclawMeta(fm.Metadata),
	})
	if err != nil {
		return RewriteResult{}, fmt.Errorf("openclaw: stringify adapter_origin: %w", err)
	}
	metadata := map[string]string{"adapter_origin": originJSON}
	if len(warnings) > 0 {
		warningsJSON, err := jsonStringify(map[string]any{
			"install_blocks_skipped": warnings,
		})
		if err != nil {
			return RewriteResult{}, fmt.Errorf("openclaw: stringify adapter_warnings: %w", err)
		}
		metadata["adapter_warnings"] = warningsJSON
	}

	tars := tarsFrontmatter{
		Name:          fm.Name,
		Description:   fm.Description,
		UserInvocable: true,
		RequiresBins:  fm.RequiresBins,
		OS:            fm.OS,
		Metadata:      metadata,
	}

	yamlBytes, err := marshalFrontmatter(tars)
	if err != nil {
		return RewriteResult{}, fmt.Errorf("openclaw: marshal frontmatter: %w", err)
	}

	var out bytes.Buffer
	out.WriteString("---\n")
	out.Write(yamlBytes)
	if !bytes.HasSuffix(yamlBytes, []byte("\n")) {
		out.WriteByte('\n')
	}
	out.WriteString("---\n")
	out.WriteString(body)

	return RewriteResult{
		Converted: out.Bytes(),
		Warnings:  warnings,
	}, nil
}

// cloneOpenclawMeta returns a defensive copy of metadata.openclaw so the
// emitted frontmatter does not alias the parsed input.
func cloneOpenclawMeta(meta map[string]any) any {
	oc := openclawMeta(meta)
	if oc == nil {
		return nil
	}
	return deepCloneAny(oc)
}

func deepCloneAny(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		// stable order for golden file friendliness
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			out[k] = deepCloneAny(x[k])
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = deepCloneAny(item)
		}
		return out
	default:
		return v
	}
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
