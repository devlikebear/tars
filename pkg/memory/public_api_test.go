package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/devlikebear/tars/pkg/memory"
)

func TestFileBackedMemoryThroughPublicPackage(t *testing.T) {
	root := t.TempDir()
	backend := memory.NewFileBackend(root, nil)
	at := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)

	if err := backend.AppendMemoryNote(context.Background(), at, "project prefers small pkg surfaces"); err != nil {
		t.Fatalf("AppendMemoryNote() error = %v", err)
	}
	notes, err := memory.ListMemoryNotesByPrefix(root, "project prefers", 10)
	if err != nil {
		t.Fatalf("ListMemoryNotesByPrefix() error = %v", err)
	}
	if len(notes) != 1 || notes[0].Text != "project prefers small pkg surfaces" {
		t.Fatalf("notes = %+v", notes)
	}

	err = backend.AppendExperience(context.Background(), memory.Experience{
		Timestamp: at,
		Category:  "design",
		Summary:   "Public package surfaces should stay additive.",
		Tags:      []string{"pkg"},
	})
	if err != nil {
		t.Fatalf("AppendExperience() error = %v", err)
	}
	experiences, err := backend.SearchExperiences(context.Background(), memory.SearchOptions{Query: "additive", Limit: 5})
	if err != nil {
		t.Fatalf("SearchExperiences() error = %v", err)
	}
	if len(experiences) != 1 || experiences[0].Category != "design" {
		t.Fatalf("experiences = %+v", experiences)
	}
}

func TestPublicMemoryHelpers(t *testing.T) {
	root := t.TempDir()
	if got := memory.NormalizeEmbedProvider(" Gemini "); got != "gemini" {
		t.Fatalf("NormalizeEmbedProvider() = %q", got)
	}
	if len(memory.SupportedEmbedProviders()) == 0 {
		t.Fatalf("SupportedEmbedProviders() is empty")
	}
	if !memory.IsSupportedEmbedProvider("gemini") {
		t.Fatalf("IsSupportedEmbedProvider() = false")
	}
	if err := memory.ValidateSemanticConfig(memory.SemanticConfig{}); err != nil {
		t.Fatalf("ValidateSemanticConfig(disabled) error = %v", err)
	}
	service := memory.NewService(root, memory.ServiceOptions{Config: memory.SemanticConfig{}})
	if service == nil {
		t.Fatalf("NewService() = nil")
	}
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("EnsureWorkspace() error = %v", err)
	}
	if len(memory.WorkspaceBootstrapFileSpecs()) == 0 {
		t.Fatalf("WorkspaceBootstrapFileSpecs() is empty")
	}
	if _, ok := memory.WorkspaceBootstrapFileSpecFor("USER.md"); !ok {
		t.Fatalf("WorkspaceBootstrapFileSpecFor(USER.md) = false")
	}
	at := time.Date(2026, 5, 21, 13, 0, 0, 0, time.UTC)
	if err := memory.AppendDailyLog(root, at, "daily entry"); err != nil {
		t.Fatalf("AppendDailyLog() error = %v", err)
	}
	if err := memory.AppendMemoryNote(root, at, "note from helper"); err != nil {
		t.Fatalf("AppendMemoryNote() error = %v", err)
	}
	if err := memory.AppendExperience(root, memory.Experience{Timestamp: at, Summary: "helper experience"}); err != nil {
		t.Fatalf("AppendExperience() error = %v", err)
	}
	found, err := memory.SearchExperiences(root, memory.SearchOptions{Query: "helper"})
	if err != nil {
		t.Fatalf("SearchExperiences() error = %v", err)
	}
	if len(found) == 0 {
		t.Fatalf("SearchExperiences() returned no items")
	}

	backend := memory.NewFileBackend(root, nil)
	candidate, inserted, err := memory.AppendInboxCandidateIfNew(context.Background(), root, backend, memory.MemoryCandidate{
		Category: "design",
		Summary:  "Keep public package boundaries small.",
	})
	if err != nil {
		t.Fatalf("AppendInboxCandidateIfNew() error = %v", err)
	}
	if !inserted || candidate.ID == "" {
		t.Fatalf("candidate = %+v inserted=%v", candidate, inserted)
	}
	candidates, err := memory.ListMemoryCandidates(root, memory.MemoryCandidateListOptions{Status: memory.MemoryCandidateStatusPending})
	if err != nil {
		t.Fatalf("ListMemoryCandidates() error = %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("len(candidates) = %d, want 1", len(candidates))
	}
	reviewed, err := memory.ReviewMemoryCandidate(context.Background(), root, backend, candidate.ID, memory.MemoryCandidateReview{Action: memory.MemoryCandidateActionApprove})
	if err != nil {
		t.Fatalf("ReviewMemoryCandidate() error = %v", err)
	}
	if reviewed.Status != memory.MemoryCandidateStatusApproved {
		t.Fatalf("reviewed.Status = %q", reviewed.Status)
	}
}
