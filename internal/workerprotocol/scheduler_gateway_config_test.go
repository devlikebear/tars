package workerprotocol

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenConfiguredSchedulerExecutorRunsContainerThroughRemoteLifecycle(t *testing.T) {
	t.Parallel()

	seed := make([]byte, ed25519.SeedSize)
	seed[0] = 61
	privateKey := ed25519.NewKeyFromSeed(seed)
	workerRoot := t.TempDir()
	workerConfigPath := filepath.Join(t.TempDir(), "worker.json")
	writeJSONConfig(t, workerConfigPath, WorkerServiceConfig{
		SchemaVersion: workerServiceConfigSchemaVersion,
		WorkerID:      "container-worker", RootDir: workerRoot, StatePath: filepath.Join(workerRoot, "state.json"),
		PublicKey: base64.RawStdEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey)), MaxTokenTTLSeconds: 300,
		WireLimits: WireLimits{MaxRequestBytes: 4 << 20, MaxResponseBytes: 4 << 20},
		Container: ContainerWorkerConfig{
			RuntimePath: "/usr/bin/docker", Image: "worker@sha256:" + strings.Repeat("d", 64),
			Command: []string{"/usr/local/bin/tars-task-harness"}, CPUs: "1", PIDsLimit: 64,
		},
	})
	gatewayConfigPath := filepath.Join(t.TempDir(), "gateway.json")
	writeJSONConfig(t, gatewayConfigPath, SchedulerGatewayConfig{
		SchemaVersion: schedulerGatewayConfigSchemaVersion, Adapter: "remote-container",
		WorkerID: "container-worker", Transport: SchedulerTransportInProcess,
		PrivateKey:      base64.RawStdEncoding.EncodeToString(privateKey),
		LeaseTTLSeconds: 120, TokenTTLSeconds: 60, SyncMode: SyncModeDirectory,
		Policy:    DefaultExecutionPolicy(),
		InProcess: SchedulerInProcessConfig{WorkerConfigPath: workerConfigPath},
	})
	controller, err := OpenController(ControllerOptions{StatePath: filepath.Join(t.TempDir(), "controller.json")})
	if err != nil {
		t.Fatal(err)
	}
	response, err := json.Marshal(ContainerTaskResponse{
		Succeeded: true, Output: json.RawMessage(`{"summary":"container lifecycle passed"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingProcessRunner{result: ProcessResult{Stdout: append(response, '\n')}}
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "task.txt"), []byte("task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	executor, err := OpenConfiguredSchedulerExecutor(ConfiguredSchedulerExecutorOptions{
		ConfigPath: gatewayConfigPath, SourceDir: source, DataDir: t.TempDir(),
		Controller: controller, Runner: runner,
	})
	if err != nil {
		t.Fatalf("open configured scheduler executor: %v", err)
	}
	result, err := executor.Execute(context.Background(), testRemoteSchedulerExecution("attempt-container"))
	if err != nil {
		t.Fatalf("execute configured remote container: %v", err)
	}
	if !result.Succeeded || !strings.Contains(string(result.OutputJSON), "container lifecycle passed") {
		t.Fatalf("configured remote result=%+v", result)
	}
	if executor.Adapter() != "remote-container" || runner.spec.InheritEnv {
		t.Fatalf("configured executor adapter=%q process=%+v", executor.Adapter(), runner.spec)
	}
	if placement := controller.Snapshot().Placements["placement-attempt-container"]; placement.State != PlacementStateDestroyed {
		t.Fatalf("container lifecycle placement=%+v", placement)
	}
}

func TestOpenConfiguredSchedulerExecutorRejectsReadableKeyAndMismatchedWorkerKey(t *testing.T) {
	t.Parallel()

	seed := make([]byte, ed25519.SeedSize)
	seed[0] = 62
	privateKey := ed25519.NewKeyFromSeed(seed)
	workerSeed := make([]byte, ed25519.SeedSize)
	workerSeed[0] = 63
	workerPrivateKey := ed25519.NewKeyFromSeed(workerSeed)
	workerRoot := t.TempDir()
	workerConfigPath := filepath.Join(t.TempDir(), "worker.json")
	writeJSONConfig(t, workerConfigPath, WorkerServiceConfig{
		SchemaVersion: workerServiceConfigSchemaVersion,
		WorkerID:      "worker-a", RootDir: workerRoot, StatePath: filepath.Join(workerRoot, "state.json"),
		PublicKey: base64.RawStdEncoding.EncodeToString(workerPrivateKey.Public().(ed25519.PublicKey)), MaxTokenTTLSeconds: 300,
		Container: ContainerWorkerConfig{
			RuntimePath: "/usr/bin/docker", Image: "worker@sha256:" + strings.Repeat("e", 64), Command: []string{"worker"},
		},
	})
	gatewayConfigPath := filepath.Join(t.TempDir(), "gateway.json")
	writeJSONConfig(t, gatewayConfigPath, SchedulerGatewayConfig{
		SchemaVersion: schedulerGatewayConfigSchemaVersion, Adapter: "remote-container",
		WorkerID: "worker-a", Transport: SchedulerTransportInProcess,
		PrivateKey:      base64.RawStdEncoding.EncodeToString(privateKey),
		LeaseTTLSeconds: 120, TokenTTLSeconds: 60, SyncMode: SyncModeDirectory,
		Policy: DefaultExecutionPolicy(), InProcess: SchedulerInProcessConfig{WorkerConfigPath: workerConfigPath},
	})
	controller, err := OpenController(ControllerOptions{StatePath: filepath.Join(t.TempDir(), "controller.json")})
	if err != nil {
		t.Fatal(err)
	}
	opts := ConfiguredSchedulerExecutorOptions{
		ConfigPath: gatewayConfigPath, SourceDir: t.TempDir(), DataDir: t.TempDir(), Controller: controller,
		Runner: &recordingProcessRunner{},
	}
	if _, err := OpenConfiguredSchedulerExecutor(opts); err == nil {
		t.Fatal("gateway accepted a worker configured with a different task verification key")
	}
	if err := os.Chmod(gatewayConfigPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenConfiguredSchedulerExecutor(opts); err == nil {
		t.Fatal("gateway accepted a group/world-readable task signing key")
	}
}

func writeJSONConfig(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSchedulerGatewayConfigRejectsUnknownFieldsAndTrailingData(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "gateway.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":1,"private_key":"secret","unknown":true}{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSchedulerGatewayConfig(path); err == nil {
		t.Fatal("gateway config accepted an unknown field and trailing document")
	}
}

func TestSchedulerGatewayConfigRejectsNetworkedPolicyAndSymlinkedPrivateState(t *testing.T) {
	t.Parallel()

	policy := DefaultExecutionPolicy()
	policy.Egress = EgressPolicy{Mode: EgressAllowlist, AllowHosts: []string{"api.example.com"}}
	config := SchedulerGatewayConfig{
		SchemaVersion: schedulerGatewayConfigSchemaVersion, Adapter: "remote", WorkerID: "worker",
		Transport: SchedulerTransportInProcess, PrivateKey: "present",
		LeaseTTLSeconds: 120, TokenTTLSeconds: 60, SyncMode: SyncModeDirectory,
		Policy: policy, InProcess: SchedulerInProcessConfig{WorkerConfigPath: "/etc/tars/worker.json"},
	}
	if err := validateSchedulerGatewayConfig(&config); err == nil {
		t.Fatal("configured scheduler worker accepted a network-enabled execution policy")
	}

	workspace := t.TempDir()
	privateStateInsideWorkspace := filepath.Join(workspace, "private-state")
	if err := os.Mkdir(privateStateInsideWorkspace, 0o700); err != nil {
		t.Fatal(err)
	}
	dataLink := filepath.Join(t.TempDir(), "data-link")
	if err := os.Symlink(privateStateInsideWorkspace, dataLink); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "gateway.json")
	if err := os.WriteFile(configPath, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := secureSchedulerDirectories(workspace, dataLink, configPath); err == nil {
		t.Fatal("scheduler accepted a symlinked private state directory inside the workspace")
	}
}
