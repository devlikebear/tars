package project

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/session"
)

func TestStoreCreateWritesProjectMarkdown(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	store := NewStore(root, func() time.Time { return now })

	created, err := store.Create(CreateInput{
		Name:            "Alpha Maint",
		Type:            "development",
		GitRepo:         "https://example.com/acme/alpha.git",
		Objective:       "Maintain the alpha production service",
		WorkflowProfile: " Software-Dev ",
		WorkflowRules: []WorkflowRule{
			{Name: "require_tests", Params: map[string]string{"command": "go test ./..."}},
			{Name: "require_review", Params: map[string]string{"mode": "human"}},
		},
		Instructions: "Always run tests before deploy",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if strings.TrimSpace(created.ID) == "" {
		t.Fatalf("expected non-empty project id")
	}
	if created.Name != "Alpha Maint" {
		t.Fatalf("expected name Alpha Maint, got %q", created.Name)
	}
	if created.Status != "active" {
		t.Fatalf("expected active status, got %q", created.Status)
	}
	if created.Type != "development" {
		t.Fatalf("expected development type, got %q", created.Type)
	}
	if created.WorkflowProfile != "software-dev" {
		t.Fatalf("expected normalized workflow profile, got %q", created.WorkflowProfile)
	}
	if len(created.WorkflowRules) != 2 {
		t.Fatalf("expected workflow rules to be stored, got %+v", created.WorkflowRules)
	}

	projectPath := filepath.Join(root, "projects", created.ID, "PROJECT.md")
	raw, err := os.ReadFile(projectPath)
	if err != nil {
		t.Fatalf("read project markdown: %v", err)
	}
	content := string(raw)
	if !strings.Contains(content, "id: "+created.ID) {
		t.Fatalf("expected id frontmatter in PROJECT.md, got %q", content)
	}
	if !strings.Contains(content, "objective: \"Maintain the alpha production service\"") {
		t.Fatalf("expected objective frontmatter in PROJECT.md, got %q", content)
	}
	if !strings.Contains(content, "workflow_profile: \"software-dev\"") {
		t.Fatalf("expected workflow_profile frontmatter in PROJECT.md, got %q", content)
	}
	if !strings.Contains(content, "workflow_rules:") || !strings.Contains(content, "name: \"require_tests\"") {
		t.Fatalf("expected workflow_rules frontmatter in PROJECT.md, got %q", content)
	}
	if !strings.Contains(content, "Always run tests before deploy") {
		t.Fatalf("expected body instructions in PROJECT.md, got %q", content)
	}

	listed, err := store.List()
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected one project, got %d", len(listed))
	}
	if listed[0].ID != created.ID {
		t.Fatalf("expected listed id %q, got %q", created.ID, listed[0].ID)
	}
}

func TestStoreUpdateAndArchive(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	store := NewStore(root, func() time.Time { return now })

	created, err := store.Create(CreateInput{Name: "Research X", Type: "research"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	updated, err := store.Update(created.ID, UpdateInput{
		Status:       stringPtr("paused"),
		Objective:    stringPtr("Track new LLM benchmark papers"),
		ToolsAllow:   []string{"web_search", "read_file"},
		Instructions: stringPtr("Summarize weekly and store artifacts under this directory"),
	})
	if err != nil {
		t.Fatalf("update project: %v", err)
	}
	if updated.Status != "paused" {
		t.Fatalf("expected paused status, got %q", updated.Status)
	}
	if updated.Objective != "Track new LLM benchmark papers" {
		t.Fatalf("unexpected objective: %q", updated.Objective)
	}
	if got := strings.Join(updated.ToolsAllow, ","); got != "web_search,read_file" {
		t.Fatalf("unexpected tools_allow: %q", got)
	}
	if !strings.Contains(updated.Body, "Summarize weekly") {
		t.Fatalf("expected updated body, got %q", updated.Body)
	}

	archived, err := store.Archive(created.ID)
	if err != nil {
		t.Fatalf("archive project: %v", err)
	}
	if archived.Status != "archived" {
		t.Fatalf("expected archived status, got %q", archived.Status)
	}

	activity, err := store.ListActivity(created.ID, 10)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if len(activity) < 3 {
		t.Fatalf("expected at least 3 activity items, got %d", len(activity))
	}
	if activity[0].Kind != ActivityKindProjectArchived {
		t.Fatalf("expected newest archived activity, got %+v", activity[0])
	}
	if activity[1].Kind != ActivityKindProjectUpdated {
		t.Fatalf("expected update activity second, got %+v", activity[1])
	}
	if activity[len(activity)-1].Kind != ActivityKindProjectCreated {
		t.Fatalf("expected oldest created activity, got %+v", activity[len(activity)-1])
	}
}

func TestStoreCreate_AppendsWarningForUnknownWorkflowProfile(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	store := NewStore(root, func() time.Time { return now })

	created, err := store.Create(CreateInput{
		Name:            "Custom Profile Project",
		WorkflowProfile: "softwre-dev",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	activity, err := store.ListActivity(created.ID, 10)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	found := false
	for _, item := range activity {
		if item.Kind == ActivityKindDecision && item.Status == "warning" && strings.Contains(item.Message, "softwre-dev") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected workflow profile warning activity, got %+v", activity)
	}
}

func TestStoreUpdatePreservesExistingCollectionsWhenInputSlicesAreEmpty(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	store := NewStore(root, func() time.Time { return now })

	created, err := store.Create(CreateInput{Name: "Policy Project"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	seeded, err := store.Update(created.ID, UpdateInput{
		ToolsAllow:         []string{"read_file", "read_file", " exec "},
		ToolsAllowGroups:   []string{"memory"},
		ToolsAllowPatterns: []string{"^list_"},
		ToolsDeny:          []string{"write_file"},
		ToolsRiskMax:       stringPtr(" Medium "),
		SkillsAllow:        []string{"deploy"},
		MCPServers:         []string{"filesystem"},
		SecretsRefs:        []string{"VAULT/prod/db"},
	})
	if err != nil {
		t.Fatalf("seed project policy: %v", err)
	}

	preserved, err := store.Update(created.ID, UpdateInput{
		ToolsAllow:         []string{},
		ToolsAllowGroups:   []string{},
		ToolsAllowPatterns: []string{},
		ToolsDeny:          []string{},
		SkillsAllow:        []string{},
		MCPServers:         []string{},
		SecretsRefs:        []string{},
	})
	if err != nil {
		t.Fatalf("preserve project policy: %v", err)
	}

	if got := strings.Join(seeded.ToolsAllow, ","); got != "read_file,exec" {
		t.Fatalf("unexpected seeded tools_allow: %q", got)
	}
	if got := strings.Join(preserved.ToolsAllow, ","); got != "read_file,exec" {
		t.Fatalf("expected tools_allow to be preserved, got %q", got)
	}
	if got := strings.Join(preserved.ToolsAllowGroups, ","); got != "memory" {
		t.Fatalf("expected tools_allow_groups to be preserved, got %q", got)
	}
	if got := strings.Join(preserved.ToolsAllowPatterns, ","); got != "^list_" {
		t.Fatalf("expected tools_allow_patterns to be preserved, got %q", got)
	}
	if got := strings.Join(preserved.ToolsDeny, ","); got != "write_file" {
		t.Fatalf("expected tools_deny to be preserved, got %q", got)
	}
	if preserved.ToolsRiskMax != "medium" {
		t.Fatalf("expected tools_risk_max to stay normalized, got %q", preserved.ToolsRiskMax)
	}
	if got := strings.Join(preserved.SkillsAllow, ","); got != "deploy" {
		t.Fatalf("expected skills_allow to be preserved, got %q", got)
	}
	if got := strings.Join(preserved.MCPServers, ","); got != "filesystem" {
		t.Fatalf("expected mcp_servers to be preserved, got %q", got)
	}
	if got := strings.Join(preserved.SecretsRefs, ","); got != "VAULT/prod/db" {
		t.Fatalf("expected secrets_refs to be preserved, got %q", got)
	}
}

func TestParseProjectDocument_Roundtrip(t *testing.T) {
	raw := `---
id: proj_demo
name: Demo Project
type: operations
status: active
created_at: "2026-02-22T12:00:00Z"
updated_at: "2026-02-22T12:00:00Z"
objective: "Keep service SLO above 99.9%"
tools_allow:
  - read_file
  - exec
skills_allow:
  - deploy
workflow_profile: "software-dev"
workflow_rules:
  - name: "require_tests"
    params:
      "command": "go test ./..."
  - name: "require_review"
    params:
      "mode": "human"
mcp_servers:
  - filesystem
secrets_refs:
  - VAULT/prod/db
---
Operate this project carefully.
`

	parsed, err := parseDocument(raw)
	if err != nil {
		t.Fatalf("parse project document: %v", err)
	}
	if parsed.ID != "proj_demo" {
		t.Fatalf("expected proj_demo, got %q", parsed.ID)
	}
	if parsed.Objective != "Keep service SLO above 99.9%" {
		t.Fatalf("unexpected objective: %q", parsed.Objective)
	}
	if got := strings.Join(parsed.ToolsAllow, ","); got != "read_file,exec" {
		t.Fatalf("unexpected tools_allow: %q", got)
	}
	if parsed.WorkflowProfile != "software-dev" {
		t.Fatalf("unexpected workflow profile: %q", parsed.WorkflowProfile)
	}
	if len(parsed.WorkflowRules) != 2 || parsed.WorkflowRules[0].Name != "require_tests" {
		t.Fatalf("unexpected workflow rules: %+v", parsed.WorkflowRules)
	}
	if !strings.Contains(parsed.Body, "Operate this project") {
		t.Fatalf("expected markdown body, got %q", parsed.Body)
	}

	encoded := buildDocument(parsed)
	if !strings.Contains(encoded, "id: proj_demo") {
		t.Fatalf("expected id in encoded document, got %q", encoded)
	}
	if !strings.Contains(encoded, "Operate this project carefully") {
		t.Fatalf("expected body in encoded document, got %q", encoded)
	}
}

func TestStoreFinalizeBrief_IsIdempotentAcrossConcurrentCalls(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 2, 22, 13, 0, 0, 0, time.UTC)
	storeA := NewStore(root, func() time.Time { return now })
	storeB := NewStore(root, func() time.Time { return now })
	sessionStore := session.NewStore(root)

	title := "Orbit Hearts"
	goal := "Ship the first serial arc"
	status := "ready"
	if _, err := storeA.UpdateBrief("sess-1", BriefUpdateInput{
		Title:  &title,
		Goal:   &goal,
		Status: &status,
	}); err != nil {
		t.Fatalf("update brief: %v", err)
	}

	start := make(chan struct{})
	type finalizeResult struct {
		projectID string
		err       error
	}
	results := make(chan finalizeResult, 2)
	var wg sync.WaitGroup
	for _, store := range []*Store{storeA, storeB} {
		wg.Add(1)
		go func(store *Store) {
			defer wg.Done()
			<-start
			created, _, err := store.FinalizeBrief("sess-1", sessionStore)
			results <- finalizeResult{projectID: created.ID, err: err}
		}(store)
	}
	close(start)
	wg.Wait()
	close(results)

	successes := 0
	for result := range results {
		if result.err == nil {
			successes++
			continue
		}
		if !strings.Contains(strings.ToLower(result.err.Error()), "already finalized") {
			t.Fatalf("expected already-finalized error, got %v", result.err)
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one successful finalize, got %d", successes)
	}

	projects, err := storeA.List()
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected one created project after concurrent finalize, got %+v", projects)
	}
}

func TestSplitFrontmatter(t *testing.T) {
	t.Run("closing delimiter without body", func(t *testing.T) {
		meta, body, hasMeta, err := splitFrontmatter("---\nname: demo\n---")
		if err != nil {
			t.Fatalf("split frontmatter: %v", err)
		}
		if !hasMeta {
			t.Fatalf("expected frontmatter to be detected")
		}
		if meta != "name: demo" {
			t.Fatalf("unexpected meta: %q", meta)
		}
		if body != "" {
			t.Fatalf("expected empty body, got %q", body)
		}
	})

	t.Run("unterminated frontmatter", func(t *testing.T) {
		_, _, _, err := splitFrontmatter("---\nname: demo\nbody")
		if err == nil {
			t.Fatalf("expected unterminated frontmatter error")
		}
		if !strings.Contains(err.Error(), "unterminated frontmatter") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func stringPtr(v string) *string {
	return &v
}
