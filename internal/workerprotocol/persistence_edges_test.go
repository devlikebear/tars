package workerprotocol

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileRemoteRunStoreRejectsCorruptOrUnboundJournalState(t *testing.T) {
	t.Parallel()

	if _, err := NewFileRemoteRunStore(" "); err == nil {
		t.Fatal("blank remote run root was accepted")
	}
	root := filepath.Join(t.TempDir(), "remote-runs")
	store, err := NewFileRemoteRunStore(root)
	if err != nil {
		t.Fatal(err)
	}
	input := validRemoteRunInput("attempt-edge")
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.Prepare(canceled, input); !errors.Is(err, context.Canceled) {
		t.Fatalf("Prepare() canceled error=%v", err)
	}
	if _, _, err := store.Load(canceled, input.AttemptID); !errors.Is(err, context.Canceled) {
		t.Fatalf("Load() canceled error=%v", err)
	}
	if err := store.Delete(canceled, input.AttemptID); !errors.Is(err, context.Canceled) {
		t.Fatalf("Delete() canceled error=%v", err)
	}
	if err := store.Prepare(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if err := store.Prepare(context.Background(), input); err != nil {
		t.Fatalf("idempotent Prepare(): %v", err)
	}
	changed := input
	changed.Request = json.RawMessage(`{"changed":true}`)
	if err := store.Prepare(context.Background(), changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed prepared input error=%v", err)
	}
	if err := store.RecordResult(context.Background(), changed, RemoteRunResult{Succeeded: true}); !errors.Is(err, ErrConflict) {
		t.Fatalf("unprepared result error=%v", err)
	}
	if err := store.Delete(context.Background(), input.AttemptID); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), input.AttemptID); err != nil {
		t.Fatalf("idempotent Delete(): %v", err)
	}
	if _, found, err := store.Load(context.Background(), input.AttemptID); err != nil || found {
		t.Fatalf("deleted state found=%v err=%v", found, err)
	}

	var nilStore *FileRemoteRunStore
	if err := nilStore.Prepare(context.Background(), input); err == nil {
		t.Fatal("nil store prepared input")
	}
	if err := nilStore.RecordResult(context.Background(), input, RemoteRunResult{}); err == nil {
		t.Fatal("nil store recorded result")
	}
	if _, _, err := nilStore.Load(context.Background(), "../escape"); err == nil {
		t.Fatal("unsafe attempt ID loaded")
	}
	if err := nilStore.Delete(context.Background(), ""); err == nil {
		t.Fatal("blank attempt ID deleted")
	}

	path := filepath.Join(root, input.AttemptID+".json")
	corruptStates := []string{
		`{`,
		`{"schema_version":1} {}`,
		`{"schema_version":2,"attempt_id":"attempt-edge","input":{"attempt_id":"attempt-edge"}}`,
		`{"schema_version":1,"attempt_id":"other","input":{"attempt_id":"attempt-edge"}}`,
		`{"schema_version":1,"attempt_id":"attempt-edge","input":{"attempt_id":"other"}}`,
		`{"schema_version":1,"attempt_id":"attempt-edge","input":{"attempt_id":"attempt-edge","redact_values":["secret"]}}`,
		`{"schema_version":1,"attempt_id":"attempt-edge","input":{"attempt_id":"attempt-edge"},"unknown":true}`,
	}
	for index, raw := range corruptStates {
		if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, found, err := store.Load(context.Background(), input.AttemptID); err == nil || found {
			t.Errorf("corrupt state %d found=%v err=%v", index, found, err)
		}
	}
}

func TestPersistedRemoteResultValidationCoversEveryUntrustedField(t *testing.T) {
	t.Parallel()

	invalid := []RemoteRunResult{
		{Payload: json.RawMessage(`{`)},
		{Artifacts: []ReleasedArtifact{{URI: "file:///tmp/a", Digest: "sha256:a"}}},
		{Artifacts: []ReleasedArtifact{{Name: "a", Digest: "sha256:a"}}},
		{Artifacts: []ReleasedArtifact{{Name: "a", URI: "file:///tmp/a"}}},
		{Artifacts: []ReleasedArtifact{{Name: "a", URI: "file:///tmp/a", Digest: "sha256:a", SizeBytes: -1}}},
		{RejectedArtifacts: []RejectedArtifact{{Reason: "unsafe"}}},
		{RejectedArtifacts: []RejectedArtifact{{Name: "a"}}},
		{Checkpoint: &CheckpointPayload{ID: "bad id", Digest: "sha256:a"}},
		{Checkpoint: &CheckpointPayload{ID: "checkpoint-a"}},
	}
	for index, result := range invalid {
		if err := validatePersistedRemoteResult(result); err == nil {
			t.Errorf("invalid remote result %d accepted: %+v", index, result)
		}
	}
	valid := RemoteRunResult{
		Succeeded: true, Payload: json.RawMessage(`{"ok":true}`),
		Artifacts:         []ReleasedArtifact{{Name: "a", URI: "file:///tmp/a", Digest: "sha256:a", SizeBytes: 1}},
		RejectedArtifacts: []RejectedArtifact{{Name: "b", Reason: "scanner rejected"}},
		Checkpoint:        &CheckpointPayload{ID: "checkpoint-a", Digest: "sha256:a"},
	}
	if err := validatePersistedRemoteResult(valid); err != nil {
		t.Fatalf("valid remote result: %v", err)
	}
}

func TestWorkLedgerControlEventMappingAndSanitization(t *testing.T) {
	t.Parallel()

	if _, err := NewWorkLedgerSink(nil, "actor"); err == nil {
		t.Fatal("nil ledger sink was created")
	}
	var nilSink *WorkLedgerSink
	if err := nilSink.Record(context.Background(), ControlEvent{}); err == nil {
		t.Fatal("nil ledger sink recorded an event")
	}

	types := []string{
		"placement.created", string(MessageProvision), string(MessageSync), string(MessageLease),
		string(MessageHeartbeat), string(MessageExecute), string(MessageStream), string(MessageCheckpoint),
		string(MessageCollect), string(MessageDestroy), string(MessageLost), string(MessageReclaim), string(MessageRehydrate),
	}
	for _, eventType := range types {
		if got, ok := workLedgerEventType(eventType); !ok || got == "" {
			t.Errorf("event type %q mapping=(%q,%v)", eventType, got, ok)
		}
	}
	if got, ok := workLedgerEventType("credentials.rotated"); ok || got != "" {
		t.Fatalf("unsupported event mapping=(%q,%v)", got, ok)
	}

	payloadCases := map[string][]string{
		string(MessageProvision):  {"environment_id", "manifest_digest", "policy"},
		string(MessageSync):       {"mode", "digest", "file_count", "total_bytes"},
		string(MessageLease):      {"lease_ttl_ms", "usage"},
		string(MessageHeartbeat):  {"lease_ttl_ms", "usage"},
		string(MessageExecute):    {"resume", "checkpoint_id", "checkpoint_digest", "request_digest"},
		string(MessageStream):     {"kind", "text_bytes", "payload_digest"},
		string(MessageCheckpoint): {"checkpoint_id", "digest", "uri_digest"},
		string(MessageCollect):    {"complete", "succeeded", "snapshot_digest", "artifact_count"},
		string(MessageRehydrate):  {"replacement_worker_id", "environment_id", "snapshot_digest", "checkpoint_id", "checkpoint_digest"},
	}
	for eventType, keys := range payloadCases {
		raw := map[string]any{"task_token": "must-not-persist", "arbitrary": "drop"}
		for _, key := range keys {
			raw[key] = "safe"
		}
		encoded, _ := json.Marshal(raw)
		payload := workLedgerEventPayload(ControlEvent{Type: eventType, Payload: encoded})
		if _, leaked := payload["task_token"]; leaked {
			t.Fatalf("event %s leaked task token: %+v", eventType, payload)
		}
		if _, leaked := payload["arbitrary"]; leaked {
			t.Fatalf("event %s leaked arbitrary payload: %+v", eventType, payload)
		}
		for _, key := range keys {
			if payload[key] != "safe" {
				t.Errorf("event %s omitted %s: %+v", eventType, key, payload)
			}
		}
	}
	if payload := workLedgerEventPayload(ControlEvent{Type: "unknown", Payload: json.RawMessage(`{`)}); payload["payload_digest"] == nil {
		t.Fatalf("invalid JSON payload omitted digest: %+v", payload)
	}
	if len(safeLedgerPayloadKeys("unknown")) != 0 {
		t.Fatal("unknown event received safe payload keys")
	}
}

func TestSSHTransportErrorEvidenceIsBoundedAndCredentialFree(t *testing.T) {
	t.Parallel()

	token := "tars-task-v1.secret.signature"
	request := testWireRequest("worker-a", "placement-a", MessageExecute, ExecutePayload{TaskToken: token, Request: json.RawMessage(`{"safe":true}`)})
	runner := &recordingProcessRunner{
		result: ProcessResult{Stderr: []byte("Bearer raw-secret " + token + "\n" + strings.Repeat("x", 700))},
		err:    errors.New("ssh failed"),
	}
	transport, err := NewSSHTransport(SSHTransportOptions{
		SSHPath: "/usr/bin/ssh", Host: "2001:db8::1", User: "worker", Port: 22,
		IdentityFile: "/keys/id", KnownHostsFile: "/keys/known_hosts", Runner: runner,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(transport.arguments(), " "), "worker@[2001:db8::1]") {
		t.Fatalf("IPv6 SSH args=%q", transport.arguments())
	}
	_, err = transport.Exchange(context.Background(), request)
	if err == nil {
		t.Fatal("SSH failure was ignored")
	}
	message := err.Error()
	if strings.Contains(message, token) || strings.Contains(message, "raw-secret") || len(message) > 650 {
		t.Fatalf("unsafe SSH error=%q", message)
	}
	redacted := redactSSHFailure("line one\nline two "+token, request)
	if strings.Contains(redacted, token) || strings.Contains(redacted, "\n") {
		t.Fatalf("redacted SSH failure=%q", redacted)
	}

	transport.runner = &recordingProcessRunner{err: ErrTransportLimit}
	if _, err := transport.Exchange(context.Background(), request); !errors.Is(err, ErrTransportLimit) {
		t.Fatalf("transport limit error=%v", err)
	}
	var nilTransport *SSHTransport
	if _, err := nilTransport.Exchange(context.Background(), request); !errors.Is(err, ErrTransportConfig) {
		t.Fatalf("nil transport error=%v", err)
	}
}

func TestWorkerPolicyHostAndOwnershipValidationEdges(t *testing.T) {
	t.Parallel()

	validHosts := []string{"worker.example.com", "sub-1.example.com", "127.0.0.1"}
	for _, host := range validHosts {
		if !validEgressHost(host) {
			t.Errorf("valid egress host %q rejected", host)
		}
	}
	invalidHosts := []string{"", "localhost", ".example.com", "example.com.", "-bad.example.com", "bad-.example.com", "bad_name.example.com", "https://example.com", "user@example.com"}
	for _, host := range invalidHosts {
		if validEgressHost(host) {
			t.Errorf("invalid egress host %q accepted", host)
		}
	}
	if err := (ExecutionPolicy{Egress: EgressPolicy{Mode: EgressAllowlist}, Limits: DefaultExecutionPolicy().Limits}).Validate(); err == nil {
		t.Fatal("empty egress allowlist accepted")
	}
	for _, mode := range []SyncMode{"", "copy"} {
		if err := (SyncSpec{Mode: mode, SourceOwner: OwnerGateway, WorkspaceOwner: OwnerWorker, ArtifactOwner: OwnerGateway}).Validate(); err == nil {
			t.Errorf("invalid sync mode %q accepted", mode)
		}
	}
	if err := (SyncSpec{Mode: SyncModeDirectory, SourceOwner: OwnerWorker, WorkspaceOwner: OwnerWorker, ArtifactOwner: OwnerGateway}).Validate(); err == nil {
		t.Fatal("ambiguous source ownership accepted")
	}
	if !safeAbsoluteProcessPath("/usr/bin/ssh") || safeAbsoluteProcessPath("usr/bin/ssh") || safeAbsoluteProcessPath("/tmp/../usr/bin/ssh") || safeAbsoluteProcessPath("/tmp/-ssh") {
		t.Fatal("absolute process path validation did not fail closed")
	}
}

func validRemoteRunInput(attemptID string) RemoteRunInput {
	return RemoteRunInput{
		PlacementID: "placement-" + attemptID, EnvironmentID: "environment-" + attemptID,
		WorkspaceID: "workspace-a", WorkID: "work-a", StepID: "step-a", AttemptID: attemptID,
		Policy: DefaultExecutionPolicy(), Workspace: testRemoteWorkspaceBundle(), Request: json.RawMessage(`{"safe":true}`),
	}
}

func TestRemoteRunCloneDropsCredentialsAndDeepCopiesMutableData(t *testing.T) {
	t.Parallel()

	input := validRemoteRunInput("attempt-clone")
	input.RedactValues = []string{"secret"}
	clone := cloneRemoteRunInput(input)
	if len(clone.RedactValues) != 0 {
		t.Fatalf("clone retained redaction secret: %+v", clone.RedactValues)
	}
	input.Request[0] = 'x'
	input.Workspace.Files[0].Data[0] = 'x'
	if bytes.Equal(input.Request, clone.Request) || bytes.Equal(input.Workspace.Files[0].Data, clone.Workspace.Files[0].Data) {
		t.Fatal("remote run clone shared mutable request or workspace bytes")
	}
}

func TestCoordinatorConstructionAndInputValidationFailClosed(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 3, 13, 0, 0, 0, time.UTC)
	seed := make([]byte, ed25519.SeedSize)
	seed[0] = 73
	issuer, err := NewTaskTokenIssuer(TaskTokenIssuerOptions{
		PrivateKey: ed25519.NewKeyFromSeed(seed), MaxTTL: 2 * time.Minute, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	executor := &recordingReferenceExecutor{}
	worker, err := NewReferenceWorker(ReferenceWorkerOptions{
		WorkerID: "worker-edge", RootDir: t.TempDir(), TokenVerifier: issuer.PublicVerifier(), Executor: executor,
	})
	if err != nil {
		t.Fatal(err)
	}
	transport, err := NewInProcessTransport(worker, DefaultWireLimits())
	if err != nil {
		t.Fatal(err)
	}
	controller, err := OpenController(ControllerOptions{StatePath: filepath.Join(t.TempDir(), "controller.json")})
	if err != nil {
		t.Fatal(err)
	}
	quarantine, err := NewArtifactQuarantine(ArtifactQuarantineOptions{RootDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	base := GatewayCoordinatorOptions{
		Controller: controller, WorkerID: "worker-edge", TransportName: "in-process", Endpoint: "local://worker-edge",
		Capabilities: executor.Capabilities(), Transport: transport, TokenIssuer: issuer, Quarantine: quarantine,
		LeaseTTL: time.Minute, TokenTTL: 30 * time.Second,
	}
	invalid := []func(*GatewayCoordinatorOptions){
		func(opts *GatewayCoordinatorOptions) { opts.Controller = nil },
		func(opts *GatewayCoordinatorOptions) { opts.Transport = nil },
		func(opts *GatewayCoordinatorOptions) { opts.TokenIssuer = nil },
		func(opts *GatewayCoordinatorOptions) { opts.Quarantine = nil },
		func(opts *GatewayCoordinatorOptions) { opts.WorkerID = "bad worker" },
		func(opts *GatewayCoordinatorOptions) { opts.TransportName = "" },
		func(opts *GatewayCoordinatorOptions) { opts.Endpoint = "" },
		func(opts *GatewayCoordinatorOptions) { opts.Capabilities.EgressPolicy = false },
		func(opts *GatewayCoordinatorOptions) { opts.Capabilities.ResourceLimits = false },
		func(opts *GatewayCoordinatorOptions) { opts.LeaseTTL = 0 },
		func(opts *GatewayCoordinatorOptions) { opts.TokenTTL = 0 },
		func(opts *GatewayCoordinatorOptions) { opts.TokenTTL = 2 * time.Minute },
	}
	for index, mutate := range invalid {
		opts := base
		mutate(&opts)
		if _, err := NewGatewayCoordinator(opts); err == nil {
			t.Fatalf("invalid coordinator options %d accepted: %+v", index, opts)
		}
	}
	coordinator, err := NewGatewayCoordinator(base)
	if err != nil || coordinator.now == nil {
		t.Fatalf("valid coordinator = %+v err=%v", coordinator, err)
	}
	var nilCoordinator *GatewayCoordinator
	if _, err := nilCoordinator.Run(context.Background(), validRemoteRunInput("nil-run")); err == nil {
		t.Fatal("nil coordinator ran work")
	}
	if _, err := nilCoordinator.RecoverPrepared(context.Background(), validRemoteRunInput("nil-recovery")); err == nil {
		t.Fatal("nil coordinator recovered work")
	}
	if _, err := nilCoordinator.Resume(context.Background(), RemoteRecoveryInput{}); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("nil coordinator resume error=%v", err)
	}
	if err := nilCoordinator.FinalizeRecorded(context.Background(), validRemoteRunInput("nil-finalize"), RemoteRunResult{}); err == nil {
		t.Fatal("nil coordinator finalized work")
	}

	valid := validRemoteRunInput("input-validation")
	if err := validateRemoteRunInput(valid); err != nil {
		t.Fatalf("valid remote input: %v", err)
	}
	invalidInputs := []func(*RemoteRunInput){
		func(input *RemoteRunInput) { input.PlacementID = "" },
		func(input *RemoteRunInput) { input.EnvironmentID = "" },
		func(input *RemoteRunInput) { input.WorkspaceID = "" },
		func(input *RemoteRunInput) { input.WorkID = "" },
		func(input *RemoteRunInput) { input.StepID = "" },
		func(input *RemoteRunInput) { input.AttemptID = "" },
		func(input *RemoteRunInput) { input.Policy.Limits.CPUSeconds = 0 },
		func(input *RemoteRunInput) { input.Workspace.Manifest.Digest = "sha256:wrong" },
		func(input *RemoteRunInput) { input.Request = json.RawMessage(`{`) },
	}
	for index, mutate := range invalidInputs {
		input := valid
		input.Workspace = cloneWorkspaceBundle(valid.Workspace)
		mutate(&input)
		if err := validateRemoteRunInput(input); err == nil {
			t.Fatalf("invalid remote input %d accepted: %+v", index, input)
		}
	}
	for _, testCase := range []struct {
		payload json.RawMessage
		want    bool
		ok      bool
	}{
		{json.RawMessage(`{"succeeded":true}`), true, true},
		{json.RawMessage(`{"succeeded":false}`), false, true},
		{json.RawMessage(`{"summary":"missing"}`), false, false},
		{json.RawMessage(`{`), false, false},
	} {
		got, err := remoteResponseSucceeded(testCase.payload)
		if (err == nil) != testCase.ok || got != testCase.want {
			t.Errorf("remote response %s = %v err=%v", testCase.payload, got, err)
		}
	}
	snapshot := ControllerSnapshot{Events: []ControlEvent{
		{PlacementID: "placement-a", Type: MessageExecute.String(), Sequence: 2},
		{PlacementID: "placement-b", Type: MessageExecute.String(), Sequence: 9},
		{PlacementID: "placement-a", Type: MessageCollect.String(), Sequence: 7},
		{PlacementID: "placement-a", Type: MessageExecute.String(), Sequence: 5},
	}}
	if got := latestPlacementEventSequence(snapshot, "placement-a", MessageExecute); got != 5 {
		t.Fatalf("latest execute sequence=%d", got)
	}
	if got := latestPlacementEventSequence(snapshot, "missing", MessageExecute); got != 0 {
		t.Fatalf("missing execute sequence=%d", got)
	}
}

func TestReferenceWorkerPersistenceAndResultValidationRejectTampering(t *testing.T) {
	t.Parallel()

	seed := make([]byte, ed25519.SeedSize)
	seed[0] = 74
	issuer, err := NewTaskTokenIssuer(TaskTokenIssuerOptions{PrivateKey: ed25519.NewKeyFromSeed(seed), MaxTTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	executor := &recordingReferenceExecutor{}
	root := t.TempDir()
	if _, err := NewReferenceWorker(ReferenceWorkerOptions{
		WorkerID: "worker-edge", RootDir: "", TokenVerifier: issuer.PublicVerifier(), Executor: executor,
	}); err == nil {
		t.Fatal("reference worker accepted an empty root")
	}
	if _, err := NewReferenceWorker(ReferenceWorkerOptions{
		WorkerID: "worker-edge", RootDir: root, StatePath: root,
		TokenVerifier: issuer.PublicVerifier(), Executor: executor,
	}); err == nil {
		t.Fatal("reference worker accepted its root as the state file")
	}
	if _, err := NewReferenceWorker(ReferenceWorkerOptions{
		WorkerID: "worker-edge", RootDir: root, StatePath: filepath.Join(root, "placements", "state.json"),
		TokenVerifier: issuer.PublicVerifier(), Executor: executor,
	}); err == nil {
		t.Fatal("reference worker accepted a placement-owned state file")
	}
	stateTarget := filepath.Join(t.TempDir(), "state-target.json")
	if err := os.WriteFile(stateTarget, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stateLink := filepath.Join(t.TempDir(), "state-link.json")
	if err := os.Symlink(stateTarget, stateLink); err != nil {
		t.Fatal(err)
	}
	if _, err := NewReferenceWorker(ReferenceWorkerOptions{
		WorkerID: "worker-edge", RootDir: t.TempDir(), StatePath: stateLink,
		TokenVerifier: issuer.PublicVerifier(), Executor: executor,
	}); err == nil {
		t.Fatal("reference worker accepted a symlinked state file")
	}
	if _, err := NewReferenceWorker(ReferenceWorkerOptions{
		WorkerID: "worker-edge", RootDir: t.TempDir(), TokenVerifier: issuer.PublicVerifier(),
		Executor: limitedReferenceExecutor{},
	}); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("unenforced executor capabilities error=%v", err)
	}
	for index, raw := range []string{`{`, `{"schema_version":2,"worker_id":"worker-edge"}`} {
		stateDir := t.TempDir()
		statePath := filepath.Join(stateDir, "worker-state.json")
		if err := os.WriteFile(statePath, []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := NewReferenceWorker(ReferenceWorkerOptions{
			WorkerID: "worker-edge", RootDir: t.TempDir(), StatePath: statePath,
			TokenVerifier: issuer.PublicVerifier(), Executor: executor,
		}); err == nil {
			t.Fatalf("corrupt reference state %d accepted", index)
		}
	}

	worker, err := NewReferenceWorker(ReferenceWorkerOptions{
		WorkerID: "worker-edge", RootDir: t.TempDir(), TokenVerifier: issuer.PublicVerifier(), Executor: executor,
	})
	if err != nil {
		t.Fatal(err)
	}
	var nilWorker *ReferenceWorker
	if _, err := nilWorker.Handle(context.Background(), WireRequest{}); err == nil {
		t.Fatal("nil reference worker handled a request")
	}
	binding := TaskTokenBinding{
		WorkspaceID: "workspace-a", WorkID: "work-a", StepID: "step-a", AttemptID: "attempt-a",
		PlacementID: "placement-a", WorkerID: worker.workerID,
	}
	validEnvironment := ReferenceEnvironment{
		EnvironmentID: "environment-a", PlacementID: "placement-a", Binding: binding,
		RootDir: filepath.Join(worker.placements, "placement-a"), Policy: DefaultExecutionPolicy(),
		State: PlacementStateReady, LastSequence: 1,
	}
	if err := worker.validatePersistedEnvironment("placement-a", validEnvironment); err != nil {
		t.Fatalf("valid persisted environment: %v", err)
	}
	invalidEnvironments := []func(*ReferenceEnvironment){
		func(environment *ReferenceEnvironment) { environment.PlacementID = "other" },
		func(environment *ReferenceEnvironment) { environment.Binding.PlacementID = "other" },
		func(environment *ReferenceEnvironment) { environment.Binding.WorkerID = "other" },
		func(environment *ReferenceEnvironment) { environment.RootDir = worker.placements },
		func(environment *ReferenceEnvironment) { environment.LastSequence = 0 },
		func(environment *ReferenceEnvironment) { environment.State = "unknown" },
		func(environment *ReferenceEnvironment) { environment.Binding.WorkID = "" },
		func(environment *ReferenceEnvironment) { environment.Policy.Limits.CPUSeconds = 0 },
	}
	for index, mutate := range invalidEnvironments {
		environment := validEnvironment
		mutate(&environment)
		if err := worker.validatePersistedEnvironment("placement-a", environment); err == nil {
			t.Fatalf("invalid persisted environment %d accepted: %+v", index, environment)
		}
	}
	for _, state := range []PlacementState{
		PlacementStatePending, PlacementStateProvisioning, PlacementStateSyncing, PlacementStateReady,
		PlacementStateExecuting, PlacementStateCheckpointed, PlacementStateCollecting,
		PlacementStateCompleted, PlacementStateFailed, PlacementStateLost, PlacementStateReclaiming,
		PlacementStateRehydrating, PlacementStateDestroyed,
	} {
		if !validPlacementState(state) {
			t.Errorf("valid placement state %q rejected", state)
		}
	}
	if validPlacementState("unknown") {
		t.Fatal("unknown placement state accepted")
	}

	policy := DefaultExecutionPolicy()
	data := []byte("safe")
	validResult := ReferenceExecutionResult{
		Payload:   json.RawMessage(`{"succeeded":true}`),
		Artifacts: []WireArtifact{{Name: "result.txt", Digest: digestBytes(data), Data: data}},
	}
	if err := validateReferenceExecutionResult(validResult, policy, nil); err != nil {
		t.Fatalf("valid reference result: %v", err)
	}
	invalidResults := []struct {
		result    ReferenceExecutionResult
		policy    ExecutionPolicy
		forbidden []string
	}{
		{ReferenceExecutionResult{Payload: json.RawMessage(`{`)}, policy, nil},
		{ReferenceExecutionResult{Payload: json.RawMessage(`{"token":"secret"}`)}, policy, []string{"secret"}},
		{ReferenceExecutionResult{Artifacts: []WireArtifact{{Name: "result.txt", Digest: digestBytes(data), Data: data}}}, policy, []string{"safe"}},
		{validResult, ExecutionPolicy{Egress: policy.Egress, Limits: ResourceLimits{CPUSeconds: 1, MemoryMB: 1, DiskMB: 1, MaxOutputBytes: 1}}, nil},
		{ReferenceExecutionResult{Checkpoint: &CheckpointPayload{ID: "bad id", Digest: "sha256:x"}}, policy, nil},
		{ReferenceExecutionResult{Checkpoint: &CheckpointPayload{ID: "checkpoint-a"}}, policy, nil},
	}
	for index, testCase := range invalidResults {
		if err := validateReferenceExecutionResult(testCase.result, testCase.policy, testCase.forbidden); err == nil {
			t.Fatalf("invalid reference result %d accepted: %+v", index, testCase.result)
		}
	}
	if !containsForbiddenResultValue([]byte("prefix-secret-suffix"), []string{"", "secret"}) ||
		containsForbiddenResultValue([]byte("safe"), []string{"secret"}) {
		t.Fatal("forbidden result value classification failed")
	}

	if err := os.MkdirAll(validEnvironment.RootDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := worker.loadResultLocked(validEnvironment); err == nil {
		t.Fatal("missing reference result loaded")
	}
	if err := os.WriteFile(worker.resultPath(validEnvironment), []byte(`{`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := worker.loadResultLocked(validEnvironment); err == nil {
		t.Fatal("corrupt reference result loaded")
	}
	if err := worker.saveResultLocked(validEnvironment, validResult); err != nil {
		t.Fatal(err)
	}
	tampered := validEnvironment
	tampered.ResultDigest = "sha256:tampered"
	if _, err := worker.loadResultLocked(tampered); !errors.Is(err, ErrManifestMismatch) {
		t.Fatalf("tampered reference result error=%v", err)
	}
	if loaded, err := worker.loadResultLocked(validEnvironment); err != nil || !bytes.Equal(loaded.Payload, validResult.Payload) {
		t.Fatalf("loaded reference result=%+v err=%v", loaded, err)
	}
}

type limitedReferenceExecutor struct{}

func (limitedReferenceExecutor) Capabilities() WorkerCapabilities {
	return WorkerCapabilities{}
}

func (limitedReferenceExecutor) Execute(context.Context, ReferenceExecutionRequest) (ReferenceExecutionResult, error) {
	return ReferenceExecutionResult{}, nil
}
