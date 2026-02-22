package project

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const projectDocumentName = "PROJECT.md"

type Project struct {
	ID                string   `json:"id" yaml:"id"`
	Name              string   `json:"name" yaml:"name"`
	Type              string   `json:"type" yaml:"type"`
	Status            string   `json:"status" yaml:"status"`
	GitRepo           string   `json:"git_repo,omitempty" yaml:"git_repo,omitempty"`
	CreatedAt         string   `json:"created_at" yaml:"created_at"`
	UpdatedAt         string   `json:"updated_at" yaml:"updated_at"`
	Objective         string   `json:"objective,omitempty" yaml:"objective,omitempty"`
	ToolsAllow        []string `json:"tools_allow,omitempty" yaml:"tools_allow,omitempty"`
	ToolsAllowGroups  []string `json:"tools_allow_groups,omitempty" yaml:"tools_allow_groups,omitempty"`
	ToolsAllowPatterns []string `json:"tools_allow_patterns,omitempty" yaml:"tools_allow_patterns,omitempty"`
	ToolsDeny         []string `json:"tools_deny,omitempty" yaml:"tools_deny,omitempty"`
	ToolsRiskMax      string   `json:"tools_risk_max,omitempty" yaml:"tools_risk_max,omitempty"`
	SkillsAllow       []string `json:"skills_allow,omitempty" yaml:"skills_allow,omitempty"`
	MCPServers        []string `json:"mcp_servers,omitempty" yaml:"mcp_servers,omitempty"`
	SecretsRefs       []string `json:"secrets_refs,omitempty" yaml:"secrets_refs,omitempty"`
	Body              string   `json:"body,omitempty" yaml:"-"`
	Path              string   `json:"path,omitempty" yaml:"-"`
}

type CreateInput struct {
	Name         string
	Type         string
	GitRepo      string
	Objective    string
	Instructions string
	CloneRepo    bool
}

type UpdateInput struct {
	Name               *string
	Type               *string
	Status             *string
	GitRepo            *string
	Objective          *string
	Instructions       *string
	ToolsAllow         []string
	ToolsAllowGroups   []string
	ToolsAllowPatterns []string
	ToolsDeny          []string
	ToolsRiskMax       *string
	SkillsAllow        []string
	MCPServers         []string
	SecretsRefs        []string
}

type Store struct {
	workspaceDir string
	nowFn        func() time.Time
}

func NewStore(workspaceDir string, nowFn func() time.Time) *Store {
	if nowFn == nil {
		nowFn = time.Now
	}
	return &Store{
		workspaceDir: strings.TrimSpace(workspaceDir),
		nowFn:        nowFn,
	}
}

func (s *Store) Create(input CreateInput) (Project, error) {
	if s == nil {
		return Project{}, fmt.Errorf("project store is nil")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Project{}, fmt.Errorf("name is required")
	}
	now := s.nowFn().UTC().Format(time.RFC3339)
	id := newProjectID(name)
	project := Project{
		ID:        id,
		Name:      name,
		Type:      normalizeType(input.Type),
		Status:    "active",
		GitRepo:   strings.TrimSpace(input.GitRepo),
		CreatedAt: now,
		UpdatedAt: now,
		Objective: strings.TrimSpace(input.Objective),
		Body:      strings.TrimSpace(input.Instructions),
	}
	if err := s.write(project); err != nil {
		return Project{}, err
	}
	if input.CloneRepo && project.GitRepo != "" {
		if err := cloneProjectRepo(filepath.Join(s.workspaceDir, "projects", project.ID), project.GitRepo); err != nil {
			return Project{}, err
		}
	}
	return s.Get(project.ID)
}

func (s *Store) List() ([]Project, error) {
	if s == nil {
		return nil, fmt.Errorf("project store is nil")
	}
	root := filepath.Join(s.workspaceDir, "projects")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []Project{}, nil
		}
		return nil, err
	}
	projects := make([]Project, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := strings.TrimSpace(entry.Name())
		if id == "" {
			continue
		}
		item, err := s.Get(id)
		if err != nil {
			continue
		}
		projects = append(projects, item)
	}
	sort.Slice(projects, func(i, j int) bool {
		left := parseTime(projects[i].UpdatedAt)
		right := parseTime(projects[j].UpdatedAt)
		if left.Equal(right) {
			return projects[i].ID < projects[j].ID
		}
		return left.After(right)
	})
	if projects == nil {
		return []Project{}, nil
	}
	return projects, nil
}

func (s *Store) Get(id string) (Project, error) {
	if s == nil {
		return Project{}, fmt.Errorf("project store is nil")
	}
	projectID := strings.TrimSpace(id)
	if projectID == "" {
		return Project{}, fmt.Errorf("project id is required")
	}
	path := filepath.Join(s.workspaceDir, "projects", projectID, projectDocumentName)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Project{}, fmt.Errorf("project not found: %s", projectID)
		}
		return Project{}, err
	}
	parsed, err := parseDocument(string(raw))
	if err != nil {
		return Project{}, err
	}
	if strings.TrimSpace(parsed.ID) == "" {
		parsed.ID = projectID
	}
	parsed.Path = filepath.Join(s.workspaceDir, "projects", projectID)
	parsed.Type = normalizeType(parsed.Type)
	parsed.Status = normalizeStatus(parsed.Status)
	return parsed, nil
}

func (s *Store) Update(id string, input UpdateInput) (Project, error) {
	if s == nil {
		return Project{}, fmt.Errorf("project store is nil")
	}
	item, err := s.Get(id)
	if err != nil {
		return Project{}, err
	}
	if input.Name != nil {
		v := strings.TrimSpace(*input.Name)
		if v == "" {
			return Project{}, fmt.Errorf("name is required")
		}
		item.Name = v
	}
	if input.Type != nil {
		item.Type = normalizeType(*input.Type)
	}
	if input.Status != nil {
		item.Status = normalizeStatus(*input.Status)
	}
	if input.GitRepo != nil {
		item.GitRepo = strings.TrimSpace(*input.GitRepo)
	}
	if input.Objective != nil {
		item.Objective = strings.TrimSpace(*input.Objective)
	}
	if input.Instructions != nil {
		item.Body = strings.TrimSpace(*input.Instructions)
	}
	if len(input.ToolsAllow) > 0 {
		item.ToolsAllow = normalizeList(input.ToolsAllow)
	}
	if len(input.ToolsAllowGroups) > 0 {
		item.ToolsAllowGroups = normalizeList(input.ToolsAllowGroups)
	}
	if len(input.ToolsAllowPatterns) > 0 {
		item.ToolsAllowPatterns = normalizeList(input.ToolsAllowPatterns)
	}
	if len(input.ToolsDeny) > 0 {
		item.ToolsDeny = normalizeList(input.ToolsDeny)
	}
	if input.ToolsRiskMax != nil {
		item.ToolsRiskMax = strings.TrimSpace(strings.ToLower(*input.ToolsRiskMax))
	}
	if len(input.SkillsAllow) > 0 {
		item.SkillsAllow = normalizeList(input.SkillsAllow)
	}
	if len(input.MCPServers) > 0 {
		item.MCPServers = normalizeList(input.MCPServers)
	}
	if len(input.SecretsRefs) > 0 {
		item.SecretsRefs = normalizeList(input.SecretsRefs)
	}
	item.UpdatedAt = s.nowFn().UTC().Format(time.RFC3339)
	if err := s.write(item); err != nil {
		return Project{}, err
	}
	return s.Get(item.ID)
}

func (s *Store) Archive(id string) (Project, error) {
	status := "archived"
	return s.Update(id, UpdateInput{Status: &status})
}

func (s *Store) write(project Project) error {
	project.ID = strings.TrimSpace(project.ID)
	if project.ID == "" {
		return fmt.Errorf("project id is required")
	}
	project.Name = strings.TrimSpace(project.Name)
	if project.Name == "" {
		return fmt.Errorf("project name is required")
	}
	project.Type = normalizeType(project.Type)
	project.Status = normalizeStatus(project.Status)
	if project.CreatedAt == "" {
		project.CreatedAt = s.nowFn().UTC().Format(time.RFC3339)
	}
	if project.UpdatedAt == "" {
		project.UpdatedAt = s.nowFn().UTC().Format(time.RFC3339)
	}
	project.ToolsAllow = normalizeList(project.ToolsAllow)
	project.ToolsAllowGroups = normalizeList(project.ToolsAllowGroups)
	project.ToolsAllowPatterns = normalizeList(project.ToolsAllowPatterns)
	project.ToolsDeny = normalizeList(project.ToolsDeny)
	project.SkillsAllow = normalizeList(project.SkillsAllow)
	project.MCPServers = normalizeList(project.MCPServers)
	project.SecretsRefs = normalizeList(project.SecretsRefs)
	project.ToolsRiskMax = strings.TrimSpace(strings.ToLower(project.ToolsRiskMax))

	dir := filepath.Join(s.workspaceDir, "projects", project.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, projectDocumentName)
	return os.WriteFile(path, []byte(buildDocument(project)), 0o644)
}

func parseDocument(raw string) (Project, error) {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	metaRaw, body, hasMeta, err := splitFrontmatter(normalized)
	if err != nil {
		return Project{}, err
	}
	if !hasMeta {
		return Project{Body: strings.TrimSpace(normalized)}, nil
	}
	parsed := map[string]any{}
	if err := yaml.Unmarshal([]byte(metaRaw), &parsed); err != nil {
		return Project{}, fmt.Errorf("parse project frontmatter: %w", err)
	}
	item := Project{
		ID:                 mapString(parsed, "id"),
		Name:               mapString(parsed, "name"),
		Type:               normalizeType(mapString(parsed, "type")),
		Status:             normalizeStatus(mapString(parsed, "status")),
		GitRepo:            mapString(parsed, "git_repo", "git-repo"),
		CreatedAt:          mapString(parsed, "created_at", "created-at"),
		UpdatedAt:          mapString(parsed, "updated_at", "updated-at"),
		Objective:          mapString(parsed, "objective"),
		ToolsAllow:         mapStringList(parsed, "tools_allow", "tools-allow"),
		ToolsAllowGroups:   mapStringList(parsed, "tools_allow_groups", "tools-allow-groups"),
		ToolsAllowPatterns: mapStringList(parsed, "tools_allow_patterns", "tools-allow-patterns"),
		ToolsDeny:          mapStringList(parsed, "tools_deny", "tools-deny"),
		ToolsRiskMax:       mapString(parsed, "tools_risk_max", "tools-risk-max"),
		SkillsAllow:        mapStringList(parsed, "skills_allow", "skills-allow"),
		MCPServers:         mapStringList(parsed, "mcp_servers", "mcp-servers"),
		SecretsRefs:        mapStringList(parsed, "secrets_refs", "secrets-refs"),
		Body:               strings.TrimSpace(body),
	}
	if item.CreatedAt == "" {
		item.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if item.UpdatedAt == "" {
		item.UpdatedAt = item.CreatedAt
	}
	return item, nil
}

func buildDocument(project Project) string {
	project.Type = normalizeType(project.Type)
	project.Status = normalizeStatus(project.Status)
	var b strings.Builder
	b.WriteString("---\n")
	_, _ = fmt.Fprintf(&b, "id: %s\n", strings.TrimSpace(project.ID))
	_, _ = fmt.Fprintf(&b, "name: %s\n", quoteYAML(project.Name))
	_, _ = fmt.Fprintf(&b, "type: %s\n", strings.TrimSpace(project.Type))
	_, _ = fmt.Fprintf(&b, "status: %s\n", strings.TrimSpace(project.Status))
	if v := strings.TrimSpace(project.GitRepo); v != "" {
		_, _ = fmt.Fprintf(&b, "git_repo: %s\n", quoteYAML(v))
	}
	if v := strings.TrimSpace(project.CreatedAt); v != "" {
		_, _ = fmt.Fprintf(&b, "created_at: %s\n", quoteYAML(v))
	}
	if v := strings.TrimSpace(project.UpdatedAt); v != "" {
		_, _ = fmt.Fprintf(&b, "updated_at: %s\n", quoteYAML(v))
	}
	if v := strings.TrimSpace(project.Objective); v != "" {
		_, _ = fmt.Fprintf(&b, "objective: %s\n", quoteYAML(v))
	}
	writeList := func(key string, values []string) {
		vals := normalizeList(values)
		if len(vals) == 0 {
			return
		}
		_, _ = fmt.Fprintf(&b, "%s:\n", key)
		for _, item := range vals {
			_, _ = fmt.Fprintf(&b, "  - %s\n", quoteYAML(item))
		}
	}
	writeList("tools_allow", project.ToolsAllow)
	writeList("tools_allow_groups", project.ToolsAllowGroups)
	writeList("tools_allow_patterns", project.ToolsAllowPatterns)
	writeList("tools_deny", project.ToolsDeny)
	if v := strings.TrimSpace(project.ToolsRiskMax); v != "" {
		_, _ = fmt.Fprintf(&b, "tools_risk_max: %s\n", quoteYAML(v))
	}
	writeList("skills_allow", project.SkillsAllow)
	writeList("mcp_servers", project.MCPServers)
	writeList("secrets_refs", project.SecretsRefs)
	b.WriteString("---\n")
	if body := strings.TrimSpace(project.Body); body != "" {
		b.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func splitFrontmatter(raw string) (string, string, bool, error) {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return "", normalized, false, nil
	}
	rest := normalized[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		if strings.HasSuffix(rest, "\n---") {
			return rest[:len(rest)-len("\n---")], "", true, nil
		}
		return "", "", false, fmt.Errorf("unterminated frontmatter")
	}
	meta := rest[:end]
	body := rest[end+len("\n---\n"):]
	return meta, body, true, nil
}

func mapString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			return strings.TrimSpace(fmt.Sprint(value))
		}
	}
	return ""
}

func mapStringList(values map[string]any, keys ...string) []string {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}
		switch v := raw.(type) {
		case []any:
			items := make([]string, 0, len(v))
			for _, entry := range v {
				items = append(items, fmt.Sprint(entry))
			}
			return normalizeList(items)
		case []string:
			return normalizeList(v)
		case string:
			return normalizeList([]string{v})
		}
	}
	return nil
}

func normalizeType(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "development", "research", "operations", "general":
		return v
	default:
		return "general"
	}
}

func normalizeStatus(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "active", "paused", "completed", "archived":
		return v
	default:
		return "active"
	}
}

func normalizeList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func quoteYAML(v string) string {
	trimmed := strings.TrimSpace(v)
	trimmed = strings.ReplaceAll(trimmed, "\"", "\\\"")
	return "\"" + trimmed + "\""
}

func newProjectID(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = strings.ReplaceAll(slug, " ", "-")
	var b strings.Builder
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	base := strings.Trim(b.String(), "-_")
	if base == "" {
		base = "project"
	}
	randPart := make([]byte, 3)
	if _, err := rand.Read(randPart); err != nil {
		return base + "-" + fmt.Sprint(time.Now().UTC().UnixNano())
	}
	return base + "-" + hex.EncodeToString(randPart)
}

func parseTime(raw string) time.Time {
	v := strings.TrimSpace(raw)
	if v == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}

func cloneProjectRepo(projectDir string, repo string) error {
	target := filepath.Join(projectDir, "repo")
	if _, err := os.Stat(target); err == nil {
		return nil
	}
	cmd := exec.Command("git", "clone", "--depth", "1", repo, target)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("clone project repo: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
