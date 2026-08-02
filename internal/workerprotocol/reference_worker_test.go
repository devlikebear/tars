package workerprotocol

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReferenceWorkerRunsVersionedLifecycleWithoutExposingTaskToken(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 2, 16, 0, 0, 0, time.UTC)
	seed := make([]byte, ed25519.SeedSize)
	seed[0] = 7
	issuer, err := NewTaskTokenIssuer(TaskTokenIssuerOptions{
		PrivateKey: ed25519.NewKeyFromSeed(seed), MaxTTL: 5 * time.Minute, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new token issuer: %v", err)
	}
	executor := &recordingReferenceExecutor{
		result: ReferenceExecutionResult{
			Payload:    json.RawMessage(`{"succeeded":true}`),
			Artifacts:  []WireArtifact{{Name: "result.txt", MediaType: "text/plain", Data: []byte("done\n"), Digest: digestBytes([]byte("done\n"))}},
			Checkpoint: &CheckpointPayload{ID: "checkpoint-a", Digest: "sha256:checkpoint"},
		},
	}
	worker, err := NewReferenceWorker(ReferenceWorkerOptions{
		WorkerID: "worker-a", RootDir: t.TempDir(), TokenVerifier: issuer.PublicVerifier(),
		Executor: executor, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new reference worker: %v", err)
	}
	transport, err := NewInProcessTransport(worker, WireLimits{MaxRequestBytes: 4 << 20, MaxResponseBytes: 4 << 20})
	if err != nil {
		t.Fatalf("new in-process transport: %v", err)
	}
	binding := TaskTokenBinding{
		WorkspaceID: "workspace-a", WorkID: "work-a", StepID: "step-a", AttemptID: "attempt-a",
		PlacementID: "placement-a", WorkerID: "worker-a",
	}
	exchange := func(sequence int64, messageType MessageType, payload any, workspace *WorkspaceBundle) WireResponse {
		t.Helper()
		envelope := testEnvelope("worker-a", "placement-a", sequence, messageType, payload)
		request := WireRequest{ProtocolVersion: ProtocolVersionV1, RequestID: envelope.MessageID, Envelope: envelope, Workspace: workspace}
		response, exchangeErr := transport.Exchange(context.Background(), request)
		if exchangeErr != nil {
			t.Fatalf("exchange %s: %v", messageType, exchangeErr)
		}
		if !response.Accepted {
			t.Fatalf("worker rejected %s: %+v", messageType, response)
		}
		return response
	}
	exchange(1, MessageProvision, ProvisionPayload{
		EnvironmentID: "env-a", RootDir: "/must/be/ignored", Policy: DefaultExecutionPolicy(), Binding: binding,
	}, nil)

	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "task.txt"), []byte("work\n"), 0o644); err != nil {
		t.Fatalf("write workspace source: %v", err)
	}
	bundle, err := BuildWorkspaceBundle(context.Background(), WorkspaceBundleOptions{RootDir: source, Mode: SyncModeDirectory})
	if err != nil {
		t.Fatalf("build workspace bundle: %v", err)
	}
	exchange(2, MessageSync, SyncPayload{Mode: SyncModeDirectory, Digest: bundle.Manifest.Digest}, &bundle)
	exchange(3, MessageLease, LeasePayload{LeaseTTLMS: 60_000}, nil)
	token, _, err := issuer.Issue(binding, []TaskScope{TaskScopeExecute, TaskScopeCollect, TaskScopeDestroy}, time.Minute)
	if err != nil {
		t.Fatalf("issue task token: %v", err)
	}
	execute := exchange(4, MessageExecute, ExecutePayload{TaskToken: token, Request: json.RawMessage(`{"objective":"run"}`)}, nil)
	if len(execute.Artifacts) != 1 || executor.calls != 1 || executor.request.Binding != binding {
		t.Fatalf("execute response=%+v executor=%+v", execute, executor)
	}
	if executor.request.RootDir == "/must/be/ignored" {
		t.Fatal("worker trusted gateway-provided root path")
	}
	if raw, err := os.ReadFile(filepath.Join(executor.request.RootDir, "task.txt")); err != nil || string(raw) != "work\n" {
		t.Fatalf("executor workspace=%q error=%v", raw, err)
	}
	collect := exchange(5, MessageCollect, CollectPayload{Complete: true, Succeeded: true, TaskToken: token}, nil)
	if len(collect.Artifacts) != 1 {
		t.Fatalf("collect response=%+v", collect)
	}
	exchange(6, MessageDestroy, DestroyPayload{Reason: "completed", TaskToken: token}, nil)
	if _, err := os.Stat(executor.request.RootDir); !os.IsNotExist(err) {
		t.Fatalf("destroyed environment remains: %v", err)
	}
	snapshotJSON, err := json.Marshal(worker.Snapshot())
	if err != nil {
		t.Fatalf("marshal worker snapshot: %v", err)
	}
	if containsBytes(snapshotJSON, []byte(token)) || containsBytes(snapshotJSON, []byte("objective")) {
		t.Fatalf("worker state persisted task token or request: %s", snapshotJSON)
	}
}

func TestReferenceWorkerRejectsWrongTaskBindingBeforeExecution(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 2, 16, 0, 0, 0, time.UTC)
	seed := make([]byte, ed25519.SeedSize)
	seed[0] = 9
	issuer, err := NewTaskTokenIssuer(TaskTokenIssuerOptions{
		PrivateKey: ed25519.NewKeyFromSeed(seed), MaxTTL: time.Minute, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new token issuer: %v", err)
	}
	executor := &recordingReferenceExecutor{}
	worker, err := NewReferenceWorker(ReferenceWorkerOptions{
		WorkerID: "worker-a", RootDir: t.TempDir(), TokenVerifier: issuer.PublicVerifier(), Executor: executor,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new reference worker: %v", err)
	}
	binding := TaskTokenBinding{WorkspaceID: "ws", WorkID: "work", StepID: "step", AttemptID: "attempt", PlacementID: "placement", WorkerID: "worker-a"}
	if response, err := worker.Handle(context.Background(), wireRequestForSequence("worker-a", "placement", 1, MessageProvision, ProvisionPayload{
		EnvironmentID: "env", Policy: DefaultExecutionPolicy(), Binding: binding,
	}, nil)); err != nil || !response.Accepted {
		t.Fatalf("provision reference worker: response=%+v error=%v", response, err)
	}
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "task.txt"), []byte("work"), 0o644); err != nil {
		t.Fatal(err)
	}
	bundle, err := BuildWorkspaceBundle(context.Background(), WorkspaceBundleOptions{RootDir: source, Mode: SyncModeDirectory})
	if err != nil {
		t.Fatal(err)
	}
	if response, err := worker.Handle(context.Background(), wireRequestForSequence("worker-a", "placement", 2, MessageSync, SyncPayload{Mode: SyncModeDirectory, Digest: bundle.Manifest.Digest}, &bundle)); err != nil || !response.Accepted {
		t.Fatalf("sync reference worker: response=%+v error=%v", response, err)
	}
	if response, err := worker.Handle(context.Background(), wireRequestForSequence("worker-a", "placement", 3, MessageLease, LeasePayload{LeaseTTLMS: 60_000}, nil)); err != nil || !response.Accepted {
		t.Fatalf("lease reference worker: response=%+v error=%v", response, err)
	}
	wrong := binding
	wrong.WorkerID = "worker-b"
	token, _, err := issuer.Issue(wrong, []TaskScope{TaskScopeExecute}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	response, err := worker.Handle(context.Background(), wireRequestForSequence("worker-a", "placement", 4, MessageExecute, ExecutePayload{TaskToken: token}, nil))
	if err != nil {
		t.Fatalf("handle rejected execution: %v", err)
	}
	if response.Accepted || response.ErrorCode != "authorization_denied" || executor.calls != 0 {
		t.Fatalf("wrong-bound execution response=%+v calls=%d", response, executor.calls)
	}
}

type recordingReferenceExecutor struct {
	request ReferenceExecutionRequest
	result  ReferenceExecutionResult
	calls   int
}

func (*recordingReferenceExecutor) Capabilities() WorkerCapabilities {
	return WorkerCapabilities{Resume: true, Streaming: true, Checkpoints: true, EgressPolicy: true, ResourceLimits: true, ArtifactScan: true}
}

func (executor *recordingReferenceExecutor) Execute(_ context.Context, request ReferenceExecutionRequest) (ReferenceExecutionResult, error) {
	executor.calls++
	executor.request = request
	return executor.result, nil
}

func wireRequestForSequence(workerID, placementID string, sequence int64, messageType MessageType, payload any, workspace *WorkspaceBundle) WireRequest {
	envelope := testEnvelope(workerID, placementID, sequence, messageType, payload)
	return WireRequest{ProtocolVersion: ProtocolVersionV1, RequestID: envelope.MessageID, Envelope: envelope, Workspace: workspace}
}

func containsBytes(raw, target []byte) bool {
	for index := 0; index+len(target) <= len(raw); index++ {
		if string(raw[index:index+len(target)]) == string(target) {
			return true
		}
	}
	return false
}
