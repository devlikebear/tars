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

const defaultStepLeaseDuration = time.Minute

const stepScheduleSelect = `SELECT
    schema_version, workspace_id, work_id, step_id, policy_json, lease_owner,
    lease_expires_at, last_heartbeat_at, COALESCE(active_attempt_id, ''),
    attempt_count, cycle_attempt_count, consumed_iterations, consumed_tokens, consumed_cost_usd,
    next_action, last_disposition, blocked_reason, human_resume_required, updated_at
FROM step_schedules`

func (s *Store) ConfigureStepSchedule(ctx context.Context, input ConfigureStepScheduleInput) (StepSchedule, error) {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.WorkID) == "" || strings.TrimSpace(input.StepID) == "" || strings.TrimSpace(input.ActorID) == "" {
		return StepSchedule{}, fmt.Errorf("workstore: workspace, work, step, and actor are required for schedule configuration")
	}
	policy, err := normalizeStepSchedulePolicy(input.Policy)
	if err != nil {
		return StepSchedule{}, err
	}
	policyJSON, err := json.Marshal(policy)
	if err != nil {
		return StepSchedule{}, fmt.Errorf("workstore: encode step schedule policy: %w", err)
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return StepSchedule{}, fmt.Errorf("workstore: begin configure step schedule: %w", err)
	}
	defer rollback(tx)
	if _, err := getStepTx(ctx, tx, input.WorkspaceID, input.WorkID, input.StepID, ""); err != nil {
		return StepSchedule{}, err
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO step_schedules (
			schema_version, workspace_id, work_id, step_id, policy_json, next_action, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (step_id) DO UPDATE SET
			policy_json = excluded.policy_json,
			updated_at = excluded.updated_at
		WHERE step_schedules.workspace_id = excluded.workspace_id
			AND step_schedules.work_id = excluded.work_id
			AND step_schedules.lease_owner = ''
	`, recordSchemaVersion, input.WorkspaceID, input.WorkID, input.StepID, policyJSON, StepExecutionActionExecute, now.UnixMilli())
	if err != nil {
		return StepSchedule{}, fmt.Errorf("workstore: configure step schedule: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return StepSchedule{}, fmt.Errorf("workstore: inspect step schedule configuration: %w", err)
	}
	if changed == 0 {
		return StepSchedule{}, ErrClaimConflict
	}
	if err := s.insertEventTx(ctx, tx, eventInput{
		WorkspaceID: input.WorkspaceID,
		WorkID:      input.WorkID,
		StepID:      input.StepID,
		Type:        EventTypeStepScheduleConfigured,
		ActorID:     input.ActorID,
		PayloadJSON: policyJSON,
		CreatedAt:   now,
	}); err != nil {
		return StepSchedule{}, err
	}
	schedule, err := getStepScheduleTx(ctx, tx, input.WorkspaceID, input.WorkID, input.StepID)
	if err != nil {
		return StepSchedule{}, err
	}
	if err := tx.Commit(); err != nil {
		return StepSchedule{}, fmt.Errorf("workstore: commit step schedule configuration: %w", err)
	}
	return schedule, nil
}

func (s *Store) GetStepSchedule(ctx context.Context, workspaceID, workID, stepID string) (StepSchedule, error) {
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(workID) == "" || strings.TrimSpace(stepID) == "" {
		return StepSchedule{}, fmt.Errorf("workstore: workspace, work, and step are required for schedule lookup")
	}
	schedule, err := scanStepSchedule(s.db.QueryRowContext(ctx, stepScheduleSelect+" WHERE workspace_id = ? AND work_id = ? AND step_id = ?", workspaceID, workID, stepID))
	if errors.Is(err, sql.ErrNoRows) {
		return StepSchedule{}, ErrNotFound
	}
	if err != nil {
		return StepSchedule{}, fmt.Errorf("workstore: get step schedule: %w", err)
	}
	return schedule, nil
}

func (s *Store) PromoteReadySteps(ctx context.Context, input PromoteReadyStepsInput) ([]Step, error) {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.ActorID) == "" {
		return nil, fmt.Errorf("workstore: workspace and actor are required to promote ready steps")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("workstore: begin promote ready steps: %w", err)
	}
	defer rollback(tx)

	query := `
		SELECT s.id
		FROM steps s
		JOIN step_schedules schedule ON schedule.step_id = s.id
		WHERE s.workspace_id = ?
			AND s.state = ?
			AND schedule.human_resume_required = 0
			AND NOT EXISTS (
				SELECT 1
				FROM step_dependencies dependency
				JOIN steps prerequisite ON prerequisite.id = dependency.depends_on_step_id
				WHERE dependency.workspace_id = s.workspace_id
					AND dependency.work_id = s.work_id
					AND dependency.step_id = s.id
					AND prerequisite.state <> ?
			)
	`
	args := []any{input.WorkspaceID, WorkStateTodo, WorkStateDone}
	if strings.TrimSpace(input.WorkID) != "" {
		query += " AND s.work_id = ?"
		args = append(args, input.WorkID)
	}
	query += " ORDER BY s.position, s.created_at, s.id"
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("workstore: select ready step candidates: %w", err)
	}
	var stepIDs []string
	for rows.Next() {
		var stepID string
		if err := rows.Scan(&stepID); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("workstore: scan ready step candidate: %w", err)
		}
		stepIDs = append(stepIDs, stepID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("workstore: iterate ready step candidates: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("workstore: close ready step candidates: %w", err)
	}

	promoted := make([]Step, 0, len(stepIDs))
	for _, stepID := range stepIDs {
		step, err := getStepByIDTx(ctx, tx, input.WorkspaceID, stepID)
		if err != nil {
			return nil, err
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE steps
			SET state = ?, actor_id = ?, version = version + 1, updated_at = ?, completed_at = NULL
			WHERE workspace_id = ? AND work_id = ? AND id = ? AND state = ?
		`, WorkStateReady, input.ActorID, now.UnixMilli(), input.WorkspaceID, step.WorkID, step.ID, WorkStateTodo)
		if err != nil {
			return nil, fmt.Errorf("workstore: promote step ready: %w", err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("workstore: inspect ready promotion: %w", err)
		}
		if changed == 0 {
			continue
		}
		if err := s.insertEventTx(ctx, tx, eventInput{
			WorkspaceID: input.WorkspaceID,
			WorkID:      step.WorkID,
			StepID:      step.ID,
			Type:        EventTypeStepReady,
			FromState:   step.State,
			ToState:     WorkStateReady,
			ActorID:     input.ActorID,
			PayloadJSON: mustJSON(map[string]any{"reason": "dependencies_satisfied"}),
			CreatedAt:   now,
		}); err != nil {
			return nil, err
		}
		updated, err := getStepTx(ctx, tx, input.WorkspaceID, step.WorkID, step.ID, "")
		if err != nil {
			return nil, err
		}
		promoted = append(promoted, updated)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("workstore: commit ready step promotion: %w", err)
	}
	return promoted, nil
}

func (s *Store) ClaimReadyStep(ctx context.Context, input ClaimReadyStepInput) (StepClaim, error) {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.WorkerID) == "" || strings.TrimSpace(input.Adapter) == "" || strings.TrimSpace(input.ActorID) == "" {
		return StepClaim{}, fmt.Errorf("workstore: workspace, worker, adapter, and actor are required to claim a step")
	}
	leaseDuration := input.LeaseDuration
	if leaseDuration <= 0 {
		leaseDuration = defaultStepLeaseDuration
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	now := s.now().UTC()
	leaseExpiresAt := now.Add(leaseDuration)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return StepClaim{}, fmt.Errorf("workstore: begin claim ready step: %w", err)
	}
	defer rollback(tx)

	query := `
		SELECT s.id
		FROM steps s
		JOIN step_schedules schedule ON schedule.step_id = s.id
		WHERE s.workspace_id = ? AND s.state = ?
			AND schedule.lease_owner = ''
			AND schedule.human_resume_required = 0
	`
	args := []any{input.WorkspaceID, WorkStateReady}
	if strings.TrimSpace(input.WorkID) != "" {
		query += " AND s.work_id = ?"
		args = append(args, input.WorkID)
	}
	query += " ORDER BY s.position, s.created_at, s.id LIMIT 1"
	var stepID string
	if err := tx.QueryRowContext(ctx, query, args...).Scan(&stepID); errors.Is(err, sql.ErrNoRows) {
		return StepClaim{}, ErrNoReadyStep
	} else if err != nil {
		return StepClaim{}, fmt.Errorf("workstore: select claim candidate: %w", err)
	}
	step, err := getStepByIDTx(ctx, tx, input.WorkspaceID, stepID)
	if err != nil {
		return StepClaim{}, err
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE steps
		SET state = ?, actor_id = ?, version = version + 1, updated_at = ?, completed_at = NULL
		WHERE workspace_id = ? AND work_id = ? AND id = ? AND state = ?
	`, WorkStateRunning, input.ActorID, now.UnixMilli(), input.WorkspaceID, step.WorkID, step.ID, WorkStateReady)
	if err != nil {
		return StepClaim{}, fmt.Errorf("workstore: claim ready step state: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return StepClaim{}, fmt.Errorf("workstore: inspect ready step claim: %w", err)
	}
	if changed == 0 {
		return StepClaim{}, ErrNoReadyStep
	}
	result, err = tx.ExecContext(ctx, `
		UPDATE step_schedules
		SET lease_owner = ?, lease_expires_at = ?, last_heartbeat_at = ?,
			attempt_count = attempt_count + 1,
			cycle_attempt_count = cycle_attempt_count + 1, updated_at = ?
		WHERE workspace_id = ? AND work_id = ? AND step_id = ?
			AND lease_owner = '' AND human_resume_required = 0
	`, input.WorkerID, leaseExpiresAt.UnixMilli(), now.UnixMilli(), now.UnixMilli(), input.WorkspaceID, step.WorkID, step.ID)
	if err != nil {
		return StepClaim{}, fmt.Errorf("workstore: claim ready step lease: %w", err)
	}
	changed, err = result.RowsAffected()
	if err != nil {
		return StepClaim{}, fmt.Errorf("workstore: inspect ready step lease: %w", err)
	}
	if changed == 0 {
		return StepClaim{}, ErrClaimConflict
	}
	schedule, err := getStepScheduleTx(ctx, tx, input.WorkspaceID, step.WorkID, step.ID)
	if err != nil {
		return StepClaim{}, err
	}
	attemptID, err := s.newID("att")
	if err != nil {
		return StepClaim{}, err
	}
	attemptKey := fmt.Sprintf("scheduler:%s:attempt:%d", step.ID, schedule.AttemptCount)
	inputJSON := mustJSON(map[string]any{"next_action": schedule.NextAction, "worker_id": input.WorkerID})
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO attempts (
			schema_version, id, workspace_id, work_id, step_id, idempotency_key,
			causation_id, attempt_number, adapter, status, actor_id, input_json,
			output_json, error_text, started_at, finished_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '{}', '', ?, NULL, ?, ?)
	`, recordSchemaVersion, attemptID, input.WorkspaceID, step.WorkID, step.ID, attemptKey,
		"scheduler:"+step.ID, schedule.AttemptCount, input.Adapter, AttemptStatusRunning,
		input.WorkerID, inputJSON, now.UnixMilli(), now.UnixMilli(), now.UnixMilli()); err != nil {
		return StepClaim{}, fmt.Errorf("workstore: create claimed attempt: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE step_schedules SET active_attempt_id = ?, updated_at = ?
		WHERE workspace_id = ? AND work_id = ? AND step_id = ? AND lease_owner = ?
	`, attemptID, now.UnixMilli(), input.WorkspaceID, step.WorkID, step.ID, input.WorkerID); err != nil {
		return StepClaim{}, fmt.Errorf("workstore: attach claimed attempt: %w", err)
	}
	if err := s.insertEventTx(ctx, tx, eventInput{
		WorkspaceID: input.WorkspaceID,
		WorkID:      step.WorkID,
		StepID:      step.ID,
		AttemptID:   attemptID,
		Type:        EventTypeAttemptCreated,
		ActorID:     input.ActorID,
		PayloadJSON: mustJSON(map[string]any{"number": schedule.AttemptCount, "adapter": input.Adapter, "status": AttemptStatusRunning}),
		CreatedAt:   now,
	}); err != nil {
		return StepClaim{}, err
	}
	if err := s.insertEventTx(ctx, tx, eventInput{
		WorkspaceID: input.WorkspaceID,
		WorkID:      step.WorkID,
		StepID:      step.ID,
		AttemptID:   attemptID,
		Type:        EventTypeStepClaimed,
		FromState:   WorkStateReady,
		ToState:     WorkStateRunning,
		ActorID:     input.ActorID,
		PayloadJSON: mustJSON(map[string]any{
			"worker_id": input.WorkerID, "lease_expires_at": leaseExpiresAt,
			"attempt_number": schedule.AttemptCount, "cycle_attempt_number": schedule.CycleAttemptCount,
			"next_action": schedule.NextAction,
		}),
		CreatedAt: now,
	}); err != nil {
		return StepClaim{}, err
	}
	claimedStep, err := getStepTx(ctx, tx, input.WorkspaceID, step.WorkID, step.ID, "")
	if err != nil {
		return StepClaim{}, err
	}
	attempt, err := getAttemptTx(ctx, tx, input.WorkspaceID, step.WorkID, attemptID, "")
	if err != nil {
		return StepClaim{}, err
	}
	schedule, err = getStepScheduleTx(ctx, tx, input.WorkspaceID, step.WorkID, step.ID)
	if err != nil {
		return StepClaim{}, err
	}
	if err := tx.Commit(); err != nil {
		return StepClaim{}, fmt.Errorf("workstore: commit ready step claim: %w", err)
	}
	return StepClaim{Step: claimedStep, Attempt: attempt, Schedule: schedule}, nil
}

func (s *Store) HeartbeatStepClaim(ctx context.Context, input HeartbeatStepClaimInput) (StepSchedule, error) {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.WorkID) == "" || strings.TrimSpace(input.StepID) == "" || strings.TrimSpace(input.AttemptID) == "" || strings.TrimSpace(input.WorkerID) == "" || strings.TrimSpace(input.ActorID) == "" {
		return StepSchedule{}, fmt.Errorf("workstore: complete claim identity is required for heartbeat")
	}
	leaseDuration := input.LeaseDuration
	if leaseDuration <= 0 {
		leaseDuration = defaultStepLeaseDuration
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	now := s.now().UTC()
	leaseExpiresAt := now.Add(leaseDuration)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return StepSchedule{}, fmt.Errorf("workstore: begin step heartbeat: %w", err)
	}
	defer rollback(tx)
	schedule, err := getStepScheduleTx(ctx, tx, input.WorkspaceID, input.WorkID, input.StepID)
	if err != nil {
		return StepSchedule{}, err
	}
	if schedule.LeaseOwner != input.WorkerID || schedule.ActiveAttemptID != input.AttemptID {
		return StepSchedule{}, ErrClaimConflict
	}
	if schedule.LeaseExpiresAt == nil || !schedule.LeaseExpiresAt.After(now) {
		return StepSchedule{}, ErrClaimExpired
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE step_schedules
		SET lease_expires_at = ?, last_heartbeat_at = ?, updated_at = ?
		WHERE workspace_id = ? AND work_id = ? AND step_id = ?
			AND lease_owner = ? AND active_attempt_id = ? AND lease_expires_at > ?
	`, leaseExpiresAt.UnixMilli(), now.UnixMilli(), now.UnixMilli(), input.WorkspaceID,
		input.WorkID, input.StepID, input.WorkerID, input.AttemptID, now.UnixMilli())
	if err != nil {
		return StepSchedule{}, fmt.Errorf("workstore: heartbeat step claim: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return StepSchedule{}, fmt.Errorf("workstore: inspect step heartbeat: %w", err)
	}
	if changed == 0 {
		return StepSchedule{}, ErrClaimConflict
	}
	if err := s.insertEventTx(ctx, tx, eventInput{
		WorkspaceID: input.WorkspaceID,
		WorkID:      input.WorkID,
		StepID:      input.StepID,
		AttemptID:   input.AttemptID,
		Type:        EventTypeStepHeartbeat,
		ActorID:     input.ActorID,
		PayloadJSON: mustJSON(map[string]any{"worker_id": input.WorkerID, "lease_expires_at": leaseExpiresAt}),
		CreatedAt:   now,
	}); err != nil {
		return StepSchedule{}, err
	}
	schedule, err = getStepScheduleTx(ctx, tx, input.WorkspaceID, input.WorkID, input.StepID)
	if err != nil {
		return StepSchedule{}, err
	}
	if err := tx.Commit(); err != nil {
		return StepSchedule{}, fmt.Errorf("workstore: commit step heartbeat: %w", err)
	}
	return schedule, nil
}

func (s *Store) ReleaseStepClaim(ctx context.Context, input ReleaseStepClaimInput) (StepResolution, error) {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.WorkID) == "" || strings.TrimSpace(input.StepID) == "" || strings.TrimSpace(input.AttemptID) == "" || strings.TrimSpace(input.WorkerID) == "" || strings.TrimSpace(input.ActorID) == "" || strings.TrimSpace(input.Reason) == "" {
		return StepResolution{}, fmt.Errorf("workstore: workspace, work, step, attempt, worker, actor, and reason are required to release a claim")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return StepResolution{}, fmt.Errorf("workstore: begin release step claim: %w", err)
	}
	defer rollback(tx)
	schedule, err := getStepScheduleTx(ctx, tx, input.WorkspaceID, input.WorkID, input.StepID)
	if err != nil {
		return StepResolution{}, err
	}
	if schedule.LeaseOwner != input.WorkerID || schedule.ActiveAttemptID != input.AttemptID {
		return StepResolution{}, ErrClaimConflict
	}
	if schedule.LeaseExpiresAt == nil || !schedule.LeaseExpiresAt.After(now) {
		return StepResolution{}, ErrClaimExpired
	}
	step, err := getStepTx(ctx, tx, input.WorkspaceID, input.WorkID, input.StepID, "")
	if err != nil {
		return StepResolution{}, err
	}
	attempt, err := getAttemptTx(ctx, tx, input.WorkspaceID, input.WorkID, input.AttemptID, "")
	if err != nil {
		return StepResolution{}, err
	}
	if step.State != WorkStateRunning || attempt.Status != AttemptStatusRunning {
		return StepResolution{}, ErrClaimConflict
	}
	if err := execOne(ctx, tx, `
		UPDATE attempts
		SET status = ?, error_text = ?, finished_at = ?, updated_at = ?
		WHERE workspace_id = ? AND work_id = ? AND id = ? AND status = ?
	`, "release scheduled attempt", AttemptStatusCancelled, input.Reason, now.UnixMilli(), now.UnixMilli(),
		input.WorkspaceID, input.WorkID, input.AttemptID, AttemptStatusRunning); err != nil {
		return StepResolution{}, err
	}
	if err := execOne(ctx, tx, `
		UPDATE steps
		SET state = ?, actor_id = ?, version = version + 1, updated_at = ?, completed_at = NULL
		WHERE workspace_id = ? AND work_id = ? AND id = ? AND state = ?
	`, "release scheduled step", WorkStateReady, input.ActorID, now.UnixMilli(), input.WorkspaceID,
		input.WorkID, input.StepID, WorkStateRunning); err != nil {
		return StepResolution{}, err
	}
	if err := execOne(ctx, tx, `
		UPDATE step_schedules
		SET lease_owner = '', lease_expires_at = NULL, last_heartbeat_at = NULL,
			active_attempt_id = NULL,
			cycle_attempt_count = CASE WHEN cycle_attempt_count > 0 THEN cycle_attempt_count - 1 ELSE 0 END,
			updated_at = ?
		WHERE workspace_id = ? AND work_id = ? AND step_id = ?
			AND lease_owner = ? AND active_attempt_id = ?
	`, "release step schedule", now.UnixMilli(), input.WorkspaceID, input.WorkID, input.StepID,
		input.WorkerID, input.AttemptID); err != nil {
		return StepResolution{}, err
	}
	if err := s.insertEventTx(ctx, tx, eventInput{
		WorkspaceID: input.WorkspaceID, WorkID: input.WorkID, StepID: input.StepID,
		AttemptID: input.AttemptID, Type: EventTypeAttemptCompleted, ActorID: input.ActorID,
		PayloadJSON: mustJSON(map[string]any{"status": AttemptStatusCancelled, "reason": input.Reason, "released": true}),
		CreatedAt:   now,
	}); err != nil {
		return StepResolution{}, err
	}
	if err := s.insertEventTx(ctx, tx, eventInput{
		WorkspaceID: input.WorkspaceID, WorkID: input.WorkID, StepID: input.StepID,
		AttemptID: input.AttemptID, Type: EventTypeStepReleased,
		FromState: WorkStateRunning, ToState: WorkStateReady, ActorID: input.ActorID,
		PayloadJSON: mustJSON(map[string]any{"worker_id": input.WorkerID, "reason": input.Reason}), CreatedAt: now,
	}); err != nil {
		return StepResolution{}, err
	}
	step, err = getStepTx(ctx, tx, input.WorkspaceID, input.WorkID, input.StepID, "")
	if err != nil {
		return StepResolution{}, err
	}
	attempt, err = getAttemptTx(ctx, tx, input.WorkspaceID, input.WorkID, input.AttemptID, "")
	if err != nil {
		return StepResolution{}, err
	}
	schedule, err = getStepScheduleTx(ctx, tx, input.WorkspaceID, input.WorkID, input.StepID)
	if err != nil {
		return StepResolution{}, err
	}
	if err := tx.Commit(); err != nil {
		return StepResolution{}, fmt.Errorf("workstore: commit release step claim: %w", err)
	}
	return StepResolution{Step: step, Attempt: attempt, Schedule: schedule}, nil
}

func (s *Store) CompleteStepAttempt(ctx context.Context, input CompleteStepAttemptInput) (StepResolution, error) {
	if err := validateCompleteStepAttemptInput(input); err != nil {
		return StepResolution{}, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return StepResolution{}, fmt.Errorf("workstore: begin complete step attempt: %w", err)
	}
	defer rollback(tx)
	resolution, err := s.resolveStepClaimTx(ctx, tx, input, false, false, s.now().UTC())
	if err != nil {
		return StepResolution{}, err
	}
	if err := tx.Commit(); err != nil {
		return StepResolution{}, fmt.Errorf("workstore: commit step attempt completion: %w", err)
	}
	return resolution, nil
}

func (s *Store) ReclaimExpiredStepClaims(ctx context.Context, input ReclaimExpiredStepClaimsInput) ([]StepResolution, error) {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.ActorID) == "" {
		return nil, fmt.Errorf("workstore: workspace and actor are required to reclaim step claims")
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		reason = "step lease expired"
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("workstore: begin reclaim expired claims: %w", err)
	}
	defer rollback(tx)
	query := `
		SELECT work_id, step_id, lease_owner, COALESCE(active_attempt_id, '')
		FROM step_schedules
		WHERE workspace_id = ? AND lease_owner <> '' AND lease_expires_at <= ?
	`
	args := []any{input.WorkspaceID, now.UnixMilli()}
	if strings.TrimSpace(input.WorkID) != "" {
		query += " AND work_id = ?"
		args = append(args, input.WorkID)
	}
	query += " ORDER BY lease_expires_at, step_id"
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("workstore: list expired step claims: %w", err)
	}
	type expiredClaim struct {
		workID, stepID, workerID, attemptID string
	}
	var expired []expiredClaim
	for rows.Next() {
		var claim expiredClaim
		if err := rows.Scan(&claim.workID, &claim.stepID, &claim.workerID, &claim.attemptID); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("workstore: scan expired step claim: %w", err)
		}
		expired = append(expired, claim)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("workstore: iterate expired step claims: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("workstore: close expired step claims: %w", err)
	}

	resolutions := make([]StepResolution, 0, len(expired))
	for _, claim := range expired {
		resolution, err := s.resolveStepClaimTx(ctx, tx, CompleteStepAttemptInput{
			WorkspaceID: input.WorkspaceID,
			WorkID:      claim.workID,
			StepID:      claim.stepID,
			AttemptID:   claim.attemptID,
			WorkerID:    claim.workerID,
			Succeeded:   false,
			ErrorText:   reason,
			ActorID:     input.ActorID,
		}, true, true, now)
		if err != nil {
			return nil, err
		}
		resolutions = append(resolutions, resolution)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("workstore: commit expired step reclaim: %w", err)
	}
	return resolutions, nil
}

func (s *Store) ResumeScheduledStep(ctx context.Context, input ResumeScheduledStepInput) (StepResolution, error) {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.WorkID) == "" || strings.TrimSpace(input.StepID) == "" || strings.TrimSpace(input.ActorID) == "" || strings.TrimSpace(input.Reason) == "" {
		return StepResolution{}, fmt.Errorf("workstore: workspace, work, step, actor, and reason are required to resume a step")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return StepResolution{}, fmt.Errorf("workstore: begin resume scheduled step: %w", err)
	}
	defer rollback(tx)
	step, err := getStepTx(ctx, tx, input.WorkspaceID, input.WorkID, input.StepID, "")
	if err != nil {
		return StepResolution{}, err
	}
	schedule, err := getStepScheduleTx(ctx, tx, input.WorkspaceID, input.WorkID, input.StepID)
	if err != nil {
		return StepResolution{}, err
	}
	if (step.State != WorkStateBlocked && step.State != WorkStateReview) || !schedule.HumanResumeRequired {
		return StepResolution{}, ErrInvalidTransition
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE steps SET state = ?, actor_id = ?, version = version + 1, updated_at = ?, completed_at = NULL
		WHERE workspace_id = ? AND work_id = ? AND id = ? AND state IN (?, ?)
	`, WorkStateReady, input.ActorID, now.UnixMilli(), input.WorkspaceID, input.WorkID,
		input.StepID, WorkStateBlocked, WorkStateReview); err != nil {
		return StepResolution{}, fmt.Errorf("workstore: resume scheduled step state: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE step_schedules
		SET lease_owner = '', lease_expires_at = NULL, last_heartbeat_at = NULL,
			active_attempt_id = NULL, cycle_attempt_count = 0, consumed_iterations = 0,
			consumed_tokens = 0, consumed_cost_usd = 0, next_action = ?,
			last_disposition = '', blocked_reason = '', human_resume_required = 0,
			updated_at = ?
		WHERE workspace_id = ? AND work_id = ? AND step_id = ?
	`, StepExecutionActionExecute, now.UnixMilli(), input.WorkspaceID, input.WorkID, input.StepID); err != nil {
		return StepResolution{}, fmt.Errorf("workstore: reset resumed step schedule: %w", err)
	}
	if err := s.insertEventTx(ctx, tx, eventInput{
		WorkspaceID: input.WorkspaceID,
		WorkID:      input.WorkID,
		StepID:      input.StepID,
		Type:        EventTypeStepResumed,
		FromState:   step.State,
		ToState:     WorkStateReady,
		ActorID:     input.ActorID,
		PayloadJSON: mustJSON(map[string]any{"reason": input.Reason}),
		CreatedAt:   now,
	}); err != nil {
		return StepResolution{}, err
	}
	step, err = getStepTx(ctx, tx, input.WorkspaceID, input.WorkID, input.StepID, "")
	if err != nil {
		return StepResolution{}, err
	}
	schedule, err = getStepScheduleTx(ctx, tx, input.WorkspaceID, input.WorkID, input.StepID)
	if err != nil {
		return StepResolution{}, err
	}
	if err := tx.Commit(); err != nil {
		return StepResolution{}, fmt.Errorf("workstore: commit scheduled step resume: %w", err)
	}
	return StepResolution{Step: step, Schedule: schedule}, nil
}

func (s *Store) CancelScheduledStep(ctx context.Context, input CancelScheduledStepInput) (StepResolution, error) {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.WorkID) == "" || strings.TrimSpace(input.StepID) == "" || strings.TrimSpace(input.ActorID) == "" || strings.TrimSpace(input.Reason) == "" {
		return StepResolution{}, fmt.Errorf("workstore: workspace, work, step, actor, and reason are required to cancel a scheduled step")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return StepResolution{}, fmt.Errorf("workstore: begin cancel scheduled step: %w", err)
	}
	defer rollback(tx)
	step, err := getStepTx(ctx, tx, input.WorkspaceID, input.WorkID, input.StepID, "")
	if err != nil {
		return StepResolution{}, err
	}
	schedule, err := getStepScheduleTx(ctx, tx, input.WorkspaceID, input.WorkID, input.StepID)
	if err != nil {
		return StepResolution{}, err
	}
	if step.State == WorkStateCancelled {
		return StepResolution{Step: step, Schedule: schedule}, nil
	}
	if step.State == WorkStateDone || !canTransition(step.State, WorkStateCancelled) {
		return StepResolution{}, ErrInvalidTransition
	}
	var attempt Attempt
	if schedule.ActiveAttemptID != "" {
		attempt, err = getAttemptTx(ctx, tx, input.WorkspaceID, input.WorkID, schedule.ActiveAttemptID, "")
		if err != nil {
			return StepResolution{}, err
		}
		if attempt.Status != AttemptStatusRunning {
			return StepResolution{}, ErrClaimConflict
		}
		if err := execOne(ctx, tx, `
			UPDATE attempts SET status = ?, error_text = ?, finished_at = ?, updated_at = ?
			WHERE workspace_id = ? AND work_id = ? AND id = ? AND status = ?
		`, "cancel scheduled attempt", AttemptStatusCancelled, input.Reason, now.UnixMilli(), now.UnixMilli(),
			input.WorkspaceID, input.WorkID, attempt.ID, AttemptStatusRunning); err != nil {
			return StepResolution{}, err
		}
		if err := s.insertEventTx(ctx, tx, eventInput{
			WorkspaceID: input.WorkspaceID, WorkID: input.WorkID, StepID: input.StepID,
			AttemptID: attempt.ID, Type: EventTypeAttemptCompleted, ActorID: input.ActorID,
			PayloadJSON: mustJSON(map[string]any{"status": AttemptStatusCancelled, "reason": input.Reason}), CreatedAt: now,
		}); err != nil {
			return StepResolution{}, err
		}
	} else if step.State == WorkStateRunning {
		return StepResolution{}, ErrClaimConflict
	}
	if err := execOne(ctx, tx, `
		UPDATE steps SET state = ?, actor_id = ?, version = version + 1, updated_at = ?, completed_at = ?
		WHERE workspace_id = ? AND work_id = ? AND id = ? AND state = ?
	`, "cancel scheduled step", WorkStateCancelled, input.ActorID, now.UnixMilli(), now.UnixMilli(),
		input.WorkspaceID, input.WorkID, input.StepID, step.State); err != nil {
		return StepResolution{}, err
	}
	if err := execOne(ctx, tx, `
		UPDATE step_schedules
		SET lease_owner = '', lease_expires_at = NULL, last_heartbeat_at = NULL,
			active_attempt_id = NULL, blocked_reason = ?, human_resume_required = 0, updated_at = ?
		WHERE workspace_id = ? AND work_id = ? AND step_id = ?
	`, "cancel step schedule", input.Reason, now.UnixMilli(), input.WorkspaceID, input.WorkID, input.StepID); err != nil {
		return StepResolution{}, err
	}
	if err := s.insertEventTx(ctx, tx, eventInput{
		WorkspaceID: input.WorkspaceID, WorkID: input.WorkID, StepID: input.StepID,
		AttemptID: schedule.ActiveAttemptID, Type: EventTypeStepCancelled,
		FromState: step.State, ToState: WorkStateCancelled, ActorID: input.ActorID,
		PayloadJSON: mustJSON(map[string]any{"reason": input.Reason}), CreatedAt: now,
	}); err != nil {
		return StepResolution{}, err
	}
	step, err = getStepTx(ctx, tx, input.WorkspaceID, input.WorkID, input.StepID, "")
	if err != nil {
		return StepResolution{}, err
	}
	if attempt.ID != "" {
		attempt, err = getAttemptTx(ctx, tx, input.WorkspaceID, input.WorkID, attempt.ID, "")
		if err != nil {
			return StepResolution{}, err
		}
	}
	schedule, err = getStepScheduleTx(ctx, tx, input.WorkspaceID, input.WorkID, input.StepID)
	if err != nil {
		return StepResolution{}, err
	}
	if err := tx.Commit(); err != nil {
		return StepResolution{}, fmt.Errorf("workstore: commit cancel scheduled step: %w", err)
	}
	return StepResolution{Step: step, Attempt: attempt, Schedule: schedule}, nil
}

func (s *Store) resolveStepClaimTx(ctx context.Context, tx *sql.Tx, input CompleteStepAttemptInput, allowExpired, reclaimed bool, now time.Time) (StepResolution, error) {
	schedule, err := getStepScheduleTx(ctx, tx, input.WorkspaceID, input.WorkID, input.StepID)
	if err != nil {
		return StepResolution{}, err
	}
	if schedule.LeaseOwner != input.WorkerID || schedule.ActiveAttemptID != input.AttemptID {
		return StepResolution{}, ErrClaimConflict
	}
	if !allowExpired && (schedule.LeaseExpiresAt == nil || !schedule.LeaseExpiresAt.After(now)) {
		return StepResolution{}, ErrClaimExpired
	}
	step, err := getStepTx(ctx, tx, input.WorkspaceID, input.WorkID, input.StepID, "")
	if err != nil {
		return StepResolution{}, err
	}
	if step.State != WorkStateRunning {
		return StepResolution{}, ErrClaimConflict
	}
	attempt, err := getAttemptTx(ctx, tx, input.WorkspaceID, input.WorkID, input.AttemptID, "")
	if err != nil {
		return StepResolution{}, err
	}
	if attempt.Status != AttemptStatusRunning {
		return StepResolution{}, ErrClaimConflict
	}
	outputJSON, err := normalizedJSON(input.OutputJSON)
	if err != nil {
		return StepResolution{}, fmt.Errorf("workstore: completed attempt output: %w", err)
	}
	attemptStatus := AttemptStatusFailed
	if input.Succeeded {
		attemptStatus = AttemptStatusSucceeded
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE attempts
		SET status = ?, output_json = ?, error_text = ?, finished_at = ?, updated_at = ?
		WHERE workspace_id = ? AND work_id = ? AND id = ? AND status = ?
	`, attemptStatus, outputJSON, strings.TrimSpace(input.ErrorText), now.UnixMilli(), now.UnixMilli(),
		input.WorkspaceID, input.WorkID, input.AttemptID, AttemptStatusRunning); err != nil {
		return StepResolution{}, fmt.Errorf("workstore: finish scheduled attempt: %w", err)
	}

	consumedIterations := schedule.ConsumedIterations + input.Usage.Iterations
	consumedTokens := schedule.ConsumedTokens + input.Usage.Tokens
	consumedCost := schedule.ConsumedCostUSD + input.Usage.CostUSD
	disposition := StepDispositionDone
	nextAction := StepExecutionActionExecute
	nextState := WorkStateDone
	humanResume := false
	blockedReason := ""
	if !input.Succeeded {
		disposition = chooseFailureDisposition(schedule.Policy, schedule.CycleAttemptCount, consumedIterations, consumedTokens, consumedCost)
		nextAction, nextState, humanResume = dispositionOutcome(disposition)
		if humanResume {
			blockedReason = strings.TrimSpace(input.ErrorText)
			if blockedReason == "" {
				blockedReason = "scheduled attempt failed"
			}
		}
	}
	completedAt := any(nil)
	if nextState == WorkStateDone || nextState == WorkStateCancelled {
		completedAt = now.UnixMilli()
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE steps
		SET state = ?, actor_id = ?, version = version + 1, updated_at = ?, completed_at = ?
		WHERE workspace_id = ? AND work_id = ? AND id = ? AND state = ?
	`, nextState, input.ActorID, now.UnixMilli(), completedAt, input.WorkspaceID,
		input.WorkID, input.StepID, WorkStateRunning); err != nil {
		return StepResolution{}, fmt.Errorf("workstore: resolve scheduled step state: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE step_schedules
		SET lease_owner = '', lease_expires_at = NULL, last_heartbeat_at = NULL,
			active_attempt_id = NULL, consumed_iterations = ?, consumed_tokens = ?,
			consumed_cost_usd = ?, next_action = ?, last_disposition = ?,
			blocked_reason = ?, human_resume_required = ?, updated_at = ?
		WHERE workspace_id = ? AND work_id = ? AND step_id = ?
	`, consumedIterations, consumedTokens, consumedCost, nextAction, disposition,
		blockedReason, boolInt(humanResume), now.UnixMilli(), input.WorkspaceID, input.WorkID, input.StepID); err != nil {
		return StepResolution{}, fmt.Errorf("workstore: resolve scheduled step lease: %w", err)
	}
	if err := s.insertEventTx(ctx, tx, eventInput{
		WorkspaceID: input.WorkspaceID,
		WorkID:      input.WorkID,
		StepID:      input.StepID,
		AttemptID:   input.AttemptID,
		Type:        EventTypeAttemptCompleted,
		ActorID:     input.ActorID,
		PayloadJSON: mustJSON(map[string]any{
			"status": attemptStatus, "error": strings.TrimSpace(input.ErrorText), "usage": input.Usage,
		}),
		CreatedAt: now,
	}); err != nil {
		return StepResolution{}, err
	}
	decisionType := eventTypeForDisposition(disposition)
	if reclaimed {
		decisionType = EventTypeStepReclaimed
	}
	if err := s.insertEventTx(ctx, tx, eventInput{
		WorkspaceID: input.WorkspaceID,
		WorkID:      input.WorkID,
		StepID:      input.StepID,
		AttemptID:   input.AttemptID,
		Type:        decisionType,
		FromState:   WorkStateRunning,
		ToState:     nextState,
		ActorID:     input.ActorID,
		PayloadJSON: mustJSON(map[string]any{
			"disposition": disposition, "next_action": nextAction, "reason": strings.TrimSpace(input.ErrorText),
			"attempt_count": schedule.AttemptCount, "reclaimed": reclaimed,
		}),
		CreatedAt: now,
	}); err != nil {
		return StepResolution{}, err
	}
	step, err = getStepTx(ctx, tx, input.WorkspaceID, input.WorkID, input.StepID, "")
	if err != nil {
		return StepResolution{}, err
	}
	attempt, err = getAttemptTx(ctx, tx, input.WorkspaceID, input.WorkID, input.AttemptID, "")
	if err != nil {
		return StepResolution{}, err
	}
	schedule, err = getStepScheduleTx(ctx, tx, input.WorkspaceID, input.WorkID, input.StepID)
	if err != nil {
		return StepResolution{}, err
	}
	return StepResolution{Step: step, Attempt: attempt, Schedule: schedule, Disposition: disposition}, nil
}

func chooseFailureDisposition(policy StepSchedulePolicy, attemptCount, consumedIterations int, consumedTokens int64, consumedCost float64) StepDisposition {
	if policy.MaxAttempts > 0 && attemptCount >= policy.MaxAttempts ||
		policy.MaxIterations > 0 && consumedIterations >= policy.MaxIterations ||
		policy.MaxTokens > 0 && consumedTokens >= policy.MaxTokens ||
		policy.MaxCostUSD > 0 && consumedCost >= policy.MaxCostUSD {
		return escalationDisposition(policy.EscalationState)
	}
	if attemptCount <= policy.RetryLimit {
		return StepDispositionRetry
	}
	if attemptCount <= policy.RetryLimit+policy.ReplanLimit {
		return StepDispositionReplan
	}
	if attemptCount <= policy.RetryLimit+policy.ReplanLimit+policy.DecomposeLimit {
		return StepDispositionDecompose
	}
	return escalationDisposition(policy.EscalationState)
}

func dispositionOutcome(disposition StepDisposition) (StepExecutionAction, WorkState, bool) {
	switch disposition {
	case StepDispositionRetry:
		return StepExecutionActionRetry, WorkStateReady, false
	case StepDispositionReplan:
		return StepExecutionActionReplan, WorkStateReady, false
	case StepDispositionDecompose:
		return StepExecutionActionDecompose, WorkStateReady, false
	case StepDispositionBlocked:
		return StepExecutionActionExecute, WorkStateBlocked, true
	case StepDispositionReview:
		return StepExecutionActionExecute, WorkStateReview, true
	default:
		return StepExecutionActionExecute, WorkStateDone, false
	}
}

func eventTypeForDisposition(disposition StepDisposition) EventType {
	switch disposition {
	case StepDispositionRetry:
		return EventTypeStepRetryScheduled
	case StepDispositionReplan:
		return EventTypeStepReplanScheduled
	case StepDispositionDecompose:
		return EventTypeStepDecomposeScheduled
	case StepDispositionReview:
		return EventTypeStepReviewRequested
	case StepDispositionBlocked:
		return EventTypeStepBlocked
	default:
		return EventTypeStepCompleted
	}
}

func escalationDisposition(state WorkState) StepDisposition {
	if state == WorkStateBlocked {
		return StepDispositionBlocked
	}
	return StepDispositionReview
}

func normalizeStepSchedulePolicy(policy StepSchedulePolicy) (StepSchedulePolicy, error) {
	if policy.RetryLimit < 0 || policy.ReplanLimit < 0 || policy.DecomposeLimit < 0 || policy.MaxAttempts < 0 || policy.MaxIterations < 0 || policy.MaxTokens < 0 || policy.MaxCostUSD < 0 {
		return StepSchedulePolicy{}, fmt.Errorf("workstore: step schedule policy budgets cannot be negative")
	}
	minimumAttempts := policy.RetryLimit + policy.ReplanLimit + policy.DecomposeLimit + 1
	if policy.MaxAttempts == 0 {
		policy.MaxAttempts = minimumAttempts
	}
	if policy.MaxAttempts < minimumAttempts {
		return StepSchedulePolicy{}, fmt.Errorf("workstore: max attempts must cover retry, replan, and decompose limits plus escalation")
	}
	if policy.EscalationState == "" {
		policy.EscalationState = WorkStateReview
	}
	if policy.EscalationState != WorkStateReview && policy.EscalationState != WorkStateBlocked {
		return StepSchedulePolicy{}, fmt.Errorf("workstore: escalation state must be review or blocked")
	}
	return policy, nil
}

func validateCompleteStepAttemptInput(input CompleteStepAttemptInput) error {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.WorkID) == "" || strings.TrimSpace(input.StepID) == "" || strings.TrimSpace(input.AttemptID) == "" || strings.TrimSpace(input.WorkerID) == "" || strings.TrimSpace(input.ActorID) == "" {
		return fmt.Errorf("workstore: complete claim identity is required")
	}
	if input.Usage.Iterations < 0 || input.Usage.Tokens < 0 || input.Usage.CostUSD < 0 {
		return fmt.Errorf("workstore: attempt usage cannot be negative")
	}
	return nil
}

func getStepScheduleTx(ctx context.Context, tx *sql.Tx, workspaceID, workID, stepID string) (StepSchedule, error) {
	schedule, err := scanStepSchedule(tx.QueryRowContext(ctx, stepScheduleSelect+" WHERE workspace_id = ? AND work_id = ? AND step_id = ?", workspaceID, workID, stepID))
	if errors.Is(err, sql.ErrNoRows) {
		return StepSchedule{}, ErrNotFound
	}
	if err != nil {
		return StepSchedule{}, fmt.Errorf("workstore: query step schedule: %w", err)
	}
	return schedule, nil
}

func queryStepSchedules(ctx context.Context, tx *sql.Tx, workspaceID, workID string) ([]StepSchedule, error) {
	rows, err := tx.QueryContext(ctx, stepScheduleSelect+`
		WHERE workspace_id = ? AND work_id = ?
		ORDER BY (
			SELECT position FROM steps WHERE steps.id = step_schedules.step_id
		), updated_at, step_id
	`, workspaceID, workID)
	if err != nil {
		return nil, fmt.Errorf("workstore: query projection step schedules: %w", err)
	}
	defer closeRows(rows)
	var schedules []StepSchedule
	for rows.Next() {
		schedule, err := scanStepSchedule(rows)
		if err != nil {
			return nil, fmt.Errorf("workstore: scan projection step schedule: %w", err)
		}
		schedules = append(schedules, schedule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("workstore: iterate projection step schedules: %w", err)
	}
	return schedules, nil
}

func getStepByIDTx(ctx context.Context, tx *sql.Tx, workspaceID, stepID string) (Step, error) {
	step, err := scanStep(tx.QueryRowContext(ctx, stepSelect+" WHERE workspace_id = ? AND id = ?", workspaceID, stepID))
	if errors.Is(err, sql.ErrNoRows) {
		return Step{}, ErrNotFound
	}
	if err != nil {
		return Step{}, fmt.Errorf("workstore: query step by id: %w", err)
	}
	return step, nil
}

func scanStepSchedule(row scanner) (StepSchedule, error) {
	var schedule StepSchedule
	var policyJSON jsonValue
	var leaseExpiresAt, lastHeartbeatAt sql.NullInt64
	var humanResume int
	var updatedAt int64
	if err := row.Scan(
		&schedule.SchemaVersion, &schedule.WorkspaceID, &schedule.WorkID, &schedule.StepID,
		&policyJSON, &schedule.LeaseOwner, &leaseExpiresAt, &lastHeartbeatAt,
		&schedule.ActiveAttemptID, &schedule.AttemptCount, &schedule.CycleAttemptCount, &schedule.ConsumedIterations,
		&schedule.ConsumedTokens, &schedule.ConsumedCostUSD, &schedule.NextAction,
		&schedule.LastDisposition, &schedule.BlockedReason, &humanResume, &updatedAt,
	); err != nil {
		return StepSchedule{}, err
	}
	if err := json.Unmarshal(policyJSON, &schedule.Policy); err != nil {
		return StepSchedule{}, fmt.Errorf("workstore: decode step schedule policy: %w", err)
	}
	schedule.LeaseExpiresAt = timeFromNull(leaseExpiresAt)
	schedule.LastHeartbeatAt = timeFromNull(lastHeartbeatAt)
	schedule.HumanResumeRequired = humanResume != 0
	schedule.UpdatedAt = time.UnixMilli(updatedAt).UTC()
	return schedule, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func execOne(ctx context.Context, tx *sql.Tx, query, operation string, args ...any) error {
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("workstore: %s: %w", operation, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("workstore: inspect %s: %w", operation, err)
	}
	if changed != 1 {
		return ErrClaimConflict
	}
	return nil
}
