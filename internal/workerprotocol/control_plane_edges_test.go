package workerprotocol

import (
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

func TestOSProcessRunnerBoundsOutputAndPreservesExitEvidence(t *testing.T) {
	t.Parallel()

	runner := OSProcessRunner{}
	if _, err := runner.Run(context.Background(), ProcessSpec{}); !errors.Is(err, ErrTransportConfig) {
		t.Fatalf("empty process spec error = %v", err)
	}
	result, err := runner.Run(context.Background(), ProcessSpec{
		Path: "/bin/sh", Args: []string{"-c", "printf stdout; printf stderr >&2"},
		Env: []string{"PATH=/usr/bin:/bin"}, MaxOutputBytes: 64,
	})
	if err != nil || result.ExitCode != 0 || string(result.Stdout) != "stdout" || string(result.Stderr) != "stderr" {
		t.Fatalf("successful process result = %+v err=%v", result, err)
	}
	result, err = runner.Run(context.Background(), ProcessSpec{
		Path: "/bin/sh", Args: []string{"-c", "printf failed >&2; exit 7"},
		InheritEnv: true, MaxOutputBytes: 64,
	})
	if err == nil || result.ExitCode != 7 || string(result.Stderr) != "failed" {
		t.Fatalf("failed process result = %+v err=%v", result, err)
	}
	result, err = runner.Run(context.Background(), ProcessSpec{
		Path: "/bin/sh", Args: []string{"-c", "printf 123456789"}, MaxOutputBytes: 4,
	})
	if !errors.Is(err, ErrTransportLimit) || string(result.Stdout) != "1234" {
		t.Fatalf("bounded process result = %+v err=%v", result, err)
	}

	buffer := newBoundedProcessBuffer(3)
	if written, err := buffer.Write([]byte("abcd")); err != nil || written != 4 || string(buffer.Bytes()) != "abc" || !buffer.overflow {
		t.Fatalf("bounded buffer written=%d bytes=%q overflow=%v err=%v", written, buffer.Bytes(), buffer.overflow, err)
	}
	if written, err := buffer.Write([]byte("z")); err != nil || written != 1 || string(buffer.Bytes()) != "abc" {
		t.Fatalf("full bounded buffer written=%d bytes=%q err=%v", written, buffer.Bytes(), err)
	}
}

func TestWireCodecRejectsAmbiguousOrUnboundFrames(t *testing.T) {
	t.Parallel()

	request := testWireRequest("worker-a", "placement-a", MessageHeartbeat, HeartbeatPayload{LeaseTTLMS: 1000})
	limits := WireLimits{MaxRequestBytes: 1 << 20, MaxResponseBytes: 1 << 20}
	encoded, err := encodeWireRequest(request, limits)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := decodeWireRequest(encoded, limits); err != nil || decoded.RequestID != request.RequestID {
		t.Fatalf("decoded request = %+v err=%v", decoded, err)
	}
	response := WireResponse{ProtocolVersion: ProtocolVersionV1, RequestID: request.RequestID, Accepted: true, Payload: json.RawMessage(`{"ok":true}`)}
	encodedResponse, err := encodeWireResponse(response, request.RequestID, limits)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := decodeWireResponse(encodedResponse, request.RequestID, limits); err != nil || !decoded.Accepted {
		t.Fatalf("decoded response = %+v err=%v", decoded, err)
	}

	if (WireLimits{}).Validate() == nil {
		t.Fatal("zero wire limits accepted")
	}
	if _, err := encodeWireRequest(request, WireLimits{}); !errors.Is(err, ErrTransportConfig) {
		t.Fatalf("invalid request limits error = %v", err)
	}
	if _, err := encodeWireRequest(request, WireLimits{MaxRequestBytes: 1, MaxResponseBytes: 1}); !errors.Is(err, ErrTransportLimit) {
		t.Fatalf("request limit error = %v", err)
	}
	if _, err := decodeWireRequest(encoded, WireLimits{MaxRequestBytes: 1, MaxResponseBytes: 1}); !errors.Is(err, ErrTransportLimit) {
		t.Fatalf("decode request limit error = %v", err)
	}
	for _, raw := range [][]byte{nil, []byte("{}\n{}"), []byte(`{"unknown":true}`), []byte(`{} {}`)} {
		if err := decodeSingleJSONLine(raw, &WireRequest{}); !errors.Is(err, ErrWireContract) {
			t.Fatalf("ambiguous frame %q error = %v", raw, err)
		}
	}
	badRequest := request
	badRequest.RequestID = "different"
	if err := badRequest.Validate(); !errors.Is(err, ErrWireContract) {
		t.Fatalf("unbound request error = %v", err)
	}
	badRequest = request
	badRequest.Workspace = &WorkspaceBundle{}
	if err := badRequest.Validate(); !errors.Is(err, ErrWireContract) {
		t.Fatalf("workspace on heartbeat error = %v", err)
	}

	badResponses := []WireResponse{
		{ProtocolVersion: "2", RequestID: request.RequestID, Accepted: true},
		{ProtocolVersion: ProtocolVersionV1, RequestID: request.RequestID, Accepted: false},
		{ProtocolVersion: ProtocolVersionV1, RequestID: request.RequestID, Accepted: true, Payload: json.RawMessage(`{`)},
		{ProtocolVersion: ProtocolVersionV1, RequestID: request.RequestID, Accepted: true, Artifacts: []WireArtifact{{Name: "../secret", Digest: "sha256:x"}}},
		{ProtocolVersion: ProtocolVersionV1, RequestID: request.RequestID, Accepted: true, Artifacts: []WireArtifact{{Name: "safe.txt"}}},
		{ProtocolVersion: ProtocolVersionV1, RequestID: request.RequestID, Accepted: true, Checkpoint: &CheckpointPayload{}},
	}
	for index, candidate := range badResponses {
		if err := candidate.Validate(request.RequestID); !errors.Is(err, ErrWireContract) {
			t.Fatalf("bad response %d error = %v", index, err)
		}
	}
	if _, err := encodeWireResponse(response, request.RequestID, WireLimits{MaxRequestBytes: 1, MaxResponseBytes: 1}); !errors.Is(err, ErrTransportLimit) {
		t.Fatalf("response limit error = %v", err)
	}
	if _, err := decodeWireResponse(encodedResponse, request.RequestID, WireLimits{MaxRequestBytes: 1, MaxResponseBytes: 1}); !errors.Is(err, ErrTransportLimit) {
		t.Fatalf("decode response limit error = %v", err)
	}
	var nilTransport *InProcessTransport
	if _, err := nilTransport.Exchange(context.Background(), request); !errors.Is(err, ErrTransportConfig) {
		t.Fatalf("nil transport error = %v", err)
	}
	if _, err := NewInProcessTransport(nil, limits); !errors.Is(err, ErrTransportConfig) {
		t.Fatalf("nil handler error = %v", err)
	}
}

func TestSchedulerGatewayValidationRequiresPrivateIsolatedBounds(t *testing.T) {
	t.Parallel()

	base := SchedulerGatewayConfig{
		SchemaVersion: 1, Adapter: "remote-ssh", WorkerID: "worker-a", Transport: SchedulerTransportSSH,
		PrivateKey: "private", LeaseTTLSeconds: 60, TokenTTLSeconds: 30, SyncMode: SyncModeDirectory,
		Policy: DefaultExecutionPolicy(), Capabilities: WorkerCapabilities{EgressPolicy: true, ResourceLimits: true, ArtifactScan: true},
	}
	if err := validateSchedulerGatewayConfig(&base); err != nil || base.WireLimits == (WireLimits{}) || base.BundleLimits == (WorkspaceBundleLimits{}) {
		t.Fatalf("valid scheduler config = %+v err=%v", base, err)
	}
	mutations := []func(*SchedulerGatewayConfig){
		func(config *SchedulerGatewayConfig) { config.SchemaVersion = 2 },
		func(config *SchedulerGatewayConfig) { config.TokenTTLSeconds = 61 },
		func(config *SchedulerGatewayConfig) { config.SyncMode = "invalid" },
		func(config *SchedulerGatewayConfig) {
			config.Policy.Egress.Mode = EgressAllowlist
			config.Policy.Egress.AllowHosts = []string{"example.test"}
		},
		func(config *SchedulerGatewayConfig) { config.BundleLimits.MaxFiles = -1 },
		func(config *SchedulerGatewayConfig) { config.SyncMode = SyncModeGit; config.GitPath = "git" },
		func(config *SchedulerGatewayConfig) {
			config.Transport = SchedulerTransportInProcess
			config.InProcess.WorkerConfigPath = "relative.json"
		},
		func(config *SchedulerGatewayConfig) { config.Capabilities = WorkerCapabilities{} },
		func(config *SchedulerGatewayConfig) {
			config.WireLimits = WireLimits{MaxRequestBytes: -1, MaxResponseBytes: 1}
		},
		func(config *SchedulerGatewayConfig) { config.Transport = "unknown" },
	}
	for index, mutate := range mutations {
		candidate := base
		candidate.Policy.Egress.AllowHosts = append([]string(nil), base.Policy.Egress.AllowHosts...)
		mutate(&candidate)
		if err := validateSchedulerGatewayConfig(&candidate); err == nil {
			t.Fatalf("unsafe scheduler config %d accepted: %+v", index, candidate)
		}
	}
	if err := validateSchedulerGatewayConfig(nil); !errors.Is(err, ErrTransportConfig) {
		t.Fatalf("nil scheduler config error = %v", err)
	}

	source := t.TempDir()
	outside := t.TempDir()
	configPath := filepath.Join(outside, "gateway.json")
	if err := os.WriteFile(configPath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(outside, "data")
	canonicalSource, canonicalData, err := secureSchedulerDirectories(source, data, configPath)
	if err != nil || canonicalSource == "" || canonicalData == "" {
		t.Fatalf("secure scheduler directories source=%q data=%q err=%v", canonicalSource, canonicalData, err)
	}
	if _, _, err := secureSchedulerDirectories("", data, configPath); err == nil {
		t.Fatal("empty scheduler source accepted")
	}
	if _, _, err := secureSchedulerDirectories(source, filepath.Join(source, "data"), configPath); err == nil {
		t.Fatal("workspace-overlapping scheduler data accepted")
	}
	insideConfig := filepath.Join(source, "gateway.json")
	if err := os.WriteFile(insideConfig, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := secureSchedulerDirectories(source, data, insideConfig); err == nil {
		t.Fatal("workspace scheduler config accepted")
	}
	if !schedulerPathsOverlap(source, filepath.Join(source, "child")) || schedulerPathsOverlap(source, outside) {
		t.Fatal("scheduler overlap classification is incorrect")
	}
}

func TestReferenceWorkerRejectsInvalidTransitionsWithoutAdvancingSequence(t *testing.T) {
	t.Parallel()

	if (*ReferenceWorker)(nil).VerificationKeyID() != "" || (*ReferenceWorker)(nil).MaxTaskTokenTTL() != 0 ||
		(*ReferenceWorker)(nil).RootDir() != "" || len((*ReferenceWorker)(nil).Snapshot().Environments) != 0 {
		t.Fatal("nil reference worker accessors are not safe")
	}
	if _, err := NewReferenceWorker(ReferenceWorkerOptions{}); err == nil {
		t.Fatal("empty reference worker options accepted")
	}

	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	seed := make([]byte, ed25519.SeedSize)
	seed[0] = 91
	issuer, err := NewTaskTokenIssuer(TaskTokenIssuerOptions{
		PrivateKey: ed25519.NewKeyFromSeed(seed), MaxTTL: time.Minute, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	executor := &recordingReferenceExecutor{}
	worker, err := NewReferenceWorker(ReferenceWorkerOptions{
		WorkerID: "worker-a", RootDir: t.TempDir(), TokenVerifier: issuer.PublicVerifier(), Executor: executor,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if response, err := worker.Handle(context.Background(), WireRequest{}); err == nil || response.Accepted {
		t.Fatalf("invalid wire request response=%+v err=%v", response, err)
	}
	register := wireRequestForSequence("worker-b", "", 1, MessageRegister, RegisterPayload{}, nil)
	if response, err := worker.Handle(context.Background(), register); err != nil || response.Accepted || response.ErrorCode != "worker_mismatch" {
		t.Fatalf("wrong worker response=%+v err=%v", response, err)
	}
	unsupported := wireRequestForSequence("worker-a", "", 1, MessageExecute, ExecutePayload{}, nil)
	if response := worker.handleLocked(context.Background(), unsupported); response.Accepted || response.ErrorCode != "placement_required" {
		t.Fatalf("placement-required response=%+v", response)
	}

	binding := TaskTokenBinding{
		WorkspaceID: "workspace", WorkID: "work", StepID: "step", AttemptID: "attempt",
		PlacementID: "placement", WorkerID: "worker-a",
	}
	provision := wireRequestForSequence("worker-a", "placement", 1, MessageProvision, ProvisionPayload{
		EnvironmentID: "environment", Binding: binding, Policy: DefaultExecutionPolicy(),
	}, nil)
	if response, err := worker.Handle(context.Background(), provision); err != nil || !response.Accepted {
		t.Fatalf("provision response=%+v err=%v", response, err)
	}

	rejections := []struct {
		message MessageType
		payload any
		code    string
	}{
		{MessageProvision, ProvisionPayload{EnvironmentID: "environment", Binding: binding, Policy: DefaultExecutionPolicy()}, "invalid_transition"},
		{MessageSync, SyncPayload{}, "invalid_transition"},
		{MessageLease, LeasePayload{LeaseTTLMS: 1000}, "invalid_transition"},
		{MessageExecute, ExecutePayload{}, "invalid_transition"},
		{MessageCheckpoint, CheckpointPayload{}, "invalid_transition"},
		{MessageCollect, CollectPayload{}, "invalid_transition"},
		{MessageDestroy, json.RawMessage(`{"task_token":1}`), "invalid_destroy"},
	}
	for index, testCase := range rejections {
		request := wireRequestForSequence("worker-a", "placement", 2, testCase.message, testCase.payload, nil)
		request.Envelope.MessageID += "-" + strings.Repeat("x", index+1)
		request.Envelope.IdempotencyKey += "-" + strings.Repeat("x", index+1)
		request.RequestID = request.Envelope.MessageID
		response, err := worker.Handle(context.Background(), request)
		if err != nil || response.Accepted || response.ErrorCode != testCase.code {
			t.Fatalf("rejection %d response=%+v err=%v", index, response, err)
		}
	}
	snapshot := worker.Snapshot()
	if snapshot.Environments["placement"].LastSequence != 1 || executor.calls != 0 {
		t.Fatalf("rejections advanced worker state: %+v calls=%d", snapshot, executor.calls)
	}
}
