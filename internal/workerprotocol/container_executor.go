package workerprotocol

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/secrets"
)

const containerTaskSchemaVersion = 1

var (
	containerCPUFormat = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?$`)
	containerImageName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/:@-]*$`)
)

type ContainerReferenceExecutorOptions struct {
	RuntimePath    string
	Image          string
	Command        []string
	CPUs           string
	PIDsLimit      int
	Runner         ProcessRunner
	SupportsResume bool
}

type ContainerReferenceExecutor struct {
	runtimePath    string
	image          string
	command        []string
	cpus           string
	pidsLimit      int
	runner         ProcessRunner
	supportsResume bool
}

type ContainerTaskRequest struct {
	SchemaVersion   int              `json:"schema_version"`
	ProtocolVersion string           `json:"protocol_version"`
	Binding         TaskTokenBinding `json:"binding"`
	Policy          ExecutionPolicy  `json:"policy"`
	Workspace       WorkspaceBundle  `json:"workspace"`
	Request         json.RawMessage  `json:"request,omitempty"`
	Resume          bool             `json:"resume,omitempty"`
	Checkpoint      *Checkpoint      `json:"checkpoint,omitempty"`
}

type ContainerTaskResponse struct {
	Succeeded  bool               `json:"succeeded"`
	Output     json.RawMessage    `json:"output,omitempty"`
	Error      string             `json:"error,omitempty"`
	Artifacts  []WireArtifact     `json:"artifacts,omitempty"`
	Checkpoint *CheckpointPayload `json:"checkpoint,omitempty"`
}

func NewContainerReferenceExecutor(opts ContainerReferenceExecutorOptions) (*ContainerReferenceExecutor, error) {
	if !safeAbsoluteProcessPath(opts.RuntimePath) || !validPinnedContainerImage(opts.Image) || len(opts.Command) == 0 {
		return nil, fmt.Errorf("%w: container runtime, immutable image, and command are required", ErrTransportConfig)
	}
	for _, argument := range opts.Command {
		if !safeContainerArgument(argument) {
			return nil, fmt.Errorf("%w: invalid container command argument", ErrTransportConfig)
		}
	}
	if strings.TrimSpace(opts.CPUs) == "" {
		opts.CPUs = "1"
	}
	cpuLimit, cpuErr := strconv.ParseFloat(opts.CPUs, 64)
	if !containerCPUFormat.MatchString(opts.CPUs) || cpuErr != nil || cpuLimit <= 0 {
		return nil, fmt.Errorf("%w: invalid container CPU limit", ErrTransportConfig)
	}
	if opts.PIDsLimit <= 0 {
		opts.PIDsLimit = 128
	}
	if opts.Runner == nil {
		opts.Runner = OSProcessRunner{}
	}
	return &ContainerReferenceExecutor{
		runtimePath: opts.RuntimePath, image: opts.Image, command: append([]string(nil), opts.Command...),
		cpus: opts.CPUs, pidsLimit: opts.PIDsLimit, runner: opts.Runner, supportsResume: opts.SupportsResume,
	}, nil
}

func (executor *ContainerReferenceExecutor) Capabilities() WorkerCapabilities {
	if executor == nil {
		return WorkerCapabilities{}
	}
	return WorkerCapabilities{
		Resume: executor.supportsResume, Streaming: false, Checkpoints: executor.supportsResume,
		EgressPolicy: true, ResourceLimits: true, ArtifactScan: true,
	}
}

func (executor *ContainerReferenceExecutor) Execute(ctx context.Context, request ReferenceExecutionRequest) (ReferenceExecutionResult, error) {
	if executor == nil || executor.runner == nil {
		return ReferenceExecutionResult{}, ErrTransportConfig
	}
	if err := validateTaskTokenBinding(request.Binding); err != nil {
		return ReferenceExecutionResult{}, err
	}
	if err := request.Policy.Validate(); err != nil {
		return ReferenceExecutionResult{}, err
	}
	if request.Policy.Egress.Mode != EgressDeny {
		return ReferenceExecutionResult{}, fmt.Errorf("%w: container worker supports default-deny egress only", ErrInvalidPolicy)
	}
	if request.Resume && (!executor.supportsResume || request.Checkpoint == nil) {
		return ReferenceExecutionResult{}, fmt.Errorf("%w: container worker cannot resume request", ErrInvalidTransition)
	}
	workspaceLimits := containerWorkspaceLimits(request.Policy)
	workspace, err := BuildWorkspaceBundle(ctx, WorkspaceBundleOptions{
		RootDir: request.RootDir, Mode: SyncModeDirectory, Limits: workspaceLimits,
	})
	if err != nil {
		return ReferenceExecutionResult{}, err
	}
	wireRequest := ContainerTaskRequest{
		SchemaVersion: containerTaskSchemaVersion, ProtocolVersion: ProtocolVersionV1,
		Binding: request.Binding, Policy: request.Policy, Workspace: workspace,
		Request: append(json.RawMessage(nil), request.Request...), Resume: request.Resume,
		Checkpoint: cloneCheckpoint(request.Checkpoint),
	}
	stdin, err := json.Marshal(wireRequest)
	if err != nil {
		return ReferenceExecutionResult{}, fmt.Errorf("workerprotocol: encode container task request: %w", err)
	}
	stdin = append(stdin, '\n')
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(request.Policy.Limits.CPUSeconds)*time.Second)
	defer cancel()
	result, err := executor.runner.Run(runCtx, ProcessSpec{
		Path: executor.runtimePath, Args: executor.arguments(request.Policy), Stdin: stdin,
		InheritEnv: false, MaxOutputBytes: request.Policy.Limits.MaxOutputBytes,
	})
	if int64(len(result.Stdout)) > request.Policy.Limits.MaxOutputBytes || int64(len(result.Stderr)) > request.Policy.Limits.MaxOutputBytes {
		return ReferenceExecutionResult{}, ErrTransportLimit
	}
	if err != nil {
		return ReferenceExecutionResult{}, fmt.Errorf("workerprotocol: isolated container task failed: %w", err)
	}
	var response ContainerTaskResponse
	if err := decodeSingleJSONLine(result.Stdout, &response); err != nil {
		return ReferenceExecutionResult{}, err
	}
	if len(response.Output) > 0 && !json.Valid(response.Output) {
		return ReferenceExecutionResult{}, ErrWireContract
	}
	payload, err := json.Marshal(map[string]any{
		"succeeded": response.Succeeded,
		"output":    json.RawMessage(response.Output),
		"error":     secrets.RedactPreview(response.Error, 512),
	})
	if err != nil {
		return ReferenceExecutionResult{}, fmt.Errorf("workerprotocol: encode container task result: %w", err)
	}
	return ReferenceExecutionResult{
		Payload: payload, Artifacts: cloneWireArtifacts(response.Artifacts),
		Checkpoint: cloneCheckpointPayload(response.Checkpoint),
	}, nil
}

func (executor *ContainerReferenceExecutor) arguments(policy ExecutionPolicy) []string {
	workspaceTmpfs := fmt.Sprintf("/workspace:rw,nosuid,nodev,size=%dm", policy.Limits.DiskMB)
	tmpSize := policy.Limits.MemoryMB / 8
	if tmpSize < 16 {
		tmpSize = 16
	}
	if tmpSize > 256 {
		tmpSize = 256
	}
	tmpTmpfs := fmt.Sprintf("/tmp:rw,noexec,nosuid,nodev,size=%dm", tmpSize)
	args := []string{
		"run", "--rm", "--network", "none", "--read-only",
		"--cpus", executor.cpus, "--memory", strconv.FormatInt(policy.Limits.MemoryMB, 10) + "m",
		"--pids-limit", strconv.Itoa(executor.pidsLimit),
		"--tmpfs", workspaceTmpfs, "--tmpfs", tmpTmpfs,
		"--workdir", "/workspace", executor.image,
	}
	return append(args, executor.command...)
}

func containerWorkspaceLimits(policy ExecutionPolicy) WorkspaceBundleLimits {
	maxBytes := policy.Limits.DiskMB << 20
	defaults := DefaultWorkspaceBundleLimits()
	if maxBytes > defaults.MaxBytes {
		maxBytes = defaults.MaxBytes
	}
	maxFileBytes := defaults.MaxFileBytes
	if maxFileBytes > maxBytes {
		maxFileBytes = maxBytes
	}
	return WorkspaceBundleLimits{MaxFiles: defaults.MaxFiles, MaxFileBytes: maxFileBytes, MaxBytes: maxBytes}
}

func validPinnedContainerImage(image string) bool {
	image = strings.TrimSpace(image)
	if !containerImageName.MatchString(image) {
		return false
	}
	marker := "@sha256:"
	index := strings.LastIndex(image, marker)
	if index <= 0 || index+len(marker)+64 != len(image) {
		return false
	}
	_, err := hex.DecodeString(image[index+len(marker):])
	return err == nil
}

func safeContainerArgument(argument string) bool {
	return argument != "" && len(argument) <= 4096 && !strings.ContainsAny(argument, "\x00\r\n")
}

var _ ReferenceExecutor = (*ContainerReferenceExecutor)(nil)
