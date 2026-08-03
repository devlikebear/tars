package workstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *Store) CreateCapabilityVersion(ctx context.Context, input CreateCapabilityVersionInput) (CapabilityVersion, error) {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.WorkID) == "" || strings.TrimSpace(input.CandidateID) == "" || strings.TrimSpace(input.CapabilityName) == "" || strings.TrimSpace(input.ContentDigest) == "" || strings.TrimSpace(input.ActorID) == "" {
		return CapabilityVersion{}, fmt.Errorf("workstore: invalid capability version input")
	}
	state := input.InitialState
	if state == "" {
		state = CapabilityStateCandidate
	}
	if !validCapabilityState(state) {
		return CapabilityVersion{}, fmt.Errorf("workstore: invalid capability state %q", state)
	}
	snapshot, err := normalizedJSON(input.SnapshotJSON)
	if err != nil {
		return CapabilityVersion{}, fmt.Errorf("workstore: capability snapshot json: %w", err)
	}
	provenance, err := normalizedJSON(input.ProvenanceJSON)
	if err != nil {
		return CapabilityVersion{}, fmt.Errorf("workstore: capability provenance json: %w", err)
	}
	permissions := input.PermissionsJSON
	if len(permissions) == 0 {
		permissions = json.RawMessage(`[]`)
	}
	permissions, err = normalizedJSON(permissions)
	if err != nil {
		return CapabilityVersion{}, fmt.Errorf("workstore: capability permissions json: %w", err)
	}
	rollout, err := normalizedJSON(input.RolloutJSON)
	if err != nil {
		return CapabilityVersion{}, fmt.Errorf("workstore: capability rollout json: %w", err)
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CapabilityVersion{}, fmt.Errorf("workstore: begin create capability version: %w", err)
	}
	defer rollback(tx)
	if err := ensureWorkTx(ctx, tx, input.WorkspaceID, input.WorkID); err != nil {
		return CapabilityVersion{}, err
	}
	if existing, err := getCapabilityVersionByCandidateTx(ctx, tx, input.WorkspaceID, input.CandidateID); err == nil {
		if err := tx.Commit(); err != nil {
			return CapabilityVersion{}, fmt.Errorf("workstore: commit idempotent capability version: %w", err)
		}
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return CapabilityVersion{}, err
	}

	versionNumber := 1
	previousVersionID := ""
	var latestVersion int
	err = tx.QueryRowContext(ctx, `
		SELECT id, version
		FROM capability_versions
		WHERE workspace_id = ? AND capability_name = ?
		ORDER BY version DESC
		LIMIT 1
	`, input.WorkspaceID, input.CapabilityName).Scan(&previousVersionID, &latestVersion)
	if err == nil {
		versionNumber = latestVersion + 1
	} else if !errors.Is(err, sql.ErrNoRows) {
		return CapabilityVersion{}, fmt.Errorf("workstore: query previous capability version: %w", err)
	}
	rollbackTargetID := strings.TrimSpace(input.RollbackTargetID)
	if rollbackTargetID == "" {
		rollbackTargetID = previousVersionID
	}
	if rollbackTargetID != "" {
		if err := ensureCapabilityVersionTx(ctx, tx, input.WorkspaceID, rollbackTargetID); err != nil {
			return CapabilityVersion{}, fmt.Errorf("workstore: rollback target: %w", err)
		}
	}
	id, err := s.newID("cap")
	if err != nil {
		return CapabilityVersion{}, err
	}
	now := s.now().UTC()
	var promotedAt any
	if state == CapabilityStatePromoted {
		promotedAt = now.UnixMilli()
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO capability_versions (
			schema_version, id, workspace_id, work_id, candidate_id,
			capability_name, version, state, content_digest, snapshot_json,
			provenance_json, permissions_json, previous_version_id,
			rollback_target_id, rollout_json, actor_id, created_at, updated_at,
			promoted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, recordSchemaVersion, id, input.WorkspaceID, input.WorkID, input.CandidateID,
		input.CapabilityName, versionNumber, state, input.ContentDigest, snapshot,
		provenance, permissions, nullableString(previousVersionID), nullableString(rollbackTargetID),
		rollout, input.ActorID, now.UnixMilli(), now.UnixMilli(), promotedAt)
	if err != nil {
		return CapabilityVersion{}, fmt.Errorf("workstore: insert capability version: %w", err)
	}
	if err := s.insertEventTx(ctx, tx, eventInput{
		WorkspaceID: input.WorkspaceID,
		WorkID:      input.WorkID,
		Type:        EventTypeCapabilityVersionCreated,
		ActorID:     input.ActorID,
		PayloadJSON: mustJSON(map[string]any{
			"capability_version_id": id,
			"candidate_id":          input.CandidateID,
			"capability_name":       input.CapabilityName,
			"version":               versionNumber,
			"state":                 state,
		}),
		CreatedAt: now,
	}); err != nil {
		return CapabilityVersion{}, err
	}
	created, err := getCapabilityVersionTx(ctx, tx, input.WorkspaceID, id)
	if err != nil {
		return CapabilityVersion{}, err
	}
	if err := tx.Commit(); err != nil {
		return CapabilityVersion{}, fmt.Errorf("workstore: commit create capability version: %w", err)
	}
	return created, nil
}

func (s *Store) GetCapabilityVersion(ctx context.Context, workspaceID, versionID string) (CapabilityVersion, error) {
	version, err := scanCapabilityVersion(s.db.QueryRowContext(ctx, capabilityVersionSelect+" WHERE workspace_id = ? AND id = ?", workspaceID, versionID))
	if errors.Is(err, sql.ErrNoRows) {
		return CapabilityVersion{}, ErrNotFound
	}
	if err != nil {
		return CapabilityVersion{}, fmt.Errorf("workstore: get capability version: %w", err)
	}
	return version, nil
}

func (s *Store) GetCapabilityVersionByCandidate(ctx context.Context, workspaceID, candidateID string) (CapabilityVersion, error) {
	version, err := scanCapabilityVersion(s.db.QueryRowContext(ctx, capabilityVersionSelect+" WHERE workspace_id = ? AND candidate_id = ?", workspaceID, candidateID))
	if errors.Is(err, sql.ErrNoRows) {
		return CapabilityVersion{}, ErrNotFound
	}
	if err != nil {
		return CapabilityVersion{}, fmt.Errorf("workstore: get capability version by candidate: %w", err)
	}
	return version, nil
}

func (s *Store) ListCapabilityVersions(ctx context.Context, filter ListCapabilityVersionsFilter) ([]CapabilityVersion, error) {
	if strings.TrimSpace(filter.WorkspaceID) == "" {
		return nil, fmt.Errorf("workstore: workspace id is required")
	}
	query := capabilityVersionSelect + " WHERE workspace_id = ?"
	args := []any{filter.WorkspaceID}
	if name := strings.TrimSpace(filter.CapabilityName); name != "" {
		query += " AND capability_name = ?"
		args = append(args, name)
	}
	if candidateID := strings.TrimSpace(filter.CandidateID); candidateID != "" {
		query += " AND candidate_id = ?"
		args = append(args, candidateID)
	}
	if len(filter.States) > 0 {
		placeholders := make([]string, 0, len(filter.States))
		for _, state := range filter.States {
			if !validCapabilityState(state) {
				return nil, fmt.Errorf("workstore: invalid capability state %q", state)
			}
			placeholders = append(placeholders, "?")
			args = append(args, state)
		}
		query += " AND state IN (" + strings.Join(placeholders, ",") + ")"
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	query += " ORDER BY updated_at DESC, capability_name, version DESC LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("workstore: list capability versions: %w", err)
	}
	defer closeRows(rows)
	versions := make([]CapabilityVersion, 0)
	for rows.Next() {
		version, err := scanCapabilityVersion(rows)
		if err != nil {
			return nil, fmt.Errorf("workstore: scan capability version: %w", err)
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("workstore: iterate capability versions: %w", err)
	}
	return versions, nil
}

func (s *Store) TransitionCapabilityVersion(ctx context.Context, input TransitionCapabilityVersionInput) (CapabilityVersion, error) {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.VersionID) == "" || strings.TrimSpace(input.ActorID) == "" || !validCapabilityState(input.ToState) {
		return CapabilityVersion{}, fmt.Errorf("workstore: invalid capability transition input")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CapabilityVersion{}, fmt.Errorf("workstore: begin capability transition: %w", err)
	}
	defer rollback(tx)
	current, err := getCapabilityVersionTx(ctx, tx, input.WorkspaceID, input.VersionID)
	if err != nil {
		return CapabilityVersion{}, err
	}
	if current.State != input.ExpectedState {
		return CapabilityVersion{}, fmt.Errorf("%w: capability %s is %s, expected %s", ErrConflict, current.ID, current.State, input.ExpectedState)
	}
	if !canTransitionCapability(current.State, input.ToState) {
		return CapabilityVersion{}, fmt.Errorf("%w: capability %s to %s", ErrInvalidTransition, current.State, input.ToState)
	}
	approvalID := strings.TrimSpace(input.ApprovalID)
	if approvalID == "" {
		approvalID = current.ApprovalID
	}
	if err := validateCapabilityGateTx(ctx, tx, current, input.ToState, approvalID); err != nil {
		return CapabilityVersion{}, err
	}
	rollbackTargetID := strings.TrimSpace(input.RollbackTargetID)
	if rollbackTargetID == "" {
		rollbackTargetID = current.RollbackTargetID
	}
	if rollbackTargetID != "" {
		if err := ensureCapabilityVersionTx(ctx, tx, input.WorkspaceID, rollbackTargetID); err != nil {
			return CapabilityVersion{}, fmt.Errorf("workstore: rollback target: %w", err)
		}
	}
	rollout := current.RolloutJSON
	if len(input.RolloutJSON) > 0 {
		rollout, err = normalizedJSON(input.RolloutJSON)
		if err != nil {
			return CapabilityVersion{}, fmt.Errorf("workstore: capability rollout json: %w", err)
		}
	}
	now := s.now().UTC()
	promotedAt := nullableTime(current.PromotedAt)
	rolledBackAt := nullableTime(current.RolledBackAt)
	if input.ToState == CapabilityStatePromoted {
		promotedAt = now.UnixMilli()
	}
	if input.ToState == CapabilityStateRolledBack {
		rolledBackAt = now.UnixMilli()
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE capability_versions
		SET state = ?, approval_id = ?, rollback_target_id = ?, rollout_json = ?,
		    updated_at = ?, promoted_at = ?, rolled_back_at = ?
		WHERE workspace_id = ? AND id = ? AND state = ?
	`, input.ToState, nullableString(approvalID), nullableString(rollbackTargetID), rollout,
		now.UnixMilli(), promotedAt, rolledBackAt, input.WorkspaceID, input.VersionID, input.ExpectedState)
	if err != nil {
		return CapabilityVersion{}, fmt.Errorf("workstore: update capability state: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return CapabilityVersion{}, fmt.Errorf("workstore: inspect capability transition: %w", err)
	}
	if updated != 1 {
		return CapabilityVersion{}, ErrConflict
	}
	if err := s.insertEventTx(ctx, tx, eventInput{
		WorkspaceID: current.WorkspaceID,
		WorkID:      current.WorkID,
		Type:        EventTypeCapabilityTransitioned,
		ActorID:     input.ActorID,
		PayloadJSON: mustJSON(map[string]any{
			"capability_version_id": current.ID,
			"from_state":            current.State,
			"to_state":              input.ToState,
			"reason":                input.Reason,
		}),
		CreatedAt: now,
	}); err != nil {
		return CapabilityVersion{}, err
	}
	version, err := getCapabilityVersionTx(ctx, tx, input.WorkspaceID, input.VersionID)
	if err != nil {
		return CapabilityVersion{}, err
	}
	if err := tx.Commit(); err != nil {
		return CapabilityVersion{}, fmt.Errorf("workstore: commit capability transition: %w", err)
	}
	return version, nil
}

func (s *Store) CreateEvaluationRun(ctx context.Context, input CreateEvaluationRunInput) (EvaluationRun, error) {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.WorkID) == "" || strings.TrimSpace(input.CapabilityVersionID) == "" || strings.TrimSpace(input.IdempotencyKey) == "" || strings.TrimSpace(input.ActorID) == "" || !validEvaluationStage(input.Stage) || !validEvaluationStatus(input.Status) {
		return EvaluationRun{}, fmt.Errorf("workstore: invalid evaluation run input")
	}
	metrics, err := normalizedJSON(input.MetricsJSON)
	if err != nil {
		return EvaluationRun{}, fmt.Errorf("workstore: evaluation metrics json: %w", err)
	}
	delta, err := normalizedJSON(input.DeltaJSON)
	if err != nil {
		return EvaluationRun{}, fmt.Errorf("workstore: evaluation delta json: %w", err)
	}
	report, err := normalizedJSON(input.ReportJSON)
	if err != nil {
		return EvaluationRun{}, fmt.Errorf("workstore: evaluation report json: %w", err)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return EvaluationRun{}, fmt.Errorf("workstore: begin create evaluation: %w", err)
	}
	defer rollback(tx)
	version, err := getCapabilityVersionTx(ctx, tx, input.WorkspaceID, input.CapabilityVersionID)
	if err != nil {
		return EvaluationRun{}, err
	}
	if version.WorkID != input.WorkID {
		return EvaluationRun{}, fmt.Errorf("workstore: evaluation work does not match capability version")
	}
	if input.BaselineVersionID != "" {
		if err := ensureCapabilityVersionTx(ctx, tx, input.WorkspaceID, input.BaselineVersionID); err != nil {
			return EvaluationRun{}, fmt.Errorf("workstore: evaluation baseline: %w", err)
		}
	}
	if input.ProofID != "" {
		var proofWorkID string
		if err := tx.QueryRowContext(ctx, "SELECT work_id FROM proofs WHERE workspace_id = ? AND id = ?", input.WorkspaceID, input.ProofID).Scan(&proofWorkID); err != nil {
			return EvaluationRun{}, fmt.Errorf("workstore: evaluation proof: %w", err)
		}
		if proofWorkID != input.WorkID {
			return EvaluationRun{}, fmt.Errorf("workstore: evaluation proof work mismatch")
		}
	}
	id, err := s.newID("eval")
	if err != nil {
		return EvaluationRun{}, err
	}
	now := s.now().UTC()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO evaluation_runs (
			schema_version, id, workspace_id, work_id, capability_version_id,
			idempotency_key, stage, status, baseline_version_id, metrics_json,
			delta_json, report_json, proof_id, actor_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (workspace_id, capability_version_id, idempotency_key) DO NOTHING
	`, recordSchemaVersion, id, input.WorkspaceID, input.WorkID, input.CapabilityVersionID,
		input.IdempotencyKey, input.Stage, input.Status, nullableString(input.BaselineVersionID),
		metrics, delta, report, nullableString(input.ProofID), input.ActorID, now.UnixMilli(), now.UnixMilli())
	if err != nil {
		return EvaluationRun{}, fmt.Errorf("workstore: insert evaluation run: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return EvaluationRun{}, fmt.Errorf("workstore: inspect evaluation insert: %w", err)
	}
	if inserted == 1 {
		if err := s.insertEventTx(ctx, tx, eventInput{
			WorkspaceID: input.WorkspaceID,
			WorkID:      input.WorkID,
			Type:        EventTypeCapabilityEvaluationRecorded,
			ActorID:     input.ActorID,
			PayloadJSON: mustJSON(map[string]any{
				"capability_version_id": input.CapabilityVersionID,
				"evaluation_run_id":     id,
				"stage":                 input.Stage,
				"status":                input.Status,
			}),
			CreatedAt: now,
		}); err != nil {
			return EvaluationRun{}, err
		}
	}
	run, err := getEvaluationRunTx(ctx, tx, input.WorkspaceID, input.CapabilityVersionID, id, input.IdempotencyKey)
	if err != nil {
		return EvaluationRun{}, err
	}
	if err := tx.Commit(); err != nil {
		return EvaluationRun{}, fmt.Errorf("workstore: commit evaluation run: %w", err)
	}
	return run, nil
}

func (s *Store) ListEvaluationRuns(ctx context.Context, workspaceID, capabilityVersionID string) ([]EvaluationRun, error) {
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(capabilityVersionID) == "" {
		return nil, fmt.Errorf("workstore: workspace id and capability version id are required")
	}
	rows, err := s.db.QueryContext(ctx, evaluationRunSelect+" WHERE workspace_id = ? AND capability_version_id = ? ORDER BY created_at, id", workspaceID, capabilityVersionID)
	if err != nil {
		return nil, fmt.Errorf("workstore: list evaluation runs: %w", err)
	}
	defer closeRows(rows)
	runs := make([]EvaluationRun, 0)
	for rows.Next() {
		run, err := scanEvaluationRun(rows)
		if err != nil {
			return nil, fmt.Errorf("workstore: scan evaluation run: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("workstore: iterate evaluation runs: %w", err)
	}
	return runs, nil
}

func (s *Store) RecordCapabilityOutcome(ctx context.Context, input RecordCapabilityOutcomeInput) (CapabilityOutcome, error) {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.CapabilityVersionID) == "" || strings.TrimSpace(input.WorkID) == "" || strings.TrimSpace(input.IdempotencyKey) == "" || strings.TrimSpace(input.ActorID) == "" || !validCapabilityOutcomeStatus(input.Status) || !validProofStatus(input.VerifierStatus) || input.CostUSD < 0 || input.LatencyMS < 0 {
		return CapabilityOutcome{}, fmt.Errorf("workstore: invalid capability outcome input")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CapabilityOutcome{}, fmt.Errorf("workstore: begin capability outcome: %w", err)
	}
	defer rollback(tx)
	version, err := getCapabilityVersionTx(ctx, tx, input.WorkspaceID, input.CapabilityVersionID)
	if err != nil {
		return CapabilityOutcome{}, err
	}
	if err := ensureReferencesTx(ctx, tx, input.WorkspaceID, input.WorkID, "", input.AttemptID); err != nil {
		return CapabilityOutcome{}, err
	}
	id, err := s.newID("cout")
	if err != nil {
		return CapabilityOutcome{}, err
	}
	now := s.now().UTC()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO capability_outcomes (
			schema_version, id, workspace_id, capability_version_id, work_id,
			attempt_id, idempotency_key, status, verifier_status, cost_usd,
			latency_ms, actor_id, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (workspace_id, capability_version_id, idempotency_key) DO NOTHING
	`, recordSchemaVersion, id, input.WorkspaceID, input.CapabilityVersionID, input.WorkID,
		nullableString(input.AttemptID), input.IdempotencyKey, input.Status, input.VerifierStatus,
		input.CostUSD, input.LatencyMS, input.ActorID, now.UnixMilli())
	if err != nil {
		return CapabilityOutcome{}, fmt.Errorf("workstore: insert capability outcome: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return CapabilityOutcome{}, fmt.Errorf("workstore: inspect capability outcome insert: %w", err)
	}
	if inserted == 1 {
		if err := s.insertEventTx(ctx, tx, eventInput{
			WorkspaceID: input.WorkspaceID,
			WorkID:      input.WorkID,
			AttemptID:   input.AttemptID,
			Type:        EventTypeCapabilityOutcomeRecorded,
			ActorID:     input.ActorID,
			PayloadJSON: mustJSON(map[string]any{
				"capability_version_id": input.CapabilityVersionID,
				"outcome_id":            id,
				"status":                input.Status,
				"verifier_status":       input.VerifierStatus,
			}),
			CreatedAt: now,
		}); err != nil {
			return CapabilityOutcome{}, err
		}
		if version.State == CapabilityStatePromoted && capabilityOutcomeIsRegression(input) {
			rollout := map[string]any{}
			_ = json.Unmarshal(version.RolloutJSON, &rollout)
			rollout["review_required"] = true
			rollout["regression_detected"] = true
			rollout["regression_outcome_id"] = id
			rolloutJSON, marshalErr := json.Marshal(rollout)
			if marshalErr != nil {
				return CapabilityOutcome{}, fmt.Errorf("workstore: encode capability regression rollout: %w", marshalErr)
			}
			if _, updateErr := tx.ExecContext(ctx, `
				UPDATE capability_versions
				SET rollout_json = ?, updated_at = ?
				WHERE workspace_id = ? AND id = ?
			`, rolloutJSON, now.UnixMilli(), version.WorkspaceID, version.ID); updateErr != nil {
				return CapabilityOutcome{}, fmt.Errorf("workstore: flag capability regression: %w", updateErr)
			}
			if err := s.insertEventTx(ctx, tx, eventInput{
				WorkspaceID:    version.WorkspaceID,
				WorkID:         version.WorkID,
				Type:           EventTypeCapabilityRegressionDetected,
				ActorID:        input.ActorID,
				IdempotencyKey: "capability-regression:" + id,
				PayloadJSON: mustJSON(map[string]any{
					"capability_version_id": version.ID,
					"outcome_id":            id,
					"outcome_work_id":       input.WorkID,
					"status":                input.Status,
					"verifier_status":       input.VerifierStatus,
					"review_required":       true,
				}),
				CreatedAt: now,
			}); err != nil {
				return CapabilityOutcome{}, err
			}
		}
	}
	outcome, err := getCapabilityOutcomeTx(ctx, tx, input.WorkspaceID, input.CapabilityVersionID, id, input.IdempotencyKey)
	if err != nil {
		return CapabilityOutcome{}, err
	}
	if err := tx.Commit(); err != nil {
		return CapabilityOutcome{}, fmt.Errorf("workstore: commit capability outcome: %w", err)
	}
	return outcome, nil
}

func capabilityOutcomeIsRegression(input RecordCapabilityOutcomeInput) bool {
	return input.Status == CapabilityOutcomeFailed || input.VerifierStatus == ProofStatusFailed || input.VerifierStatus == ProofStatusStale
}

func (s *Store) ListCapabilityOutcomes(ctx context.Context, workspaceID, capabilityVersionID string) ([]CapabilityOutcome, error) {
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(capabilityVersionID) == "" {
		return nil, fmt.Errorf("workstore: workspace id and capability version id are required")
	}
	rows, err := s.db.QueryContext(ctx, capabilityOutcomeSelect+" WHERE workspace_id = ? AND capability_version_id = ? ORDER BY created_at, id", workspaceID, capabilityVersionID)
	if err != nil {
		return nil, fmt.Errorf("workstore: list capability outcomes: %w", err)
	}
	defer closeRows(rows)
	outcomes := make([]CapabilityOutcome, 0)
	for rows.Next() {
		outcome, err := scanCapabilityOutcome(rows)
		if err != nil {
			return nil, fmt.Errorf("workstore: scan capability outcome: %w", err)
		}
		outcomes = append(outcomes, outcome)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("workstore: iterate capability outcomes: %w", err)
	}
	return outcomes, nil
}

func validateCapabilityGateTx(ctx context.Context, tx *sql.Tx, current CapabilityVersion, to CapabilityState, approvalID string) error {
	requiredStage := EvaluationStage("")
	switch to {
	case CapabilityStateOfflineEval:
		requiredStage = EvaluationStageSandbox
	case CapabilityStateShadow:
		requiredStage = EvaluationStageOffline
	case CapabilityStateApproved:
		requiredStage = EvaluationStageShadow
	case CapabilityStatePromoted:
		requiredStage = EvaluationStageCanary
	}
	if requiredStage != "" {
		var status EvaluationStatus
		err := tx.QueryRowContext(ctx, `
			SELECT status
			FROM evaluation_runs
			WHERE workspace_id = ? AND capability_version_id = ? AND stage = ?
			ORDER BY created_at DESC, id DESC
			LIMIT 1
		`, current.WorkspaceID, current.ID, requiredStage).Scan(&status)
		if errors.Is(err, sql.ErrNoRows) || status != EvaluationStatusPassed {
			return fmt.Errorf("%w: %s evaluation must pass before %s", ErrCapabilityGate, requiredStage, to)
		}
		if err != nil {
			return fmt.Errorf("workstore: query capability evaluation gate: %w", err)
		}
	}
	if to == CapabilityStateApproved {
		if approvalID == "" {
			return fmt.Errorf("%w: approved human approval is required", ErrCapabilityGate)
		}
		var status ApprovalStatus
		var workID string
		if err := tx.QueryRowContext(ctx, "SELECT status, work_id FROM approvals WHERE workspace_id = ? AND id = ?", current.WorkspaceID, approvalID).Scan(&status, &workID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: approval record not found", ErrCapabilityGate)
			}
			return fmt.Errorf("workstore: query capability approval gate: %w", err)
		}
		if status != ApprovalStatusApproved || workID != current.WorkID {
			return fmt.Errorf("%w: approval must be approved and linked to capability work", ErrCapabilityGate)
		}
	}
	if (to == CapabilityStateCanary || to == CapabilityStatePromoted) && approvalID == "" {
		return fmt.Errorf("%w: capability approval is required before %s", ErrCapabilityGate, to)
	}
	return nil
}

func validCapabilityState(state CapabilityState) bool {
	switch state {
	case CapabilityStateCandidate, CapabilityStateDraft, CapabilityStateSandbox,
		CapabilityStateOfflineEval, CapabilityStateShadow, CapabilityStateApproved,
		CapabilityStateCanary, CapabilityStatePromoted, CapabilityStateRolledBack,
		CapabilityStateRejected:
		return true
	default:
		return false
	}
}

func canTransitionCapability(from, to CapabilityState) bool {
	if to == CapabilityStateRejected {
		return from != CapabilityStatePromoted && from != CapabilityStateRolledBack && from != CapabilityStateRejected
	}
	switch from {
	case CapabilityStateCandidate:
		return to == CapabilityStateDraft
	case CapabilityStateDraft:
		return to == CapabilityStateSandbox
	case CapabilityStateSandbox:
		return to == CapabilityStateOfflineEval
	case CapabilityStateOfflineEval:
		return to == CapabilityStateShadow
	case CapabilityStateShadow:
		return to == CapabilityStateApproved
	case CapabilityStateApproved:
		return to == CapabilityStateCanary
	case CapabilityStateCanary:
		return to == CapabilityStatePromoted
	case CapabilityStatePromoted:
		return to == CapabilityStateRolledBack
	default:
		return false
	}
}

func validEvaluationStage(stage EvaluationStage) bool {
	switch stage {
	case EvaluationStageSandbox, EvaluationStageOffline, EvaluationStageShadow, EvaluationStageCanary:
		return true
	default:
		return false
	}
}

func validEvaluationStatus(status EvaluationStatus) bool {
	switch status {
	case EvaluationStatusPending, EvaluationStatusPassed, EvaluationStatusFailed:
		return true
	default:
		return false
	}
}

func validCapabilityOutcomeStatus(status CapabilityOutcomeStatus) bool {
	switch status {
	case CapabilityOutcomeSucceeded, CapabilityOutcomeFailed, CapabilityOutcomeCancelled:
		return true
	default:
		return false
	}
}

func ensureCapabilityVersionTx(ctx context.Context, tx *sql.Tx, workspaceID, versionID string) error {
	var found string
	err := tx.QueryRowContext(ctx, "SELECT id FROM capability_versions WHERE workspace_id = ? AND id = ?", workspaceID, versionID).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("workstore: query capability version: %w", err)
	}
	return nil
}

const capabilityVersionSelect = `SELECT
    schema_version, id, workspace_id, work_id, candidate_id, capability_name,
    version, state, content_digest, snapshot_json, provenance_json,
    permissions_json, COALESCE(approval_id, ''), COALESCE(previous_version_id, ''),
    COALESCE(rollback_target_id, ''), rollout_json, actor_id, created_at,
    updated_at, promoted_at, rolled_back_at
FROM capability_versions`

func scanCapabilityVersion(row scanner) (CapabilityVersion, error) {
	var version CapabilityVersion
	var snapshot, provenance, permissions, rollout jsonValue
	var createdAt, updatedAt int64
	var promotedAt, rolledBackAt sql.NullInt64
	err := row.Scan(
		&version.SchemaVersion, &version.ID, &version.WorkspaceID, &version.WorkID,
		&version.CandidateID, &version.CapabilityName, &version.Version, &version.State,
		&version.ContentDigest, &snapshot, &provenance, &permissions, &version.ApprovalID,
		&version.PreviousVersionID, &version.RollbackTargetID, &rollout, &version.ActorID,
		&createdAt, &updatedAt, &promotedAt, &rolledBackAt,
	)
	if err != nil {
		return CapabilityVersion{}, err
	}
	version.SnapshotJSON = json.RawMessage(snapshot)
	version.ProvenanceJSON = json.RawMessage(provenance)
	version.PermissionsJSON = json.RawMessage(permissions)
	version.RolloutJSON = json.RawMessage(rollout)
	version.CreatedAt = time.UnixMilli(createdAt).UTC()
	version.UpdatedAt = time.UnixMilli(updatedAt).UTC()
	version.PromotedAt = timeFromNull(promotedAt)
	version.RolledBackAt = timeFromNull(rolledBackAt)
	return version, nil
}

func getCapabilityVersionTx(ctx context.Context, tx *sql.Tx, workspaceID, versionID string) (CapabilityVersion, error) {
	version, err := scanCapabilityVersion(tx.QueryRowContext(ctx, capabilityVersionSelect+" WHERE workspace_id = ? AND id = ?", workspaceID, versionID))
	if errors.Is(err, sql.ErrNoRows) {
		return CapabilityVersion{}, ErrNotFound
	}
	if err != nil {
		return CapabilityVersion{}, fmt.Errorf("workstore: query capability version: %w", err)
	}
	return version, nil
}

func getCapabilityVersionByCandidateTx(ctx context.Context, tx *sql.Tx, workspaceID, candidateID string) (CapabilityVersion, error) {
	version, err := scanCapabilityVersion(tx.QueryRowContext(ctx, capabilityVersionSelect+" WHERE workspace_id = ? AND candidate_id = ?", workspaceID, candidateID))
	if errors.Is(err, sql.ErrNoRows) {
		return CapabilityVersion{}, ErrNotFound
	}
	if err != nil {
		return CapabilityVersion{}, fmt.Errorf("workstore: query capability version by candidate: %w", err)
	}
	return version, nil
}

const evaluationRunSelect = `SELECT
    schema_version, id, workspace_id, work_id, capability_version_id,
    idempotency_key, stage, status, COALESCE(baseline_version_id, ''),
    metrics_json, delta_json, report_json, COALESCE(proof_id, ''), actor_id,
    created_at, updated_at
FROM evaluation_runs`

func scanEvaluationRun(row scanner) (EvaluationRun, error) {
	var run EvaluationRun
	var metrics, delta, report jsonValue
	var createdAt, updatedAt int64
	err := row.Scan(
		&run.SchemaVersion, &run.ID, &run.WorkspaceID, &run.WorkID,
		&run.CapabilityVersionID, &run.IdempotencyKey, &run.Stage, &run.Status,
		&run.BaselineVersionID, &metrics, &delta, &report, &run.ProofID,
		&run.ActorID, &createdAt, &updatedAt,
	)
	if err != nil {
		return EvaluationRun{}, err
	}
	run.MetricsJSON = json.RawMessage(metrics)
	run.DeltaJSON = json.RawMessage(delta)
	run.ReportJSON = json.RawMessage(report)
	run.CreatedAt = time.UnixMilli(createdAt).UTC()
	run.UpdatedAt = time.UnixMilli(updatedAt).UTC()
	return run, nil
}

func getEvaluationRunTx(ctx context.Context, tx *sql.Tx, workspaceID, versionID, runID, idempotencyKey string) (EvaluationRun, error) {
	query := evaluationRunSelect + " WHERE workspace_id = ? AND capability_version_id = ? AND id = ?"
	args := []any{workspaceID, versionID, runID}
	if idempotencyKey != "" {
		query = evaluationRunSelect + " WHERE workspace_id = ? AND capability_version_id = ? AND idempotency_key = ?"
		args = []any{workspaceID, versionID, idempotencyKey}
	}
	run, err := scanEvaluationRun(tx.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return EvaluationRun{}, ErrNotFound
	}
	if err != nil {
		return EvaluationRun{}, fmt.Errorf("workstore: query evaluation run: %w", err)
	}
	return run, nil
}

const capabilityOutcomeSelect = `SELECT
    schema_version, id, workspace_id, capability_version_id, work_id,
    COALESCE(attempt_id, ''), idempotency_key, status, verifier_status,
    cost_usd, latency_ms, actor_id, created_at
FROM capability_outcomes`

func scanCapabilityOutcome(row scanner) (CapabilityOutcome, error) {
	var outcome CapabilityOutcome
	var createdAt int64
	err := row.Scan(
		&outcome.SchemaVersion, &outcome.ID, &outcome.WorkspaceID,
		&outcome.CapabilityVersionID, &outcome.WorkID, &outcome.AttemptID,
		&outcome.IdempotencyKey, &outcome.Status, &outcome.VerifierStatus,
		&outcome.CostUSD, &outcome.LatencyMS, &outcome.ActorID, &createdAt,
	)
	if err != nil {
		return CapabilityOutcome{}, err
	}
	outcome.CreatedAt = time.UnixMilli(createdAt).UTC()
	return outcome, nil
}

func getCapabilityOutcomeTx(ctx context.Context, tx *sql.Tx, workspaceID, versionID, outcomeID, idempotencyKey string) (CapabilityOutcome, error) {
	query := capabilityOutcomeSelect + " WHERE workspace_id = ? AND capability_version_id = ? AND id = ?"
	args := []any{workspaceID, versionID, outcomeID}
	if idempotencyKey != "" {
		query = capabilityOutcomeSelect + " WHERE workspace_id = ? AND capability_version_id = ? AND idempotency_key = ?"
		args = []any{workspaceID, versionID, idempotencyKey}
	}
	outcome, err := scanCapabilityOutcome(tx.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return CapabilityOutcome{}, ErrNotFound
	}
	if err != nil {
		return CapabilityOutcome{}, fmt.Errorf("workstore: query capability outcome: %w", err)
	}
	return outcome, nil
}

func queryCapabilityVersions(ctx context.Context, tx *sql.Tx, workspaceID, workID string) ([]CapabilityVersion, error) {
	rows, err := tx.QueryContext(ctx, capabilityVersionSelect+" WHERE workspace_id = ? AND work_id = ? ORDER BY version, id", workspaceID, workID)
	if err != nil {
		return nil, fmt.Errorf("workstore: query projection capability versions: %w", err)
	}
	defer closeRows(rows)
	versions := make([]CapabilityVersion, 0)
	for rows.Next() {
		version, err := scanCapabilityVersion(rows)
		if err != nil {
			return nil, fmt.Errorf("workstore: scan projection capability version: %w", err)
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

func queryEvaluationRuns(ctx context.Context, tx *sql.Tx, workspaceID, workID string) ([]EvaluationRun, error) {
	rows, err := tx.QueryContext(ctx, evaluationRunSelect+" WHERE workspace_id = ? AND work_id = ? ORDER BY created_at, id", workspaceID, workID)
	if err != nil {
		return nil, fmt.Errorf("workstore: query projection evaluation runs: %w", err)
	}
	defer closeRows(rows)
	runs := make([]EvaluationRun, 0)
	for rows.Next() {
		run, err := scanEvaluationRun(rows)
		if err != nil {
			return nil, fmt.Errorf("workstore: scan projection evaluation run: %w", err)
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func queryCapabilityOutcomes(ctx context.Context, tx *sql.Tx, workspaceID, workID string) ([]CapabilityOutcome, error) {
	rows, err := tx.QueryContext(ctx, capabilityOutcomeSelect+" WHERE workspace_id = ? AND work_id = ? ORDER BY created_at, id", workspaceID, workID)
	if err != nil {
		return nil, fmt.Errorf("workstore: query projection capability outcomes: %w", err)
	}
	defer closeRows(rows)
	outcomes := make([]CapabilityOutcome, 0)
	for rows.Next() {
		outcome, err := scanCapabilityOutcome(rows)
		if err != nil {
			return nil, fmt.Errorf("workstore: scan projection capability outcome: %w", err)
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes, rows.Err()
}
