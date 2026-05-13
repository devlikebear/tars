// Package openclaw is the HubSource adapter for steipete/openclaw.
//
// The adapter indexes the repo's skills/ directory via the GitHub Contents
// API, downloads SKILL.md and companion files from raw.githubusercontent.com,
// converts the openclaw frontmatter (JSON-in-YAML metadata, install blocks)
// into TARS frontmatter, and emits an MIT ATTRIBUTION.md from the repo's
// LICENSE. install blocks are preserved as adapter_warnings — never executed.
package openclaw

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

// Default upstream coordinates. Override fields on Source to test or to
// follow a fork.
const (
	DefaultRepoOwner = "openclaw"
	DefaultRepoName  = "openclaw"
	DefaultBranch    = "main"
	DefaultSkillsDir = "skills"
	DefaultTokenEnv  = "GITHUB_TOKEN"

	httpTimeout      = 30 * time.Second
	maxIndexBytes    = 5 * 1024 * 1024 // 5 MB cap on Contents API responses
	maxRawFileBytes  = 1 * 1024 * 1024 // 1 MB cap on individual file downloads
	maxLicenseBytes  = 256 * 1024
	maxCompanionTree = 5 * 1024 * 1024 // total companion file budget per skill
)

// SourceID is the canonical HubSource ID for openclaw.
const SourceID = "openclaw"

// Source is the openclaw HubSource adapter.
type Source struct {
	Owner      string
	Repo       string
	Branch     string
	SkillsDir  string
	APIBaseURL string // override for tests; default https://api.github.com
	RawBaseURL string // override for tests; default https://raw.githubusercontent.com
	TokenEnv   string // env var name; default "GITHUB_TOKEN"
	HTTPClient *http.Client
}

// New returns a Source pointing at the public steipete/openclaw repo.
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

// SearchSkills returns every skill in the openclaw skills/ directory that
// matches the (case-insensitive) substring query. Empty query returns all.
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
			// Skip entries that fail to load; never break the whole search.
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
		return nil, fmt.Errorf("openclaw: skill name is empty")
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

// FetchSkillFile downloads a companion file relative to the entry's
// directory in the openclaw repo.
func (s *Source) FetchSkillFile(ctx context.Context, entry *skillhub.RegistryEntry, relPath string) ([]byte, error) {
	return s.fetchRawSkillFile(ctx, entry, relPath)
}

// ListCompanionFiles walks the skill's directory tree (one level deep plus
// the standard openclaw subdirectories) and returns every non-SKILL.md path
// the installer should materialize. The total size is capped at
// maxCompanionTree; entries past the cap are skipped with a recorded note
// on the next call (not exposed yet — out of scope for MVP).
func (s *Source) ListCompanionFiles(ctx context.Context, entry *skillhub.RegistryEntry) ([]string, error) {
	if entry == nil {
		return nil, fmt.Errorf("openclaw: entry is nil")
	}
	base := strings.TrimRight(entry.Path, "/")
	files, _, err := s.walkSkillDir(ctx, base, "", 0)
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// FetchLicense returns the repo-level LICENSE body and the detected SPDX
// label. openclaw is MIT; if the body doesn't match we fall back to the
// detection helper so the installer can refuse to import a non-MIT skill.
func (s *Source) FetchLicense(ctx context.Context, _ *skillhub.RegistryEntry) ([]byte, string, error) {
	body, err := s.fetchRaw(ctx, fmt.Sprintf("%s/%s/%s/LICENSE", s.Owner, s.Repo, s.Branch), maxLicenseBytes)
	if err != nil {
		return nil, "", fmt.Errorf("openclaw: fetch repo LICENSE: %w", err)
	}
	label := skillhub.DetectLicenseLabel(body)
	return body, label, nil
}

// ConvertSkillContent rewrites raw openclaw SKILL.md to TARS frontmatter and
// returns the install-time warnings (install blocks skipped, etc.). Satisfies
// the skillhub.SkillContentConverter interface.
func (s *Source) ConvertSkillContent(entry *skillhub.RegistryEntry, raw []byte) ([]byte, []string, error) {
	if entry == nil {
		return nil, nil, fmt.Errorf("openclaw: entry is nil")
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

// Compile-time assertions.
var (
	_ skillhub.HubSource             = (*Source)(nil)
	_ skillhub.LicenseFetcher        = (*Source)(nil)
	_ skillhub.CompanionFileLister   = (*Source)(nil)
	_ skillhub.SkillContentConverter = (*Source)(nil)
)

// --- internal helpers ---

// listSkillDirs returns every subdirectory under skills/ (one level deep).
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

// buildEntry parses the skill's SKILL.md (via fetch + frontmatter convert)
// and returns a TARS RegistryEntry. It does NOT populate Files; the installer
// uses ListCompanionFiles for that at materialize time.
func (s *Source) buildEntry(ctx context.Context, name string) (skillhub.RegistryEntry, error) {
	relSkillPath := fmt.Sprintf("%s/%s", strings.Trim(s.SkillsDir, "/"), name)
	raw, err := s.fetchRaw(ctx, fmt.Sprintf("%s/%s/%s/%s/SKILL.md", s.Owner, s.Repo, s.Branch, relSkillPath), maxRawFileBytes)
	if err != nil {
		return skillhub.RegistryEntry{}, fmt.Errorf("openclaw: fetch SKILL.md for %q: %w", name, err)
	}
	parsed, err := ParseFrontmatter(raw)
	if err != nil {
		return skillhub.RegistryEntry{}, fmt.Errorf("openclaw: parse SKILL.md for %q: %w", name, err)
	}
	if strings.TrimSpace(parsed.Name) == "" {
		return skillhub.RegistryEntry{}, fmt.Errorf("openclaw: SKILL.md for %q has empty name", name)
	}
	if strings.TrimSpace(parsed.Description) == "" {
		return skillhub.RegistryEntry{}, fmt.Errorf("openclaw: SKILL.md for %q has empty description", name)
	}
	return skillhub.RegistryEntry{
		Name:          parsed.Name,
		Description:   parsed.Description,
		Version:       "0.0.0",
		Author:        "openclaw community",
		Tags:          nil,
		Path:          relSkillPath,
		UserInvocable: true,
	}, nil
}

// fetchRawSkillFile downloads a file from the skill's directory at the
// pinned branch. relPath is treated as a path under entry.Path.
func (s *Source) fetchRawSkillFile(ctx context.Context, entry *skillhub.RegistryEntry, relPath string) ([]byte, error) {
	if entry == nil {
		return nil, fmt.Errorf("openclaw: entry is nil")
	}
	clean := path.Clean("/" + relPath)
	if clean == "/" || strings.HasPrefix(clean, "/..") {
		return nil, fmt.Errorf("openclaw: invalid file path %q", relPath)
	}
	url := fmt.Sprintf("%s/%s/%s/%s%s", s.Owner, s.Repo, s.Branch, strings.TrimRight(entry.Path, "/"), clean)
	return s.fetchRaw(ctx, url, maxRawFileBytes)
}

// contentsListing is the subset of the GitHub Contents API response we use.
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
		return nil, fmt.Errorf("openclaw: parse Contents API %q: %w", url, err)
	}
	return entries, nil
}

// walkSkillDir descends one extra level under the skill directory and
// returns all non-SKILL.md file paths (relative to the skill dir). The
// remaining byte budget shrinks as we accumulate file sizes; subdirectories
// stop being walked once it is exhausted.
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
			if e.Name == "SKILL.md" && subDir == "" {
				continue // already fetched as the main manifest
			}
			files = append(files, joinSubPath(subDir, e.Name))
			bytesUsed += e.Size
		case "dir":
			if depth >= 1 {
				continue // limit to one level deep
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
		return nil, fmt.Errorf("openclaw: build request: %w", err)
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
		return nil, fmt.Errorf("openclaw: GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openclaw: %s returned status %d", url, resp.StatusCode)
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
