package executionplane

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const defaultContainerTmpfs = "/tmp:rw,noexec,nosuid,size=256m"

type ContainerOptions struct {
	Runtime         string
	Image           string
	BaseProvider    EnvironmentProvider
	Runner          CommandRunner
	CPUs            string
	Memory          string
	PidsLimit       int
	EgressAllowlist []string
	Now             func() time.Time
}

type ContainerEnvironmentProvider struct {
	runtime      string
	image        string
	baseProvider EnvironmentProvider
	runner       CommandRunner
	cpus         string
	memory       string
	pidsLimit    int
	now          func() time.Time
}

type containerEnvironmentMetadata struct {
	SchemaVersion int         `json:"schema_version"`
	Runtime       string      `json:"runtime"`
	Image         string      `json:"image"`
	ContainerID   string      `json:"container_id"`
	ContainerName string      `json:"container_name"`
	NetworkMode   string      `json:"network_mode"`
	ReadOnlyRoot  bool        `json:"read_only_root"`
	CPUs          string      `json:"cpus"`
	Memory        string      `json:"memory"`
	PidsLimit     int         `json:"pids_limit"`
	Base          Environment `json:"base_environment"`
}

func NewContainerEnvironmentProvider(opts ContainerOptions) (*ContainerEnvironmentProvider, error) {
	runtimeName := strings.TrimSpace(opts.Runtime)
	image := strings.TrimSpace(opts.Image)
	if runtimeName == "" || image == "" {
		return nil, fmt.Errorf("executionplane: container runtime and image are required")
	}
	if len(opts.EgressAllowlist) > 0 {
		return nil, fmt.Errorf("executionplane: container egress allowlist is unsupported; default-deny network mode is the only enforced policy")
	}
	if opts.BaseProvider == nil {
		return nil, fmt.Errorf("executionplane: isolated base environment provider is required")
	}
	baseCapabilities := opts.BaseProvider.Capabilities()
	if !baseCapabilities.Recoverable || !baseCapabilities.Snapshot || !baseCapabilities.Cleanup || !baseCapabilities.FilesystemIsolation {
		return nil, fmt.Errorf("executionplane: container base provider must be isolated, recoverable, snapshot-capable, and disposable")
	}
	if opts.Runner == nil {
		opts.Runner = OSCommandRunner{}
	}
	if strings.TrimSpace(opts.CPUs) == "" {
		opts.CPUs = "1"
	}
	if strings.TrimSpace(opts.Memory) == "" {
		opts.Memory = "1g"
	}
	if opts.PidsLimit <= 0 {
		opts.PidsLimit = 128
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &ContainerEnvironmentProvider{
		runtime: runtimeName, image: image, baseProvider: opts.BaseProvider, runner: opts.Runner,
		cpus: strings.TrimSpace(opts.CPUs), memory: strings.TrimSpace(opts.Memory),
		pidsLimit: opts.PidsLimit, now: opts.Now,
	}, nil
}

func (provider *ContainerEnvironmentProvider) Name() string { return "container" }

func (provider *ContainerEnvironmentProvider) Capabilities() EnvironmentCapabilities {
	return EnvironmentCapabilities{
		Recoverable: true, Snapshot: true, Cleanup: true, FilesystemIsolation: true,
		CredentialIsolation: true, EgressPolicy: true,
	}
}

func (provider *ContainerEnvironmentProvider) Provision(ctx context.Context, request ProvisionRequest) (Environment, error) {
	base, err := provider.baseProvider.Provision(ctx, request)
	if err != nil {
		return Environment{}, fmt.Errorf("executionplane: provision container filesystem: %w", err)
	}
	attemptID := strings.TrimSpace(request.Execution.Claim.Attempt.ID)
	if !safeStateID.MatchString(attemptID) {
		_ = provider.baseProvider.Destroy(context.Background(), base)
		return Environment{}, fmt.Errorf("executionplane: invalid container attempt id")
	}
	containerName := "tars-" + strings.ToLower(attemptID)
	args := []string{
		"create", "--name", containerName,
		"--network", "none", "--read-only",
		"--cpus", provider.cpus, "--memory", provider.memory,
		"--pids-limit", strconv.Itoa(provider.pidsLimit),
		"--tmpfs", defaultContainerTmpfs,
		"--mount", "type=bind,source=" + base.RootDir + ",target=/workspace",
		"--workdir", "/workspace",
		provider.image, "tail", "-f", "/dev/null",
	}
	created, err := provider.runtimeCommand(ctx, args...)
	if err != nil {
		_ = provider.baseProvider.Destroy(context.Background(), base)
		return Environment{}, fmt.Errorf("executionplane: create container: %w", err)
	}
	containerID := strings.TrimSpace(created.Stdout)
	if containerID == "" {
		containerID = containerName
	}
	if _, err := provider.runtimeCommand(ctx, "start", containerName); err != nil {
		_, _ = provider.runtimeCommand(context.Background(), "rm", "-f", containerName)
		_ = provider.baseProvider.Destroy(context.Background(), base)
		return Environment{}, fmt.Errorf("executionplane: start container: %w", err)
	}
	metadata := containerEnvironmentMetadata{
		SchemaVersion: lifecycleSchemaVersion, Runtime: provider.runtime, Image: provider.image,
		ContainerID: containerID, ContainerName: containerName, NetworkMode: "none", ReadOnlyRoot: true,
		CPUs: provider.cpus, Memory: provider.memory, PidsLimit: provider.pidsLimit, Base: base,
	}
	metadataJSON, _ := json.Marshal(metadata)
	now := provider.now().UTC()
	return Environment{
		SchemaVersion: lifecycleSchemaVersion, ID: "container:" + attemptID, Kind: provider.Name(),
		RootDir: base.RootDir, SourceDir: base.SourceDir, MetadataJSON: metadataJSON,
		ProvisionedAt: now, UpdatedAt: now,
	}, nil
}

func (provider *ContainerEnvironmentProvider) Recover(ctx context.Context, environment Environment) (Environment, error) {
	metadata, err := provider.decodeMetadata(environment)
	if err != nil {
		return Environment{}, err
	}
	base, err := provider.baseProvider.Recover(ctx, metadata.Base)
	if err != nil {
		return Environment{}, fmt.Errorf("executionplane: recover container filesystem: %w", err)
	}
	inspected, err := provider.runtimeCommand(ctx, "inspect", "--format", "{{.State.Running}}", metadata.ContainerName)
	if err != nil {
		return Environment{}, fmt.Errorf("executionplane: inspect container: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(inspected.Stdout), "true") {
		if _, err := provider.runtimeCommand(ctx, "start", metadata.ContainerName); err != nil {
			return Environment{}, fmt.Errorf("executionplane: restart container: %w", err)
		}
	}
	metadata.Base = base
	metadataJSON, _ := json.Marshal(metadata)
	environment.RootDir = base.RootDir
	environment.SourceDir = base.SourceDir
	environment.MetadataJSON = metadataJSON
	environment.UpdatedAt = provider.now().UTC()
	return environment, nil
}

func (provider *ContainerEnvironmentProvider) Sync(ctx context.Context, environment Environment) (EnvironmentSnapshot, error) {
	metadata, err := provider.decodeMetadata(environment)
	if err != nil {
		return EnvironmentSnapshot{}, err
	}
	return provider.baseProvider.Sync(ctx, metadata.Base)
}

func (provider *ContainerEnvironmentProvider) Destroy(ctx context.Context, environment Environment) error {
	metadata, err := provider.decodeMetadata(environment)
	if err != nil {
		return err
	}
	if _, err := provider.runtimeCommand(ctx, "rm", "-f", metadata.ContainerName); err != nil {
		return fmt.Errorf("executionplane: remove container: %w", err)
	}
	return provider.baseProvider.Destroy(ctx, metadata.Base)
}

func (provider *ContainerEnvironmentProvider) decodeMetadata(environment Environment) (containerEnvironmentMetadata, error) {
	if environment.Kind != provider.Name() || !strings.HasPrefix(environment.ID, "container:") {
		return containerEnvironmentMetadata{}, fmt.Errorf("executionplane: container environment does not belong to this provider")
	}
	var metadata containerEnvironmentMetadata
	if err := json.Unmarshal(environment.MetadataJSON, &metadata); err != nil {
		return containerEnvironmentMetadata{}, fmt.Errorf("executionplane: decode container metadata: %w", err)
	}
	if metadata.SchemaVersion != lifecycleSchemaVersion || metadata.Runtime != provider.runtime || metadata.Image != provider.image ||
		strings.TrimSpace(metadata.ContainerID) == "" || !strings.HasPrefix(metadata.ContainerName, "tars-") ||
		metadata.NetworkMode != "none" || !metadata.ReadOnlyRoot || filepath.Clean(metadata.Base.RootDir) != filepath.Clean(environment.RootDir) {
		return containerEnvironmentMetadata{}, fmt.Errorf("executionplane: invalid container environment metadata")
	}
	return metadata, nil
}

func (provider *ContainerEnvironmentProvider) runtimeCommand(ctx context.Context, args ...string) (CommandResult, error) {
	return provider.runner.Run(ctx, CommandSpec{Command: provider.runtime, Args: append([]string(nil), args...), InheritEnv: true})
}

var _ EnvironmentProvider = (*ContainerEnvironmentProvider)(nil)
