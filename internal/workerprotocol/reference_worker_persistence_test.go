package workerprotocol

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReferenceWorkerRestartsBetweenSSHFramesAndReplaysExecutionReceipt(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 2, 18, 0, 0, 0, time.UTC)
	seed := make([]byte, ed25519.SeedSize)
	seed[0] = 21
	issuer, err := NewTaskTokenIssuer(TaskTokenIssuerOptions{
		PrivateKey: ed25519.NewKeyFromSeed(seed), MaxTTL: 5 * time.Minute, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	statePath := filepath.Join(root, "worker-state.json")
	binding := TaskTokenBinding{
		WorkspaceID: "workspace-a", WorkID: "work-a", StepID: "step-a", AttemptID: "attempt-a",
		PlacementID: "placement-a", WorkerID: "worker-a",
	}
	token, _, err := issuer.Issue(binding, []TaskScope{TaskScopeExecute, TaskScopeCollect, TaskScopeDestroy}, 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	result := ReferenceExecutionResult{
		Payload:   json.RawMessage(`{"succeeded":true,"summary":"restart-safe"}`),
		Artifacts: []WireArtifact{{Name: "result.txt", MediaType: "text/plain", Data: []byte("result\n"), Digest: digestBytes([]byte("result\n"))}},
	}
	firstExecutor := &recordingReferenceExecutor{result: result}
	first := openPersistentReferenceWorker(t, root, statePath, issuer, firstExecutor, now)
	assertReferenceAccepted(t, first, wireRequestForSequence("worker-a", "placement-a", 1, MessageProvision, ProvisionPayload{
		EnvironmentID: "env-a", Policy: DefaultExecutionPolicy(), Binding: binding,
	}, nil))
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "task.txt"), []byte("task"), 0o644); err != nil {
		t.Fatal(err)
	}
	bundle, err := BuildWorkspaceBundle(context.Background(), WorkspaceBundleOptions{RootDir: source, Mode: SyncModeDirectory})
	if err != nil {
		t.Fatal(err)
	}
	assertReferenceAccepted(t, first, wireRequestForSequence("worker-a", "placement-a", 2, MessageSync, SyncPayload{Mode: SyncModeDirectory, Digest: bundle.Manifest.Digest}, &bundle))
	assertReferenceAccepted(t, first, wireRequestForSequence("worker-a", "placement-a", 3, MessageLease, LeasePayload{LeaseTTLMS: 120_000}, nil))
	executeRequest := wireRequestForSequence("worker-a", "placement-a", 4, MessageExecute, ExecutePayload{
		TaskToken: token, Request: json.RawMessage(`{"objective":"must-not-persist"}`),
	}, nil)
	executeResponse := assertReferenceAccepted(t, first, executeRequest)
	if firstExecutor.calls != 1 || len(executeResponse.Artifacts) != 1 {
		t.Fatalf("first execution calls=%d response=%+v", firstExecutor.calls, executeResponse)
	}

	secondExecutor := &recordingReferenceExecutor{result: ReferenceExecutionResult{}}
	second := openPersistentReferenceWorker(t, root, statePath, issuer, secondExecutor, now)
	replayed := assertReferenceAccepted(t, second, executeRequest)
	if secondExecutor.calls != 0 || len(replayed.Artifacts) != 1 || string(replayed.Payload) != string(result.Payload) {
		t.Fatalf("execution receipt was not replayed: calls=%d response=%+v", secondExecutor.calls, replayed)
	}
	collect := assertReferenceAccepted(t, second, wireRequestForSequence("worker-a", "placement-a", 5, MessageCollect, CollectPayload{
		Complete: false, TaskToken: token,
	}, nil))
	if len(collect.Artifacts) != 1 {
		t.Fatalf("restarted worker lost artifacts: %+v", collect)
	}
	assertReferenceAccepted(t, second, wireRequestForSequence("worker-a", "placement-a", 6, MessageCollect, CollectPayload{
		Complete: true, Succeeded: true, TaskToken: token,
	}, nil))
	assertReferenceAccepted(t, second, wireRequestForSequence("worker-a", "placement-a", 7, MessageDestroy, DestroyPayload{
		Reason: "done", TaskToken: token,
	}, nil))

	persisted, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read worker state: %v", err)
	}
	for _, forbidden := range [][]byte{[]byte(token), []byte("must-not-persist"), []byte("tars-task-v1")} {
		if bytes.Contains(persisted, forbidden) {
			t.Fatalf("worker state persisted forbidden task data %q: %s", forbidden, persisted)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "placements", "placement-a")); !os.IsNotExist(err) {
		t.Fatalf("destroy did not remove placement files: %v", err)
	}
	third := openPersistentReferenceWorker(t, root, statePath, issuer, &recordingReferenceExecutor{}, now)
	if environment := third.Snapshot().Environments["placement-a"]; environment.State != PlacementStateDestroyed || environment.LastSequence != 7 {
		t.Fatalf("destroyed state did not survive restart: %+v", environment)
	}
}

func openPersistentReferenceWorker(t *testing.T, root, statePath string, issuer *TaskTokenIssuer, executor ReferenceExecutor, now time.Time) *ReferenceWorker {
	t.Helper()
	worker, err := NewReferenceWorker(ReferenceWorkerOptions{
		WorkerID: "worker-a", RootDir: root, StatePath: statePath,
		TokenVerifier: issuer.PublicVerifier(), Executor: executor, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("open persistent reference worker: %v", err)
	}
	return worker
}

func assertReferenceAccepted(t *testing.T, worker *ReferenceWorker, request WireRequest) WireResponse {
	t.Helper()
	response, err := worker.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("handle %s: %v", request.Envelope.Type, err)
	}
	if !response.Accepted {
		t.Fatalf("worker rejected %s: %+v", request.Envelope.Type, response)
	}
	return response
}
