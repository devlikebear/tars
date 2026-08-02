package executionplane

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalEnvironmentProviderPreservesSourceAndSnapshotsChanges(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "result.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	provider, err := NewLocalEnvironmentProvider(root)
	if err != nil {
		t.Fatalf("new local provider: %v", err)
	}
	environment, err := provider.Provision(context.Background(), ProvisionRequest{Execution: testExecution(), SourceDir: root})
	if err != nil {
		t.Fatalf("provision local environment: %v", err)
	}
	first, err := provider.Sync(context.Background(), environment)
	if err != nil {
		t.Fatalf("first local snapshot: %v", err)
	}
	if err := os.WriteFile(path, []byte("after\n"), 0o600); err != nil {
		t.Fatalf("update source: %v", err)
	}
	second, err := provider.Sync(context.Background(), environment)
	if err != nil {
		t.Fatalf("second local snapshot: %v", err)
	}
	if first.Digest == "" || first.Digest == second.Digest {
		t.Fatalf("snapshot digests first=%q second=%q", first.Digest, second.Digest)
	}
	recovered, err := provider.Recover(context.Background(), environment)
	canonicalRoot, canonicalErr := filepath.EvalSymlinks(root)
	if err != nil || canonicalErr != nil || recovered.RootDir != canonicalRoot {
		t.Fatalf("recover local environment = %#v, %v", recovered, err)
	}
	if err := provider.Destroy(context.Background(), environment); err != nil {
		t.Fatalf("destroy local environment: %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "after\n" {
		t.Fatalf("local provider changed source: %q, %v", got, err)
	}
	capabilities := provider.Capabilities()
	if !capabilities.Recoverable || !capabilities.Snapshot || capabilities.FilesystemIsolation || capabilities.EgressPolicy {
		t.Fatalf("local capabilities = %#v", capabilities)
	}
}

func TestFileStateStorePersistsOnlySecretFreeLifecycleState(t *testing.T) {
	t.Parallel()

	store, err := NewFileStateStore(t.TempDir())
	if err != nil {
		t.Fatalf("new file state store: %v", err)
	}
	state := LifecycleState{
		SchemaVersion: 1, AttemptID: "attempt-safe", Phase: EventWorkerStarted,
		Environment:  Environment{SchemaVersion: 1, ID: "env-safe", Kind: "local", RootDir: "/workspace"},
		CredentialID: "grant-safe",
	}
	if err := store.Save(context.Background(), state); err != nil {
		t.Fatalf("save lifecycle state: %v", err)
	}
	loaded, found, err := store.Load(context.Background(), state.AttemptID)
	if err != nil || !found || loaded.CredentialID != "grant-safe" || loaded.Environment.ID != "env-safe" {
		t.Fatalf("loaded state = %#v, found=%v, err=%v", loaded, found, err)
	}
	path := store.statePath(state.AttemptID)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat lifecycle state: %v", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("state permissions = %o, want private", info.Mode().Perm())
	}
	if err := store.Delete(context.Background(), state.AttemptID); err != nil {
		t.Fatalf("delete lifecycle state: %v", err)
	}
	if _, found, err := store.Load(context.Background(), state.AttemptID); err != nil || found {
		t.Fatalf("deleted state found=%v err=%v", found, err)
	}
	if err := store.Save(context.Background(), LifecycleState{AttemptID: "../escape"}); err == nil {
		t.Fatal("state store accepted a traversal attempt id")
	}
}
