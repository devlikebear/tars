package memory

import (
	"context"
	"testing"
	"time"
)

func TestAppendInboxCandidateAndReviewActions(t *testing.T) {
	root := t.TempDir()
	if err := EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	ctx := context.Background()
	backend := NewFileBackend(root, nil)
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)

	candidate, added, err := AppendInboxCandidateIfNew(ctx, root, backend, MemoryCandidate{
		Category:      "preference",
		Summary:       "I prefer concise Korean explanations",
		Tags:          []string{"auto", "user-preference"},
		SourceSession: "sess-1",
		Importance:    6,
		Auto:          true,
		Provenance: MemoryCandidateProvenance{
			Source:        "reflection",
			SessionID:     "sess-1",
			MessageRange:  "messages:0-1",
			SourceSummary: "User stated a response style preference.",
			ExtractedAt:   now,
		},
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("append candidate: %v", err)
	}
	if !added || candidate.ID == "" || candidate.Status != MemoryCandidateStatusPending {
		t.Fatalf("expected pending candidate, got added=%v candidate=%+v", added, candidate)
	}

	duplicate, added, err := AppendInboxCandidateIfNew(ctx, root, backend, candidate)
	if err != nil {
		t.Fatalf("append duplicate: %v", err)
	}
	if added || duplicate.ID != candidate.ID {
		t.Fatalf("expected duplicate to reuse existing candidate, got added=%v duplicate=%+v", added, duplicate)
	}

	reviewed, err := ReviewMemoryCandidate(ctx, root, backend, candidate.ID, MemoryCandidateReview{
		Action: MemoryCandidateActionApprove,
	})
	if err != nil {
		t.Fatalf("approve candidate: %v", err)
	}
	if reviewed.Status != MemoryCandidateStatusApproved {
		t.Fatalf("expected approved candidate, got %+v", reviewed)
	}
	experiences, err := SearchExperiences(root, SearchOptions{Query: "concise Korean", Limit: 5})
	if err != nil {
		t.Fatalf("search experiences: %v", err)
	}
	if len(experiences) != 1 {
		t.Fatalf("expected approved candidate to append one experience, got %+v", experiences)
	}

	candidates, err := ListMemoryCandidates(root, MemoryCandidateListOptions{Status: MemoryCandidateStatusApproved})
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ID != candidate.ID {
		t.Fatalf("expected approved candidate in list, got %+v", candidates)
	}
}

func TestInboxCandidateHintsSimilarAndConflictingExperiences(t *testing.T) {
	root := t.TempDir()
	if err := EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	ctx := context.Background()
	backend := NewFileBackend(root, nil)
	if err := backend.AppendExperience(ctx, Experience{
		Timestamp:     time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC),
		Category:      "preference",
		Summary:       "I prefer concise Korean answers",
		SourceSession: "sess-old",
		Importance:    7,
	}); err != nil {
		t.Fatalf("append existing experience: %v", err)
	}
	if err := backend.AppendExperience(ctx, Experience{
		Timestamp:     time.Date(2026, 5, 1, 8, 5, 0, 0, time.UTC),
		Category:      "preference",
		Summary:       "I prefer detailed English walkthroughs",
		SourceSession: "sess-old",
		Importance:    7,
	}); err != nil {
		t.Fatalf("append conflicting experience: %v", err)
	}

	candidate, added, err := AppendInboxCandidateIfNew(ctx, root, backend, MemoryCandidate{
		Category:      "preference",
		Summary:       "I prefer concise Korean explanations",
		SourceSession: "sess-new",
		Importance:    6,
		CreatedAt:     time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("append candidate: %v", err)
	}
	if !added {
		t.Fatalf("expected new candidate")
	}
	if len(candidate.Similar) == 0 {
		t.Fatalf("expected similar hint, got %+v", candidate)
	}
	if len(candidate.Conflicts) == 0 {
		t.Fatalf("expected conflict hint, got %+v", candidate)
	}

	merged, err := ReviewMemoryCandidate(ctx, root, backend, candidate.ID, MemoryCandidateReview{
		Action:      MemoryCandidateActionMerge,
		MergeTarget: candidate.Similar[0].Summary,
	})
	if err != nil {
		t.Fatalf("merge candidate: %v", err)
	}
	if merged.Status != MemoryCandidateStatusMerged || merged.MergedInto == "" {
		t.Fatalf("expected merged candidate with target, got %+v", merged)
	}
}
