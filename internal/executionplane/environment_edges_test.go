package executionplane

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestArtifactCollectorBoundsIdentityPathsAndOutput(t *testing.T) {
	t.Parallel()

	if _, err := NewFileArtifactCollector(ArtifactCollectorOptions{}); err == nil {
		t.Fatal("collector accepted a blank artifact root")
	}
	if _, err := NewFileArtifactCollector(ArtifactCollectorOptions{RootDir: t.TempDir(), Paths: []string{"/absolute"}}); err == nil {
		t.Fatal("collector accepted an absolute source path")
	}
	environmentRoot := t.TempDir()
	artifactRoot := t.TempDir()
	for _, entry := range []struct {
		path string
		raw  string
	}{
		{"reports/a.txt", "alpha"}, {"reports/b.txt", "bravo"},
		{"reports/secret-token.txt", "must-not-copy"}, {"node_modules/module.txt", "excluded"},
		{".tars/private.txt", "private"}, {".git/config", "private"},
	} {
		path := filepath.Join(environmentRoot, filepath.FromSlash(entry.path))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(entry.raw), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	collector, err := NewFileArtifactCollector(ArtifactCollectorOptions{RootDir: artifactRoot, Paths: []string{"reports/*.txt"}, MaxFiles: 1, MaxBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if collector.maxFiles != 1 || collector.maxBytes != 1024 {
		t.Fatalf("collector bounds=%+v", collector)
	}
	if _, err := collector.Collect(context.Background(), CollectRequest{Execution: testExecution(), Environment: Environment{RootDir: environmentRoot}}); err == nil || !strings.Contains(err.Error(), "artifact count exceeds") {
		t.Fatalf("artifact count error=%v", err)
	}

	collector, err = NewFileArtifactCollector(ArtifactCollectorOptions{RootDir: t.TempDir(), Paths: []string{"reports/a.txt"}, MaxBytes: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := collector.Collect(context.Background(), CollectRequest{Execution: testExecution(), Environment: Environment{RootDir: environmentRoot}}); err == nil || !strings.Contains(err.Error(), "artifact bytes exceed") {
		t.Fatalf("artifact byte error=%v", err)
	}

	invalidIdentity := testExecution()
	invalidIdentity.Work.ID = "../escape"
	collector, err = NewFileArtifactCollector(ArtifactCollectorOptions{RootDir: t.TempDir(), Paths: []string{"reports/a.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := collector.Collect(context.Background(), CollectRequest{Execution: invalidIdentity, Environment: Environment{RootDir: environmentRoot}}); err == nil || !strings.Contains(err.Error(), "invalid durable identity") {
		t.Fatalf("identity error=%v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := collector.Collect(canceled, CollectRequest{Execution: testExecution(), Environment: Environment{RootDir: environmentRoot}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled collection error=%v", err)
	}

	invalidGlob, err := NewFileArtifactCollector(ArtifactCollectorOptions{RootDir: t.TempDir(), Paths: []string{"["}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := invalidGlob.candidateFiles(context.Background(), environmentRoot); err == nil || !strings.Contains(err.Error(), "invalid artifact glob") {
		t.Fatalf("invalid glob error=%v", err)
	}
	if _, err := collectGitPatch(context.Background(), environmentRoot); err == nil {
		t.Fatal("non-Git directory produced a patch")
	}
}

func TestArtifactSanitizersAndURIContracts(t *testing.T) {
	t.Parallel()

	for _, name := range []string{".git", ".TARS", " node_modules "} {
		if !excludedArtifactDirectory(name) {
			t.Errorf("excluded directory %q accepted", name)
		}
	}
	if excludedArtifactDirectory("reports") {
		t.Fatal("safe reports directory was excluded")
	}
	for _, path := range []string{".env", ".env.production", "id_rsa", "id_ed25519", "credentials", "credentials.json", "client.pem", "private.key", "api-credential.txt", "secret.txt", "access-token.json"} {
		if !sensitiveArtifactPath(path) {
			t.Errorf("sensitive path %q accepted", path)
		}
	}
	if sensitiveArtifactPath("report.txt") {
		t.Fatal("safe report path marked sensitive")
	}
	if got := string(redactArtifactBytes([]byte("one secret two"), []string{"", "secret"})); got != "one [REDACTED] two" {
		t.Fatalf("redacted bytes=%q", got)
	}
	transcript, err := encodeTranscript([]TranscriptEntry{{Sequence: 1, Type: "assistant", Text: "secret"}}, []string{"secret"})
	if err != nil || !strings.HasSuffix(string(transcript), "\n") || strings.Contains(string(transcript), "secret") {
		t.Fatalf("encoded transcript=%q err=%v", transcript, err)
	}

	root := t.TempDir()
	artifact, err := writeCollectedArtifact(root, "file", "nested/report.txt", []byte("report"))
	if err != nil || artifact.Digest == "" || artifact.MediaType == "" || artifact.SizeBytes != 6 {
		t.Fatalf("written artifact=%+v err=%v", artifact, err)
	}
	path, err := filepathFromURI(artifact.URI)
	if err != nil || filepath.Clean(path) != filepath.Join(root, "nested", "report.txt") {
		t.Fatalf("artifact path=%q err=%v", path, err)
	}
	for _, raw := range []string{"https://example.test/report", "file://remotehost/tmp/report", "://bad"} {
		if _, err := filepathFromURI(raw); err == nil {
			t.Errorf("unsafe artifact URI %q accepted", raw)
		}
	}
	if _, err := writeCollectedArtifact(root, "file", "../escape", []byte("x")); err == nil {
		t.Fatal("artifact destination traversal accepted")
	}
}

func TestFileStateStoreRejectsCancellationCorruptionAndUnsafeIdentity(t *testing.T) {
	t.Parallel()

	if _, err := NewFileStateStore(" "); err == nil {
		t.Fatal("blank state root was accepted")
	}
	store, err := NewFileStateStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	state := LifecycleState{SchemaVersion: lifecycleSchemaVersion, AttemptID: "attempt-edge", Phase: EventWorkerStarted}
	if err := store.Save(canceled, state); !errors.Is(err, context.Canceled) {
		t.Fatalf("Save() canceled error=%v", err)
	}
	if _, _, err := store.Load(canceled, state.AttemptID); !errors.Is(err, context.Canceled) {
		t.Fatalf("Load() canceled error=%v", err)
	}
	if err := store.Delete(canceled, state.AttemptID); !errors.Is(err, context.Canceled) {
		t.Fatalf("Delete() canceled error=%v", err)
	}
	for _, invalid := range []LifecycleState{{AttemptID: "attempt"}, {SchemaVersion: lifecycleSchemaVersion, AttemptID: "../escape"}} {
		if err := store.Save(context.Background(), invalid); err == nil {
			t.Errorf("invalid state accepted: %+v", invalid)
		}
	}
	var nilStore *FileStateStore
	if err := nilStore.Save(context.Background(), state); err == nil {
		t.Fatal("nil state store saved")
	}
	if _, _, err := nilStore.Load(context.Background(), "bad id"); err == nil {
		t.Fatal("nil state store loaded")
	}
	if err := nilStore.Delete(context.Background(), ""); err == nil {
		t.Fatal("nil state store deleted")
	}

	path := store.statePath(state.AttemptID)
	for _, raw := range []string{
		`{`,
		`{"schema_version":2,"attempt_id":"attempt-edge"}`,
		`{"schema_version":1,"attempt_id":"other"}`,
	} {
		if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, found, err := store.Load(context.Background(), state.AttemptID); err == nil || found {
			t.Errorf("corrupt state %q found=%v err=%v", raw, found, err)
		}
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), state.AttemptID); err != nil {
		t.Fatalf("idempotent delete: %v", err)
	}
}

func TestLocalEnvironmentProviderRejectsForeignOrUnavailableRoots(t *testing.T) {
	t.Parallel()

	if _, err := NewLocalEnvironmentProvider(" "); err == nil {
		t.Fatal("blank local root was accepted")
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewLocalEnvironmentProvider(file); err == nil {
		t.Fatal("file was accepted as local root")
	}
	root := t.TempDir()
	provider, err := NewLocalEnvironmentProvider(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Provision(context.Background(), ProvisionRequest{Execution: testExecution(), SourceDir: t.TempDir()}); err == nil {
		t.Fatal("foreign local source was provisioned")
	}
	if _, err := provider.Recover(context.Background(), Environment{Kind: "container", RootDir: root}); err == nil {
		t.Fatal("foreign local environment was recovered")
	}
	if _, err := provider.Recover(context.Background(), Environment{Kind: provider.Name(), RootDir: filepath.Join(root, "missing")}); err == nil {
		t.Fatal("missing local environment was recovered")
	}
	if err := provider.Destroy(context.Background(), Environment{Kind: "container"}); err == nil {
		t.Fatal("foreign local environment was destroyed")
	}
}

func TestContainerProviderCleansBaseEnvironmentOnProvisionFailures(t *testing.T) {
	t.Parallel()

	isolated := EnvironmentCapabilities{Recoverable: true, Snapshot: true, Cleanup: true, FilesystemIsolation: true}
	if _, err := NewContainerEnvironmentProvider(ContainerOptions{}); err == nil {
		t.Fatal("blank container config was accepted")
	}
	if _, err := NewContainerEnvironmentProvider(ContainerOptions{Runtime: "docker", Image: "image", BaseProvider: &fakeEnvironmentProvider{capabilities: EnvironmentCapabilities{Recoverable: true}}}); err == nil {
		t.Fatal("non-isolated base provider was accepted")
	}
	destroyed := 0
	base := &fakeEnvironmentProvider{
		name: "managed", capabilities: isolated,
		provision: func(context.Context, ProvisionRequest) (Environment, error) {
			return Environment{SchemaVersion: 1, ID: "base", Kind: "managed", RootDir: "/managed/base", SourceDir: "/source"}, nil
		},
		destroy: func(context.Context, Environment) error { destroyed++; return nil },
	}
	provider, err := NewContainerEnvironmentProvider(ContainerOptions{Runtime: "docker", Image: "image@sha256:a", BaseProvider: base, Runner: &recordingCommandRunner{}})
	if err != nil {
		t.Fatal(err)
	}
	if provider.cpus != "1" || provider.memory != "1g" || provider.pidsLimit != 128 || provider.Name() != "container" {
		t.Fatalf("container defaults=%+v", provider)
	}
	invalid := testExecution()
	invalid.Claim.Attempt.ID = "bad id"
	if _, err := provider.Provision(context.Background(), ProvisionRequest{Execution: invalid, SourceDir: "/source"}); err == nil || destroyed != 1 {
		t.Fatalf("invalid attempt error=%v destroyed=%d", err, destroyed)
	}

	createErr := errors.New("create failed")
	provider.runner = &recordingCommandRunner{run: func(CommandSpec) (CommandResult, error) { return CommandResult{}, createErr }}
	if _, err := provider.Provision(context.Background(), ProvisionRequest{Execution: testExecution(), SourceDir: "/source"}); !errors.Is(err, createErr) || destroyed != 2 {
		t.Fatalf("create error=%v destroyed=%d", err, destroyed)
	}

	startCalls := 0
	provider.runner = &recordingCommandRunner{run: func(spec CommandSpec) (CommandResult, error) {
		startCalls++
		if spec.Args[0] == "create" {
			return CommandResult{Stdout: "container-id"}, nil
		}
		if spec.Args[0] == "start" {
			return CommandResult{}, errors.New("start failed")
		}
		return CommandResult{}, nil
	}}
	if _, err := provider.Provision(context.Background(), ProvisionRequest{Execution: testExecution(), SourceDir: "/source"}); err == nil || destroyed != 3 || startCalls != 3 {
		t.Fatalf("start failure error=%v destroyed=%d calls=%d", err, destroyed, startCalls)
	}
}

func TestContainerProviderRejectsForgedMetadataAndRecoveryFailures(t *testing.T) {
	t.Parallel()

	baseEnvironment := Environment{SchemaVersion: 1, ID: "base", Kind: "managed", RootDir: "/managed/base", SourceDir: "/source"}
	base := &fakeEnvironmentProvider{
		name: "managed", capabilities: EnvironmentCapabilities{Recoverable: true, Snapshot: true, Cleanup: true, FilesystemIsolation: true},
		recover: func(_ context.Context, environment Environment) (Environment, error) { return environment, nil },
		sync: func(context.Context, Environment) (EnvironmentSnapshot, error) {
			return EnvironmentSnapshot{Digest: "sha256:base"}, nil
		},
		destroy: func(context.Context, Environment) error { return nil },
	}
	runner := &recordingCommandRunner{run: func(spec CommandSpec) (CommandResult, error) {
		if spec.Args[0] == "inspect" {
			return CommandResult{Stdout: "false"}, nil
		}
		return CommandResult{}, nil
	}}
	provider, err := NewContainerEnvironmentProvider(ContainerOptions{Runtime: "docker", Image: "image@sha256:a", BaseProvider: base, Runner: runner, Now: func() time.Time { return time.Unix(1, 0) }})
	if err != nil {
		t.Fatal(err)
	}
	metadata := containerEnvironmentMetadata{
		SchemaVersion: lifecycleSchemaVersion, Runtime: "docker", Image: "image@sha256:a",
		ContainerID: "container-id", ContainerName: "tars-attempt-1", NetworkMode: "none", ReadOnlyRoot: true, Base: baseEnvironment,
	}
	raw, _ := json.Marshal(metadata)
	environment := Environment{SchemaVersion: 1, ID: "container:attempt-1", Kind: provider.Name(), RootDir: baseEnvironment.RootDir, MetadataJSON: raw}
	recovered, err := provider.Recover(context.Background(), environment)
	if err != nil || recovered.UpdatedAt.Unix() != 1 || len(runner.specs) != 2 || runner.specs[1].Args[0] != "start" {
		t.Fatalf("recovered container=%+v calls=%+v err=%v", recovered, runner.specs, err)
	}
	if snapshot, err := provider.Sync(context.Background(), environment); err != nil || snapshot.Digest != "sha256:base" {
		t.Fatalf("container snapshot=%+v err=%v", snapshot, err)
	}

	for _, forged := range []Environment{
		{ID: "container:attempt-1", Kind: "local", RootDir: baseEnvironment.RootDir, MetadataJSON: raw},
		{ID: "local:attempt-1", Kind: provider.Name(), RootDir: baseEnvironment.RootDir, MetadataJSON: raw},
		{ID: "container:attempt-1", Kind: provider.Name(), RootDir: baseEnvironment.RootDir, MetadataJSON: json.RawMessage(`{`)},
	} {
		if _, err := provider.Recover(context.Background(), forged); err == nil {
			t.Errorf("forged container metadata accepted: %+v", forged)
		}
	}
	metadata.NetworkMode = "bridge"
	forgedRaw, _ := json.Marshal(metadata)
	if _, err := provider.Recover(context.Background(), Environment{ID: environment.ID, Kind: environment.Kind, RootDir: environment.RootDir, MetadataJSON: forgedRaw}); err == nil {
		t.Fatal("networked container metadata was accepted")
	}

	base.recover = func(context.Context, Environment) (Environment, error) { return Environment{}, errors.New("base lost") }
	if _, err := provider.Recover(context.Background(), environment); err == nil || !strings.Contains(err.Error(), "recover container filesystem") {
		t.Fatalf("base recovery error=%v", err)
	}
}

func TestManagedWorktreeOwnershipHelpersFailClosed(t *testing.T) {
	t.Parallel()

	if _, err := ensurePrivateDirectory(" "); err == nil {
		t.Fatal("blank managed root was accepted")
	}
	root := t.TempDir()
	marker := worktreeMarker{SchemaVersion: lifecycleSchemaVersion, EnvironmentID: "worktree:attempt", SourceDir: "/source", Head: "abc"}
	if err := writeWorktreeMarker(root, marker); err != nil {
		t.Fatal(err)
	}
	loaded, err := readWorktreeMarker(root)
	if err != nil || loaded.EnvironmentID != marker.EnvironmentID {
		t.Fatalf("marker=%+v err=%v", loaded, err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(worktreeMarkerRelativePath)), []byte(`{`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readWorktreeMarker(root); err == nil {
		t.Fatal("corrupt ownership marker was accepted")
	}
	if !sameOrWithin(root, root) || !sameOrWithin(root, filepath.Join(root, "child")) || sameOrWithin(root, filepath.Dir(root)) {
		t.Fatal("managed path containment check failed")
	}
	if digestRaw([]byte("same")) != digestRaw([]byte("same")) || digestRaw([]byte("same")) == digestRaw([]byte("different")) {
		t.Fatal("raw digest is not deterministic")
	}
	if _, err := NewManagedWorktreeProvider(root, t.TempDir()); err == nil {
		t.Fatal("non-Git source was accepted")
	}
}
