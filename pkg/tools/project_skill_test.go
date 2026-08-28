package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/skill"
)

func TestProjectSkillToolCreatesCwdLocalSkill(t *testing.T) {
	workspace := t.TempDir()
	cwd := filepath.Join(workspace, "project")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("mkdir cwd: %v", err)
	}

	tl := NewProjectSkillToolWithPolicy(NewPathPolicy(workspace, []string{cwd}, cwd))
	result, err := tl.Execute(context.Background(), mustJSON(t, map[string]any{
		"kind":              "skill",
		"name":              "project-review",
		"description":       "Review this project using local conventions.",
		"body":              "Use the local test matrix before proposing changes.",
		"recommended_tools": []string{"bash", "read_file"},
	}))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got %s", result.Text())
	}

	path := filepath.Join(cwd, ".tars", "skills", "project-review", "SKILL.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read skill: %v", err)
	}
	text := string(raw)
	for _, want := range []string{
		"name: project-review",
		"description: Review this project using local conventions.",
		"slash: project-review",
		"user_invocable: true",
		"recommended_tools:",
		"- bash",
		"Use the local test matrix before proposing changes.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("skill file missing %q:\n%s", want, text)
		}
	}

	snapshot, err := skill.Load(skill.LoadOptions{
		Sources: []skill.SourceDir{{Source: skill.SourceSessionCwd, Dir: filepath.Join(cwd, ".tars", "skills")}},
	})
	if err != nil {
		t.Fatalf("load skill: %v", err)
	}
	if len(snapshot.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", snapshot.Diagnostics)
	}
	if len(snapshot.Skills) != 1 || snapshot.Skills[0].Name != "project-review" {
		t.Fatalf("expected project-review skill, got %+v", snapshot.Skills)
	}
}

func TestProjectSkillToolCreatesCwdLocalCommand(t *testing.T) {
	workspace := t.TempDir()
	cwd := filepath.Join(workspace, "project")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("mkdir cwd: %v", err)
	}

	tl := NewProjectSkillToolWithPolicy(NewPathPolicy(workspace, []string{cwd}, cwd))
	result, err := tl.Execute(context.Background(), mustJSON(t, map[string]any{
		"kind":              "command",
		"name":              "빠른검토",
		"description":       "Run a local review command.",
		"body":              "Review only the files mentioned by the user.",
		"recommended_tools": []string{"read_file"},
	}))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got %s", result.Text())
	}

	path := filepath.Join(cwd, ".tars", "commands", "빠른검토.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read command: %v", err)
	}
	text := string(raw)
	for _, want := range []string{
		"name: 빠른검토",
		"description: Run a local review command.",
		"slash: 빠른검토",
		"user_invocable: true",
		"recommended_tools:",
		"- read_file",
		"Review only the files mentioned by the user.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("command file missing %q:\n%s", want, text)
		}
	}
}

func TestProjectSkillToolRequiresExplicitOverwrite(t *testing.T) {
	workspace := t.TempDir()
	cwd := filepath.Join(workspace, "project")
	existing := filepath.Join(cwd, ".tars", "commands")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatalf("mkdir commands: %v", err)
	}
	path := filepath.Join(existing, "ship.md")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatalf("seed command: %v", err)
	}

	tl := NewProjectSkillToolWithPolicy(NewPathPolicy(workspace, []string{cwd}, cwd))
	result, err := tl.Execute(context.Background(), mustJSON(t, map[string]any{
		"kind": "command",
		"name": "ship",
		"body": "Ship the current change.",
	}))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Text(), "already exists") {
		t.Fatalf("expected already exists error, got %s", result.Text())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read command: %v", err)
	}
	if string(raw) != "original" {
		t.Fatalf("expected file to remain untouched, got %q", string(raw))
	}
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return raw
}
