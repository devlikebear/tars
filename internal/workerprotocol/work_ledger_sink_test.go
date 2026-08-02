package workerprotocol

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/devlikebear/tars/internal/workstore"
)

func TestWorkLedgerSinkAuditsPlacementWithoutPersistingTaskToken(t *testing.T) {
	t.Parallel()

	store, err := workstore.Open(context.Background(), filepath.Join(t.TempDir(), "ledger.db"), workstore.Options{})
	if err != nil {
		t.Fatalf("open work ledger: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	work, err := store.CreateWork(context.Background(), workstore.CreateWorkInput{
		WorkspaceID: "workspace-a", Kind: "remote-test", IdempotencyKey: "remote-test",
		Title: "Remote test", InitialState: workstore.WorkStateRunning, ActorID: "tester",
	})
	if err != nil {
		t.Fatalf("create work: %v", err)
	}
	step, err := store.CreateStep(context.Background(), workstore.CreateStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, IdempotencyKey: "step-a",
		Title: "Remote step", State: workstore.WorkStateRunning, Position: 1, ActorID: "tester",
	})
	if err != nil {
		t.Fatalf("create step: %v", err)
	}
	attempt, err := store.CreateAttempt(context.Background(), workstore.CreateAttemptInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: step.ID,
		IdempotencyKey: "attempt-a", Number: 1, Adapter: "remote-worker",
		Status: workstore.AttemptStatusRunning, ActorID: "tester",
	})
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	sink, err := NewWorkLedgerSink(store, "worker-control")
	if err != nil {
		t.Fatalf("new work ledger sink: %v", err)
	}
	event := ControlEvent{
		ID: "placement-a:execute:1", MessageID: "placement-a:execute:1",
		IdempotencyKey: "placement-a:execute:1", Type: string(MessageExecute), Entity: "placement",
		WorkerID: "worker-a", PlacementID: "placement-a", WorkspaceID: work.WorkspaceID,
		WorkID: work.ID, StepID: step.ID, AttemptID: attempt.ID, Sequence: 4,
		FromState: string(PlacementStateReady), ToState: string(PlacementStateExecuting),
		Payload: json.RawMessage(`{"task_token":"must-not-persist","resume":false}`),
	}
	if err := sink.Record(context.Background(), event); err != nil {
		t.Fatalf("record worker event: %v", err)
	}
	if err := sink.Record(context.Background(), event); err != nil {
		t.Fatalf("record duplicate worker event: %v", err)
	}
	projection, err := store.GetWorkProjection(context.Background(), work.WorkspaceID, work.ID)
	if err != nil {
		t.Fatalf("get work projection: %v", err)
	}
	count := 0
	for _, item := range projection.Events {
		if item.Type != workstore.EventTypeWorkerExecutionStarted {
			continue
		}
		count++
		if bytes.Contains(item.PayloadJSON, []byte("must-not-persist")) || bytes.Contains(item.PayloadJSON, []byte("task_token")) {
			t.Fatalf("worker event persisted task token: %s", item.PayloadJSON)
		}
	}
	if count != 1 {
		t.Fatalf("worker execution event count=%d want 1", count)
	}
}
