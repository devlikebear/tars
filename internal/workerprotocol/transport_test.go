package workerprotocol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestInProcessTransportUsesVersionedJSONLContract(t *testing.T) {
	t.Parallel()

	handler := &recordingWireHandler{}
	transport, err := NewInProcessTransport(handler, WireLimits{MaxRequestBytes: 1 << 20, MaxResponseBytes: 1 << 20})
	if err != nil {
		t.Fatalf("new in-process transport: %v", err)
	}
	request := testWireRequest("worker-a", "placement-a", MessageHeartbeat, HeartbeatPayload{LeaseTTLMS: 1000})
	response, err := transport.Exchange(context.Background(), request)
	if err != nil {
		t.Fatalf("exchange in-process request: %v", err)
	}
	if !response.Accepted || response.RequestID != request.RequestID || handler.request.RequestID != request.RequestID {
		t.Fatalf("response=%+v handler request=%+v", response, handler.request)
	}
	request.Envelope.MessageID = "mutated-after-exchange"
	if handler.request.Envelope.MessageID == request.Envelope.MessageID {
		t.Fatal("in-process transport bypassed wire isolation")
	}
}

func TestSSHTransportSendsSecretsOnlyThroughBoundedStdin(t *testing.T) {
	t.Parallel()

	token := "tars-task-v1.payload.signature"
	request := testWireRequest("worker-a", "placement-a", MessageExecute, ExecutePayload{TaskToken: token, Request: json.RawMessage(`{"objective":"safe"}`)})
	response := WireResponse{ProtocolVersion: ProtocolVersionV1, RequestID: request.RequestID, Accepted: true}
	responseJSON, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	runner := &recordingProcessRunner{result: ProcessResult{Stdout: append(responseJSON, '\n')}}
	transport, err := NewSSHTransport(SSHTransportOptions{
		SSHPath: "/usr/bin/ssh", Host: "worker.example.com", User: "tars-worker", Port: 2222,
		IdentityFile: "/gateway/keys/worker_ed25519", KnownHostsFile: "/gateway/ssh/known_hosts",
		WorkerConfigPath: "/etc/tars/worker.json",
		Runner:           runner, Limits: WireLimits{MaxRequestBytes: 1 << 20, MaxResponseBytes: 1 << 20},
	})
	if err != nil {
		t.Fatalf("new SSH transport: %v", err)
	}
	got, err := transport.Exchange(context.Background(), request)
	if err != nil {
		t.Fatalf("exchange SSH request: %v", err)
	}
	if !got.Accepted || got.RequestID != request.RequestID {
		t.Fatalf("SSH response=%+v", got)
	}
	if runner.spec.Path != "/usr/bin/ssh" || runner.spec.InheritEnv || len(runner.spec.Env) != 0 {
		t.Fatalf("unsafe SSH process spec: %+v", runner.spec)
	}
	joinedArgs := strings.Join(runner.spec.Args, " ")
	if strings.Contains(joinedArgs, token) || strings.Contains(joinedArgs, "safe") {
		t.Fatalf("task content leaked through process arguments: %s", joinedArgs)
	}
	if !bytes.Contains(runner.spec.Stdin, []byte(token)) || runner.spec.Stdin[len(runner.spec.Stdin)-1] != '\n' {
		t.Fatalf("task token was not sent as one JSONL frame: %q", runner.spec.Stdin)
	}
	wantArgs := []string{
		"-T", "-F", "/dev/null", "-o", "BatchMode=yes", "-o", "ClearAllForwardings=yes",
		"-o", "PermitLocalCommand=no", "-o", "RequestTTY=no", "-o", "StrictHostKeyChecking=yes",
		"-o", "IdentitiesOnly=yes", "-o", "UserKnownHostsFile=/gateway/ssh/known_hosts",
		"-i", "/gateway/keys/worker_ed25519", "-p", "2222", "tars-worker@worker.example.com",
		"tars", "worker", "serve", "--stdio", "--protocol", ProtocolVersionV1,
		"--config", "/etc/tars/worker.json",
	}
	if !equalStrings(runner.spec.Args, wantArgs) {
		t.Fatalf("SSH args=%q want %q", runner.spec.Args, wantArgs)
	}
}

func TestSSHTransportRejectsInjectionAndBoundedOutput(t *testing.T) {
	t.Parallel()

	base := SSHTransportOptions{
		SSHPath: "/usr/bin/ssh", Host: "worker.example.com", User: "worker", Port: 22,
		IdentityFile: "/keys/id_ed25519", KnownHostsFile: "/ssh/known_hosts",
		Runner: &recordingProcessRunner{}, Limits: WireLimits{MaxRequestBytes: 1024, MaxResponseBytes: 128},
	}
	for _, mutate := range []func(*SSHTransportOptions){
		func(input *SSHTransportOptions) { input.Host = "worker.example.com -o ProxyCommand=evil" },
		func(input *SSHTransportOptions) { input.User = "worker;evil" },
		func(input *SSHTransportOptions) { input.IdentityFile = "-oProxyCommand=evil" },
		func(input *SSHTransportOptions) { input.KnownHostsFile = "known hosts" },
		func(input *SSHTransportOptions) { input.WorkerConfigPath = "--config=evil" },
	} {
		candidate := base
		mutate(&candidate)
		if _, err := NewSSHTransport(candidate); !errors.Is(err, ErrTransportConfig) {
			t.Fatalf("hostile SSH config error=%v want ErrTransportConfig", err)
		}
	}

	runner := &recordingProcessRunner{result: ProcessResult{Stdout: bytes.Repeat([]byte("x"), 129)}}
	base.Runner = runner
	transport, err := NewSSHTransport(base)
	if err != nil {
		t.Fatalf("new bounded SSH transport: %v", err)
	}
	if _, err := transport.Exchange(context.Background(), testWireRequest("worker-a", "placement-a", MessageHeartbeat, HeartbeatPayload{})); !errors.Is(err, ErrTransportLimit) {
		t.Fatalf("oversized response error=%v want ErrTransportLimit", err)
	}
}

func testWireRequest(workerID, placementID string, messageType MessageType, payload any) WireRequest {
	envelope := testEnvelope(workerID, placementID, 1, messageType, payload)
	return WireRequest{ProtocolVersion: ProtocolVersionV1, RequestID: envelope.MessageID, Envelope: envelope}
}

type recordingWireHandler struct {
	request WireRequest
}

func (handler *recordingWireHandler) Handle(_ context.Context, request WireRequest) (WireResponse, error) {
	handler.request = request
	return WireResponse{ProtocolVersion: ProtocolVersionV1, RequestID: request.RequestID, Accepted: true}, nil
}

type recordingProcessRunner struct {
	spec   ProcessSpec
	result ProcessResult
	err    error
}

func (runner *recordingProcessRunner) Run(_ context.Context, spec ProcessSpec) (ProcessResult, error) {
	runner.spec = spec
	return runner.result, runner.err
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
