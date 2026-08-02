package workstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func (s *Store) RecordExecutionEvent(ctx context.Context, input RecordExecutionEventInput) (Event, error) {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.WorkID) == "" ||
		strings.TrimSpace(input.StepID) == "" || strings.TrimSpace(input.AttemptID) == "" ||
		strings.TrimSpace(input.ActorID) == "" || strings.TrimSpace(input.IdempotencyKey) == "" ||
		!validExecutionEventType(input.Type) {
		return Event{}, fmt.Errorf("workstore: invalid execution event input")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Event{}, fmt.Errorf("workstore: begin execution event: %w", err)
	}
	defer rollback(tx)
	if err := ensureReferencesTx(ctx, tx, input.WorkspaceID, input.WorkID, input.StepID, input.AttemptID); err != nil {
		return Event{}, err
	}
	existing, err := getExecutionEventTx(ctx, tx, input)
	if err == nil {
		if err := tx.Commit(); err != nil {
			return Event{}, fmt.Errorf("workstore: commit idempotent execution event: %w", err)
		}
		return existing, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Event{}, err
	}
	now := s.now().UTC()
	if err := s.insertEventTx(ctx, tx, eventInput{
		WorkspaceID: input.WorkspaceID, WorkID: input.WorkID, StepID: input.StepID,
		AttemptID: input.AttemptID, Type: input.Type, ActorID: input.ActorID,
		CausationID: input.CausationID, IdempotencyKey: input.IdempotencyKey,
		PayloadJSON: input.PayloadJSON, CreatedAt: now,
	}); err != nil {
		return Event{}, err
	}
	event, err := getExecutionEventTx(ctx, tx, input)
	if err != nil {
		return Event{}, err
	}
	if err := tx.Commit(); err != nil {
		return Event{}, fmt.Errorf("workstore: commit execution event: %w", err)
	}
	return event, nil
}

func getExecutionEventTx(ctx context.Context, tx *sql.Tx, input RecordExecutionEventInput) (Event, error) {
	event, err := scanEvent(tx.QueryRowContext(ctx,
		eventSelect+" WHERE workspace_id = ? AND work_id = ? AND event_type = ? AND idempotency_key = ?",
		input.WorkspaceID, input.WorkID, input.Type, input.IdempotencyKey,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return Event{}, ErrNotFound
	}
	if err != nil {
		return Event{}, fmt.Errorf("workstore: query execution event: %w", err)
	}
	return event, nil
}

func validExecutionEventType(eventType EventType) bool {
	switch eventType {
	case EventTypeExecutionEnvironmentProvisioned,
		EventTypeExecutionCredentialsIssued,
		EventTypeExecutionWorkerStarted,
		EventTypeExecutionCheckpointRecorded,
		EventTypeExecutionEnvironmentSynced,
		EventTypeExecutionArtifactsCollected,
		EventTypeExecutionCredentialsRevoked,
		EventTypeExecutionEnvironmentDestroyed,
		EventTypeExecutionRecoveryStarted,
		EventTypeExecutionWorkerCancelled,
		EventTypeWorkerPlacementCreated,
		EventTypeWorkerEnvironmentProvisioned,
		EventTypeWorkerWorkspaceSynced,
		EventTypeWorkerLeaseGranted,
		EventTypeWorkerHeartbeatObserved,
		EventTypeWorkerExecutionStarted,
		EventTypeWorkerStreamObserved,
		EventTypeWorkerCheckpointRecorded,
		EventTypeWorkerArtifactsCollected,
		EventTypeWorkerPlacementDestroyed,
		EventTypeWorkerLost,
		EventTypeWorkerReclaimed,
		EventTypeWorkerRehydrated:
		return true
	default:
		return false
	}
}
