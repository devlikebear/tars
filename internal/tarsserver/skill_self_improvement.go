package tarsserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/skill"
	"github.com/devlikebear/tars/internal/workstore"
)

const capabilityLifecycleActor = "self-improvement"

type skillCapabilityLifecycle struct {
	workspaceDir string
	ledger       *workstore.Store
	mu           sync.Mutex
}

type skillCapabilityLifecycleResult struct {
	Candidate   skill.ExtractionCandidate     `json:"candidate"`
	Capability  workstore.CapabilityVersion   `json:"capability"`
	Evaluations []workstore.EvaluationRun     `json:"evaluations"`
	Outcomes    []workstore.CapabilityOutcome `json:"outcomes,omitempty"`
	Draft       skillCreatorDraftResponse     `json:"draft,omitempty"`
	Saved       skillCreatorSaveResponse      `json:"saved,omitempty"`
	Diff        string                        `json:"diff,omitempty"`
}

type capabilityEvaluationMetrics struct {
	SuccessRate      float64 `json:"success_rate"`
	VerificationRate float64 `json:"verification_rate"`
	CostUSD          float64 `json:"cost_usd"`
	LatencyMS        int64   `json:"latency_ms"`
}

func newSkillCapabilityLifecycle(workspaceDir string, ledger *workstore.Store) *skillCapabilityLifecycle {
	if ledger == nil {
		return nil
	}
	return &skillCapabilityLifecycle{workspaceDir: strings.TrimSpace(workspaceDir), ledger: ledger}
}

func (l *skillCapabilityLifecycle) SyncCandidates(ctx context.Context, candidates []skill.ExtractionCandidate) ([]workstore.CapabilityVersion, error) {
	if l == nil || l.ledger == nil {
		return nil, fmt.Errorf("reviewed self-improvement requires the Work Ledger")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	ordered := append([]skill.ExtractionCandidate(nil), candidates...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].CreatedAt.Equal(ordered[j].CreatedAt) {
			return ordered[i].ID < ordered[j].ID
		}
		return ordered[i].CreatedAt.Before(ordered[j].CreatedAt)
	})
	versions := make([]workstore.CapabilityVersion, 0, len(ordered))
	for _, candidate := range ordered {
		version, _, err := l.ensureCandidate(ctx, candidate)
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, nil
}

func (l *skillCapabilityLifecycle) Apply(ctx context.Context, candidateID string, action skill.ExtractionCandidateAction) (skillCapabilityLifecycleResult, error) {
	if l == nil || l.ledger == nil {
		return skillCapabilityLifecycleResult{}, fmt.Errorf("reviewed self-improvement requires the Work Ledger")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	candidate, err := skill.FindExtractionCandidate(l.workspaceDir, candidateID)
	if err != nil {
		return skillCapabilityLifecycleResult{}, err
	}
	version, draft, err := l.ensureCandidate(ctx, candidate)
	if err != nil {
		return skillCapabilityLifecycleResult{}, err
	}

	switch action {
	case skill.ExtractionCandidateActionEvaluate:
		version, err = l.evaluateThroughShadow(ctx, version, draft)
	case skill.ExtractionCandidateActionApprove:
		version, err = l.evaluateThroughShadow(ctx, version, draft)
		if err == nil {
			version, candidate, err = l.approveAndCanary(ctx, version, candidate, draft)
		}
	case skill.ExtractionCandidateActionPromote:
		version, err = l.promote(ctx, version, draft)
	case skill.ExtractionCandidateActionRollback:
		version, err = l.rollback(ctx, version)
	case skill.ExtractionCandidateActionReject:
		version, candidate, err = l.reject(ctx, version, candidate)
	default:
		err = fmt.Errorf("unknown skill extraction action: %s", action)
	}
	if err != nil {
		return skillCapabilityLifecycleResult{}, err
	}
	evaluations, err := l.ledger.ListEvaluationRuns(ctx, version.WorkspaceID, version.ID)
	if err != nil {
		return skillCapabilityLifecycleResult{}, err
	}
	outcomes, err := l.ledger.ListCapabilityOutcomes(ctx, version.WorkspaceID, version.ID)
	if err != nil {
		return skillCapabilityLifecycleResult{}, err
	}
	return skillCapabilityLifecycleResult{
		Candidate:   candidate,
		Capability:  version,
		Evaluations: evaluations,
		Outcomes:    outcomes,
		Draft:       draft,
		Saved:       savedCapabilityResponse(l.workspaceDir, version, action),
		Diff:        l.capabilityDiff(ctx, version, draft),
	}, nil
}

func (l *skillCapabilityLifecycle) ensureCandidate(ctx context.Context, candidate skill.ExtractionCandidate) (workstore.CapabilityVersion, skillCreatorDraftResponse, error) {
	if existing, err := l.ledger.GetCapabilityVersionByCandidate(ctx, defaultWorkspaceID, candidate.ID); err == nil {
		draft, decodeErr := decodeCapabilityDraft(existing.SnapshotJSON)
		return existing, draft, decodeErr
	} else if !errors.Is(err, workstore.ErrNotFound) {
		return workstore.CapabilityVersion{}, skillCreatorDraftResponse{}, err
	}

	draft, legacyActive, err := l.draftForCandidate(ctx, candidate)
	if err != nil {
		return workstore.CapabilityVersion{}, skillCreatorDraftResponse{}, err
	}
	snapshot, err := json.Marshal(draft)
	if err != nil {
		return workstore.CapabilityVersion{}, skillCreatorDraftResponse{}, fmt.Errorf("marshal capability draft: %w", err)
	}
	provenance, err := json.Marshal(map[string]any{
		"candidate":     candidate,
		"migration":     "skill_inbox_v1",
		"legacy_status": candidate.Status,
	})
	if err != nil {
		return workstore.CapabilityVersion{}, skillCreatorDraftResponse{}, fmt.Errorf("marshal capability provenance: %w", err)
	}
	permissions, err := json.Marshal(capabilityPermissions(draft))
	if err != nil {
		return workstore.CapabilityVersion{}, skillCreatorDraftResponse{}, fmt.Errorf("marshal capability permissions: %w", err)
	}
	initialCapabilityState := workstore.CapabilityStateCandidate
	initialWorkState := workstore.WorkStateReview
	switch {
	case candidate.Status == skill.ExtractionCandidateStatusRejected:
		initialCapabilityState = workstore.CapabilityStateRejected
		initialWorkState = workstore.WorkStateCancelled
	case candidate.Status == skill.ExtractionCandidateStatusApproved && legacyActive:
		initialCapabilityState = workstore.CapabilityStatePromoted
		initialWorkState = workstore.WorkStateDone
	}
	workMetadata, _ := json.Marshal(map[string]any{
		"candidate_id":    candidate.ID,
		"capability_name": draft.Name,
		"source_session":  candidate.SourceSession,
	})
	work, err := l.ledger.CreateWork(ctx, workstore.CreateWorkInput{
		WorkspaceID:    defaultWorkspaceID,
		Kind:           "capability-improvement",
		Source:         "skill-inbox",
		SourceID:       candidate.ID,
		IdempotencyKey: "capability:candidate:" + candidate.ID,
		Title:          "Review capability " + draft.Name,
		Objective:      candidate.Summary,
		MetadataJSON:   workMetadata,
		InitialState:   initialWorkState,
		ActorID:        capabilityLifecycleActor,
	})
	if err != nil {
		return workstore.CapabilityVersion{}, skillCreatorDraftResponse{}, err
	}
	digest := sha256.Sum256(snapshot)
	version, err := l.ledger.CreateCapabilityVersion(ctx, workstore.CreateCapabilityVersionInput{
		WorkspaceID:     defaultWorkspaceID,
		WorkID:          work.ID,
		CandidateID:     candidate.ID,
		CapabilityName:  draft.Name,
		InitialState:    initialCapabilityState,
		ContentDigest:   "sha256:" + hex.EncodeToString(digest[:]),
		SnapshotJSON:    snapshot,
		ProvenanceJSON:  provenance,
		PermissionsJSON: permissions,
		RolloutJSON:     json.RawMessage(`{"mode":"none","percent":0}`),
		ActorID:         capabilityLifecycleActor,
	})
	if err != nil {
		return workstore.CapabilityVersion{}, skillCreatorDraftResponse{}, err
	}
	return version, draft, nil
}

func (l *skillCapabilityLifecycle) draftForCandidate(ctx context.Context, candidate skill.ExtractionCandidate) (skillCreatorDraftResponse, bool, error) {
	if candidate.Status == skill.ExtractionCandidateStatusApproved && strings.TrimSpace(candidate.DraftPath) != "" {
		if draft, err := loadLegacyCapabilityDraft(l.workspaceDir, candidate); err == nil {
			return draft, true, nil
		}
	}
	name := strings.TrimSpace(candidate.Name)
	managed, err := l.ledger.ListCapabilityVersions(ctx, workstore.ListCapabilityVersionsFilter{
		WorkspaceID:    defaultWorkspaceID,
		CapabilityName: name,
		Limit:          1,
	})
	if err != nil {
		return skillCreatorDraftResponse{}, false, err
	}
	if len(managed) == 0 {
		name = uniqueSkillDraftName(l.workspaceDir, name)
	}
	draft, err := buildSkillCreatorDraft(skillCreatorDraftRequest{
		Name:             name,
		Description:      firstNonEmptyString(candidate.Summary, candidate.Trigger),
		Category:         "session",
		Language:         "shell",
		Layout:           "single_file",
		UseCase:          firstNonEmptyString(candidate.UseCase, candidate.Summary),
		RecommendedTools: candidate.RecommendedTools,
	})
	if err != nil {
		return skillCreatorDraftResponse{}, false, err
	}
	draft.Files = addExtractionProvenanceToSkillDraft(draft.Files, candidate)
	return draft, false, nil
}

func (l *skillCapabilityLifecycle) evaluateThroughShadow(ctx context.Context, version workstore.CapabilityVersion, draft skillCreatorDraftResponse) (workstore.CapabilityVersion, error) {
	var err error
	if version.State == workstore.CapabilityStateCandidate {
		version, err = l.transition(ctx, version, workstore.CapabilityStateDraft, "", nil, "generated a reviewable draft")
		if err != nil {
			return version, err
		}
	}
	if version.State == workstore.CapabilityStateDraft {
		version, err = l.transition(ctx, version, workstore.CapabilityStateSandbox, "", nil, "start isolated sandbox smoke")
		if err != nil {
			return version, err
		}
	}
	if version.State == workstore.CapabilityStateSandbox {
		passed, err := l.ensureExecutableEvaluation(ctx, version, draft, workstore.EvaluationStageSandbox)
		if err != nil {
			return version, err
		}
		if !passed {
			return version, fmt.Errorf("sandbox smoke evaluation failed")
		}
		version, err = l.transition(ctx, version, workstore.CapabilityStateOfflineEval, "", nil, "sandbox smoke passed")
		if err != nil {
			return version, err
		}
	}
	if version.State == workstore.CapabilityStateOfflineEval {
		passed, err := l.ensureOfflineEvaluation(ctx, version, draft)
		if err != nil {
			return version, err
		}
		if !passed {
			return version, fmt.Errorf("offline capability evaluation failed")
		}
		version, err = l.transition(ctx, version, workstore.CapabilityStateShadow, "", nil, "offline evaluation passed")
		if err != nil {
			return version, err
		}
	}
	if version.State == workstore.CapabilityStateShadow {
		passed, err := l.ensureExecutableEvaluation(ctx, version, draft, workstore.EvaluationStageShadow)
		if err != nil {
			return version, err
		}
		if !passed {
			return version, fmt.Errorf("shadow evaluation failed")
		}
	}
	return version, nil
}

func (l *skillCapabilityLifecycle) approveAndCanary(ctx context.Context, version workstore.CapabilityVersion, candidate skill.ExtractionCandidate, draft skillCreatorDraftResponse) (workstore.CapabilityVersion, skill.ExtractionCandidate, error) {
	if version.State == workstore.CapabilityStateCanary || version.State == workstore.CapabilityStatePromoted {
		return version, candidate, nil
	}
	if version.State != workstore.CapabilityStateShadow {
		return version, candidate, fmt.Errorf("capability must finish shadow evaluation before approval (current: %s)", version.State)
	}
	approval, err := l.ledger.CreateApproval(ctx, workstore.CreateApprovalInput{
		WorkspaceID:    version.WorkspaceID,
		WorkID:         version.WorkID,
		IdempotencyKey: "capability:" + version.ID + ":human-approval",
		Authority:      "human",
		Status:         workstore.ApprovalStatusApproved,
		Request:        fmt.Sprintf("Approve %s v%d for canary rollout", version.CapabilityName, version.Version),
		Reason:         "operator approved reviewed evaluation evidence",
		ActorID:        "operator",
		ReviewerID:     "operator",
	})
	if err != nil {
		return version, candidate, err
	}
	version, err = l.transition(ctx, version, workstore.CapabilityStateApproved, approval.ID, nil, "human approval recorded")
	if err != nil {
		return version, candidate, err
	}
	rollout := json.RawMessage(`{"mode":"operator-canary","percent":5,"rollback":"one-action"}`)
	version, err = l.transition(ctx, version, workstore.CapabilityStateCanary, "", rollout, "start operator-scoped canary")
	if err != nil {
		return version, candidate, err
	}
	passed, err := l.ensureExecutableEvaluation(ctx, version, draft, workstore.EvaluationStageCanary)
	if err != nil {
		return version, candidate, err
	}
	if !passed {
		return version, candidate, fmt.Errorf("canary evaluation failed")
	}
	reviewed, err := skill.ReviewExtractionCandidate(l.workspaceDir, candidate.ID, skill.ExtractionCandidateReview{
		Action:    skill.ExtractionCandidateActionApprove,
		DraftPath: "work-ledger://capabilities/" + version.ID,
		DraftName: version.CapabilityName,
	})
	if err != nil {
		return version, candidate, err
	}
	return version, reviewed, nil
}

func (l *skillCapabilityLifecycle) promote(ctx context.Context, version workstore.CapabilityVersion, draft skillCreatorDraftResponse) (workstore.CapabilityVersion, error) {
	if version.State == workstore.CapabilityStatePromoted {
		return version, nil
	}
	if version.State != workstore.CapabilityStateCanary {
		return version, fmt.Errorf("capability must pass canary before promotion (current: %s)", version.State)
	}
	var promoted workstore.CapabilityVersion
	err := replaceCapabilitySkill(l.workspaceDir, draft, func() error {
		var err error
		promoted, err = l.transition(ctx, version, workstore.CapabilityStatePromoted, "", json.RawMessage(`{"mode":"full","percent":100,"rollback":"one-action"}`), "promoted after approved canary")
		return err
	})
	if err != nil {
		return version, err
	}
	if work, err := l.ledger.GetWork(ctx, promoted.WorkspaceID, promoted.WorkID); err == nil && work.State == workstore.WorkStateReview {
		_, _ = l.ledger.TransitionWork(ctx, workstore.TransitionWorkInput{
			WorkspaceID:     work.WorkspaceID,
			WorkID:          work.ID,
			ToState:         workstore.WorkStateDone,
			ExpectedVersion: work.Version,
			ActorID:         capabilityLifecycleActor,
			IdempotencyKey:  "capability:" + promoted.ID + ":work-done",
			Reason:          "capability promoted",
		})
	}
	return promoted, nil
}

func (l *skillCapabilityLifecycle) rollback(ctx context.Context, version workstore.CapabilityVersion) (workstore.CapabilityVersion, error) {
	if version.State == workstore.CapabilityStateRolledBack {
		return version, nil
	}
	if version.State != workstore.CapabilityStatePromoted {
		return version, fmt.Errorf("only a promoted capability can be rolled back (current: %s)", version.State)
	}
	transition := func() error {
		var err error
		version, err = l.transition(ctx, version, workstore.CapabilityStateRolledBack, "", json.RawMessage(`{"mode":"rolled-back","percent":0}`), "operator requested rollback")
		return err
	}
	if version.RollbackTargetID == "" {
		if err := removeCapabilitySkill(l.workspaceDir, version.CapabilityName, transition); err != nil {
			return version, err
		}
		return version, nil
	}
	target, err := l.ledger.GetCapabilityVersion(ctx, version.WorkspaceID, version.RollbackTargetID)
	if err != nil {
		return version, fmt.Errorf("load rollback target: %w", err)
	}
	draft, err := decodeCapabilityDraft(target.SnapshotJSON)
	if err != nil {
		return version, fmt.Errorf("decode rollback target: %w", err)
	}
	if err := replaceCapabilitySkill(l.workspaceDir, draft, transition); err != nil {
		return version, err
	}
	return version, nil
}

func (l *skillCapabilityLifecycle) reject(ctx context.Context, version workstore.CapabilityVersion, candidate skill.ExtractionCandidate) (workstore.CapabilityVersion, skill.ExtractionCandidate, error) {
	if version.State != workstore.CapabilityStateRejected {
		updated, err := l.transition(ctx, version, workstore.CapabilityStateRejected, "", nil, "operator rejected candidate")
		if err != nil {
			return version, candidate, err
		}
		version = updated
	}
	reviewed, err := skill.ReviewExtractionCandidate(l.workspaceDir, candidate.ID, skill.ExtractionCandidateReview{Action: skill.ExtractionCandidateActionReject})
	if err != nil {
		return version, candidate, err
	}
	if work, err := l.ledger.GetWork(ctx, version.WorkspaceID, version.WorkID); err == nil && work.State == workstore.WorkStateReview {
		_, _ = l.ledger.TransitionWork(ctx, workstore.TransitionWorkInput{
			WorkspaceID: work.WorkspaceID, WorkID: work.ID, ToState: workstore.WorkStateCancelled,
			ExpectedVersion: work.Version, ActorID: capabilityLifecycleActor,
			IdempotencyKey: "capability:" + version.ID + ":work-cancelled", Reason: "candidate rejected",
		})
	}
	return version, reviewed, nil
}

func (l *skillCapabilityLifecycle) transition(ctx context.Context, version workstore.CapabilityVersion, to workstore.CapabilityState, approvalID string, rollout json.RawMessage, reason string) (workstore.CapabilityVersion, error) {
	return l.ledger.TransitionCapabilityVersion(ctx, workstore.TransitionCapabilityVersionInput{
		WorkspaceID:   version.WorkspaceID,
		VersionID:     version.ID,
		ExpectedState: version.State,
		ToState:       to,
		ApprovalID:    approvalID,
		RolloutJSON:   rollout,
		ActorID:       capabilityLifecycleActor,
		Reason:        reason,
	})
}

func (l *skillCapabilityLifecycle) ensureExecutableEvaluation(ctx context.Context, version workstore.CapabilityVersion, draft skillCreatorDraftResponse, stage workstore.EvaluationStage) (bool, error) {
	if existing, ok, err := l.evaluationForStage(ctx, version, stage); err != nil {
		return false, err
	} else if ok {
		return existing.Status == workstore.EvaluationStatusPassed, nil
	}
	result, runErr := testSkillCreatorDraft(ctx, l.workspaceDir, draft)
	if strings.TrimSpace(result.SandboxPath) != "" {
		_ = os.RemoveAll(result.SandboxPath)
	}
	status := workstore.EvaluationStatusPassed
	if runErr != nil || !result.Success {
		status = workstore.EvaluationStatusFailed
	}
	metrics := capabilityEvaluationMetrics{CostUSD: 0, LatencyMS: result.DurationMS}
	if result.Success {
		metrics.SuccessRate = 1
		metrics.VerificationRate = 1
	}
	report := map[string]any{
		"stage":                stage,
		"success":              result.Success,
		"exit_code":            result.ExitCode,
		"stdout":               result.Stdout,
		"stderr":               result.Stderr,
		"tool_trail":           result.ToolTrail,
		"permission_expansion": l.permissionExpansion(ctx, version),
		"content_diff":         l.capabilityDiff(ctx, version, draft),
	}
	if runErr != nil {
		report["error"] = runErr.Error()
	}
	if _, err := l.recordEvaluation(ctx, version, stage, status, metrics, report); err != nil {
		return false, err
	}
	return status == workstore.EvaluationStatusPassed, nil
}

func (l *skillCapabilityLifecycle) ensureOfflineEvaluation(ctx context.Context, version workstore.CapabilityVersion, draft skillCreatorDraftResponse) (bool, error) {
	if existing, ok, err := l.evaluationForStage(ctx, version, workstore.EvaluationStageOffline); err != nil {
		return false, err
	} else if ok {
		return existing.Status == workstore.EvaluationStatusPassed, nil
	}
	start := time.Now()
	err := validateCapabilityDraftOffline(draft)
	status := workstore.EvaluationStatusPassed
	if err != nil {
		status = workstore.EvaluationStatusFailed
	}
	metrics := capabilityEvaluationMetrics{CostUSD: 0, LatencyMS: time.Since(start).Milliseconds()}
	if err == nil {
		metrics.SuccessRate = 1
		metrics.VerificationRate = 1
	}
	report := map[string]any{
		"stage":                workstore.EvaluationStageOffline,
		"permission_expansion": l.permissionExpansion(ctx, version),
		"content_diff":         l.capabilityDiff(ctx, version, draft),
	}
	if err != nil {
		report["error"] = err.Error()
	}
	if _, recordErr := l.recordEvaluation(ctx, version, workstore.EvaluationStageOffline, status, metrics, report); recordErr != nil {
		return false, recordErr
	}
	return status == workstore.EvaluationStatusPassed, nil
}

func (l *skillCapabilityLifecycle) recordEvaluation(ctx context.Context, version workstore.CapabilityVersion, stage workstore.EvaluationStage, status workstore.EvaluationStatus, metrics capabilityEvaluationMetrics, report map[string]any) (workstore.EvaluationRun, error) {
	metricsJSON, _ := json.Marshal(metrics)
	deltaJSON, baselineID := l.evaluationDelta(ctx, version, stage, metrics)
	reportJSON, _ := json.Marshal(report)
	proofStatus := workstore.ProofStatusPassed
	if status == workstore.EvaluationStatusFailed {
		proofStatus = workstore.ProofStatusFailed
	}
	now := time.Now().UTC()
	proof, err := l.ledger.CreateProof(ctx, workstore.CreateProofInput{
		WorkspaceID:         version.WorkspaceID,
		WorkID:              version.WorkID,
		IdempotencyKey:      "capability:" + version.ID + ":" + string(stage) + ":proof",
		Kind:                "capability-" + string(stage),
		Status:              proofStatus,
		Origin:              workstore.ProofOriginIndependentVerifier,
		Summary:             fmt.Sprintf("%s evaluation %s", stage, status),
		VerifierID:          "capability-evaluator",
		Verifier:            "capability-evaluator",
		EnvironmentJSON:     json.RawMessage(`{"isolation":"skill-sandbox","network":"inherited"}`),
		InputJSON:           reportJSON,
		ArtifactDigestsJSON: json.RawMessage(`[]`),
		SubjectDigest:       version.ContentDigest,
		Rationale:           fmt.Sprintf("independent %s evaluation recorded", stage),
		ActorID:             "capability-evaluator",
		ObservedAt:          &now,
	})
	if err != nil {
		return workstore.EvaluationRun{}, err
	}
	return l.ledger.CreateEvaluationRun(ctx, workstore.CreateEvaluationRunInput{
		WorkspaceID:         version.WorkspaceID,
		WorkID:              version.WorkID,
		CapabilityVersionID: version.ID,
		IdempotencyKey:      "capability:" + version.ID + ":" + string(stage) + ":evaluation",
		Stage:               stage,
		Status:              status,
		BaselineVersionID:   baselineID,
		MetricsJSON:         metricsJSON,
		DeltaJSON:           deltaJSON,
		ReportJSON:          reportJSON,
		ProofID:             proof.ID,
		ActorID:             "capability-evaluator",
	})
}

func (l *skillCapabilityLifecycle) evaluationForStage(ctx context.Context, version workstore.CapabilityVersion, stage workstore.EvaluationStage) (workstore.EvaluationRun, bool, error) {
	runs, err := l.ledger.ListEvaluationRuns(ctx, version.WorkspaceID, version.ID)
	if err != nil {
		return workstore.EvaluationRun{}, false, err
	}
	for i := len(runs) - 1; i >= 0; i-- {
		if runs[i].Stage == stage {
			return runs[i], true, nil
		}
	}
	return workstore.EvaluationRun{}, false, nil
}

func (l *skillCapabilityLifecycle) evaluationDelta(ctx context.Context, version workstore.CapabilityVersion, stage workstore.EvaluationStage, current capabilityEvaluationMetrics) (json.RawMessage, string) {
	baseline, ok := l.baselineVersion(ctx, version)
	if !ok {
		raw, _ := json.Marshal(current)
		return raw, ""
	}
	runs, err := l.ledger.ListEvaluationRuns(ctx, baseline.WorkspaceID, baseline.ID)
	if err != nil {
		return json.RawMessage(`{}`), baseline.ID
	}
	for i := len(runs) - 1; i >= 0; i-- {
		if runs[i].Stage != stage {
			continue
		}
		var previous capabilityEvaluationMetrics
		if json.Unmarshal(runs[i].MetricsJSON, &previous) == nil {
			delta, _ := json.Marshal(capabilityEvaluationMetrics{
				SuccessRate:      current.SuccessRate - previous.SuccessRate,
				VerificationRate: current.VerificationRate - previous.VerificationRate,
				CostUSD:          current.CostUSD - previous.CostUSD,
				LatencyMS:        current.LatencyMS - previous.LatencyMS,
			})
			return delta, baseline.ID
		}
	}
	return json.RawMessage(`{}`), baseline.ID
}

func (l *skillCapabilityLifecycle) baselineVersion(ctx context.Context, version workstore.CapabilityVersion) (workstore.CapabilityVersion, bool) {
	if version.PreviousVersionID == "" {
		return workstore.CapabilityVersion{}, false
	}
	previous, err := l.ledger.GetCapabilityVersion(ctx, version.WorkspaceID, version.PreviousVersionID)
	return previous, err == nil
}

func (l *skillCapabilityLifecycle) permissionExpansion(ctx context.Context, version workstore.CapabilityVersion) []string {
	baseline, ok := l.baselineVersion(ctx, version)
	if !ok {
		var current []string
		_ = json.Unmarshal(version.PermissionsJSON, &current)
		return current
	}
	var current, previous []string
	_ = json.Unmarshal(version.PermissionsJSON, &current)
	_ = json.Unmarshal(baseline.PermissionsJSON, &previous)
	seen := make(map[string]struct{}, len(previous))
	for _, permission := range previous {
		seen[permission] = struct{}{}
	}
	expansion := make([]string, 0)
	for _, permission := range current {
		if _, ok := seen[permission]; !ok {
			expansion = append(expansion, permission)
		}
	}
	sort.Strings(expansion)
	return expansion
}

func (l *skillCapabilityLifecycle) capabilityDiff(ctx context.Context, version workstore.CapabilityVersion, draft skillCreatorDraftResponse) string {
	baseline, ok := l.baselineVersion(ctx, version)
	if !ok {
		return "new capability: " + draft.Name
	}
	previous, err := decodeCapabilityDraft(baseline.SnapshotJSON)
	if err != nil {
		return "baseline unavailable"
	}
	previousFiles := make(map[string]string, len(previous.Files))
	for _, file := range previous.Files {
		previousFiles[file.Path] = file.Content
	}
	changes := make([]string, 0)
	for _, file := range draft.Files {
		old, exists := previousFiles[file.Path]
		switch {
		case !exists:
			changes = append(changes, "+ "+file.Path)
		case old != file.Content:
			changes = append(changes, "~ "+file.Path)
		}
		delete(previousFiles, file.Path)
	}
	for path := range previousFiles {
		changes = append(changes, "- "+path)
	}
	sort.Strings(changes)
	if len(changes) == 0 {
		return "no content changes"
	}
	return strings.Join(changes, "\n")
}

func validateCapabilityDraftOffline(draft skillCreatorDraftResponse) error {
	files, err := cleanSkillCreatorDraftFiles(draft)
	if err != nil {
		return err
	}
	for _, file := range files {
		if file.Path != "SKILL.md" {
			continue
		}
		meta, _, err := skill.ParseFrontmatter(file.Content)
		if err != nil {
			return fmt.Errorf("parse SKILL.md: %w", err)
		}
		if strings.TrimSpace(meta.Name) != "" && !strings.EqualFold(meta.Name, draft.Name) {
			return fmt.Errorf("frontmatter name %q does not match draft %q", meta.Name, draft.Name)
		}
		return nil
	}
	return fmt.Errorf("SKILL.md is required")
}

func capabilityPermissions(draft skillCreatorDraftResponse) []string {
	seen := map[string]struct{}{}
	permissions := make([]string, 0, len(draft.RecommendedTools))
	for _, toolName := range draft.RecommendedTools {
		permission := "tool:" + strings.ToLower(strings.TrimSpace(toolName))
		if permission == "tool:" {
			continue
		}
		if _, ok := seen[permission]; ok {
			continue
		}
		seen[permission] = struct{}{}
		permissions = append(permissions, permission)
	}
	sort.Strings(permissions)
	return permissions
}

func decodeCapabilityDraft(raw json.RawMessage) (skillCreatorDraftResponse, error) {
	var draft skillCreatorDraftResponse
	if err := json.Unmarshal(raw, &draft); err != nil {
		return skillCreatorDraftResponse{}, fmt.Errorf("decode capability snapshot: %w", err)
	}
	if _, err := cleanSkillCreatorDraftFiles(draft); err != nil {
		return skillCreatorDraftResponse{}, err
	}
	return draft, nil
}

func loadLegacyCapabilityDraft(workspaceDir string, candidate skill.ExtractionCandidate) (skillCreatorDraftResponse, error) {
	skillsRoot, err := filepath.Abs(filepath.Join(workspaceDir, "skills"))
	if err != nil {
		return skillCreatorDraftResponse{}, err
	}
	draftPath, err := filepath.Abs(candidate.DraftPath)
	if err != nil {
		return skillCreatorDraftResponse{}, err
	}
	rel, err := filepath.Rel(skillsRoot, draftPath)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return skillCreatorDraftResponse{}, fmt.Errorf("legacy draft path is outside workspace skills")
	}
	files := make([]skillCreatorFile, 0)
	err = filepath.WalkDir(draftPath, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("legacy skill contains a symlink: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		if len(files) >= 100 {
			return fmt.Errorf("legacy skill has too many files")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > 1024*1024 {
			return fmt.Errorf("legacy skill file is unsupported: %s", path)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(draftPath, path)
		if err != nil {
			return err
		}
		files = append(files, skillCreatorFile{Path: filepath.ToSlash(relPath), Content: string(content)})
		return nil
	})
	if err != nil {
		return skillCreatorDraftResponse{}, err
	}
	name := firstNonEmptyString(candidate.DraftName, candidate.Name, filepath.Base(draftPath))
	draft := skillCreatorDraftResponse{
		Name:             name,
		Description:      firstNonEmptyString(candidate.Summary, candidate.Trigger),
		Category:         "legacy",
		Language:         "shell",
		Layout:           "single_file",
		UseCase:          firstNonEmptyString(candidate.UseCase, candidate.Summary),
		RecommendedTools: candidate.RecommendedTools,
		Files:            files,
	}
	if _, err := cleanSkillCreatorDraftFiles(draft); err != nil {
		return skillCreatorDraftResponse{}, err
	}
	return draft, nil
}

func replaceCapabilitySkill(workspaceDir string, draft skillCreatorDraftResponse, commit func() error) error {
	cleanFiles, err := cleanSkillCreatorDraftFiles(draft)
	if err != nil {
		return err
	}
	skillsRoot := filepath.Join(workspaceDir, "skills")
	if err := os.MkdirAll(skillsRoot, 0o755); err != nil {
		return fmt.Errorf("create skills root: %w", err)
	}
	stageDir, err := os.MkdirTemp(skillsRoot, ".tars-capability-stage-")
	if err != nil {
		return fmt.Errorf("create capability stage: %w", err)
	}
	defer func() { _ = os.RemoveAll(stageDir) }()
	if _, err := writeSkillCreatorDraftFiles(stageDir, cleanFiles); err != nil {
		return err
	}
	targetDir := filepath.Join(skillsRoot, draft.Name)
	backupDir := stageDir + "-backup"
	hadTarget := false
	if _, err := os.Stat(targetDir); err == nil {
		hadTarget = true
		if err := os.Rename(targetDir, backupDir); err != nil {
			return fmt.Errorf("stage existing capability for rollback: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect existing capability: %w", err)
	}
	restore := func() {
		_ = os.RemoveAll(targetDir)
		if hadTarget {
			_ = os.Rename(backupDir, targetDir)
		}
	}
	if err := os.Rename(stageDir, targetDir); err != nil {
		restore()
		return fmt.Errorf("activate capability: %w", err)
	}
	if err := commit(); err != nil {
		restore()
		return err
	}
	_ = os.RemoveAll(backupDir)
	return nil
}

func removeCapabilitySkill(workspaceDir, name string, commit func() error) error {
	if err := validateSkillCreatorName(name); err != nil {
		return err
	}
	skillsRoot := filepath.Join(workspaceDir, "skills")
	targetDir := filepath.Join(skillsRoot, name)
	backupDir, err := os.MkdirTemp(skillsRoot, ".tars-capability-rollback-")
	if err != nil {
		return fmt.Errorf("create rollback stage: %w", err)
	}
	if err := os.Remove(backupDir); err != nil {
		return fmt.Errorf("prepare rollback stage: %w", err)
	}
	if err := os.Rename(targetDir, backupDir); err != nil {
		return fmt.Errorf("stage capability removal: %w", err)
	}
	if err := commit(); err != nil {
		_ = os.Rename(backupDir, targetDir)
		return err
	}
	return os.RemoveAll(backupDir)
}

func savedCapabilityResponse(workspaceDir string, version workstore.CapabilityVersion, action skill.ExtractionCandidateAction) skillCreatorSaveResponse {
	if action != skill.ExtractionCandidateActionPromote || version.State != workstore.CapabilityStatePromoted {
		return skillCreatorSaveResponse{}
	}
	draft, err := decodeCapabilityDraft(version.SnapshotJSON)
	if err != nil {
		return skillCreatorSaveResponse{}
	}
	files := make([]string, 0, len(draft.Files))
	for _, file := range draft.Files {
		files = append(files, file.Path)
	}
	sort.Strings(files)
	return skillCreatorSaveResponse{Saved: true, Path: filepath.Join(workspaceDir, "skills", version.CapabilityName), Files: files}
}
