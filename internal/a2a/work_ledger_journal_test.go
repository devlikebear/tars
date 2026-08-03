package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/devlikebear/tars/internal/workscheduler"
	"github.com/devlikebear/tars/internal/workstore"
)

func TestWorkLedgerJournalPersistsReplaySafeExternalTaskMetadataOnly(t *testing.T) {
	store, err := workstore.Open(context.Background(), filepath.Join(t.TempDir(), "ledger.db"), workstore.Options{})
	if err != nil {
		t.Fatalf("open work ledger: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	execution := createJournalExecution(t, store)
	journal, err := NewWorkLedgerJournal(store, "a2a-gateway")
	if err != nil {
		t.Fatalf("new work ledger journal: %v", err)
	}
	events := []ExternalEvent{
		{Kind: EventTaskSubmitted, TaskID: "remote-1", ContextID: "ctx-1", State: TaskStateWorking},
		{Kind: EventTaskStateObserved, TaskID: "remote-1", ContextID: "ctx-1", State: TaskStateCompleted},
		{Kind: EventArtifactQuarantined, TaskID: "remote-1", ContextID: "ctx-1", State: TaskStateCompleted, AcceptedArtifacts: 1, QuarantinedParts: 2},
	}
	for _, event := range events {
		if err := journal.Record(context.Background(), execution, event); err != nil {
			t.Fatalf("record event: %v", err)
		}
		if err := journal.Record(context.Background(), execution, event); err != nil {
			t.Fatalf("record duplicate event: %v", err)
		}
	}
	reference, found, err := journal.Lookup(context.Background(), execution)
	if err != nil || !found || reference.TaskID != "remote-1" || reference.ContextID != "ctx-1" {
		t.Fatalf("lookup reference=%#v found=%v err=%v", reference, found, err)
	}

	projection, err := store.GetWorkProjection(context.Background(), execution.Work.WorkspaceID, execution.Work.ID)
	if err != nil {
		t.Fatalf("get work projection: %v", err)
	}
	counts := map[workstore.EventType]int{}
	for _, event := range projection.Events {
		counts[event.Type]++
		if bytes.Contains(event.PayloadJSON, []byte("Authorization")) || bytes.Contains(event.PayloadJSON, []byte("token")) ||
			bytes.Contains(event.PayloadJSON, []byte("https://")) {
			t.Fatalf("unsafe A2A payload persisted: %s", event.PayloadJSON)
		}
	}
	if counts[workstore.EventTypeA2ATaskSubmitted] != 1 || counts[workstore.EventTypeA2ATaskStateObserved] != 1 ||
		counts[workstore.EventTypeA2AArtifactQuarantined] != 1 {
		t.Fatalf("unexpected event counts: %#v", counts)
	}
}

func createJournalExecution(t *testing.T, store *workstore.Store) workscheduler.Execution {
	t.Helper()
	ctx := context.Background()
	work, err := store.CreateWork(ctx, workstore.CreateWorkInput{
		WorkspaceID: "workspace-a2a", IdempotencyKey: "a2a-work", Kind: "external-agent",
		Title: "A2A", InitialState: workstore.WorkStateRunning, ActorID: "planner",
	})
	if err != nil {
		t.Fatalf("create work: %v", err)
	}
	step, err := store.CreateStep(ctx, workstore.CreateStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, IdempotencyKey: "a2a-step",
		Title: "Delegate", State: workstore.WorkStateRunning, ActorID: "planner",
	})
	if err != nil {
		t.Fatalf("create step: %v", err)
	}
	attempt, err := store.CreateAttempt(ctx, workstore.CreateAttemptInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: step.ID,
		IdempotencyKey: "a2a-attempt", Number: 1, Adapter: AdapterName,
		Status: workstore.AttemptStatusRunning, ActorID: "scheduler", InputJSON: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	return workscheduler.Execution{Work: work, Claim: workstore.StepClaim{Step: step, Attempt: attempt}}
}
