package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_PriorityWorkspaceOverUserAndBundled(t *testing.T) {
	root := t.TempDir()
	bundledDir := filepath.Join(root, "bundled")
	userDir := filepath.Join(root, "user")
	workspaceDir := filepath.Join(root, "workspace")

	writeSkillFile(t, filepath.Join(bundledDir, "dup", "SKILL.md"), `---
name: dup
description: bundled description
---
# bundled`)
	writeSkillFile(t, filepath.Join(userDir, "dup", "SKILL.md"), `---
name: dup
description: user description
---
# user`)
	writeSkillFile(t, filepath.Join(workspaceDir, "skills", "dup", "SKILL.md"), `---
name: dup
description: workspace description
---
# workspace`)
	writeSkillFile(t, filepath.Join(userDir, "other", "SKILL.md"), "# Other skill")

	snapshot, err := Load(LoadOptions{
		Sources: []SourceDir{
			{Source: SourceBundled, Dir: bundledDir},
			{Source: SourceUser, Dir: userDir},
			{Source: SourceWorkspace, Dir: filepath.Join(workspaceDir, "skills")},
		},
	})
	if err != nil {
		t.Fatalf("load skills: %v", err)
	}
	if len(snapshot.Skills) != 2 {
		t.Fatalf("expected 2 merged skills, got %d", len(snapshot.Skills))
	}

	var dup *Definition
	for i := range snapshot.Skills {
		if snapshot.Skills[i].Name == "dup" {
			dup = &snapshot.Skills[i]
			break
		}
	}
	if dup == nil {
		t.Fatal("expected merged skill dup")
	}
	if dup.Description != "workspace description" {
		t.Fatalf("expected workspace description to win, got %q", dup.Description)
	}
	if dup.Source != SourceWorkspace {
		t.Fatalf("expected source workspace, got %q", dup.Source)
	}
}

func TestLoad_DefaultUserInvocableTrue(t *testing.T) {
	root := t.TempDir()
	workspaceDir := filepath.Join(root, "workspace", "skills")
	writeSkillFile(t, filepath.Join(workspaceDir, "simple", "SKILL.md"), `---
recommended_tools: [read_file, write_file]
recommended_project_files:
  - BRIEF.md
  - STATE.md
wake_phases: plan, draft
---
# Simple
Use it`)

	snapshot, err := Load(LoadOptions{
		Sources: []SourceDir{{Source: SourceWorkspace, Dir: workspaceDir}},
	})
	if err != nil {
		t.Fatalf("load skills: %v", err)
	}
	if len(snapshot.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(snapshot.Skills))
	}
	if !snapshot.Skills[0].UserInvocable {
		t.Fatalf("expected default user_invocable=true")
	}
	if got := strings.Join(snapshot.Skills[0].RecommendedTools, ","); got != "read_file,write_file" {
		t.Fatalf("unexpected recommended_tools: %q", got)
	}
	if got := strings.Join(snapshot.Skills[0].RecommendedProjectFiles, ","); got != "BRIEF.md,STATE.md" {
		t.Fatalf("unexpected recommended_project_files: %q", got)
	}
	if got := strings.Join(snapshot.Skills[0].WakePhases, ","); got != "plan,draft" {
		t.Fatalf("unexpected wake_phases: %q", got)
	}
}

func writeSkillFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
