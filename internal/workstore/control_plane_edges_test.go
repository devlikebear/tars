package workstore

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestSchedulerListsAndExplicitlyReclaimsActiveClaim(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	work := mustCreateWork(t, store, "workspace-active-claims", "active-claims")
	step := mustCreateScheduledStep(t, store, work, "claim", 1)
	if _, err := store.ConfigureStepSchedule(ctx, ConfigureStepScheduleInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: step.ID,
		Policy: StepSchedulePolicy{MaxAttempts: 2, RetryLimit: 1}, ActorID: "scheduler",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PromoteReadySteps(ctx, PromoteReadyStepsInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, ActorID: "scheduler",
	}); err != nil {
		t.Fatal(err)
	}
	claim := mustClaimStep(t, store, work, "worker-explicit", time.Minute)

	for _, workID := range []string{"", work.ID} {
		claims, err := store.ListActiveStepClaims(ctx, work.WorkspaceID, workID)
		if err != nil {
			t.Fatalf("list active claims for %q: %v", workID, err)
		}
		if len(claims) != 1 || claims[0].Attempt.ID != claim.Attempt.ID || claims[0].Schedule.LeaseOwner != "worker-explicit" {
			t.Fatalf("active claims for %q = %+v", workID, claims)
		}
	}
	if _, err := store.ListActiveStepClaims(ctx, "", work.ID); err == nil {
		t.Fatal("empty workspace active-claim lookup succeeded")
	}

	resolution, err := store.ReclaimStepClaim(ctx, ReclaimStepClaimInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: step.ID,
		AttemptID: claim.Attempt.ID, WorkerID: "worker-explicit", ActorID: "operator",
		Reason: "operator confirmed worker loss",
	})
	if err != nil {
		t.Fatalf("explicit reclaim: %v", err)
	}
	if resolution.Disposition != StepDispositionRetry || resolution.Step.State != WorkStateReady || resolution.Attempt.Status != AttemptStatusFailed {
		t.Fatalf("explicit reclaim = %+v", resolution)
	}
	claims, err := store.ListActiveStepClaims(ctx, work.WorkspaceID, work.ID)
	if err != nil || len(claims) != 0 {
		t.Fatalf("claims after reclaim = %+v err=%v", claims, err)
	}
	if _, err := store.ReclaimStepClaim(ctx, ReclaimStepClaimInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: step.ID,
		AttemptID: claim.Attempt.ID, WorkerID: "other-worker", ActorID: "operator", Reason: "stale reclaim",
	}); !errors.Is(err, ErrClaimConflict) {
		t.Fatalf("stale explicit reclaim error = %v", err)
	}

	cancelled, err := store.CancelScheduledStep(ctx, CancelScheduledStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: step.ID,
		ActorID: "operator", Reason: "no longer needed",
	})
	if err != nil || cancelled.Step.State != WorkStateCancelled {
		t.Fatalf("cancel ready step = %+v err=%v", cancelled, err)
	}
	replayed, err := store.CancelScheduledStep(ctx, CancelScheduledStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: step.ID,
		ActorID: "operator", Reason: "idempotent cancel",
	})
	if err != nil || replayed.Step.State != WorkStateCancelled {
		t.Fatalf("replay cancel = %+v err=%v", replayed, err)
	}
	if _, err := store.ResumeScheduledStep(ctx, ResumeScheduledStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: step.ID,
		ActorID: "operator", Reason: "cannot resume cancelled",
	}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("resume cancelled error = %v", err)
	}
}

func TestCapabilityQueriesFilterVersionedEvaluationAndOutcomeRecords(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	work := mustCreateWork(t, store, "workspace-capability-query", "capability-query")
	base := CreateCapabilityVersionInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, CandidateID: "candidate-query-v1",
		CapabilityName: "query-helper", ContentDigest: "sha256:query-v1",
		SnapshotJSON: json.RawMessage(`{"files":[]}`), ProvenanceJSON: json.RawMessage(`{"source":"test"}`),
		PermissionsJSON: json.RawMessage(`[]`), ActorID: "tester",
	}
	v1, err := store.CreateCapabilityVersion(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	base.CandidateID = "candidate-query-v2"
	base.ContentDigest = "sha256:query-v2"
	v2, err := store.CreateCapabilityVersion(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	if found, err := store.GetCapabilityVersionByCandidate(ctx, work.WorkspaceID, v2.CandidateID); err != nil || found.ID != v2.ID {
		t.Fatalf("get by candidate = %+v err=%v", found, err)
	}
	if _, err := store.GetCapabilityVersionByCandidate(ctx, work.WorkspaceID, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing candidate error = %v", err)
	}

	queries := []ListCapabilityVersionsFilter{
		{WorkspaceID: work.WorkspaceID},
		{WorkspaceID: work.WorkspaceID, CapabilityName: "query-helper", Limit: 1},
		{WorkspaceID: work.WorkspaceID, CandidateID: v1.CandidateID},
		{WorkspaceID: work.WorkspaceID, States: []CapabilityState{CapabilityStateCandidate}, Limit: 5000},
	}
	for index, query := range queries {
		versions, err := store.ListCapabilityVersions(ctx, query)
		if err != nil || len(versions) == 0 {
			t.Fatalf("list capability query %d = %+v err=%v", index, versions, err)
		}
	}
	if _, err := store.ListCapabilityVersions(ctx, ListCapabilityVersionsFilter{}); err == nil {
		t.Fatal("list capabilities without workspace succeeded")
	}
	if _, err := store.ListCapabilityVersions(ctx, ListCapabilityVersionsFilter{
		WorkspaceID: work.WorkspaceID, States: []CapabilityState{"invalid"},
	}); err == nil {
		t.Fatal("list capabilities accepted invalid state")
	}

	run := mustCreateEvaluation(t, store, v2, EvaluationStageSandbox, EvaluationStatusPassed, "query-evaluation")
	runs, err := store.ListEvaluationRuns(ctx, work.WorkspaceID, v2.ID)
	if err != nil || len(runs) != 1 || runs[0].ID != run.ID {
		t.Fatalf("evaluation runs = %+v err=%v", runs, err)
	}
	if _, err := store.ListEvaluationRuns(ctx, "", v2.ID); err == nil {
		t.Fatal("list evaluations without workspace succeeded")
	}

	later := mustCreateWork(t, store, work.WorkspaceID, "capability-query-outcome")
	outcome, err := store.RecordCapabilityOutcome(ctx, RecordCapabilityOutcomeInput{
		WorkspaceID: work.WorkspaceID, CapabilityVersionID: v2.ID, WorkID: later.ID,
		IdempotencyKey: "query-outcome", Status: CapabilityOutcomeCancelled,
		VerifierStatus: ProofStatusPending, ActorID: "scheduler",
	})
	if err != nil {
		t.Fatal(err)
	}
	outcomes, err := store.ListCapabilityOutcomes(ctx, work.WorkspaceID, v2.ID)
	if err != nil || len(outcomes) != 1 || outcomes[0].ID != outcome.ID {
		t.Fatalf("capability outcomes = %+v err=%v", outcomes, err)
	}
}

func TestControlPlanePoliciesRejectInvalidAuthorityAndNormalizeBounds(t *testing.T) {
	t.Parallel()

	valid, err := normalizeStepSchedulePolicy(StepSchedulePolicy{
		RetryLimit: 1,
		Proof: StepProofPolicy{
			Required: true, AllowLLMFallback: true, MaxLLMTokens: 100, MaxLLMCostUSD: 0.1,
			Requirements: []ProofRequirement{{
				Kind: " command ", Verifier: " verifier ", Command: " go test ./... ",
				Paths: []string{" report.json ", " "}, InputJSON: json.RawMessage(`{"ok":true}`),
			}},
		},
	})
	if err != nil || valid.MaxAttempts != 2 || valid.EscalationState != WorkStateReview ||
		valid.Proof.FailureState != WorkStateReview || valid.Proof.MinIndependentPasses != 1 ||
		len(valid.Proof.Requirements[0].Paths) != 1 || valid.Proof.Requirements[0].Kind != "command" {
		t.Fatalf("normalized policy = %+v err=%v", valid, err)
	}

	invalidPolicies := []StepSchedulePolicy{
		{RetryLimit: -1},
		{RetryLimit: 2, MaxAttempts: 2},
		{EscalationState: WorkStateDone},
		{Proof: StepProofPolicy{MinIndependentPasses: -1}},
		{Proof: StepProofPolicy{Required: true, FailureState: WorkStateDone}},
		{Proof: StepProofPolicy{Required: true, Requirements: []ProofRequirement{{Kind: "", Verifier: "v"}}}},
		{Proof: StepProofPolicy{Required: true, Requirements: []ProofRequirement{{Kind: "command", Verifier: "v"}, {Kind: "command", Verifier: "v2"}}}},
		{Proof: StepProofPolicy{Required: true, Requirements: []ProofRequirement{{Kind: "command", Verifier: "v", InputJSON: json.RawMessage(`{`)}}}},
		{Proof: StepProofPolicy{Required: true, MinIndependentPasses: 1, Requirements: []ProofRequirement{{Kind: "a", Verifier: "v"}, {Kind: "b", Verifier: "v"}}}},
		{Proof: StepProofPolicy{Required: true, AllowLLMFallback: true}},
	}
	for index, policy := range invalidPolicies {
		if _, err := normalizeStepSchedulePolicy(policy); err == nil {
			t.Fatalf("invalid policy %d accepted: %+v", index, policy)
		}
	}

	for _, disposition := range []StepDisposition{
		StepDispositionRetry, StepDispositionReplan, StepDispositionDecompose,
		StepDispositionBlocked, StepDispositionReview, StepDispositionDone,
	} {
		dispositionOutcome(disposition)
		eventTypeForDisposition(disposition)
	}
	if escalationDisposition(WorkStateBlocked) != StepDispositionBlocked || escalationDisposition(WorkStateReview) != StepDispositionReview {
		t.Fatal("unexpected escalation disposition")
	}
	budgetPolicy := StepSchedulePolicy{MaxAttempts: 2, RetryLimit: 1, ReplanLimit: 1, DecomposeLimit: 1, MaxIterations: 2, MaxTokens: 2, MaxCostUSD: 0.2}
	for _, got := range []StepDisposition{
		chooseFailureDisposition(budgetPolicy, 2, 0, 0, 0),
		chooseFailureDisposition(budgetPolicy, 0, 2, 0, 0),
		chooseFailureDisposition(budgetPolicy, 0, 0, 2, 0),
		chooseFailureDisposition(budgetPolicy, 0, 0, 0, 0.2),
		chooseFailureDisposition(StepSchedulePolicy{RetryLimit: 1}, 1, 0, 0, 0),
		chooseFailureDisposition(StepSchedulePolicy{ReplanLimit: 1}, 1, 0, 0, 0),
		chooseFailureDisposition(StepSchedulePolicy{DecomposeLimit: 1}, 1, 0, 0, 0),
	} {
		if got == "" {
			t.Fatal("empty failure disposition")
		}
	}
	if validateCompleteStepAttemptInput(CompleteStepAttemptInput{}) == nil ||
		validateCompleteStepAttemptInput(CompleteStepAttemptInput{
			WorkspaceID: "w", WorkID: "work", StepID: "step", AttemptID: "attempt", WorkerID: "worker", ActorID: "actor",
			Usage: StepAttemptUsage{Iterations: -1},
		}) == nil {
		t.Fatal("invalid completion input accepted")
	}
}

func TestControlPlaneStoresFailClosedAfterShutdown(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := store.ListCapabilityVersions(ctx, ListCapabilityVersionsFilter{WorkspaceID: "workspace"}); err == nil {
		t.Fatal("closed store listed capabilities")
	}
	if _, err := store.GetCapabilityVersion(ctx, "workspace", "version"); err == nil {
		t.Fatal("closed store got capability")
	}
	if _, err := store.GetCapabilityVersionByCandidate(ctx, "workspace", "candidate"); err == nil {
		t.Fatal("closed store got candidate")
	}
	if _, err := store.ListEvaluationRuns(ctx, "workspace", "version"); err == nil {
		t.Fatal("closed store listed evaluations")
	}
	if _, err := store.ListCapabilityOutcomes(ctx, "workspace", "version"); err == nil {
		t.Fatal("closed store listed outcomes")
	}
	if _, err := store.ConfigureStepSchedule(ctx, ConfigureStepScheduleInput{
		WorkspaceID: "workspace", WorkID: "work", StepID: "step", ActorID: "actor",
	}); err == nil {
		t.Fatal("closed store configured schedule")
	}
	if _, err := store.PromoteReadySteps(ctx, PromoteReadyStepsInput{WorkspaceID: "workspace", ActorID: "actor"}); err == nil {
		t.Fatal("closed store promoted steps")
	}
	if _, err := store.ClaimReadyStep(ctx, ClaimReadyStepInput{WorkspaceID: "workspace", WorkerID: "worker", Adapter: "local", ActorID: "actor"}); err == nil {
		t.Fatal("closed store claimed step")
	}
	if _, err := store.ListActiveStepClaims(ctx, "workspace", "work"); err == nil {
		t.Fatal("closed store listed active claims")
	}
	if _, err := store.ReclaimExpiredStepClaims(ctx, ReclaimExpiredStepClaimsInput{WorkspaceID: "workspace", ActorID: "actor", Reason: "lost"}); err == nil {
		t.Fatal("closed store reclaimed expired claims")
	}
	if _, err := store.ReclaimStepClaim(ctx, ReclaimStepClaimInput{
		WorkspaceID: "workspace", WorkID: "work", StepID: "step", AttemptID: "attempt",
		WorkerID: "worker", ActorID: "actor", Reason: "lost",
	}); err == nil {
		t.Fatal("closed store reclaimed claim")
	}
}

func TestCapabilityControlPlaneRejectsMalformedAndMismatchedRecords(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTestStore(t, filepath.Join(t.TempDir(), "ledger.db"))
	work := mustCreateWork(t, store, "workspace-capability-edges", "capability-edges")
	validVersion := CreateCapabilityVersionInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, CandidateID: "candidate-edges",
		CapabilityName: "edge-helper", ContentDigest: "sha256:edges",
		SnapshotJSON: json.RawMessage(`{"files":[]}`), ProvenanceJSON: json.RawMessage(`{"source":"test"}`),
		PermissionsJSON: json.RawMessage(`[]`), RolloutJSON: json.RawMessage(`{"percent":0}`), ActorID: "tester",
	}
	invalidVersions := []func(*CreateCapabilityVersionInput){
		func(input *CreateCapabilityVersionInput) { input.WorkspaceID = "" },
		func(input *CreateCapabilityVersionInput) { input.WorkID = "" },
		func(input *CreateCapabilityVersionInput) { input.CandidateID = "" },
		func(input *CreateCapabilityVersionInput) { input.CapabilityName = "" },
		func(input *CreateCapabilityVersionInput) { input.ContentDigest = "" },
		func(input *CreateCapabilityVersionInput) { input.ActorID = "" },
		func(input *CreateCapabilityVersionInput) { input.InitialState = "unknown" },
		func(input *CreateCapabilityVersionInput) { input.SnapshotJSON = json.RawMessage(`{`) },
		func(input *CreateCapabilityVersionInput) { input.ProvenanceJSON = json.RawMessage(`{`) },
		func(input *CreateCapabilityVersionInput) { input.PermissionsJSON = json.RawMessage(`{`) },
		func(input *CreateCapabilityVersionInput) { input.RolloutJSON = json.RawMessage(`{`) },
		func(input *CreateCapabilityVersionInput) { input.WorkID = "missing-work" },
		func(input *CreateCapabilityVersionInput) { input.RollbackTargetID = "missing-version" },
	}
	for index, mutate := range invalidVersions {
		input := validVersion
		input.CandidateID += string(rune('a' + index))
		mutate(&input)
		if _, err := store.CreateCapabilityVersion(ctx, input); err == nil {
			t.Fatalf("invalid capability version %d was accepted: %+v", index, input)
		}
	}

	version, err := store.CreateCapabilityVersion(ctx, validVersion)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := store.CreateCapabilityVersion(ctx, validVersion)
	if err != nil || replayed.ID != version.ID {
		t.Fatalf("idempotent capability replay = %+v err=%v", replayed, err)
	}
	transitionBase := TransitionCapabilityVersionInput{
		WorkspaceID: version.WorkspaceID, VersionID: version.ID, ExpectedState: version.State,
		ToState: CapabilityStateDraft, ActorID: "tester", Reason: "exercise transition validation",
	}
	invalidTransitions := []func(*TransitionCapabilityVersionInput){
		func(input *TransitionCapabilityVersionInput) { input.WorkspaceID = "" },
		func(input *TransitionCapabilityVersionInput) { input.VersionID = "" },
		func(input *TransitionCapabilityVersionInput) { input.ActorID = "" },
		func(input *TransitionCapabilityVersionInput) { input.ToState = "unknown" },
		func(input *TransitionCapabilityVersionInput) { input.ExpectedState = CapabilityStateDraft },
		func(input *TransitionCapabilityVersionInput) { input.ToState = CapabilityStatePromoted },
		func(input *TransitionCapabilityVersionInput) { input.RollbackTargetID = "missing-version" },
		func(input *TransitionCapabilityVersionInput) { input.RolloutJSON = json.RawMessage(`{`) },
	}
	for index, mutate := range invalidTransitions {
		input := transitionBase
		mutate(&input)
		if _, err := store.TransitionCapabilityVersion(ctx, input); err == nil {
			t.Fatalf("invalid capability transition %d was accepted: %+v", index, input)
		}
	}

	otherWork := mustCreateWork(t, store, work.WorkspaceID, "capability-other-work")
	validEvaluation := CreateEvaluationRunInput{
		WorkspaceID: version.WorkspaceID, WorkID: version.WorkID, CapabilityVersionID: version.ID,
		IdempotencyKey: "evaluation-edges", Stage: EvaluationStageSandbox, Status: EvaluationStatusPassed,
		MetricsJSON: json.RawMessage(`{"success_rate":1}`), DeltaJSON: json.RawMessage(`{"success_rate":0}`),
		ReportJSON: json.RawMessage(`{"summary":"passed"}`), ActorID: "evaluator",
	}
	invalidEvaluations := []func(*CreateEvaluationRunInput){
		func(input *CreateEvaluationRunInput) { input.WorkspaceID = "" },
		func(input *CreateEvaluationRunInput) { input.WorkID = "" },
		func(input *CreateEvaluationRunInput) { input.CapabilityVersionID = "" },
		func(input *CreateEvaluationRunInput) { input.IdempotencyKey = "" },
		func(input *CreateEvaluationRunInput) { input.ActorID = "" },
		func(input *CreateEvaluationRunInput) { input.Stage = "unknown" },
		func(input *CreateEvaluationRunInput) { input.Status = "unknown" },
		func(input *CreateEvaluationRunInput) { input.MetricsJSON = json.RawMessage(`{`) },
		func(input *CreateEvaluationRunInput) { input.DeltaJSON = json.RawMessage(`{`) },
		func(input *CreateEvaluationRunInput) { input.ReportJSON = json.RawMessage(`{`) },
		func(input *CreateEvaluationRunInput) { input.CapabilityVersionID = "missing-version" },
		func(input *CreateEvaluationRunInput) { input.WorkID = otherWork.ID },
		func(input *CreateEvaluationRunInput) { input.BaselineVersionID = "missing-version" },
		func(input *CreateEvaluationRunInput) { input.ProofID = "missing-proof" },
	}
	for index, mutate := range invalidEvaluations {
		input := validEvaluation
		input.IdempotencyKey += string(rune('a' + index))
		mutate(&input)
		if _, err := store.CreateEvaluationRun(ctx, input); err == nil {
			t.Fatalf("invalid evaluation %d was accepted: %+v", index, input)
		}
	}
	run, err := store.CreateEvaluationRun(ctx, validEvaluation)
	if err != nil {
		t.Fatal(err)
	}
	replayedRun, err := store.CreateEvaluationRun(ctx, validEvaluation)
	if err != nil || replayedRun.ID != run.ID {
		t.Fatalf("idempotent evaluation replay = %+v err=%v", replayedRun, err)
	}

	validOutcome := RecordCapabilityOutcomeInput{
		WorkspaceID: version.WorkspaceID, CapabilityVersionID: version.ID, WorkID: otherWork.ID,
		IdempotencyKey: "outcome-edges", Status: CapabilityOutcomeSucceeded,
		VerifierStatus: ProofStatusPassed, CostUSD: 0.01, LatencyMS: 1, ActorID: "scheduler",
	}
	invalidOutcomes := []func(*RecordCapabilityOutcomeInput){
		func(input *RecordCapabilityOutcomeInput) { input.WorkspaceID = "" },
		func(input *RecordCapabilityOutcomeInput) { input.CapabilityVersionID = "" },
		func(input *RecordCapabilityOutcomeInput) { input.WorkID = "" },
		func(input *RecordCapabilityOutcomeInput) { input.IdempotencyKey = "" },
		func(input *RecordCapabilityOutcomeInput) { input.ActorID = "" },
		func(input *RecordCapabilityOutcomeInput) { input.Status = "unknown" },
		func(input *RecordCapabilityOutcomeInput) { input.VerifierStatus = "unknown" },
		func(input *RecordCapabilityOutcomeInput) { input.CostUSD = -1 },
		func(input *RecordCapabilityOutcomeInput) { input.LatencyMS = -1 },
		func(input *RecordCapabilityOutcomeInput) { input.CapabilityVersionID = "missing-version" },
		func(input *RecordCapabilityOutcomeInput) { input.WorkID = "missing-work" },
		func(input *RecordCapabilityOutcomeInput) { input.AttemptID = "missing-attempt" },
	}
	for index, mutate := range invalidOutcomes {
		input := validOutcome
		input.IdempotencyKey += string(rune('a' + index))
		mutate(&input)
		if _, err := store.RecordCapabilityOutcome(ctx, input); err == nil {
			t.Fatalf("invalid outcome %d was accepted: %+v", index, input)
		}
	}
	outcome, err := store.RecordCapabilityOutcome(ctx, validOutcome)
	if err != nil {
		t.Fatal(err)
	}
	replayedOutcome, err := store.RecordCapabilityOutcome(ctx, validOutcome)
	if err != nil || replayedOutcome.ID != outcome.ID {
		t.Fatalf("idempotent outcome replay = %+v err=%v", replayedOutcome, err)
	}
}

func TestCapabilityControlPlaneClassifiesEveryLifecycleState(t *testing.T) {
	t.Parallel()

	validStates := []CapabilityState{
		CapabilityStateCandidate, CapabilityStateDraft, CapabilityStateSandbox,
		CapabilityStateOfflineEval, CapabilityStateShadow, CapabilityStateApproved,
		CapabilityStateCanary, CapabilityStatePromoted, CapabilityStateRolledBack,
		CapabilityStateRejected,
	}
	for _, state := range validStates {
		if !validCapabilityState(state) {
			t.Fatalf("valid capability state %q rejected", state)
		}
	}
	if validCapabilityState("unknown") {
		t.Fatal("unknown capability state accepted")
	}
	validTransitions := [][2]CapabilityState{
		{CapabilityStateCandidate, CapabilityStateDraft},
		{CapabilityStateDraft, CapabilityStateSandbox},
		{CapabilityStateSandbox, CapabilityStateOfflineEval},
		{CapabilityStateOfflineEval, CapabilityStateShadow},
		{CapabilityStateShadow, CapabilityStateApproved},
		{CapabilityStateApproved, CapabilityStateCanary},
		{CapabilityStateCanary, CapabilityStatePromoted},
		{CapabilityStatePromoted, CapabilityStateRolledBack},
		{CapabilityStateDraft, CapabilityStateRejected},
	}
	for _, transition := range validTransitions {
		if !canTransitionCapability(transition[0], transition[1]) {
			t.Fatalf("valid transition %s -> %s rejected", transition[0], transition[1])
		}
	}
	for _, state := range []CapabilityState{CapabilityStatePromoted, CapabilityStateRolledBack, CapabilityStateRejected} {
		if canTransitionCapability(state, CapabilityStateRejected) {
			t.Fatalf("terminal state %s was allowed to reject", state)
		}
	}
	if canTransitionCapability(CapabilityStateRejected, CapabilityStateDraft) ||
		canTransitionCapability(CapabilityStateDraft, CapabilityStatePromoted) {
		t.Fatal("invalid capability transition accepted")
	}

	for _, stage := range []EvaluationStage{EvaluationStageSandbox, EvaluationStageOffline, EvaluationStageShadow, EvaluationStageCanary} {
		if !validEvaluationStage(stage) {
			t.Fatalf("valid evaluation stage %q rejected", stage)
		}
	}
	if validEvaluationStage("unknown") {
		t.Fatal("unknown evaluation stage accepted")
	}
	for _, status := range []EvaluationStatus{EvaluationStatusPending, EvaluationStatusPassed, EvaluationStatusFailed} {
		if !validEvaluationStatus(status) {
			t.Fatalf("valid evaluation status %q rejected", status)
		}
	}
	if validEvaluationStatus("unknown") {
		t.Fatal("unknown evaluation status accepted")
	}
	for _, status := range []CapabilityOutcomeStatus{CapabilityOutcomeSucceeded, CapabilityOutcomeFailed, CapabilityOutcomeCancelled} {
		if !validCapabilityOutcomeStatus(status) {
			t.Fatalf("valid capability outcome %q rejected", status)
		}
	}
	if validCapabilityOutcomeStatus("unknown") {
		t.Fatal("unknown capability outcome accepted")
	}

	expectedStages := map[CapabilityState]int{
		CapabilityStateCandidate: 0, CapabilityStateDraft: 0, CapabilityStateSandbox: 0,
		CapabilityStateOfflineEval: 1, CapabilityStateShadow: 2, CapabilityStateApproved: 3,
		CapabilityStateCanary: 4, CapabilityStatePromoted: 4, CapabilityStateRejected: 0,
	}
	for state, expected := range expectedStages {
		if stages := requiredEvaluationStagesForState(state); len(stages) != expected {
			t.Fatalf("required stages for %s = %v, want %d", state, stages, expected)
		}
	}
}

func TestSchedulerControlPlaneRejectsInvalidMissingConflictingAndExpiredClaims(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 8, 3, 14, 0, 0, 0, time.UTC)
	store, err := Open(ctx, filepath.Join(t.TempDir(), "ledger.db"), Options{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if _, err := store.ConfigureStepSchedule(ctx, ConfigureStepScheduleInput{}); err == nil {
		t.Fatal("empty schedule configuration accepted")
	}
	if _, err := store.GetStepSchedule(ctx, "", "", ""); err == nil {
		t.Fatal("empty schedule lookup accepted")
	}
	if _, err := store.PromoteReadySteps(ctx, PromoteReadyStepsInput{}); err == nil {
		t.Fatal("empty ready promotion accepted")
	}
	if _, err := store.ClaimReadyStep(ctx, ClaimReadyStepInput{}); err == nil {
		t.Fatal("empty claim accepted")
	}
	if _, err := store.HeartbeatStepClaim(ctx, HeartbeatStepClaimInput{}); err == nil {
		t.Fatal("empty heartbeat accepted")
	}
	if _, err := store.ReleaseStepClaim(ctx, ReleaseStepClaimInput{}); err == nil {
		t.Fatal("empty release accepted")
	}
	if _, err := store.CompleteStepAttempt(ctx, CompleteStepAttemptInput{}); err == nil {
		t.Fatal("empty completion accepted")
	}
	if _, err := store.ReclaimExpiredStepClaims(ctx, ReclaimExpiredStepClaimsInput{}); err == nil {
		t.Fatal("empty expired reclaim accepted")
	}
	if _, err := store.ReclaimStepClaim(ctx, ReclaimStepClaimInput{}); err == nil {
		t.Fatal("empty explicit reclaim accepted")
	}
	if _, err := store.ResumeScheduledStep(ctx, ResumeScheduledStepInput{}); err == nil {
		t.Fatal("empty resume accepted")
	}
	if _, err := store.CancelScheduledStep(ctx, CancelScheduledStepInput{}); err == nil {
		t.Fatal("empty cancellation accepted")
	}
	if _, err := store.ListActiveStepClaims(ctx, "", ""); err == nil {
		t.Fatal("empty active claim query accepted")
	}

	work := mustCreateWork(t, store, "workspace-scheduler-edges", "scheduler-edges")
	if _, err := store.ConfigureStepSchedule(ctx, ConfigureStepScheduleInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: "missing-step",
		Policy: StepSchedulePolicy{MaxAttempts: 1}, ActorID: "scheduler",
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing step configuration error=%v", err)
	}
	if _, err := store.GetStepSchedule(ctx, work.WorkspaceID, work.ID, "missing-step"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing schedule lookup error=%v", err)
	}
	if _, err := store.HeartbeatStepClaim(ctx, HeartbeatStepClaimInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: "missing-step", AttemptID: "missing-attempt",
		WorkerID: "worker-a", ActorID: "worker-a",
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing heartbeat schedule error=%v", err)
	}
	if _, err := store.ReleaseStepClaim(ctx, ReleaseStepClaimInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: "missing-step", AttemptID: "missing-attempt",
		WorkerID: "worker-a", ActorID: "worker-a", Reason: "release",
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing release schedule error=%v", err)
	}
	if _, err := store.ResumeScheduledStep(ctx, ResumeScheduledStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: "missing-step", ActorID: "operator", Reason: "resume",
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing resume step error=%v", err)
	}
	if _, err := store.CancelScheduledStep(ctx, CancelScheduledStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: "missing-step", ActorID: "operator", Reason: "cancel",
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing cancel step error=%v", err)
	}
	if _, err := store.ClaimReadyStep(ctx, ClaimReadyStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, WorkerID: "worker-a", Adapter: "local", ActorID: "scheduler",
	}); !errors.Is(err, ErrNoReadyStep) {
		t.Fatalf("claim without ready step error=%v", err)
	}

	step := mustCreateScheduledStep(t, store, work, "expiry-edge", 1)
	if _, err := store.ConfigureStepSchedule(ctx, ConfigureStepScheduleInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: step.ID,
		Policy: StepSchedulePolicy{MaxAttempts: 2, RetryLimit: 1}, ActorID: "scheduler",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PromoteReadySteps(ctx, PromoteReadyStepsInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, ActorID: "scheduler",
	}); err != nil {
		t.Fatal(err)
	}
	claim := mustClaimStep(t, store, work, "worker-a", time.Second)
	if _, err := store.ConfigureStepSchedule(ctx, ConfigureStepScheduleInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: step.ID,
		Policy: StepSchedulePolicy{MaxAttempts: 3, RetryLimit: 2}, ActorID: "scheduler",
	}); !errors.Is(err, ErrClaimConflict) {
		t.Fatalf("active schedule reconfiguration error=%v", err)
	}
	if _, err := store.CompleteStepAttempt(ctx, CompleteStepAttemptInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: step.ID, AttemptID: claim.Attempt.ID,
		WorkerID: "worker-a", OutputJSON: json.RawMessage(`{`), ActorID: "worker-a",
	}); err == nil {
		t.Fatal("completion accepted malformed output")
	}
	if _, err := store.ReleaseStepClaim(ctx, ReleaseStepClaimInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: step.ID, AttemptID: claim.Attempt.ID,
		WorkerID: "worker-b", ActorID: "worker-b", Reason: "foreign release",
	}); !errors.Is(err, ErrClaimConflict) {
		t.Fatalf("foreign release error=%v", err)
	}

	now = now.Add(2 * time.Second)
	if _, err := store.HeartbeatStepClaim(ctx, HeartbeatStepClaimInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: step.ID, AttemptID: claim.Attempt.ID,
		WorkerID: "worker-a", ActorID: "worker-a",
	}); !errors.Is(err, ErrClaimExpired) {
		t.Fatalf("expired heartbeat error=%v", err)
	}
	if _, err := store.ReleaseStepClaim(ctx, ReleaseStepClaimInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: step.ID, AttemptID: claim.Attempt.ID,
		WorkerID: "worker-a", ActorID: "worker-a", Reason: "expired release",
	}); !errors.Is(err, ErrClaimExpired) {
		t.Fatalf("expired release error=%v", err)
	}
	if _, err := store.CompleteStepAttempt(ctx, CompleteStepAttemptInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: step.ID, AttemptID: claim.Attempt.ID,
		WorkerID: "worker-a", Succeeded: true, ActorID: "worker-a",
	}); !errors.Is(err, ErrClaimExpired) {
		t.Fatalf("expired completion error=%v", err)
	}
	reclaimed, err := store.ReclaimExpiredStepClaims(ctx, ReclaimExpiredStepClaimsInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, ActorID: "scheduler", Reason: "lease expired",
	})
	if err != nil || len(reclaimed) != 1 || reclaimed[0].Attempt.ID != claim.Attempt.ID {
		t.Fatalf("expired reclaim=%+v err=%v", reclaimed, err)
	}
	if _, err := store.ResumeScheduledStep(ctx, ResumeScheduledStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: step.ID, ActorID: "operator", Reason: "not gated",
	}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("ungated resume error=%v", err)
	}
}

func TestLedgerEnumsAndImportedJSONFailClosedAcrossUnknownValues(t *testing.T) {
	t.Parallel()

	for raw, want := range map[string]bool{
		"": false, "{": false, "null": false, `{}`: false, `[]`: false,
		`{"runtime":"sandbox"}`: true, `["sandbox"]`: true, `true`: true, `0`: true,
	} {
		if got := meaningfulImportJSON(json.RawMessage(raw)); got != want {
			t.Errorf("meaningful import JSON %q=%v want=%v", raw, got, want)
		}
	}
	for _, status := range []AttemptStatus{
		AttemptStatusPending, AttemptStatusRunning, AttemptStatusSucceeded, AttemptStatusFailed, AttemptStatusCancelled,
	} {
		if !validAttemptStatus(status) {
			t.Errorf("valid attempt status %q rejected", status)
		}
	}
	if validAttemptStatus("unknown") {
		t.Fatal("unknown attempt status accepted")
	}
	for _, status := range []ApprovalStatus{
		ApprovalStatusPending, ApprovalStatusApproved, ApprovalStatusDenied, ApprovalStatusExpired,
	} {
		if !validApprovalStatus(status) {
			t.Errorf("valid approval status %q rejected", status)
		}
	}
	if validApprovalStatus("unknown") {
		t.Fatal("unknown approval status accepted")
	}
	for _, status := range []ProofStatus{
		ProofStatusReported, ProofStatusPending, ProofStatusPassed, ProofStatusFailed, ProofStatusStale,
	} {
		if !validProofStatus(status) {
			t.Errorf("valid proof status %q rejected", status)
		}
	}
	if validProofStatus("unknown") {
		t.Fatal("unknown proof status accepted")
	}
	for _, origin := range []ProofOrigin{ProofOriginWorkerReport, ProofOriginIndependentVerifier, ProofOriginLegacy} {
		if !validProofOrigin(origin) {
			t.Errorf("valid proof origin %q rejected", origin)
		}
	}
	if validProofOrigin("unknown") {
		t.Fatal("unknown proof origin accepted")
	}
	for _, transition := range [][2]ProofStatus{
		{ProofStatusReported, ProofStatusPending}, {ProofStatusReported, ProofStatusStale},
		{ProofStatusPending, ProofStatusPassed}, {ProofStatusPending, ProofStatusFailed}, {ProofStatusPending, ProofStatusStale},
		{ProofStatusPassed, ProofStatusStale}, {ProofStatusFailed, ProofStatusStale},
		{ProofStatusStale, ProofStatusPending},
	} {
		if !canTransitionProof(transition[0], transition[1]) {
			t.Errorf("valid proof transition %s -> %s rejected", transition[0], transition[1])
		}
	}
	if canTransitionProof("unknown", ProofStatusPending) || canTransitionProof(ProofStatusPassed, ProofStatusPending) {
		t.Fatal("invalid proof transition accepted")
	}
	now := time.Date(2026, 8, 3, 15, 0, 0, 0, time.UTC)
	observed := now.Add(-time.Hour)
	if got := proofObservedAt(ProofStatusPassed, &observed, now); got == nil || !got.Equal(observed) {
		t.Fatalf("explicit observed time=%v", got)
	}
	if got := proofObservedAt(ProofStatusPending, nil, now); got != nil {
		t.Fatalf("pending proof observed time=%v", got)
	}
	if got := proofObservedAt(ProofStatusPassed, nil, now); got == nil || !got.Equal(now) {
		t.Fatalf("passed proof observed time=%v", got)
	}
}
