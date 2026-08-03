package tarsserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/devlikebear/tars/internal/extensions"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/skill"
	"github.com/devlikebear/tars/internal/workstore"
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

func TestResolveSkillForMessage_UsesSkillSlashAlias(t *testing.T) {
	root := t.TempDir()
	workspaceDir := filepath.Join(root, "workspace")
	manager := newTestSkillManager(t, root, workspaceDir)

	resolved := resolveSkillSelection("/plan-app build a habit tracker", manager, workspaceDir, "sess-1")
	if resolved.Definition == nil {
		t.Fatal("expected explicit slash alias to resolve")
	}
	if resolved.Definition.Name != "project-start" {
		t.Fatalf("expected canonical project-start skill, got %+v", resolved.Definition)
	}
	if resolved.Reason != "explicit_command" {
		t.Fatalf("expected explicit_command reason, got %q", resolved.Reason)
	}
}

func TestResolveCommandForMessage_UsesSessionCwdStandaloneCommand(t *testing.T) {
	resolved := resolveCommandSelectionFromDefinitions("/메모 회의 내용 저장", []skill.Definition{
		{
			Name:          "메모",
			Description:   "save explicit memory",
			Slash:         "메모",
			UserInvocable: true,
			Source:        skill.SourceSessionCwd,
			RuntimePath:   "/tmp/.tars/commands/메모.md",
		},
	})
	if resolved.Definition == nil {
		t.Fatal("expected session cwd command to resolve")
	}
	if resolved.Definition.Name != "메모" {
		t.Fatalf("expected 메모 command, got %+v", resolved.Definition)
	}
	if resolved.Reason != "explicit_command" {
		t.Fatalf("expected explicit_command reason, got %q", resolved.Reason)
	}
}

func TestResolveCommandForMessage_RespectsExplicitEmptyCommandAllowlist(t *testing.T) {
	resolved := resolveCommandSelectionFromDefinitions("/메모 회의 내용 저장", []skill.Definition{
		{
			Name:          "메모",
			Description:   "save explicit memory",
			Slash:         "메모",
			UserInvocable: true,
			Source:        skill.SourceSessionCwd,
			RuntimePath:   "/tmp/.tars/commands/메모.md",
		},
	}, session.SessionToolConfig{
		CommandsCustom: true,
	})
	if resolved.Definition != nil {
		t.Fatalf("expected empty command allowlist to disable routing, got %+v", resolved)
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

func TestResolvePromotedCapabilityVersionIDsTracksActiveSkillVersion(t *testing.T) {
	store, err := workstore.Open(context.Background(), filepath.Join(t.TempDir(), "ledger.db"), workstore.Options{})
	if err != nil {
		t.Fatalf("open work ledger: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	work, err := store.CreateWork(context.Background(), workstore.CreateWorkInput{
		WorkspaceID: "workspace-chat", Kind: "capability_review", IdempotencyKey: "review-helper",
		Title: "Review helper", InitialState: workstore.WorkStateReview, ActorID: "tester",
	})
	if err != nil {
		t.Fatalf("create capability work: %v", err)
	}
	createVersion := func(candidateID, digest string) workstore.CapabilityVersion {
		version, createErr := store.CreateCapabilityVersion(context.Background(), workstore.CreateCapabilityVersionInput{
			WorkspaceID: work.WorkspaceID, WorkID: work.ID, CandidateID: candidateID,
			CapabilityName: "review-helper", InitialState: workstore.CapabilityStatePromoted,
			ContentDigest: digest, SnapshotJSON: json.RawMessage(`{"files":[]}`),
			ProvenanceJSON: json.RawMessage(`{"source":"test"}`), PermissionsJSON: json.RawMessage(`[]`),
			ActorID: "tester",
		})
		if createErr != nil {
			t.Fatalf("create promoted capability: %v", createErr)
		}
		return version
	}
	first := createVersion("candidate-v1", "sha256:v1")
	second := createVersion("candidate-v2", "sha256:v2")

	ids, err := resolvePromotedCapabilityVersionIDs(context.Background(), store, "workspace-chat", &skill.Definition{Name: "review-helper"})
	if err != nil {
		t.Fatalf("resolve promoted capability: %v", err)
	}
	if len(ids) != 1 || ids[0] != second.ID {
		t.Fatalf("resolved capability ids=%v want latest %s", ids, second.ID)
	}
	if _, err := store.TransitionCapabilityVersion(context.Background(), workstore.TransitionCapabilityVersionInput{
		WorkspaceID: second.WorkspaceID, VersionID: second.ID,
		ExpectedState: workstore.CapabilityStatePromoted, ToState: workstore.CapabilityStateRolledBack,
		ActorID: "operator", Reason: "regression",
	}); err != nil {
		t.Fatalf("roll back latest capability: %v", err)
	}
	ids, err = resolvePromotedCapabilityVersionIDs(context.Background(), store, "workspace-chat", &skill.Definition{Name: "review-helper"})
	if err != nil {
		t.Fatalf("resolve rollback target capability: %v", err)
	}
	if len(ids) != 1 || ids[0] != first.ID {
		t.Fatalf("resolved capability ids after rollback=%v want %s", ids, first.ID)
	}
}

func newTestSkillManager(t *testing.T, root, workspaceDir string) *extensions.Manager {
	t.Helper()
	skillDir := filepath.Join(root, "skills", "project-start")
	writeSkillFile(t, filepath.Join(skillDir, "SKILL.md"), `---
name: project-start
description: start projects
user-invocable: true
slash: /plan-app
aliases: [project]
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
