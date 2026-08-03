package tarsserver

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/skill"
	"github.com/devlikebear/tars/internal/workstore"
)

func TestSkillCapabilityLifecycleRejectsCandidateDurably(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	ledger := openSkillLifecycleTestLedger(t, workspace)
	lifecycle := newSkillCapabilityLifecycle(workspace, ledger)
	createdAt := time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC)
	candidate := skill.ExtractionCandidate{
		ID: "skillcand_reject", Status: skill.ExtractionCandidateStatusPending,
		Name: "unsafe-helper", Title: "Unsafe helper", Summary: "Candidate requires operator rejection.",
		UseCase: "reject unsafe work", SourceSession: "session-reject", CreatedAt: createdAt, UpdatedAt: createdAt,
		Provenance: skill.ExtractionProvenance{Source: "session", SessionID: "session-reject", ExtractedAt: createdAt},
	}
	if _, _, err := skill.AppendExtractionCandidatesIfNew(workspace, []skill.ExtractionCandidate{candidate}); err != nil {
		t.Fatalf("seed candidate: %v", err)
	}
	versions, err := lifecycle.SyncCandidates(context.Background(), []skill.ExtractionCandidate{
		{ID: "later", Name: "later", Title: "Later", Summary: "Later", CreatedAt: createdAt.Add(time.Minute), UpdatedAt: createdAt.Add(time.Minute)},
		candidate,
	})
	if err != nil || len(versions) != 2 || versions[0].CandidateID != candidate.ID {
		t.Fatalf("sorted candidate sync versions=%+v err=%v", versions, err)
	}

	result, err := lifecycle.Apply(context.Background(), candidate.ID, skill.ExtractionCandidateActionReject)
	if err != nil {
		t.Fatalf("reject candidate: %v", err)
	}
	if result.Capability.State != workstore.CapabilityStateRejected || result.Candidate.Status != skill.ExtractionCandidateStatusRejected {
		t.Fatalf("rejected result=%+v", result)
	}
	work, err := ledger.GetWork(context.Background(), result.Capability.WorkspaceID, result.Capability.WorkID)
	if err != nil || work.State != workstore.WorkStateCancelled {
		t.Fatalf("rejected work=%+v err=%v", work, err)
	}
	result, err = lifecycle.Apply(context.Background(), candidate.ID, skill.ExtractionCandidateActionReject)
	if err != nil || result.Capability.State != workstore.CapabilityStateRejected {
		t.Fatalf("idempotent reject result=%+v err=%v", result, err)
	}
	if _, err := lifecycle.Apply(context.Background(), candidate.ID, skill.ExtractionCandidateAction("erase")); err == nil || !strings.Contains(err.Error(), "unknown skill extraction action") {
		t.Fatalf("unknown action error=%v", err)
	}

	var nilLifecycle *skillCapabilityLifecycle
	if got := newSkillCapabilityLifecycle(workspace, nil); got != nil {
		t.Fatal("nil ledger created a lifecycle")
	}
	if _, err := nilLifecycle.SyncCandidates(context.Background(), nil); err == nil {
		t.Fatal("nil lifecycle synchronized candidates")
	}
	if _, err := nilLifecycle.Apply(context.Background(), candidate.ID, skill.ExtractionCandidateActionReject); err == nil {
		t.Fatal("nil lifecycle applied an action")
	}
}

func TestLegacyCapabilityDraftImportIsBoundedAndContained(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	draftPath := filepath.Join(workspace, "skills", "legacy-helper")
	if err := os.MkdirAll(filepath.Join(draftPath, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(draftPath, "SKILL.md"), []byte("---\nname: legacy-helper\ndescription: Legacy helper\n---\n\nRun safely.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(draftPath, "scripts", "run.sh"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	candidate := skill.ExtractionCandidate{
		Name: "legacy-helper", DraftName: "legacy-helper", DraftPath: draftPath,
		Summary: "Imported helper", UseCase: "run imported helper", RecommendedTools: []string{"Bash"},
	}
	draft, err := loadLegacyCapabilityDraft(workspace, candidate)
	if err != nil {
		t.Fatalf("load contained legacy draft: %v", err)
	}
	if draft.Name != "legacy-helper" || draft.Category != "legacy" || len(draft.Files) != 2 {
		t.Fatalf("legacy draft=%+v", draft)
	}

	outside := t.TempDir()
	if _, err := loadLegacyCapabilityDraft(workspace, skill.ExtractionCandidate{DraftPath: outside}); err == nil || !strings.Contains(err.Error(), "outside workspace skills") {
		t.Fatalf("outside legacy path error=%v", err)
	}
	if _, err := loadLegacyCapabilityDraft(workspace, skill.ExtractionCandidate{DraftPath: filepath.Join(workspace, "skills")}); err == nil {
		t.Fatal("skills root was accepted as a legacy draft")
	}

	symlinkPath := filepath.Join(workspace, "skills", "symlink-helper")
	if err := os.MkdirAll(symlinkPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(draftPath, "SKILL.md"), filepath.Join(symlinkPath, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := loadLegacyCapabilityDraft(workspace, skill.ExtractionCandidate{Name: "symlink-helper", DraftPath: symlinkPath}); err == nil || !strings.Contains(err.Error(), "contains a symlink") {
		t.Fatalf("symlink legacy path error=%v", err)
	}

	largePath := filepath.Join(workspace, "skills", "large-helper")
	if err := os.MkdirAll(largePath, 0o755); err != nil {
		t.Fatal(err)
	}
	large := make([]byte, 1024*1024+1)
	if err := os.WriteFile(filepath.Join(largePath, "SKILL.md"), large, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadLegacyCapabilityDraft(workspace, skill.ExtractionCandidate{Name: "large-helper", DraftPath: largePath}); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("oversized legacy file error=%v", err)
	}
}

func TestCapabilityDraftValidationAndAtomicActivationEdges(t *testing.T) {
	t.Parallel()

	valid := skillCreatorDraftResponse{
		Name: "atomic-helper", Description: "Atomic helper", RecommendedTools: []string{" Bash ", "bash", "Memory_Search", ""},
		Files: []skillCreatorFile{{Path: "SKILL.md", Content: "---\nname: atomic-helper\ndescription: Atomic helper\n---\n\nSafe body.\n"}},
	}
	if got := capabilityPermissions(valid); len(got) != 2 || got[0] != "tool:bash" || got[1] != "tool:memory_search" {
		t.Fatalf("capability permissions=%v", got)
	}
	if err := validateCapabilityDraftOffline(valid); err != nil {
		t.Fatalf("valid offline draft: %v", err)
	}
	missing := valid
	missing.Files = []skillCreatorFile{{Path: "README.md", Content: "missing"}}
	if err := validateCapabilityDraftOffline(missing); err == nil || !strings.Contains(err.Error(), "SKILL.md is required") {
		t.Fatalf("missing SKILL.md error=%v", err)
	}
	mismatch := valid
	mismatch.Files = []skillCreatorFile{{Path: "SKILL.md", Content: "---\nname: other\ndescription: Other\n---\n"}}
	if err := validateCapabilityDraftOffline(mismatch); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mismatched name error=%v", err)
	}
	malformed := valid
	malformed.Files = []skillCreatorFile{{Path: "SKILL.md", Content: "---\nname: atomic-helper\n"}}
	if err := validateCapabilityDraftOffline(malformed); err == nil || !strings.Contains(err.Error(), "parse SKILL.md") {
		t.Fatalf("malformed frontmatter error=%v", err)
	}
	if _, err := decodeCapabilityDraft(json.RawMessage(`{`)); err == nil {
		t.Fatal("invalid capability snapshot was decoded")
	}
	invalidSnapshot, _ := json.Marshal(missing)
	if _, err := decodeCapabilityDraft(invalidSnapshot); err == nil {
		t.Fatal("snapshot without SKILL.md was decoded")
	}

	workspace := t.TempDir()
	target := filepath.Join(workspace, "skills", valid.Name)
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	old := []byte("old known-good content")
	if err := os.WriteFile(filepath.Join(target, "SKILL.md"), old, 0o600); err != nil {
		t.Fatal(err)
	}
	commitErr := errors.New("ledger commit failed")
	if err := replaceCapabilitySkill(workspace, valid, func() error { return commitErr }); !errors.Is(err, commitErr) {
		t.Fatalf("replace commit error=%v", err)
	}
	restored, err := os.ReadFile(filepath.Join(target, "SKILL.md"))
	if err != nil || string(restored) != string(old) {
		t.Fatalf("failed activation did not restore known-good content=%q err=%v", restored, err)
	}
	if err := replaceCapabilitySkill(workspace, valid, func() error { return nil }); err != nil {
		t.Fatalf("activate capability: %v", err)
	}
	active, err := os.ReadFile(filepath.Join(target, "SKILL.md"))
	if err != nil || !strings.Contains(string(active), "Safe body") {
		t.Fatalf("active capability content=%q err=%v", active, err)
	}
	if err := removeCapabilitySkill(workspace, valid.Name, func() error { return commitErr }); !errors.Is(err, commitErr) {
		t.Fatalf("remove commit error=%v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("failed removal did not restore capability: %v", err)
	}
	if err := removeCapabilitySkill(workspace, "../escape", func() error { return nil }); err == nil {
		t.Fatal("unsafe capability name was removed")
	}
	if err := removeCapabilitySkill(workspace, valid.Name, func() error { return nil }); err != nil {
		t.Fatalf("remove capability: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("removed capability still exists: %v", err)
	}
}

func TestSavedCapabilityResponseIsOnlyExposedAfterPromotion(t *testing.T) {
	t.Parallel()

	draft := skillCreatorDraftResponse{
		Name:  "saved-helper",
		Files: []skillCreatorFile{{Path: "scripts/run.sh", Content: "exit 0"}, {Path: "SKILL.md", Content: "---\nname: saved-helper\ndescription: Saved\n---\n"}},
	}
	snapshot, err := json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}
	version := workstore.CapabilityVersion{State: workstore.CapabilityStatePromoted, CapabilityName: draft.Name, SnapshotJSON: snapshot}
	if got := savedCapabilityResponse("/workspace", version, skill.ExtractionCandidateActionApprove); got.Saved {
		t.Fatalf("non-promotion action exposed saved response: %+v", got)
	}
	version.State = workstore.CapabilityStateCanary
	if got := savedCapabilityResponse("/workspace", version, skill.ExtractionCandidateActionPromote); got.Saved {
		t.Fatalf("canary exposed saved response: %+v", got)
	}
	version.State = workstore.CapabilityStatePromoted
	got := savedCapabilityResponse("/workspace", version, skill.ExtractionCandidateActionPromote)
	if !got.Saved || got.Path != filepath.Join("/workspace", "skills", draft.Name) || len(got.Files) != 2 || got.Files[0] != "SKILL.md" {
		t.Fatalf("saved response=%+v", got)
	}
	version.SnapshotJSON = json.RawMessage(`{`)
	if got := savedCapabilityResponse("/workspace", version, skill.ExtractionCandidateActionPromote); got.Saved {
		t.Fatalf("invalid snapshot exposed saved response: %+v", got)
	}
}
