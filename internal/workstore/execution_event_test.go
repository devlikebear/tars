package workstore

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestRecordExecutionEventIsIdempotentAndBoundToAttempt(t *testing.T) {
	t.Parallel()

	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "ledger.db"), Options{})
	if err != nil {
		t.Fatalf("open work store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	work, err := store.CreateWork(context.Background(), CreateWorkInput{
		WorkspaceID: "workspace-execution", IdempotencyKey: "execution-work", Kind: "workflow",
		Title: "Execution", InitialState: WorkStateRunning, ActorID: "planner",
	})
	if err != nil {
		t.Fatalf("create work: %v", err)
	}
	step, err := store.CreateStep(context.Background(), CreateStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, IdempotencyKey: "execution-step",
		Title: "Run", State: WorkStateRunning, ActorID: "planner",
	})
	if err != nil {
		t.Fatalf("create step: %v", err)
	}
	attempt, err := store.CreateAttempt(context.Background(), CreateAttemptInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: step.ID,
		IdempotencyKey: "execution-attempt", Number: 1, Adapter: "native-local",
		Status: AttemptStatusRunning, ActorID: "scheduler",
	})
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	input := RecordExecutionEventInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: step.ID, AttemptID: attempt.ID,
		Type: EventTypeExecutionEnvironmentProvisioned, ActorID: "execution-plane",
		IdempotencyKey: attempt.ID + ":environment.provisioned",
		PayloadJSON:    json.RawMessage(`{"environment_id":"env-1","provider":"local"}`),
	}
	first, err := store.RecordExecutionEvent(context.Background(), input)
	if err != nil {
		t.Fatalf("record execution event: %v", err)
	}
	second, err := store.RecordExecutionEvent(context.Background(), input)
	if err != nil {
		t.Fatalf("replay execution event: %v", err)
	}
	if first.ID != second.ID || first.AttemptID != attempt.ID || first.Type != EventTypeExecutionEnvironmentProvisioned {
		t.Fatalf("execution events first=%#v second=%#v", first, second)
	}
	events, err := store.ListEvents(context.Background(), work.WorkspaceID, work.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	count := 0
	for _, event := range events {
		if event.Type == EventTypeExecutionEnvironmentProvisioned {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("execution event count = %d, want 1", count)
	}
	input.Type = EventType("execution.untrusted")
	input.IdempotencyKey = "untrusted"
	if _, err := store.RecordExecutionEvent(context.Background(), input); err == nil {
		t.Fatal("accepted an unknown execution event type")
	}
}
