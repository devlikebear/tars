package skillhub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"
)

const (
	DefaultRegistryURL  = "https://raw.githubusercontent.com/devlikebear/tars-skills/main/registry.json"
	DefaultSkillBaseURL = "https://raw.githubusercontent.com/devlikebear/tars-skills/main"
)

// Registry fetches and searches the remote skill registry.
type Registry struct {
	RegistryURL  string
	SkillBaseURL string
	HTTPClient   *http.Client
}

// NewRegistry creates a registry with default URLs.
func NewRegistry() *Registry {
	return &Registry{
		RegistryURL:  DefaultRegistryURL,
		SkillBaseURL: DefaultSkillBaseURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// FetchIndex downloads and parses registry.json.
func (r *Registry) FetchIndex(ctx context.Context) (*RegistryIndex, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.RegistryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build registry request: %w", err)
	}
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch registry: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read registry body: %w", err)
	}
	var index RegistryIndex
	if err := json.Unmarshal(body, &index); err != nil {
		return nil, fmt.Errorf("parse registry: %w", err)
	}
	return &index, nil
}

// Search returns entries matching the query (substring match on name, description, tags).
func (r *Registry) Search(ctx context.Context, query string) ([]RegistryEntry, error) {
	index, err := r.FetchIndex(ctx)
	if err != nil {
		return nil, err
	}
	if query == "" {
		return index.Skills, nil
	}
	q := strings.ToLower(query)
	var results []RegistryEntry
	for _, entry := range index.Skills {
		if matchesQuery(entry, q) {
			results = append(results, entry)
		}
	}
	return results, nil
}

// FindByName returns the exact-match entry.
func (r *Registry) FindByName(ctx context.Context, name string) (*RegistryEntry, error) {
	index, err := r.FetchIndex(ctx)
	if err != nil {
		return nil, err
	}
	key := strings.ToLower(strings.TrimSpace(name))
	for _, entry := range index.Skills {
		if strings.ToLower(entry.Name) == key {
			return &entry, nil
		}
	}
	return nil, fmt.Errorf("skill %q not found in registry", name)
}

// FetchSkillContent downloads the SKILL.md for the given entry.
func (r *Registry) FetchSkillContent(ctx context.Context, entry *RegistryEntry) ([]byte, error) {
	return r.fetchHubFile(ctx, entry.Path, "SKILL.md", "skill")
}

// FetchFile downloads a companion file relative to the skill's path.
func (r *Registry) FetchFile(ctx context.Context, entry *RegistryEntry, relPath string) ([]byte, error) {
	return r.fetchHubFile(ctx, entry.Path, relPath, "skill file")
}

// SearchPlugins returns plugin entries matching the query.
func (r *Registry) SearchPlugins(ctx context.Context, query string) ([]PluginEntry, error) {
	index, err := r.FetchIndex(ctx)
	if err != nil {
		return nil, err
	}
	if query == "" {
		return index.Plugins, nil
	}
	q := strings.ToLower(query)
	var results []PluginEntry
	for _, p := range index.Plugins {
		if matchesPluginQuery(p, q) {
			results = append(results, p)
		}
	}
	return results, nil
}

// FindPluginByName returns the exact-match plugin entry.
func (r *Registry) FindPluginByName(ctx context.Context, name string) (*PluginEntry, error) {
	index, err := r.FetchIndex(ctx)
	if err != nil {
		return nil, err
	}
	key := strings.ToLower(strings.TrimSpace(name))
	for _, p := range index.Plugins {
		if strings.ToLower(p.Name) == key {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("plugin %q not found in registry", name)
}

// FetchPluginFile downloads a file relative to the plugin's path.
func (r *Registry) FetchPluginFile(ctx context.Context, entry *PluginEntry, relPath string) ([]byte, error) {
	return r.fetchHubFile(ctx, entry.Path, relPath, "plugin file")
}

// SearchMCPServers returns MCP entries matching the query.
func (r *Registry) SearchMCPServers(ctx context.Context, query string) ([]MCPEntry, error) {
	index, err := r.FetchIndex(ctx)
	if err != nil {
		return nil, err
	}
	if query == "" {
		return index.MCPServers, nil
	}
	q := strings.ToLower(query)
	var results []MCPEntry
	for _, entry := range index.MCPServers {
		if matchesMCPQuery(entry, q) {
			results = append(results, entry)
		}
	}
	return results, nil
}

// FindMCPByName returns the exact-match MCP entry.
func (r *Registry) FindMCPByName(ctx context.Context, name string) (*MCPEntry, error) {
	index, err := r.FetchIndex(ctx)
	if err != nil {
		return nil, err
	}
	key := strings.ToLower(strings.TrimSpace(name))
	for _, entry := range index.MCPServers {
		if strings.ToLower(entry.Name) == key {
			return &entry, nil
		}
	}
	return nil, fmt.Errorf("mcp server %q not found in registry", name)
}

// FetchMCPFile downloads a file relative to the MCP package path.
func (r *Registry) FetchMCPFile(ctx context.Context, entry *MCPEntry, relPath string) ([]byte, error) {
	return r.fetchHubFile(ctx, entry.Path, relPath, "mcp file")
}

func (r *Registry) fetchHubFile(ctx context.Context, basePath string, relPath string, label string) ([]byte, error) {
	cleanBasePath, err := cleanRegistryRelativePath(basePath)
	if err != nil {
		return nil, fmt.Errorf("invalid registry path %q: %w", basePath, err)
	}
	cleanRelPath, err := cleanRegistryRelativePath(relPath)
	if err != nil {
		return nil, fmt.Errorf("invalid registry file path %q: %w", relPath, err)
	}
	url := strings.TrimRight(r.SkillBaseURL, "/") + "/" + path.Join(cleanBasePath, cleanRelPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build %s request: %w", label, err)
	}
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", label, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s fetch returned status %d", label, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
}

func matchesPluginQuery(entry PluginEntry, q string) bool {
	if strings.Contains(strings.ToLower(entry.Name), q) {
		return true
	}
	if strings.Contains(strings.ToLower(entry.Description), q) {
		return true
	}
	for _, tag := range entry.Tags {
		if strings.Contains(strings.ToLower(tag), q) {
			return true
		}
	}
	return false
}

func matchesMCPQuery(entry MCPEntry, q string) bool {
	if strings.Contains(strings.ToLower(entry.Name), q) {
		return true
	}
	if strings.Contains(strings.ToLower(entry.Description), q) {
		return true
	}
	for _, tag := range entry.Tags {
		if strings.Contains(strings.ToLower(tag), q) {
			return true
		}
	}
	return false
}

func matchesQuery(entry RegistryEntry, q string) bool {
	if strings.Contains(strings.ToLower(entry.Name), q) {
		return true
	}
	if strings.Contains(strings.ToLower(entry.Description), q) {
		return true
	}
	for _, tag := range entry.Tags {
		if strings.Contains(strings.ToLower(tag), q) {
			return true
		}
	}
	return false
}
