package tarsserver

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/devlikebear/tars/internal/extensions"
	"github.com/devlikebear/tars/internal/project"
	"github.com/devlikebear/tars/internal/skill"
)

func TestResolveSkillForMessage_AutoRoutesProjectStartIntent(t *testing.T) {
	root := t.TempDir()
	workspaceDir := filepath.Join(root, "workspace")
	manager := newTestSkillManager(t, root, workspaceDir)

	got := resolveSkillForMessage("todo 앱 만드는 프로젝트 시작해줘", manager, workspaceDir, "sess-1")
	if got == nil {
		t.Fatal("expected project-start skill to be resolved")
	}
	if got.Name != "project-start" {
		t.Fatalf("expected project-start skill, got %+v", got)
	}
}

func TestResolveSkillForMessage_UsesProjectStartWhileBriefCollecting(t *testing.T) {
	root := t.TempDir()
	workspaceDir := filepath.Join(root, "workspace")
	manager := newTestSkillManager(t, root, workspaceDir)

	store := project.NewStore(workspaceDir, nil)
	status := "collecting"
	goal := "Ship a todo app"
	if _, err := store.UpdateBrief("sess-1", project.BriefUpdateInput{
		Goal:   &goal,
		Status: &status,
	}); err != nil {
		t.Fatalf("update brief: %v", err)
	}

	got := resolveSkillForMessage("로그인은 이메일 기반이면 돼", manager, workspaceDir, "sess-1")
	if got == nil {
		t.Fatal("expected collecting brief to continue with project-start skill")
	}
	if got.Name != "project-start" {
		t.Fatalf("expected project-start skill, got %+v", got)
	}
}

func newTestSkillManager(t *testing.T, root, workspaceDir string) *extensions.Manager {
	t.Helper()
	skillDir := filepath.Join(root, "skills", "project-start")
	writeSkillFile(t, filepath.Join(skillDir, "SKILL.md"), `---
name: project-start
description: start projects
user-invocable: true
---
# Project Start
`)
	manager, err := extensions.NewManager(extensions.Options{
		WorkspaceDir:   workspaceDir,
		SkillsEnabled:  true,
		PluginsEnabled: false,
		SkillSources: []skill.SourceDir{
			{Source: skill.SourceWorkspace, Dir: filepath.Join(root, "skills")},
		},
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if err := manager.Reload(context.Background()); err != nil {
		t.Fatalf("reload manager: %v", err)
	}
	return manager
}

func writeSkillFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
