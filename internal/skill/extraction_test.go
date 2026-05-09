package skill

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/session"
)

func TestDetectExtractionCandidatesFromSessionMessages(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	sess := session.Session{ID: "sess_skill", Title: "Release workflow"}
	messages := []session.Message{
		{ID: "m1", Role: "user", Content: "When we ship TARS issues, use GitHub Flow: PR, CI, squash merge, release verification.", Timestamp: now},
		{ID: "m2", Role: "assistant", Content: "I will follow the repeatable GitHub release workflow and verify Homebrew.", Timestamp: now.Add(time.Minute)},
		{ID: "m3", Role: "user", Content: "This PR and release verification workflow should become a reusable skill.", Timestamp: now.Add(2 * time.Minute)},
	}

	candidates := DetectExtractionCandidates(sess, messages, ExtractionOptions{Now: now, MaxCandidates: 5})
	if len(candidates) == 0 {
		t.Fatalf("expected at least one skill extraction candidate")
	}
	got := candidates[0]
	if got.Name != "github-release-flow" {
		t.Fatalf("expected github-release-flow candidate, got %+v", got)
	}
	if got.Status != ExtractionCandidateStatusPending || got.SourceSession != sess.ID {
		t.Fatalf("unexpected candidate metadata: %+v", got)
	}
	if got.Provenance.SessionID != sess.ID || got.Provenance.MessageRange != "m1..m3" {
		t.Fatalf("unexpected provenance: %+v", got.Provenance)
	}
	if got.RepeatedCount < 2 || len(got.Evidence) < 2 {
		t.Fatalf("expected repeated evidence, got %+v", got)
	}
}

func TestExtractionInboxAppendListAndReview(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	candidate := ExtractionCandidate{
		Name:          "github-release-flow",
		Title:         "GitHub Release Flow",
		Trigger:       "Use when a session repeats PR and release verification steps.",
		Summary:       "Turn the repeated GitHub PR and release flow into a reusable skill.",
		UseCase:       "run the GitHub PR, CI, squash merge, and release verification flow",
		SourceSession: "sess_skill",
		CreatedAt:     now,
		UpdatedAt:     now,
		Provenance: ExtractionProvenance{
			Source:       "session",
			SessionID:    "sess_skill",
			MessageRange: "m1..m3",
			ExtractedAt:  now,
		},
	}

	added, addedNew, err := AppendExtractionCandidatesIfNew(root, []ExtractionCandidate{candidate})
	if err != nil {
		t.Fatalf("append extraction candidate: %v", err)
	}
	if !addedNew || len(added) != 1 || added[0].ID == "" {
		t.Fatalf("expected one new candidate with id, got added=%v items=%+v", addedNew, added)
	}
	requireExtractionFileMode(t, ExtractionInboxPath(root), 0o600)
	if _, duplicateNew, err := AppendExtractionCandidatesIfNew(root, []ExtractionCandidate{candidate}); err != nil || duplicateNew {
		t.Fatalf("expected duplicate candidate to be skipped, added=%v err=%v", duplicateNew, err)
	}

	pending, err := ListExtractionCandidates(root, ExtractionCandidateListOptions{Status: ExtractionCandidateStatusPending})
	if err != nil {
		t.Fatalf("list extraction candidates: %v", err)
	}
	if len(pending) != 1 || pending[0].Name != "github-release-flow" {
		t.Fatalf("unexpected pending candidates: %+v", pending)
	}

	reviewed, err := ReviewExtractionCandidate(root, pending[0].ID, ExtractionCandidateReview{
		Action:    ExtractionCandidateActionApprove,
		DraftPath: filepath.Join(root, "skills", "github-release-flow"),
		DraftName: "github-release-flow",
	})
	if err != nil {
		t.Fatalf("review extraction candidate: %v", err)
	}
	if reviewed.Status != ExtractionCandidateStatusApproved || reviewed.DraftPath == "" {
		t.Fatalf("expected approved candidate with draft path, got %+v", reviewed)
	}
	if _, err := os.Stat(ExtractionInboxPath(root)); err != nil {
		t.Fatalf("expected extraction inbox file: %v", err)
	}
	requireExtractionFileMode(t, ExtractionInboxPath(root), 0o600)
}

func requireExtractionFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("expected %s mode %04o, got %04o", path, want, got)
	}
}
