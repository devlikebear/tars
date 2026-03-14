package project

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const projectDocumentName = "PROJECT.md"

type Project struct {
	ID                 string   `json:"id" yaml:"id"`
	Name               string   `json:"name" yaml:"name"`
	Type               string   `json:"type" yaml:"type"`
	Status             string   `json:"status" yaml:"status"`
	GitRepo            string   `json:"git_repo,omitempty" yaml:"git_repo,omitempty"`
	CreatedAt          string   `json:"created_at" yaml:"created_at"`
	UpdatedAt          string   `json:"updated_at" yaml:"updated_at"`
	Objective          string   `json:"objective,omitempty" yaml:"objective,omitempty"`
	ToolsAllow         []string `json:"tools_allow,omitempty" yaml:"tools_allow,omitempty"`
	ToolsAllowGroups   []string `json:"tools_allow_groups,omitempty" yaml:"tools_allow_groups,omitempty"`
	ToolsAllowPatterns []string `json:"tools_allow_patterns,omitempty" yaml:"tools_allow_patterns,omitempty"`
	ToolsDeny          []string `json:"tools_deny,omitempty" yaml:"tools_deny,omitempty"`
	ToolsRiskMax       string   `json:"tools_risk_max,omitempty" yaml:"tools_risk_max,omitempty"`
	SkillsAllow        []string `json:"skills_allow,omitempty" yaml:"skills_allow,omitempty"`
	MCPServers         []string `json:"mcp_servers,omitempty" yaml:"mcp_servers,omitempty"`
	SecretsRefs        []string `json:"secrets_refs,omitempty" yaml:"secrets_refs,omitempty"`
	Body               string   `json:"body,omitempty" yaml:"-"`
	Path               string   `json:"path,omitempty" yaml:"-"`
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
	if err := s.writeBoard(defaultBoard(project.ID, s.nowFn().UTC())); err != nil {
		return Project{}, err
	}
	if input.CloneRepo && project.GitRepo != "" {
		if err := cloneProjectRepo(filepath.Join(s.workspaceDir, "projects", project.ID), project.GitRepo); err != nil {
			return Project{}, err
		}
	}
	if err := s.appendSystemActivity(project.ID, ActivityAppendInput{
		Kind:    ActivityKindProjectCreated,
		Status:  project.Status,
		Message: "Project created",
	}); err != nil {
		return Project{}, err
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
	before := item
	if err := applyUpdateInput(&item, input); err != nil {
		return Project{}, err
	}
	item.UpdatedAt = s.nowFn().UTC().Format(time.RFC3339)
	if err := s.write(item); err != nil {
		return Project{}, err
	}
	updated, err := s.Get(item.ID)
	if err != nil {
		return Project{}, err
	}
	if !projectActivityChanged(before, updated) {
		return updated, nil
	}
	kind := ActivityKindProjectUpdated
	message := "Project updated"
	if before.Status != updated.Status && updated.Status == "archived" {
		kind = ActivityKindProjectArchived
		message = "Project archived"
	}
	if err := s.appendSystemActivity(updated.ID, ActivityAppendInput{
		Kind:    kind,
		Status:  updated.Status,
		Message: message,
	}); err != nil {
		return Project{}, err
	}
	return updated, nil
}

func (s *Store) Archive(id string) (Project, error) {
	status := "archived"
	return s.Update(id, UpdateInput{Status: &status})
}

func (s *Store) write(project Project) error {
	project, err := normalizeProjectForWrite(project, s.nowFn)
	if err != nil {
		return err
	}

	dir := filepath.Join(s.workspaceDir, "projects", project.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, projectDocumentName)
	return os.WriteFile(path, []byte(buildDocument(project)), 0o644)
}
