package workstore

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestEffectReceiptLifecycleIsIdempotentAndProjected(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, t.TempDir()+"/work-ledger.db")
	work := mustCreateWork(t, store, "workspace-a", "effect-lifecycle")
	step, err := store.CreateStep(ctx, CreateStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, IdempotencyKey: "send-message",
		Title: "Send message", State: WorkStateRunning, ActorID: "worker",
	})
	if err != nil {
		t.Fatalf("create step: %v", err)
	}

	input := BeginEffectReceiptInput{
		WorkspaceID:    work.WorkspaceID,
		WorkID:         work.ID,
		StepID:         step.ID,
		IdempotencyKey: "run-1:tool:send:sha256-request",
		CausationID:    "run-1",
		EffectType:     "external_message",
		Target:         "channel:ops",
		RequestDigest:  "sha256-request",
		ActorID:        "agent-runtime",
	}
	pending, err := store.BeginEffectReceipt(ctx, input)
	if err != nil {
		t.Fatalf("begin effect receipt: %v", err)
	}
	if pending.Status != EffectReceiptStatusPending || pending.ID == "" {
		t.Fatalf("unexpected pending receipt: %+v", pending)
	}
	replayed, err := store.BeginEffectReceipt(ctx, input)
	if err != nil {
		t.Fatalf("replay begin effect receipt: %v", err)
	}
	if replayed.ID != pending.ID || replayed.Status != EffectReceiptStatusPending {
		t.Fatalf("idempotent begin returned %+v, want %s", replayed, pending.ID)
	}

	committed, err := store.CommitEffectReceipt(ctx, CommitEffectReceiptInput{
		WorkspaceID:       work.WorkspaceID,
		WorkID:            work.ID,
		IdempotencyKey:    input.IdempotencyKey,
		RequestDigest:     input.RequestDigest,
		OutcomeJSON:       json.RawMessage(`{"message_id":"msg-42","result":"sent"}`),
		ExternalReference: "msg-42",
		ActorID:           "agent-runtime",
	})
	if err != nil {
		t.Fatalf("commit effect receipt: %v", err)
	}
	if committed.Status != EffectReceiptStatusCommitted || committed.CommittedAt == nil {
		t.Fatalf("unexpected committed receipt: %+v", committed)
	}
	committedAgain, err := store.CommitEffectReceipt(ctx, CommitEffectReceiptInput{
		WorkspaceID:       work.WorkspaceID,
		WorkID:            work.ID,
		IdempotencyKey:    input.IdempotencyKey,
		RequestDigest:     input.RequestDigest,
		OutcomeJSON:       json.RawMessage(`{"message_id":"msg-42","result":"sent"}`),
		ExternalReference: "msg-42",
		ActorID:           "agent-runtime",
	})
	if err != nil {
		t.Fatalf("idempotent commit effect receipt: %v", err)
	}
	if committedAgain.ID != committed.ID {
		t.Fatalf("idempotent commit changed receipt: %+v", committedAgain)
	}

	projection, err := store.GetWorkProjection(ctx, work.WorkspaceID, work.ID)
	if err != nil {
		t.Fatalf("get projection: %v", err)
	}
	if len(projection.EffectReceipts) != 1 || projection.EffectReceipts[0].ID != committed.ID {
		t.Fatalf("projection effect receipts: %+v", projection.EffectReceipts)
	}
	seenStarted := false
	seenCommitted := false
	for _, event := range projection.Events {
		seenStarted = seenStarted || event.Type == EventTypeEffectStarted
		seenCommitted = seenCommitted || event.Type == EventTypeEffectCommitted
	}
	if !seenStarted || !seenCommitted {
		t.Fatalf("effect lifecycle events: %+v", projection.Events)
	}
}

func TestEffectReceiptRejectsIdempotencyKeyReuseWithDifferentRequest(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, t.TempDir()+"/work-ledger.db")
	work := mustCreateWork(t, store, "workspace-a", "effect-conflict")
	base := BeginEffectReceiptInput{
		WorkspaceID:    work.WorkspaceID,
		WorkID:         work.ID,
		IdempotencyKey: "effect-key",
		EffectType:     "workspace_write",
		RequestDigest:  "digest-a",
		ActorID:        "worker",
	}
	if _, err := store.BeginEffectReceipt(ctx, base); err != nil {
		t.Fatalf("begin effect receipt: %v", err)
	}
	base.RequestDigest = "digest-b"
	if _, err := store.BeginEffectReceipt(ctx, base); !errors.Is(err, ErrEffectConflict) {
		t.Fatalf("different request digest error = %v, want ErrEffectConflict", err)
	}
	if _, err := store.CommitEffectReceipt(ctx, CommitEffectReceiptInput{
		WorkspaceID:    work.WorkspaceID,
		WorkID:         work.ID,
		IdempotencyKey: "effect-key",
		RequestDigest:  "digest-b",
		OutcomeJSON:    json.RawMessage(`{"ok":true}`),
		ActorID:        "worker",
	}); !errors.Is(err, ErrEffectConflict) {
		t.Fatalf("different commit digest error = %v, want ErrEffectConflict", err)
	}
}

func TestEffectReceiptPendingStateSurvivesReopen(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := dir + "/work-ledger.db"
	store, err := Open(ctx, path, Options{})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	work := mustCreateWork(t, store, "workspace-a", "pending-effect")
	receipt, err := store.BeginEffectReceipt(ctx, BeginEffectReceiptInput{
		WorkspaceID:    work.WorkspaceID,
		WorkID:         work.ID,
		IdempotencyKey: "unsafe-effect",
		EffectType:     "external_unknown",
		RequestDigest:  "sha256-request",
		ActorID:        "worker",
	})
	if err != nil {
		t.Fatalf("begin effect receipt: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	reopened, err := Open(ctx, path, Options{})
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	got, err := reopened.GetEffectReceipt(ctx, work.WorkspaceID, work.ID, receipt.IdempotencyKey)
	if err != nil {
		t.Fatalf("get pending effect receipt: %v", err)
	}
	if got.ID != receipt.ID || got.Status != EffectReceiptStatusPending || got.CommittedAt != nil {
		t.Fatalf("pending receipt after reopen: %+v", got)
	}
}
