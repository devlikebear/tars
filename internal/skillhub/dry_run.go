package skillhub

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// DryRunResult is the structured preview that PreviewInstall returns and
// Phase 5's HTTP API renders to the console. It is JSON-marshalable.
//
// The shape is deliberately a superset of InstallPreview (Phase 2): the
// short preview was a minimal "yes/no prompt payload"; DryRunResult adds
// per-file SHA256, the converted SKILL.md frontmatter snapshot, and license
// metadata so the user can verify exactly what would land on disk.
type DryRunResult struct {
	SourceID         string         `json:"source_id"`
	Ref              string         `json:"ref"`
	OriginalName     string         `json:"original_name"`
	OriginalPath     string         `json:"original_path,omitempty"`
	OriginalURL      string         `json:"original_url,omitempty"`
	CommitSHA        string         `json:"commit_sha,omitempty"`
	TargetDir        string         `json:"target_dir"`
	ConvertedSkill   SkillPreview   `json:"converted_skill"`
	Files            []FilePreview  `json:"files"`
	AdapterWarnings  []string       `json:"adapter_warnings,omitempty"`
	LicenseLabel     string         `json:"license_label,omitempty"`
	LicenseSource    string         `json:"license_source,omitempty"`
	ChecksumWarnings []string       `json:"checksum_warnings,omitempty"`
}

// SkillPreview is the subset of the converted SKILL.md frontmatter we
// surface to the user. Pulled directly from the converter output so the
// user sees what TARS would load — not what the upstream raw file looked
// like.
type SkillPreview struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Author        string   `json:"author,omitempty"`
	Version       string   `json:"version,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	UserInvocable bool     `json:"user_invocable"`
}

// FilePreview describes one file that would be materialized.
type FilePreview struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	// ExpectedSHA256 is set only for tars-hub manifests whose registry
	// entries carry pre-published checksums. The PreviewInstall caller
	// compares it against SHA256 and records mismatches in
	// ChecksumWarnings.
	ExpectedSHA256 string `json:"expected_sha256,omitempty"`
}

// PreviewInstall runs everything Install would do up to (but not including)
// the sandbox + materialize steps, then returns a structured preview the
// caller can render or feed into a confirmation prompt. Materialize is
// never reached so the workspace is untouched.
func (inst *Installer) PreviewInstall(ctx context.Context, ref string) (*DryRunResult, error) {
	sourceID, bareName := ResolveSkillRef(ref)
	src, entry, err := inst.resolveSkillSource(ctx, sourceID, bareName)
	if err != nil {
		return nil, err
	}
	return inst.buildPreviewFromSource(ctx, ref, src, entry)
}

func (inst *Installer) buildPreviewFromSource(ctx context.Context, ref string, src HubSource, entry *RegistryEntry) (*DryRunResult, error) {
	files, warnings, err := inst.downloadSkillFilesFromSource(ctx, src, entry)
	if err != nil {
		return nil, err
	}
	result := &DryRunResult{
		SourceID:        src.ID(),
		Ref:             ref,
		OriginalName:    entry.Name,
		OriginalPath:    entry.Path,
		TargetDir:       inst.skillDir(entry.Name),
		AdapterWarnings: warnings,
	}
	// Surface the converted SKILL.md so the preview reflects what TARS
	// will actually load.
	if manifest, ok := files[skillManifest]; ok {
		result.ConvertedSkill = parseSkillPreview(manifest)
	}
	// Per-file SHA256 + expected (tars-hub manifest only).
	expected := expectedChecksumMap(entry)
	result.Files = make([]FilePreview, 0, len(files))
	for _, path := range sortedFilePaths(files) {
		body := files[path]
		sum := computeSHA256Hex(body)
		fp := FilePreview{
			Path:   path,
			Size:   int64(len(body)),
			SHA256: sum,
		}
		if exp, ok := expected[path]; ok {
			fp.ExpectedSHA256 = exp
			if !strings.EqualFold(exp, sum) {
				result.ChecksumWarnings = append(result.ChecksumWarnings,
					fmt.Sprintf("checksum mismatch for %q: expected %s, computed %s", path, exp, sum))
			}
		}
		result.Files = append(result.Files, fp)
	}
	if _, ok := files[AttributionFilename]; ok {
		result.LicenseSource = AttributionFilename
		result.LicenseLabel = detectAttributionLabel(files[AttributionFilename])
	}
	return result, nil
}

func expectedChecksumMap(entry *RegistryEntry) map[string]string {
	out := make(map[string]string, len(entry.Files))
	for _, f := range entry.Files {
		if f.SHA256 != "" {
			out[f.Path] = f.SHA256
		}
	}
	return out
}

// parseSkillPreview extracts the user-facing TARS frontmatter fields from
// a converted SKILL.md. It is intentionally lenient: missing fields produce
// zero values rather than errors, since DryRunResult is informational.
func parseSkillPreview(manifest []byte) SkillPreview {
	text := strings.ReplaceAll(string(manifest), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return SkillPreview{}
	}
	rest := text[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return SkillPreview{}
	}
	block := rest[:end]
	var p SkillPreview
	for _, line := range strings.Split(block, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch key {
		case "name":
			p.Name = value
		case "description":
			p.Description = value
		case "author":
			p.Author = value
		case "version":
			p.Version = value
		case "user_invocable":
			p.UserInvocable = value == "true"
		case "tags":
			p.Tags = parseInlineList(value)
		}
	}
	return p
}

func parseInlineList(v string) []string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "[")
	v = strings.TrimSuffix(v, "]")
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, `"'`)
		if p != "" {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// detectAttributionLabel pulls the "License:" line out of an
// ATTRIBUTION.md so the preview can show the license label without
// re-fetching from the source.
func detectAttributionLabel(body []byte) string {
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "- **License**:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "- **License**:"))
		}
	}
	return ""
}
