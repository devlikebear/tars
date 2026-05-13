// Package anthropic is the HubSource adapter for anthropics/skills.
//
// The Anthropic skills repo is flat (skills/<name>/SKILL.md) with a
// per-skill LICENSE.txt — there is no repo-level LICENSE. Most skills are
// Apache-2.0 but the docx/pdf/pptx/xlsx skills are source-available
// proprietary; FetchLicense surfaces the detected label so the installer's
// attribution layer can refuse to materialize a Proprietary import.
package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/skillhub"
)

const (
	DefaultRepoOwner = "anthropics"
	DefaultRepoName  = "skills"
	DefaultBranch    = "main"
	DefaultSkillsDir = "skills"
	DefaultTokenEnv  = "GITHUB_TOKEN"

	httpTimeout     = 30 * time.Second
	maxIndexBytes   = 5 * 1024 * 1024
	maxRawFileBytes = 1 * 1024 * 1024
	maxLicenseBytes = 256 * 1024
)

// SourceID is the canonical HubSource ID for Anthropic skills.
const SourceID = "anthropic"

// Source is the Anthropic skills HubSource adapter.
type Source struct {
	Owner      string
	Repo       string
	Branch     string
	SkillsDir  string
	APIBaseURL string
	RawBaseURL string
	TokenEnv   string
	HTTPClient *http.Client
}

// New returns a Source pointing at anthropics/skills.
func New() *Source {
	return &Source{
		Owner:      DefaultRepoOwner,
		Repo:       DefaultRepoName,
		Branch:     DefaultBranch,
		SkillsDir:  DefaultSkillsDir,
		APIBaseURL: "https://api.github.com",
		RawBaseURL: "https://raw.githubusercontent.com",
		TokenEnv:   DefaultTokenEnv,
		HTTPClient: &http.Client{Timeout: httpTimeout},
	}
}

// ID returns the canonical adapter identifier.
func (s *Source) ID() string { return SourceID }

// SearchSkills lists every directory under skills/ in anthropics/skills.
// Per-skill LICENSE.txt is NOT fetched here (lazy until install) to keep
// rate-limit usage low on bare `tars skill search` calls.
func (s *Source) SearchSkills(ctx context.Context, query string) ([]skillhub.RegistryEntry, error) {
	dirs, err := s.listSkillDirs(ctx)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]skillhub.RegistryEntry, 0, len(dirs))
	for _, name := range dirs {
		if q != "" && !strings.Contains(strings.ToLower(name), q) {
			continue
		}
		entry, err := s.buildEntry(ctx, name)
		if err != nil {
			continue
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// FindSkillByName fetches and parses just the SKILL.md for the named skill.
func (s *Source) FindSkillByName(ctx context.Context, name string) (*skillhub.RegistryEntry, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, fmt.Errorf("anthropic: skill name is empty")
	}
	entry, err := s.buildEntry(ctx, trimmed)
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

// FetchSkillContent downloads the raw SKILL.md bytes for an entry.
func (s *Source) FetchSkillContent(ctx context.Context, entry *skillhub.RegistryEntry) ([]byte, error) {
	return s.fetchRawSkillFile(ctx, entry, "SKILL.md")
}

// FetchSkillFile downloads a companion file relative to the skill's
// directory. LICENSE.txt is excluded here — the installer takes it via
// FetchLicense.
func (s *Source) FetchSkillFile(ctx context.Context, entry *skillhub.RegistryEntry, relPath string) ([]byte, error) {
	return s.fetchRawSkillFile(ctx, entry, relPath)
}

// ListCompanionFiles walks the skill directory tree and returns every
// non-SKILL.md, non-LICENSE.txt file path the installer should materialize.
func (s *Source) ListCompanionFiles(ctx context.Context, entry *skillhub.RegistryEntry) ([]string, error) {
	if entry == nil {
		return nil, fmt.Errorf("anthropic: entry is nil")
	}
	base := strings.TrimRight(entry.Path, "/")
	files, _, err := s.walkSkillDir(ctx, base, "", 0)
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// FetchLicense returns the per-skill LICENSE.txt body and the detected
// SPDX-like label. Proprietary skills (docx/pdf/pptx/xlsx) are surfaced
// as LicenseProprietary so BuildAttribution refuses to materialize them.
func (s *Source) FetchLicense(ctx context.Context, entry *skillhub.RegistryEntry) ([]byte, string, error) {
	if entry == nil {
		return nil, "", fmt.Errorf("anthropic: entry is nil")
	}
	url := fmt.Sprintf("%s/%s/%s/%s/LICENSE.txt",
		s.Owner, s.Repo, s.Branch, strings.TrimRight(entry.Path, "/"))
	body, err := s.fetchRaw(ctx, url, maxLicenseBytes)
	if err != nil {
		return nil, "", fmt.Errorf("anthropic: fetch per-skill LICENSE.txt for %q: %w", entry.Name, err)
	}
	return body, skillhub.DetectLicenseLabel(body), nil
}

// Compile-time assertions.
var (
	_ skillhub.HubSource             = (*Source)(nil)
	_ skillhub.LicenseFetcher        = (*Source)(nil)
	_ skillhub.CompanionFileLister   = (*Source)(nil)
	_ skillhub.SkillContentConverter = (*Source)(nil)
)

// --- internal helpers ---

func (s *Source) listSkillDirs(ctx context.Context) ([]string, error) {
	entries, err := s.contentsAPI(ctx, s.SkillsDir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Type == "dir" {
			out = append(out, e.Name)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (s *Source) buildEntry(ctx context.Context, name string) (skillhub.RegistryEntry, error) {
	relSkillPath := fmt.Sprintf("%s/%s", strings.Trim(s.SkillsDir, "/"), name)
	raw, err := s.fetchRaw(ctx,
		fmt.Sprintf("%s/%s/%s/%s/SKILL.md", s.Owner, s.Repo, s.Branch, relSkillPath),
		maxRawFileBytes)
	if err != nil {
		return skillhub.RegistryEntry{}, fmt.Errorf("anthropic: fetch SKILL.md for %q: %w", name, err)
	}
	fm, err := ParseFrontmatter(raw)
	if err != nil {
		return skillhub.RegistryEntry{}, fmt.Errorf("anthropic: parse SKILL.md for %q: %w", name, err)
	}
	if strings.TrimSpace(fm.Name) == "" {
		return skillhub.RegistryEntry{}, fmt.Errorf("anthropic: SKILL.md for %q has empty name", name)
	}
	if strings.TrimSpace(fm.Description) == "" {
		return skillhub.RegistryEntry{}, fmt.Errorf("anthropic: SKILL.md for %q has empty description", name)
	}
	return skillhub.RegistryEntry{
		Name:          fm.Name,
		Description:   fm.Description,
		Version:       "0.0.0",
		Author:        "Anthropic, PBC",
		Path:          relSkillPath,
		UserInvocable: true,
	}, nil
}

func (s *Source) fetchRawSkillFile(ctx context.Context, entry *skillhub.RegistryEntry, relPath string) ([]byte, error) {
	if entry == nil {
		return nil, fmt.Errorf("anthropic: entry is nil")
	}
	clean := path.Clean("/" + relPath)
	if clean == "/" || strings.HasPrefix(clean, "/..") {
		return nil, fmt.Errorf("anthropic: invalid file path %q", relPath)
	}
	url := fmt.Sprintf("%s/%s/%s/%s%s", s.Owner, s.Repo, s.Branch, strings.TrimRight(entry.Path, "/"), clean)
	return s.fetchRaw(ctx, url, maxRawFileBytes)
}

type contentsListing struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
	Size int64  `json:"size"`
}

func (s *Source) contentsAPI(ctx context.Context, relPath string) ([]contentsListing, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s",
		strings.TrimRight(s.apiBase(), "/"), s.Owner, s.Repo, strings.TrimLeft(relPath, "/"), s.Branch)
	body, err := s.do(ctx, url, maxIndexBytes)
	if err != nil {
		return nil, err
	}
	var entries []contentsListing
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("anthropic: parse Contents API %q: %w", url, err)
	}
	return entries, nil
}

func (s *Source) walkSkillDir(ctx context.Context, baseDir, subDir string, depth int) ([]string, int64, error) {
	rel := baseDir
	if subDir != "" {
		rel = baseDir + "/" + subDir
	}
	entries, err := s.contentsAPI(ctx, rel)
	if err != nil {
		return nil, 0, err
	}
	var files []string
	var bytesUsed int64
	for _, e := range entries {
		switch e.Type {
		case "file":
			// LICENSE.txt is handled separately by FetchLicense; SKILL.md
			// is the main manifest.
			if subDir == "" && (e.Name == "SKILL.md" || e.Name == "LICENSE.txt") {
				continue
			}
			files = append(files, joinSubPath(subDir, e.Name))
			bytesUsed += e.Size
		case "dir":
			if depth >= 1 {
				continue
			}
			sub, used, err := s.walkSkillDir(ctx, baseDir, joinSubPath(subDir, e.Name), depth+1)
			if err != nil {
				return nil, bytesUsed, err
			}
			files = append(files, sub...)
			bytesUsed += used
		}
	}
	return files, bytesUsed, nil
}

func joinSubPath(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + "/" + name
}

func (s *Source) fetchRaw(ctx context.Context, repoPath string, limit int64) ([]byte, error) {
	url := fmt.Sprintf("%s/%s", strings.TrimRight(s.rawBase(), "/"), strings.TrimLeft(repoPath, "/"))
	return s.do(ctx, url, limit)
}

func (s *Source) do(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("anthropic: build request: %w", err)
	}
	req.Header.Set("Cache-Control", "no-cache")
	if tok := s.token(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	if strings.Contains(url, "api.github.com") {
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	}
	client := s.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: httpTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic: GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic: %s returned status %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

func (s *Source) apiBase() string {
	if v := strings.TrimSpace(s.APIBaseURL); v != "" {
		return v
	}
	return "https://api.github.com"
}

func (s *Source) rawBase() string {
	if v := strings.TrimSpace(s.RawBaseURL); v != "" {
		return v
	}
	return "https://raw.githubusercontent.com"
}

func (s *Source) token() string {
	env := strings.TrimSpace(s.TokenEnv)
	if env == "" {
		env = DefaultTokenEnv
	}
	return strings.TrimSpace(os.Getenv(env))
}
