package workerprotocol

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenConfiguredWorkerServiceUsesPublicVerificationKeyAndPinnedSandbox(t *testing.T) {
	t.Parallel()

	seed := make([]byte, ed25519.SeedSize)
	seed[0] = 31
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	root := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "worker.json")
	config := WorkerServiceConfig{
		SchemaVersion: workerServiceConfigSchemaVersion,
		WorkerID:      "worker-a", RootDir: root, StatePath: filepath.Join(root, "state.json"),
		PublicKey: base64.RawStdEncoding.EncodeToString(publicKey), MaxTokenTTLSeconds: 300,
		WireLimits: WireLimits{MaxRequestBytes: 4 << 20, MaxResponseBytes: 4 << 20},
		Container: ContainerWorkerConfig{
			RuntimePath: "/usr/bin/docker", Image: "worker@sha256:" + strings.Repeat("c", 64),
			Command: []string{"/usr/local/bin/tars-task-harness"}, CPUs: "1", PIDsLimit: 64,
		},
	}
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	service, limits, err := OpenConfiguredWorkerService(configPath, &recordingProcessRunner{})
	if err != nil {
		t.Fatalf("open configured worker service: %v", err)
	}
	if service.Snapshot().WorkerID != "worker-a" || limits != config.WireLimits {
		t.Fatalf("configured service snapshot=%+v limits=%+v", service.Snapshot(), limits)
	}
	persistedConfig, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(persistedConfig), base64.RawStdEncoding.EncodeToString(privateKey)) {
		t.Fatal("worker configuration contains a private signing key")
	}
}

func TestOpenConfiguredWorkerServiceRejectsUnknownCredentialFields(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "worker.json")
	raw := `{"schema_version":1,"worker_id":"worker","root_dir":"/tmp/worker","state_path":"/tmp/worker/state.json","public_key":"bad","max_token_ttl_seconds":60,"api_token":"must-not-be-accepted","container":{}}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := OpenConfiguredWorkerService(path, &recordingProcessRunner{}); err == nil {
		t.Fatal("worker config accepted an unknown credential field")
	}
}
