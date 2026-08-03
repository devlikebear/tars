package workstore

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
)

func TestCapabilityLifecycleRequiresIndependentEvidenceAndApproval(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	work := mustCreateWork(t, store, "workspace-a", "capability:review-helper")
	version, err := store.CreateCapabilityVersion(ctx, CreateCapabilityVersionInput{
		WorkspaceID:    work.WorkspaceID,
		WorkID:         work.ID,
		CandidateID:    "skillcand_review_helper",
		CapabilityName: "review-helper",
		ContentDigest:  "sha256:draft-v1",
		SnapshotJSON:   json.RawMessage(`{"files":[{"path":"SKILL.md","content":"draft"}]}`),
		ProvenanceJSON: json.RawMessage(`{"source":"session","session_id":"session-1"}`),
		PermissionsJSON: json.RawMessage(`[
			"tool:bash"
		]`),
		ActorID: "self-improvement",
	})
	if err != nil {
		t.Fatalf("create capability version: %v", err)
	}
	if version.Version != 1 || version.State != CapabilityStateCandidate {
		t.Fatalf("created capability = v%d/%s, want v1/candidate", version.Version, version.State)
	}

	version = mustTransitionCapability(t, store, version, CapabilityStateDraft, "")
	version = mustTransitionCapability(t, store, version, CapabilityStateSandbox, "")
	if _, err := store.TransitionCapabilityVersion(ctx, TransitionCapabilityVersionInput{
		WorkspaceID:   version.WorkspaceID,
		VersionID:     version.ID,
		ExpectedState: CapabilityStateSandbox,
		ToState:       CapabilityStateOfflineEval,
		ActorID:       "self-improvement",
	}); !errors.Is(err, ErrCapabilityGate) {
		t.Fatalf("offline transition without sandbox evidence error = %v, want ErrCapabilityGate", err)
	}

	mustCreateEvaluation(t, store, version, EvaluationStageSandbox, EvaluationStatusPassed, "sandbox:1")
	version = mustTransitionCapability(t, store, version, CapabilityStateOfflineEval, "")
	mustCreateEvaluation(t, store, version, EvaluationStageOffline, EvaluationStatusPassed, "offline:1")
	version = mustTransitionCapability(t, store, version, CapabilityStateShadow, "")
	mustCreateEvaluation(t, store, version, EvaluationStageShadow, EvaluationStatusPassed, "shadow:1")

	if _, err := store.TransitionCapabilityVersion(ctx, TransitionCapabilityVersionInput{
		WorkspaceID:   version.WorkspaceID,
		VersionID:     version.ID,
		ExpectedState: CapabilityStateShadow,
		ToState:       CapabilityStateApproved,
		ActorID:       "operator",
	}); !errors.Is(err, ErrCapabilityGate) {
		t.Fatalf("approval transition without approval record error = %v, want ErrCapabilityGate", err)
	}
	approval, err := store.CreateApproval(ctx, CreateApprovalInput{
		WorkspaceID:    work.WorkspaceID,
		WorkID:         work.ID,
		IdempotencyKey: "capability:approval:v1",
		Authority:      "human",
		Status:         ApprovalStatusApproved,
		Request:        "Approve review-helper v1 for canary",
		Reason:         "reviewed evaluation evidence",
		ActorID:        "operator",
	})
	if err != nil {
		t.Fatalf("create approval: %v", err)
	}
	version = mustTransitionCapability(t, store, version, CapabilityStateApproved, approval.ID)
	version = mustTransitionCapability(t, store, version, CapabilityStateCanary, "")
	if _, err := store.TransitionCapabilityVersion(ctx, TransitionCapabilityVersionInput{
		WorkspaceID:   version.WorkspaceID,
		VersionID:     version.ID,
		ExpectedState: CapabilityStateCanary,
		ToState:       CapabilityStatePromoted,
		ActorID:       "operator",
	}); !errors.Is(err, ErrCapabilityGate) {
		t.Fatalf("promotion without canary evidence error = %v, want ErrCapabilityGate", err)
	}
	mustCreateEvaluation(t, store, version, EvaluationStageCanary, EvaluationStatusPassed, "canary:1")
	version = mustTransitionCapability(t, store, version, CapabilityStatePromoted, "")
	if version.ApprovalID != approval.ID || version.PromotedAt == nil {
		t.Fatalf("promoted capability missing approval/timestamp: %+v", version)
	}
	export, err := store.ExportJSONL(ctx, version.WorkspaceID, filepath.Join(t.TempDir(), "capability.jsonl"))
	if err != nil {
		t.Fatalf("export capability ledger: %v", err)
	}
	if export.RecordCounts["capability_version"] != 1 || export.RecordCounts["evaluation_run"] != 4 {
		t.Fatalf("capability export counts = %+v", export.RecordCounts)
	}
	doctor, err := store.Doctor(ctx, version.WorkspaceID)
	if err != nil {
		t.Fatalf("doctor capability ledger: %v", err)
	}
	if !doctor.Healthy {
		t.Fatalf("capability ledger doctor issues = %+v", doctor.Issues)
	}
}

func TestCapabilityFailedEvaluationCannotAdvanceAndRollbackIsSingleTransition(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	work := mustCreateWork(t, store, "workspace-a", "capability:rollback")
	version, err := store.CreateCapabilityVersion(ctx, CreateCapabilityVersionInput{
		WorkspaceID:     work.WorkspaceID,
		WorkID:          work.ID,
		CandidateID:     "skillcand_rollback",
		CapabilityName:  "rollback-helper",
		ContentDigest:   "sha256:v1",
		SnapshotJSON:    json.RawMessage(`{"files":[]}`),
		ProvenanceJSON:  json.RawMessage(`{"source":"migration"}`),
		PermissionsJSON: json.RawMessage(`[]`),
		ActorID:         "tester",
	})
	if err != nil {
		t.Fatalf("create version: %v", err)
	}
	version = mustTransitionCapability(t, store, version, CapabilityStateDraft, "")
	version = mustTransitionCapability(t, store, version, CapabilityStateSandbox, "")
	mustCreateEvaluation(t, store, version, EvaluationStageSandbox, EvaluationStatusFailed, "sandbox:failed")
	if _, err := store.TransitionCapabilityVersion(ctx, TransitionCapabilityVersionInput{
		WorkspaceID:   version.WorkspaceID,
		VersionID:     version.ID,
		ExpectedState: CapabilityStateSandbox,
		ToState:       CapabilityStateOfflineEval,
		ActorID:       "tester",
	}); !errors.Is(err, ErrCapabilityGate) {
		t.Fatalf("advance after failed smoke error = %v, want ErrCapabilityGate", err)
	}

	imported, err := store.CreateCapabilityVersion(ctx, CreateCapabilityVersionInput{
		WorkspaceID:      work.WorkspaceID,
		WorkID:           work.ID,
		CandidateID:      "skillcand_promoted_legacy",
		CapabilityName:   "rollback-helper",
		InitialState:     CapabilityStatePromoted,
		ContentDigest:    "sha256:v2",
		SnapshotJSON:     json.RawMessage(`{"files":[]}`),
		ProvenanceJSON:   json.RawMessage(`{"source":"legacy_skill_inbox"}`),
		PermissionsJSON:  json.RawMessage(`[]`),
		RollbackTargetID: version.ID,
		ActorID:          "migration",
	})
	if err != nil {
		t.Fatalf("import promoted version: %v", err)
	}
	rolledBack, err := store.TransitionCapabilityVersion(ctx, TransitionCapabilityVersionInput{
		WorkspaceID:   imported.WorkspaceID,
		VersionID:     imported.ID,
		ExpectedState: CapabilityStatePromoted,
		ToState:       CapabilityStateRolledBack,
		ActorID:       "operator",
		Reason:        "canary regression",
	})
	if err != nil {
		t.Fatalf("rollback promoted version: %v", err)
	}
	if rolledBack.RolledBackAt == nil || rolledBack.State != CapabilityStateRolledBack {
		t.Fatalf("rolled back capability = %+v", rolledBack)
	}
}

func TestFailedPromotedCapabilityOutcomeFlagsReviewAndRegressionEvent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	capabilityWork := mustCreateWork(t, store, "workspace-a", "capability:regression")
	version, err := store.CreateCapabilityVersion(ctx, CreateCapabilityVersionInput{
		WorkspaceID: capabilityWork.WorkspaceID, WorkID: capabilityWork.ID,
		CandidateID: "skillcand_regression", CapabilityName: "regression-helper",
		InitialState: CapabilityStatePromoted, ContentDigest: "sha256:regression",
		SnapshotJSON: json.RawMessage(`{"files":[]}`), ProvenanceJSON: json.RawMessage(`{"legacy_status":"approved"}`),
		PermissionsJSON: json.RawMessage(`[]`), RolloutJSON: json.RawMessage(`{"mode":"full","percent":100}`), ActorID: "migration",
	})
	if err != nil {
		t.Fatalf("create promoted capability: %v", err)
	}
	laterWork := mustCreateWork(t, store, "workspace-a", "capability:regressed-work")
	_, err = store.RecordCapabilityOutcome(ctx, RecordCapabilityOutcomeInput{
		WorkspaceID: laterWork.WorkspaceID, CapabilityVersionID: version.ID, WorkID: laterWork.ID,
		IdempotencyKey: "outcome:regression", Status: CapabilityOutcomeFailed,
		VerifierStatus: ProofStatusFailed, LatencyMS: 500, ActorID: "scheduler",
	})
	if err != nil {
		t.Fatalf("record regressed outcome: %v", err)
	}
	flagged, err := store.GetCapabilityVersion(ctx, version.WorkspaceID, version.ID)
	if err != nil {
		t.Fatalf("get flagged capability: %v", err)
	}
	var rollout map[string]any
	if err := json.Unmarshal(flagged.RolloutJSON, &rollout); err != nil {
		t.Fatalf("decode rollout: %v", err)
	}
	if rollout["review_required"] != true || rollout["regression_detected"] != true {
		t.Fatalf("regression rollout flags = %+v", rollout)
	}
	events, err := store.ListEvents(ctx, capabilityWork.WorkspaceID, capabilityWork.ID)
	if err != nil {
		t.Fatalf("list capability events: %v", err)
	}
	found := false
	for _, event := range events {
		if event.Type == EventTypeCapabilityRegressionDetected {
			found = true
		}
	}
	if !found {
		t.Fatalf("capability events = %+v, missing regression detection", events)
	}
}

func TestCapabilityVersionsAreIdempotentVersionedAndLinkLaterOutcomes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	work := mustCreateWork(t, store, "workspace-a", "capability:versioning")
	input := CreateCapabilityVersionInput{
		WorkspaceID:     work.WorkspaceID,
		WorkID:          work.ID,
		CandidateID:     "skillcand_v1",
		CapabilityName:  "versioned-helper",
		ContentDigest:   "sha256:v1",
		SnapshotJSON:    json.RawMessage(`{"files":[]}`),
		ProvenanceJSON:  json.RawMessage(`{"source":"session"}`),
		PermissionsJSON: json.RawMessage(`[]`),
		ActorID:         "tester",
	}
	v1, err := store.CreateCapabilityVersion(ctx, input)
	if err != nil {
		t.Fatalf("create v1: %v", err)
	}
	replayed, err := store.CreateCapabilityVersion(ctx, input)
	if err != nil {
		t.Fatalf("replay v1: %v", err)
	}
	if replayed.ID != v1.ID || replayed.Version != 1 {
		t.Fatalf("replayed version = %s/v%d, want %s/v1", replayed.ID, replayed.Version, v1.ID)
	}
	input.CandidateID = "skillcand_v2"
	input.ContentDigest = "sha256:v2"
	v2, err := store.CreateCapabilityVersion(ctx, input)
	if err != nil {
		t.Fatalf("create v2: %v", err)
	}
	if v2.Version != 2 || v2.PreviousVersionID != v1.ID {
		t.Fatalf("v2 = %+v, want version 2 with previous %s", v2, v1.ID)
	}

	laterWork := mustCreateWork(t, store, "workspace-a", "later-work")
	outcome, err := store.RecordCapabilityOutcome(ctx, RecordCapabilityOutcomeInput{
		WorkspaceID:         laterWork.WorkspaceID,
		CapabilityVersionID: v2.ID,
		WorkID:              laterWork.ID,
		IdempotencyKey:      "outcome:later-work",
		Status:              CapabilityOutcomeSucceeded,
		VerifierStatus:      ProofStatusPassed,
		CostUSD:             0.04,
		LatencyMS:           1250,
		ActorID:             "scheduler",
	})
	if err != nil {
		t.Fatalf("record outcome: %v", err)
	}
	outcomes, err := store.ListCapabilityOutcomes(ctx, laterWork.WorkspaceID, v2.ID)
	if err != nil {
		t.Fatalf("list outcomes: %v", err)
	}
	if len(outcomes) != 1 || outcomes[0].ID != outcome.ID || outcomes[0].WorkID != laterWork.ID {
		t.Fatalf("outcomes = %+v, want linked later work", outcomes)
	}
	capabilityProjection, err := store.GetWorkProjection(ctx, work.WorkspaceID, work.ID)
	if err != nil {
		t.Fatalf("get capability work projection: %v", err)
	}
	if len(capabilityProjection.CapabilityVersions) != 2 {
		t.Fatalf("capability projection versions = %+v", capabilityProjection.CapabilityVersions)
	}
	outcomeProjection, err := store.GetWorkProjection(ctx, laterWork.WorkspaceID, laterWork.ID)
	if err != nil {
		t.Fatalf("get outcome work projection: %v", err)
	}
	if len(outcomeProjection.CapabilityOutcomes) != 1 || outcomeProjection.CapabilityOutcomes[0].CapabilityVersionID != v2.ID {
		t.Fatalf("outcome projection = %+v", outcomeProjection.CapabilityOutcomes)
	}
}

func mustTransitionCapability(t *testing.T, store *Store, current CapabilityVersion, to CapabilityState, approvalID string) CapabilityVersion {
	t.Helper()
	updated, err := store.TransitionCapabilityVersion(context.Background(), TransitionCapabilityVersionInput{
		WorkspaceID:   current.WorkspaceID,
		VersionID:     current.ID,
		ExpectedState: current.State,
		ToState:       to,
		ApprovalID:    approvalID,
		ActorID:       "tester",
	})
	if err != nil {
		t.Fatalf("transition %s to %s: %v", current.State, to, err)
	}
	return updated
}

func mustCreateEvaluation(t *testing.T, store *Store, version CapabilityVersion, stage EvaluationStage, status EvaluationStatus, key string) EvaluationRun {
	t.Helper()
	run, err := store.CreateEvaluationRun(context.Background(), CreateEvaluationRunInput{
		WorkspaceID:         version.WorkspaceID,
		WorkID:              version.WorkID,
		CapabilityVersionID: version.ID,
		IdempotencyKey:      key,
		Stage:               stage,
		Status:              status,
		MetricsJSON:         json.RawMessage(`{"success_rate":1,"verification_rate":1,"cost_usd":0,"latency_ms":10}`),
		DeltaJSON:           json.RawMessage(`{"success_rate":0}`),
		ReportJSON:          json.RawMessage(`{"summary":"test"}`),
		ActorID:             "independent-evaluator",
	})
	if err != nil {
		t.Fatalf("create %s evaluation: %v", stage, err)
	}
	return run
}
