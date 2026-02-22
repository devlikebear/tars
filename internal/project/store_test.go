package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreCreateWritesProjectMarkdown(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	store := NewStore(root, func() time.Time { return now })

	created, err := store.Create(CreateInput{
		Name:         "Alpha Maint",
		Type:         "development",
		GitRepo:      "https://example.com/acme/alpha.git",
		Objective:    "Maintain the alpha production service",
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
		Status:      stringPtr("paused"),
		Objective:   stringPtr("Track new LLM benchmark papers"),
		ToolsAllow:  []string{"web_search", "read_file"},
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

func stringPtr(v string) *string {
	return &v
}
