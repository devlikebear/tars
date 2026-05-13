// Package hermes is the HubSource adapter for NousResearch/hermes-agent.
//
// hermes skills live two levels deep — `skills/<category>/<name>/SKILL.md`
// — so the adapter indexes the repo via a single recursive Trees API call
// instead of walking categories one Contents request at a time. Frontmatter
// is standard YAML (no JSON-in-YAML quirks); the rewriter only has to copy
// fields through and preserve hermes-specific metadata under
// `metadata.adapter_origin.hermes`.
package hermes

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
	DefaultRepoOwner = "NousResearch"
	DefaultRepoName  = "hermes-agent"
	DefaultBranch    = "main"
	DefaultSkillsDir = "skills"
	DefaultTokenEnv  = "GITHUB_TOKEN"

	httpTimeout     = 30 * time.Second
	maxTreesBytes   = 10 * 1024 * 1024 // hermes Trees recursive can be larger than openclaw
	maxRawFileBytes = 1 * 1024 * 1024
	maxLicenseBytes = 256 * 1024
)

// SourceID is the canonical HubSource ID for hermes.
const SourceID = "hermes"

// Source is the hermes HubSource adapter.
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

// New returns a Source pointing at the public NousResearch/hermes-agent repo.
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

// SearchSkills returns every hermes skill that matches the (case-insensitive)
// substring query. Skills are listed by indexing the repo's full tree once.
func (s *Source) SearchSkills(ctx context.Context, query string) ([]skillhub.RegistryEntry, error) {
	skillPaths, err := s.indexSkillPaths(ctx)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]skillhub.RegistryEntry, 0, len(skillPaths))
	for _, p := range skillPaths {
		entry, err := s.buildEntry(ctx, p)
		if err != nil {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(entry.Name), q) && !strings.Contains(strings.ToLower(entry.Description), q) {
			continue
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// FindSkillByName resolves a bare skill name to its hermes path (handling
// the case/name uniqueness across categories).
func (s *Source) FindSkillByName(ctx context.Context, name string) (*skillhub.RegistryEntry, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, fmt.Errorf("hermes: skill name is empty")
	}
	skillPaths, err := s.indexSkillPaths(ctx)
	if err != nil {
		return nil, err
	}
	key := strings.ToLower(trimmed)
	var matches []string
	for _, p := range skillPaths {
		// p looks like "skills/<category>/<name>"; pull out <name>.
		segments := strings.Split(p, "/")
		if len(segments) >= 1 && strings.ToLower(segments[len(segments)-1]) == key {
			matches = append(matches, p)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("hermes: skill %q not found", name)
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("hermes: skill %q is ambiguous across categories: %s", name, strings.Join(matches, ", "))
	}
	entry, err := s.buildEntry(ctx, matches[0])
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
// directory in the hermes repo.
func (s *Source) FetchSkillFile(ctx context.Context, entry *skillhub.RegistryEntry, relPath string) ([]byte, error) {
	return s.fetchRawSkillFile(ctx, entry, relPath)
}

// ListCompanionFiles uses the cached recursive tree to enumerate every
// non-SKILL.md file under the skill's directory.
func (s *Source) ListCompanionFiles(ctx context.Context, entry *skillhub.RegistryEntry) ([]string, error) {
	if entry == nil {
		return nil, fmt.Errorf("hermes: entry is nil")
	}
	tree, err := s.fetchTree(ctx)
	if err != nil {
		return nil, err
	}
	prefix := strings.TrimRight(entry.Path, "/") + "/"
	var out []string
	for _, node := range tree {
		if node.Type != "blob" {
			continue
		}
		if !strings.HasPrefix(node.Path, prefix) {
			continue
		}
		rel := strings.TrimPrefix(node.Path, prefix)
		if rel == "" || rel == "SKILL.md" {
			continue
		}
		out = append(out, rel)
	}
	sort.Strings(out)
	return out, nil
}

// FetchLicense returns the hermes-agent repo LICENSE (MIT).
func (s *Source) FetchLicense(ctx context.Context, _ *skillhub.RegistryEntry) ([]byte, string, error) {
	body, err := s.fetchRaw(ctx, fmt.Sprintf("%s/%s/%s/LICENSE", s.Owner, s.Repo, s.Branch), maxLicenseBytes)
	if err != nil {
		return nil, "", fmt.Errorf("hermes: fetch repo LICENSE: %w", err)
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

type treeNode struct {
	Path string `json:"path"`
	Type string `json:"type"`
	Size int64  `json:"size"`
}

type treesResponse struct {
	Tree      []treeNode `json:"tree"`
	Truncated bool       `json:"truncated"`
}

// indexSkillPaths returns the directory paths of every hermes skill, i.e.
// every "skills/<category>/<name>" that contains a SKILL.md.
func (s *Source) indexSkillPaths(ctx context.Context) ([]string, error) {
	tree, err := s.fetchTree(ctx)
	if err != nil {
		return nil, err
	}
	prefix := strings.TrimRight(s.SkillsDir, "/") + "/"
	skills := make(map[string]struct{})
	for _, node := range tree {
		if node.Type != "blob" {
			continue
		}
		if !strings.HasPrefix(node.Path, prefix) {
			continue
		}
		// only nodes whose basename is exactly SKILL.md
		if !strings.HasSuffix(node.Path, "/SKILL.md") {
			continue
		}
		dir := strings.TrimSuffix(node.Path, "/SKILL.md")
		segs := strings.Split(dir, "/")
		// require "skills/<category>/<name>" — 3 segments under skillsDir.
		if len(segs) != 3 {
			continue
		}
		skills[dir] = struct{}{}
	}
	out := make([]string, 0, len(skills))
	for p := range skills {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}

// fetchTree pulls the repo's recursive tree once per call. The MVP does
// not cache; Phase 5 may add a TTL cache.
func (s *Source) fetchTree(ctx context.Context) ([]treeNode, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/git/trees/%s?recursive=1",
		strings.TrimRight(s.apiBase(), "/"), s.Owner, s.Repo, s.Branch)
	body, err := s.do(ctx, url, maxTreesBytes)
	if err != nil {
		return nil, err
	}
	var resp treesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("hermes: parse Trees response: %w", err)
	}
	// truncated is fine for MVP; we just operate on the partial tree. A
	// log line is omitted to keep the adapter dependency-free.
	return resp.Tree, nil
}

// buildEntry parses the SKILL.md at the given hermes path and returns a
// TARS RegistryEntry. The path is "skills/<category>/<name>".
func (s *Source) buildEntry(ctx context.Context, skillPath string) (skillhub.RegistryEntry, error) {
	raw, err := s.fetchRaw(ctx, fmt.Sprintf("%s/%s/%s/%s/SKILL.md", s.Owner, s.Repo, s.Branch, strings.TrimRight(skillPath, "/")), maxRawFileBytes)
	if err != nil {
		return skillhub.RegistryEntry{}, fmt.Errorf("hermes: fetch SKILL.md for %q: %w", skillPath, err)
	}
	fm, err := ParseFrontmatter(raw)
	if err != nil {
		return skillhub.RegistryEntry{}, fmt.Errorf("hermes: parse SKILL.md for %q: %w", skillPath, err)
	}
	if strings.TrimSpace(fm.Name) == "" {
		return skillhub.RegistryEntry{}, fmt.Errorf("hermes: SKILL.md %q has empty name", skillPath)
	}
	if strings.TrimSpace(fm.Description) == "" {
		return skillhub.RegistryEntry{}, fmt.Errorf("hermes: SKILL.md %q has empty description", skillPath)
	}
	version := strings.TrimSpace(fm.Version)
	if version == "" {
		version = "0.0.0"
	}
	author := strings.TrimSpace(fm.Author)
	if author == "" {
		author = "hermes-agent community"
	}
	return skillhub.RegistryEntry{
		Name:          fm.Name,
		Description:   fm.Description,
		Version:       version,
		Author:        author,
		Tags:          fm.Tags,
		Path:          skillPath,
		UserInvocable: true,
	}, nil
}

func (s *Source) fetchRawSkillFile(ctx context.Context, entry *skillhub.RegistryEntry, relPath string) ([]byte, error) {
	if entry == nil {
		return nil, fmt.Errorf("hermes: entry is nil")
	}
	clean := path.Clean("/" + relPath)
	if clean == "/" || strings.HasPrefix(clean, "/..") {
		return nil, fmt.Errorf("hermes: invalid file path %q", relPath)
	}
	url := fmt.Sprintf("%s/%s/%s/%s%s", s.Owner, s.Repo, s.Branch, strings.TrimRight(entry.Path, "/"), clean)
	return s.fetchRaw(ctx, url, maxRawFileBytes)
}

func (s *Source) fetchRaw(ctx context.Context, repoPath string, limit int64) ([]byte, error) {
	url := fmt.Sprintf("%s/%s", strings.TrimRight(s.rawBase(), "/"), strings.TrimLeft(repoPath, "/"))
	return s.do(ctx, url, limit)
}

func (s *Source) do(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("hermes: build request: %w", err)
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
		return nil, fmt.Errorf("hermes: GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hermes: %s returned status %d", url, resp.StatusCode)
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
