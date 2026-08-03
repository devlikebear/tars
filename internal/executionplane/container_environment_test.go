package executionplane

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestContainerEnvironmentProviderUsesDefaultDenyIsolationPolicy(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "host-secret-must-not-be-forwarded")

	baseDestroyed := false
	base := &fakeEnvironmentProvider{
		name:         "managed-worktree",
		capabilities: EnvironmentCapabilities{Recoverable: true, Snapshot: true, Cleanup: true, FilesystemIsolation: true},
		provision: func(context.Context, ProvisionRequest) (Environment, error) {
			return Environment{SchemaVersion: 1, ID: "worktree:attempt-1", Kind: "managed-worktree", RootDir: "/managed/attempt-1", SourceDir: "/source"}, nil
		},
		recover: func(_ context.Context, environment Environment) (Environment, error) { return environment, nil },
		sync: func(context.Context, Environment) (EnvironmentSnapshot, error) {
			return EnvironmentSnapshot{Digest: "sha256:container"}, nil
		},
		destroy: func(context.Context, Environment) error { baseDestroyed = true; return nil },
	}
	runner := &recordingCommandRunner{run: func(spec CommandSpec) (CommandResult, error) {
		if len(spec.Args) > 0 && spec.Args[0] == "create" {
			return CommandResult{Stdout: "container-id\n"}, nil
		}
		return CommandResult{}, nil
	}}
	provider, err := NewContainerEnvironmentProvider(ContainerOptions{
		Runtime: "docker", Image: "ghcr.io/devlikebear/tars-worker@sha256:abc",
		BaseProvider: base, Runner: runner, CPUs: "1.5", Memory: "2g", PidsLimit: 128,
	})
	if err != nil {
		t.Fatalf("new container provider: %v", err)
	}
	environment, err := provider.Provision(context.Background(), ProvisionRequest{Execution: testExecution(), SourceDir: "/source"})
	if err != nil {
		t.Fatalf("provision container: %v", err)
	}
	if environment.Kind != "container" || environment.RootDir != "/managed/attempt-1" {
		t.Fatalf("container environment = %#v", environment)
	}
	if len(runner.specs) < 2 {
		t.Fatalf("runtime calls = %#v", runner.specs)
	}
	create := runner.specs[0]
	joined := strings.Join(create.Args, " ")
	for _, required := range []string{"create", "--network none", "--read-only", "--cpus 1.5", "--memory 2g", "--pids-limit 128", "type=bind", "target=/workspace", "--workdir /workspace"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("container create args missing %q: %s", required, joined)
		}
	}
	if strings.Contains(joined, "host-secret-must-not-be-forwarded") || strings.Contains(strings.Join(create.Env, " "), "host-secret-must-not-be-forwarded") {
		t.Fatalf("host provider secret reached container command: %#v", create)
	}
	if !reflect.DeepEqual(runner.commandNames(), []string{"docker", "docker"}) || runner.specs[1].Args[0] != "start" {
		t.Fatalf("runtime call order = %#v", runner.specs)
	}
	var metadata containerEnvironmentMetadata
	if err := json.Unmarshal(environment.MetadataJSON, &metadata); err != nil || metadata.ContainerID != "container-id" || metadata.NetworkMode != "none" {
		t.Fatalf("container metadata = %#v, %v", metadata, err)
	}
	snapshot, err := provider.Sync(context.Background(), environment)
	if err != nil || snapshot.Digest != "sha256:container" {
		t.Fatalf("container sync = %#v, %v", snapshot, err)
	}
	if err := provider.Destroy(context.Background(), environment); err != nil {
		t.Fatalf("destroy container: %v", err)
	}
	if !baseDestroyed || runner.specs[len(runner.specs)-1].Args[0] != "rm" {
		t.Fatalf("container cleanup base=%v calls=%#v", baseDestroyed, runner.specs)
	}
	capabilities := provider.Capabilities()
	if !capabilities.Recoverable || !capabilities.Snapshot || !capabilities.Cleanup || !capabilities.FilesystemIsolation || !capabilities.CredentialIsolation || !capabilities.EgressPolicy {
		t.Fatalf("container capabilities = %#v", capabilities)
	}
}

func TestContainerEnvironmentProviderRejectsUnenforcedEgressAllowlist(t *testing.T) {
	t.Parallel()

	base := &fakeEnvironmentProvider{name: "managed", capabilities: EnvironmentCapabilities{Cleanup: true, FilesystemIsolation: true}}
	_, err := NewContainerEnvironmentProvider(ContainerOptions{
		Runtime: "docker", Image: "worker@sha256:abc", BaseProvider: base,
		Runner: &recordingCommandRunner{}, EgressAllowlist: []string{"api.example.com"},
	})
	if err == nil || !strings.Contains(err.Error(), "egress allowlist") {
		t.Fatalf("container provider error = %v", err)
	}
}

type recordingCommandRunner struct {
	specs []CommandSpec
	run   func(CommandSpec) (CommandResult, error)
}

func (runner *recordingCommandRunner) Run(_ context.Context, spec CommandSpec) (CommandResult, error) {
	runner.specs = append(runner.specs, spec)
	if runner.run == nil {
		return CommandResult{}, nil
	}
	return runner.run(spec)
}

func (runner *recordingCommandRunner) commandNames() []string {
	names := make([]string, 0, len(runner.specs))
	for _, spec := range runner.specs {
		names = append(names, spec.Command)
	}
	return names
}
