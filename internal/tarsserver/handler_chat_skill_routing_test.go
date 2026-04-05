package tarsserver

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/devlikebear/tars/internal/extensions"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/skill"
)

func TestResolveSkillForMessage_DoesNotAutoRouteNaturalLanguageKickoff(t *testing.T) {
	root := t.TempDir()
	workspaceDir := filepath.Join(root, "workspace")
	manager := newTestSkillManager(t, root, workspaceDir)

	got := resolveSkillForMessage("todo 앱 만드는 프로젝트 시작해줘", manager, workspaceDir, "sess-1")
	if got != nil {
		t.Fatalf("expected natural language kickoff to avoid implicit project-start routing, got %+v", got)
	}
}

func TestResolveSkillForMessage_NoBriefRoutingAfterProjectRemoval(t *testing.T) {
	root := t.TempDir()
	workspaceDir := filepath.Join(root, "workspace")
	manager := newTestSkillManager(t, root, workspaceDir)

	// After project package removal, brief routing always returns nil
	got := resolveSkillForMessage("로그인은 이메일 기반이면 돼", manager, workspaceDir, "sess-1")
	if got != nil {
		t.Fatalf("expected nil skill without active brief system, got %+v", got)
	}
}

func TestResolveSkillForMessage_UsesExplicitProjectStartCommand(t *testing.T) {
	root := t.TempDir()
	workspaceDir := filepath.Join(root, "workspace")
	manager := newTestSkillManager(t, root, workspaceDir)

	got := resolveSkillForMessage("/project-start 새 프로젝트 계획하자", manager, workspaceDir, "sess-1")
	if got == nil {
		t.Fatal("expected explicit project-start command to resolve")
	}
	if got.Name != "project-start" {
		t.Fatalf("expected project-start skill, got %+v", got)
	}

	resolved := resolveSkillSelection("/project-start 새 프로젝트 계획하자", manager, workspaceDir, "sess-1")
	if resolved.Definition == nil || resolved.Reason != "explicit_command" {
		t.Fatalf("expected explicit_command routing metadata, got %+v", resolved)
	}
}

func TestResolveSkillForMessage_RespectsExplicitEmptySkillAllowlist(t *testing.T) {
	root := t.TempDir()
	workspaceDir := filepath.Join(root, "workspace")
	manager := newTestSkillManager(t, root, workspaceDir)

	resolved := resolveSkillSelection("/project-start 새 프로젝트 계획하자", manager, workspaceDir, "sess-1", session.SessionToolConfig{
		SkillsCustom: true,
	})
	if resolved.Definition != nil {
		t.Fatalf("expected explicit empty skill allowlist to disable routing, got %+v", resolved)
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
