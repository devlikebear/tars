package executionplane

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/workscheduler"
	"github.com/devlikebear/tars/internal/workstore"
)

func TestWorkLedgerSinkRecordsLifecycleAndArtifactsWithoutCredentialValues(t *testing.T) {
	t.Parallel()

	store, err := workstore.Open(context.Background(), filepath.Join(t.TempDir(), "ledger.db"), workstore.Options{})
	if err != nil {
		t.Fatalf("open work store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	execution := createLedgerExecution(t, store)
	sink, err := NewWorkLedgerSink(store, "execution-plane")
	if err != nil {
		t.Fatalf("new work ledger sink: %v", err)
	}
	event := LifecycleEvent{
		Phase: EventCredentialsIssued, Execution: execution, Provider: "managed-worktree",
		Worker: "native", EnvironmentID: "env-1", CredentialID: "grant-1",
	}
	if err := sink.Record(context.Background(), event); err != nil {
		t.Fatalf("record lifecycle event: %v", err)
	}
	if err := sink.Record(context.Background(), event); err != nil {
		t.Fatalf("replay lifecycle event: %v", err)
	}
	artifact := CollectedArtifact{
		Kind: "transcript", Name: "transcript.jsonl", URI: "file:///artifacts/transcript.jsonl",
		Digest: "sha256:transcript", MediaType: "application/x-ndjson", SizeBytes: 42,
	}
	if err := sink.StoreArtifact(context.Background(), execution, artifact); err != nil {
		t.Fatalf("store artifact: %v", err)
	}
	if err := sink.StoreArtifact(context.Background(), execution, artifact); err != nil {
		t.Fatalf("replay artifact: %v", err)
	}
	projection, err := store.GetWorkProjection(context.Background(), execution.Work.WorkspaceID, execution.Work.ID)
	if err != nil {
		t.Fatalf("get work projection: %v", err)
	}
	count := 0
	for _, recorded := range projection.Events {
		if recorded.Type != workstore.EventTypeExecutionCredentialsIssued {
			continue
		}
		count++
		if strings.Contains(string(recorded.PayloadJSON), "credential_values") || !strings.Contains(string(recorded.PayloadJSON), `"credential_id":"grant-1"`) {
			t.Fatalf("unsafe lifecycle payload = %s", recorded.PayloadJSON)
		}
	}
	if count != 1 || len(projection.Artifacts) != 1 || projection.Artifacts[0].Digest != artifact.Digest {
		t.Fatalf("projection events=%d artifacts=%#v", count, projection.Artifacts)
	}
}

func createLedgerExecution(t *testing.T, store *workstore.Store) workscheduler.Execution {
	t.Helper()
	ctx := context.Background()
	work, err := store.CreateWork(ctx, workstore.CreateWorkInput{
		WorkspaceID: "workspace-sink", IdempotencyKey: "sink-work", Kind: "workflow",
		Title: "Sink", InitialState: workstore.WorkStateRunning, ActorID: "planner",
	})
	if err != nil {
		t.Fatalf("create work: %v", err)
	}
	step, err := store.CreateStep(ctx, workstore.CreateStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, IdempotencyKey: "sink-step",
		Title: "Run", State: workstore.WorkStateRunning, ActorID: "planner",
	})
	if err != nil {
		t.Fatalf("create step: %v", err)
	}
	attempt, err := store.CreateAttempt(ctx, workstore.CreateAttemptInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: step.ID,
		IdempotencyKey: "sink-attempt", Number: 1, Adapter: "native-local",
		Status: workstore.AttemptStatusRunning, ActorID: "scheduler", InputJSON: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	return workscheduler.Execution{Work: work, Claim: workstore.StepClaim{Step: step, Attempt: attempt}}
}
