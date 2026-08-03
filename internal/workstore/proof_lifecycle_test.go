package workstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestProofMigrationDowngradesUnverifiedLegacyPasses(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ledger.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, checksum TEXT NOT NULL, applied_at INTEGER NOT NULL)`); err != nil {
		t.Fatalf("create legacy migration table: %v", err)
	}
	for _, migration := range schemaMigrations[:5] {
		if _, err := db.ExecContext(ctx, migration.sql); err != nil {
			t.Fatalf("apply legacy migration %d: %v", migration.version, err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO schema_migrations (version, checksum, applied_at) VALUES (?, ?, 0)`, migration.version, checksumText(migration.sql)); err != nil {
			t.Fatalf("record legacy migration %d: %v", migration.version, err)
		}
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO works (schema_version, id, workspace_id, kind, idempotency_key, title, state, actor_id, version, created_at, updated_at)
		VALUES (1, 'wrk_legacy', 'workspace-proof', 'task', 'legacy', 'Legacy', 'done', 'worker-1', 1, 1, 1)
	`); err != nil {
		t.Fatalf("insert legacy work: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO proofs (schema_version, id, workspace_id, work_id, idempotency_key, kind, status, summary, verifier, actor_id, created_at)
		VALUES (1, 'prf_legacy', 'workspace-proof', 'wrk_legacy', 'legacy-proof', 'test', 'passed', 'worker said tests passed', 'go-test', 'worker-1', 1)
	`); err != nil {
		t.Fatalf("insert legacy proof: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	store, err := Open(ctx, path, Options{})
	if err != nil {
		t.Fatalf("migrate legacy database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	proof, err := store.GetProof(ctx, "workspace-proof", "wrk_legacy", "prf_legacy")
	if err != nil {
		t.Fatalf("get migrated proof: %v", err)
	}
	if proof.Status != ProofStatusReported || proof.Origin != ProofOriginLegacy || proof.VerifierID != "go-test" {
		t.Fatalf("migrated proof = %+v", proof)
	}
}

func TestProofLifecyclePersistsIndependentVerifierProvenance(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	work := mustCreateWork(t, store, "workspace-proof", "proof-lifecycle")

	reported, err := store.CreateProof(ctx, CreateProofInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID,
		IdempotencyKey: "proof:worker-report", Kind: "worker-result",
		Status: ProofStatusReported, Origin: ProofOriginWorkerReport,
		Summary: "worker reports success", ReporterID: "worker-1",
		InputJSON:     json.RawMessage(`{"output_digest":"sha256:output"}`),
		SubjectDigest: "sha256:before", ActorID: "worker-1",
	})
	if err != nil {
		t.Fatalf("create reported proof: %v", err)
	}
	if reported.Status != ProofStatusReported || reported.Origin != ProofOriginWorkerReport || reported.ReporterID != "worker-1" {
		t.Fatalf("reported proof provenance = %+v", reported)
	}

	pending, err := store.CreateProof(ctx, CreateProofInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID,
		IdempotencyKey: "proof:independent", Kind: "command",
		Status: ProofStatusPending, Origin: ProofOriginIndependentVerifier,
		Summary: "go package verification", ReporterID: "worker-1",
		VerifierID: "verifier-1", Verifier: "deterministic-command",
		Command:             "go test ./internal/workstore",
		EnvironmentJSON:     json.RawMessage(`{"runner":"isolated","go":"1.25"}`),
		InputJSON:           json.RawMessage(`{"paths":["internal/workstore"]}`),
		ArtifactDigestsJSON: json.RawMessage(`["sha256:artifact"]`),
		SubjectDigest:       "sha256:before", ActorID: "scheduler",
	})
	if err != nil {
		t.Fatalf("create pending proof: %v", err)
	}
	if pending.Status != ProofStatusPending || pending.VerifierID != "verifier-1" || pending.UpdatedAt.IsZero() {
		t.Fatalf("pending proof provenance = %+v", pending)
	}

	observedAt := time.Date(2026, time.August, 2, 12, 1, 0, 0, time.UTC)
	passed, err := store.TransitionProof(ctx, TransitionProofInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, ProofID: pending.ID,
		ExpectedStatus: ProofStatusPending, ToStatus: ProofStatusPassed,
		ActorID: "verifier-1", Rationale: "command exited with code 0",
		SubjectDigest: "sha256:before", ObservedAt: &observedAt,
	})
	if err != nil {
		t.Fatalf("pass independent proof: %v", err)
	}
	if passed.Status != ProofStatusPassed || passed.Rationale != "command exited with code 0" || passed.ObservedAt == nil || !passed.ObservedAt.Equal(observedAt) {
		t.Fatalf("passed proof = %+v", passed)
	}
	if _, err := store.TransitionProof(ctx, TransitionProofInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, ProofID: passed.ID,
		ExpectedStatus: ProofStatusPassed, ToStatus: ProofStatusFailed,
		ActorID: "verifier-1", Rationale: "rewrite history",
	}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("passed to failed transition error = %v, want ErrInvalidTransition", err)
	}

	stale, changed, err := store.DetectStaleProof(ctx, DetectStaleProofInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, ProofID: passed.ID,
		CurrentSubjectDigest: "sha256:after", ActorID: "verifier-1",
		Rationale: "verified files changed",
	})
	if err != nil {
		t.Fatalf("detect stale proof: %v", err)
	}
	if !changed || stale.Status != ProofStatusStale || stale.Rationale != "verified files changed" {
		t.Fatalf("stale proof = %+v changed=%v", stale, changed)
	}

	projection, err := store.GetWorkProjection(ctx, work.WorkspaceID, work.ID)
	if err != nil {
		t.Fatalf("get proof projection: %v", err)
	}
	if len(projection.Proofs) != 2 || projection.Proofs[1].EnvironmentJSON == nil || projection.Proofs[1].InputJSON == nil {
		t.Fatalf("proof projection = %+v", projection.Proofs)
	}
	seenTransition := false
	for _, event := range projection.Events {
		seenTransition = seenTransition || event.Type == EventTypeProofTransitioned
	}
	if !seenTransition {
		t.Fatal("proof lifecycle did not emit proof.transitioned")
	}
}

func TestDetectStaleProofKeepsMatchingSubjectPassed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	work := mustCreateWork(t, store, "workspace-proof", "proof-fresh")
	proof, err := store.CreateProof(ctx, CreateProofInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID,
		IdempotencyKey: "proof:fresh", Kind: "artifact",
		Status: ProofStatusPassed, Origin: ProofOriginIndependentVerifier,
		Summary: "artifact verified", ReporterID: "worker-1",
		VerifierID: "verifier-1", Verifier: "artifact-digest",
		EnvironmentJSON: json.RawMessage(`{"runner":"isolated"}`),
		SubjectDigest:   "sha256:same", Rationale: "digest matched", ActorID: "verifier-1",
	})
	if err != nil {
		t.Fatalf("create passed proof: %v", err)
	}
	fresh, changed, err := store.DetectStaleProof(ctx, DetectStaleProofInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, ProofID: proof.ID,
		CurrentSubjectDigest: "sha256:same", ActorID: "verifier-1",
	})
	if err != nil || changed || fresh.Status != ProofStatusPassed {
		t.Fatalf("fresh proof = %+v changed=%v err=%v", fresh, changed, err)
	}
}
