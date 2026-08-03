package workstore

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const effectReceiptSelect = `SELECT
    schema_version, id, workspace_id, work_id, COALESCE(step_id, ''),
    COALESCE(attempt_id, ''), idempotency_key, causation_id, effect_type,
    target, request_digest, status, outcome_json, external_reference,
    actor_id, created_at, updated_at, committed_at
FROM effect_receipts`

func (s *Store) BeginEffectReceipt(ctx context.Context, input BeginEffectReceiptInput) (EffectReceipt, error) {
	if err := validateBeginEffectReceipt(input); err != nil {
		return EffectReceipt{}, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return EffectReceipt{}, fmt.Errorf("workstore: begin effect receipt transaction: %w", err)
	}
	defer rollback(tx)
	if err := ensureReferencesTx(ctx, tx, input.WorkspaceID, input.WorkID, input.StepID, input.AttemptID); err != nil {
		return EffectReceipt{}, fmt.Errorf("workstore: effect receipt references: %w", err)
	}
	if existing, found, err := getEffectReceiptTx(ctx, tx, input.WorkspaceID, input.WorkID, input.IdempotencyKey); err != nil {
		return EffectReceipt{}, err
	} else if found {
		if !sameEffectRequest(existing, input) {
			return EffectReceipt{}, fmt.Errorf("%w: idempotency key %q has a different request", ErrEffectConflict, input.IdempotencyKey)
		}
		if err := tx.Commit(); err != nil {
			return EffectReceipt{}, fmt.Errorf("workstore: commit idempotent effect receipt read: %w", err)
		}
		return existing, nil
	}

	id, err := s.newID("efx")
	if err != nil {
		return EffectReceipt{}, err
	}
	now := s.now().UTC()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO effect_receipts (
			schema_version, id, workspace_id, work_id, step_id, attempt_id,
			idempotency_key, causation_id, effect_type, target, request_digest,
			status, outcome_json, external_reference, actor_id, created_at,
			updated_at, committed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '{}', '', ?, ?, ?, NULL)
	`, recordSchemaVersion, id, input.WorkspaceID, input.WorkID,
		nullableString(input.StepID), nullableString(input.AttemptID), input.IdempotencyKey,
		input.CausationID, input.EffectType, input.Target, input.RequestDigest,
		EffectReceiptStatusPending, input.ActorID, now.UnixMilli(), now.UnixMilli())
	if err != nil {
		return EffectReceipt{}, fmt.Errorf("workstore: insert effect receipt: %w", err)
	}
	if err := s.insertEventTx(ctx, tx, eventInput{
		WorkspaceID:    input.WorkspaceID,
		WorkID:         input.WorkID,
		StepID:         input.StepID,
		AttemptID:      input.AttemptID,
		Type:           EventTypeEffectStarted,
		ActorID:        input.ActorID,
		CausationID:    input.CausationID,
		IdempotencyKey: input.IdempotencyKey,
		PayloadJSON: mustJSON(map[string]string{
			"effect_receipt_id": id,
			"effect_type":       input.EffectType,
			"request_digest":    input.RequestDigest,
		}),
		CreatedAt: now,
	}); err != nil {
		return EffectReceipt{}, err
	}
	if err := tx.Commit(); err != nil {
		return EffectReceipt{}, fmt.Errorf("workstore: commit effect receipt: %w", err)
	}
	return s.GetEffectReceipt(ctx, input.WorkspaceID, input.WorkID, input.IdempotencyKey)
}

func (s *Store) CommitEffectReceipt(ctx context.Context, input CommitEffectReceiptInput) (EffectReceipt, error) {
	if err := validateCommitEffectReceipt(input); err != nil {
		return EffectReceipt{}, err
	}
	outcome, err := normalizedJSON(input.OutcomeJSON)
	if err != nil {
		return EffectReceipt{}, fmt.Errorf("workstore: effect receipt outcome json: %w", err)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return EffectReceipt{}, fmt.Errorf("workstore: begin effect receipt commit transaction: %w", err)
	}
	defer rollback(tx)
	existing, found, err := getEffectReceiptTx(ctx, tx, input.WorkspaceID, input.WorkID, input.IdempotencyKey)
	if err != nil {
		return EffectReceipt{}, err
	}
	if !found {
		return EffectReceipt{}, ErrNotFound
	}
	if existing.RequestDigest != input.RequestDigest {
		return EffectReceipt{}, fmt.Errorf("%w: idempotency key %q has a different request digest", ErrEffectConflict, input.IdempotencyKey)
	}
	if existing.Status == EffectReceiptStatusCommitted {
		if !bytes.Equal(existing.OutcomeJSON, outcome) || existing.ExternalReference != strings.TrimSpace(input.ExternalReference) {
			return EffectReceipt{}, fmt.Errorf("%w: committed effect receipt %q has a different outcome", ErrEffectConflict, input.IdempotencyKey)
		}
		if err := tx.Commit(); err != nil {
			return EffectReceipt{}, fmt.Errorf("workstore: commit idempotent effect receipt read: %w", err)
		}
		return existing, nil
	}

	now := s.now().UTC()
	result, err := tx.ExecContext(ctx, `
		UPDATE effect_receipts
		SET status = ?, outcome_json = ?, external_reference = ?, actor_id = ?,
			updated_at = ?, committed_at = ?
		WHERE workspace_id = ? AND work_id = ? AND idempotency_key = ?
			AND request_digest = ? AND status = ?
	`, EffectReceiptStatusCommitted, outcome, strings.TrimSpace(input.ExternalReference),
		input.ActorID, now.UnixMilli(), now.UnixMilli(), input.WorkspaceID, input.WorkID,
		input.IdempotencyKey, input.RequestDigest, EffectReceiptStatusPending)
	if err != nil {
		return EffectReceipt{}, fmt.Errorf("workstore: update effect receipt: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return EffectReceipt{}, fmt.Errorf("workstore: inspect effect receipt update: %w", err)
	}
	if rows != 1 {
		return EffectReceipt{}, fmt.Errorf("%w: effect receipt %q changed concurrently", ErrEffectConflict, input.IdempotencyKey)
	}
	if err := s.insertEventTx(ctx, tx, eventInput{
		WorkspaceID:    input.WorkspaceID,
		WorkID:         input.WorkID,
		StepID:         existing.StepID,
		AttemptID:      existing.AttemptID,
		Type:           EventTypeEffectCommitted,
		ActorID:        input.ActorID,
		CausationID:    existing.CausationID,
		IdempotencyKey: input.IdempotencyKey,
		PayloadJSON: mustJSON(map[string]string{
			"effect_receipt_id":  existing.ID,
			"external_reference": strings.TrimSpace(input.ExternalReference),
			"request_digest":     input.RequestDigest,
		}),
		CreatedAt: now,
	}); err != nil {
		return EffectReceipt{}, err
	}
	if err := tx.Commit(); err != nil {
		return EffectReceipt{}, fmt.Errorf("workstore: commit completed effect receipt: %w", err)
	}
	return s.GetEffectReceipt(ctx, input.WorkspaceID, input.WorkID, input.IdempotencyKey)
}

func (s *Store) GetEffectReceipt(ctx context.Context, workspaceID, workID, idempotencyKey string) (EffectReceipt, error) {
	receipt, err := scanEffectReceipt(s.db.QueryRowContext(ctx,
		effectReceiptSelect+" WHERE workspace_id = ? AND work_id = ? AND idempotency_key = ?",
		strings.TrimSpace(workspaceID), strings.TrimSpace(workID), strings.TrimSpace(idempotencyKey)))
	if errors.Is(err, sql.ErrNoRows) {
		return EffectReceipt{}, ErrNotFound
	}
	if err != nil {
		return EffectReceipt{}, fmt.Errorf("workstore: query effect receipt: %w", err)
	}
	return receipt, nil
}

func getEffectReceiptTx(ctx context.Context, tx *sql.Tx, workspaceID, workID, idempotencyKey string) (EffectReceipt, bool, error) {
	receipt, err := scanEffectReceipt(tx.QueryRowContext(ctx,
		effectReceiptSelect+" WHERE workspace_id = ? AND work_id = ? AND idempotency_key = ?",
		workspaceID, workID, idempotencyKey))
	if errors.Is(err, sql.ErrNoRows) {
		return EffectReceipt{}, false, nil
	}
	if err != nil {
		return EffectReceipt{}, false, fmt.Errorf("workstore: query effect receipt: %w", err)
	}
	return receipt, true, nil
}

func queryEffectReceipts(ctx context.Context, tx *sql.Tx, workspaceID, workID string) ([]EffectReceipt, error) {
	rows, err := tx.QueryContext(ctx, effectReceiptSelect+" WHERE workspace_id = ? AND work_id = ? ORDER BY created_at, id", workspaceID, workID)
	if err != nil {
		return nil, fmt.Errorf("workstore: query projection effect receipts: %w", err)
	}
	defer closeRows(rows)
	var receipts []EffectReceipt
	for rows.Next() {
		receipt, err := scanEffectReceipt(rows)
		if err != nil {
			return nil, fmt.Errorf("workstore: scan projection effect receipt: %w", err)
		}
		receipts = append(receipts, receipt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("workstore: iterate projection effect receipts: %w", err)
	}
	return receipts, nil
}

func scanEffectReceipt(row scanner) (EffectReceipt, error) {
	var receipt EffectReceipt
	var outcome jsonValue
	var createdAt, updatedAt int64
	var committedAt sql.NullInt64
	err := row.Scan(
		&receipt.SchemaVersion, &receipt.ID, &receipt.WorkspaceID, &receipt.WorkID,
		&receipt.StepID, &receipt.AttemptID, &receipt.IdempotencyKey, &receipt.CausationID,
		&receipt.EffectType, &receipt.Target, &receipt.RequestDigest, &receipt.Status,
		&outcome, &receipt.ExternalReference, &receipt.ActorID, &createdAt, &updatedAt,
		&committedAt,
	)
	if err != nil {
		return EffectReceipt{}, err
	}
	receipt.OutcomeJSON = json.RawMessage(outcome)
	receipt.CreatedAt = time.UnixMilli(createdAt).UTC()
	receipt.UpdatedAt = time.UnixMilli(updatedAt).UTC()
	receipt.CommittedAt = timeFromNull(committedAt)
	return receipt, nil
}

func sameEffectRequest(receipt EffectReceipt, input BeginEffectReceiptInput) bool {
	return receipt.WorkID == input.WorkID && receipt.StepID == strings.TrimSpace(input.StepID) &&
		receipt.AttemptID == strings.TrimSpace(input.AttemptID) && receipt.EffectType == strings.TrimSpace(input.EffectType) &&
		receipt.Target == strings.TrimSpace(input.Target) && receipt.RequestDigest == strings.TrimSpace(input.RequestDigest)
}

func validateBeginEffectReceipt(input BeginEffectReceiptInput) error {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.WorkID) == "" ||
		strings.TrimSpace(input.IdempotencyKey) == "" || strings.TrimSpace(input.EffectType) == "" ||
		strings.TrimSpace(input.RequestDigest) == "" || strings.TrimSpace(input.ActorID) == "" {
		return fmt.Errorf("workstore: workspace, work, idempotency key, effect type, request digest, and actor are required")
	}
	return nil
}

func validateCommitEffectReceipt(input CommitEffectReceiptInput) error {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.WorkID) == "" ||
		strings.TrimSpace(input.IdempotencyKey) == "" || strings.TrimSpace(input.RequestDigest) == "" ||
		strings.TrimSpace(input.ActorID) == "" {
		return fmt.Errorf("workstore: workspace, work, idempotency key, request digest, and actor are required")
	}
	return nil
}
