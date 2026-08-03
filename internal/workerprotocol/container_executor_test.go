package workerprotocol

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestContainerReferenceExecutorEnforcesDefaultDenyAndBoundedResources(t *testing.T) {
	t.Parallel()

	response := ContainerTaskResponse{
		Succeeded: true, Output: json.RawMessage(`{"summary":"done"}`),
		Artifacts: []WireArtifact{{Name: "result.txt", MediaType: "text/plain", Data: []byte("done"), Digest: digestBytes([]byte("done"))}},
	}
	rawResponse, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingProcessRunner{result: ProcessResult{Stdout: append(rawResponse, '\n')}}
	executor, err := NewContainerReferenceExecutor(ContainerReferenceExecutorOptions{
		RuntimePath: "/usr/bin/docker", Image: "registry.example.com/tars-worker@sha256:" + strings.Repeat("a", 64),
		Command: []string{"/usr/local/bin/tars-task-harness"}, CPUs: "1.5", PIDsLimit: 64,
		Runner: runner, SupportsResume: true,
	})
	if err != nil {
		t.Fatalf("new container executor: %v", err)
	}
	root := t.TempDir()
	writeWorkspaceTestFile(t, root, "task.txt", []byte("task\n"), 0o644)
	request := ReferenceExecutionRequest{
		Binding: TaskTokenBinding{WorkspaceID: "ws", WorkID: "work", StepID: "step", AttemptID: "attempt", PlacementID: "placement", WorkerID: "worker"},
		RootDir: root, Policy: DefaultExecutionPolicy(), Request: json.RawMessage(`{"objective":"private objective"}`),
	}
	result, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("execute container task: %v", err)
	}
	if len(result.Artifacts) != 1 || !bytes.Contains(result.Payload, []byte(`"succeeded":true`)) {
		t.Fatalf("container result=%+v", result)
	}
	args := strings.Join(runner.spec.Args, " ")
	for _, required := range []string{
		"run --rm", "--network none", "--read-only", "--cpus 1.5", "--memory 2048m",
		"--pids-limit 64", "/workspace:rw,nosuid,nodev,size=4096m", "--workdir /workspace",
	} {
		if !strings.Contains(args, required) {
			t.Fatalf("container args missing %q: %s", required, args)
		}
	}
	if runner.spec.InheritEnv || len(runner.spec.Env) != 0 || strings.Contains(args, "private objective") {
		t.Fatalf("task content or host environment leaked through process spec: %+v", runner.spec)
	}
	if !bytes.Contains(runner.spec.Stdin, []byte("private objective")) || !bytes.Contains(runner.spec.Stdin, []byte("task.txt")) {
		t.Fatalf("container request did not carry bounded task and workspace on stdin: %q", runner.spec.Stdin)
	}
}

func TestContainerReferenceExecutorRejectsMutableImageAndEgressAllowlist(t *testing.T) {
	t.Parallel()

	if _, err := NewContainerReferenceExecutor(ContainerReferenceExecutorOptions{
		RuntimePath: "/usr/bin/docker", Image: "registry.example.com/tars-worker:latest",
		Command: []string{"worker"}, Runner: &recordingProcessRunner{},
	}); err == nil {
		t.Fatal("mutable container image was accepted")
	}
	executor, err := NewContainerReferenceExecutor(ContainerReferenceExecutorOptions{
		RuntimePath: "/usr/bin/docker", Image: "worker@sha256:" + strings.Repeat("b", 64),
		Command: []string{"worker"}, Runner: &recordingProcessRunner{},
	})
	if err != nil {
		t.Fatal(err)
	}
	policy := DefaultExecutionPolicy()
	policy.Egress = EgressPolicy{Mode: EgressAllowlist, AllowHosts: []string{"api.example.com"}}
	if _, err := executor.Execute(context.Background(), ReferenceExecutionRequest{
		Binding: TaskTokenBinding{WorkspaceID: "ws", WorkID: "work", StepID: "step", AttemptID: "attempt", PlacementID: "placement", WorkerID: "worker"},
		RootDir: t.TempDir(), Policy: policy,
	}); err == nil {
		t.Fatal("unsupported egress allowlist was accepted")
	}
}
