package tarsserver

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/a2a"
	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/executionplane"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/devlikebear/tars/internal/workerprotocol"
	"github.com/devlikebear/tars/internal/workscheduler"
	"github.com/devlikebear/tars/internal/workstore"
	"github.com/rs/zerolog"
)

func buildWorkSchedulerIfEnabled(cfg config.Config, ledger *workstore.Store, runtime *agentruntime.Runtime, logger zerolog.Logger) (*workscheduler.Scheduler, error) {
	return buildWorkSchedulerWithRemote(cfg, ledger, runtime, nil, nil, logger)
}

func buildWorkSchedulerWithRemote(
	cfg config.Config,
	ledger *workstore.Store,
	runtime *agentruntime.Runtime,
	workerController *workerprotocol.Controller,
	remoteRunner workerprotocol.ProcessRunner,
	logger zerolog.Logger,
) (*workscheduler.Scheduler, error) {
	if !cfg.WorkLedger.SchedulerEnabled {
		return nil, nil
	}
	if !cfg.WorkLedger.Enabled || ledger == nil {
		return nil, fmt.Errorf("durable work scheduler requires work_ledger.enabled")
	}
	if runtime == nil || !runtime.Enabled() {
		return nil, fmt.Errorf("durable work scheduler requires agentruntime.enabled")
	}
	workerID := fmt.Sprintf("tarsd-%d-%d", os.Getpid(), time.Now().UnixNano())
	executor, err := buildNativeWorkExecutionPlane(cfg, ledger, runtime)
	if err != nil {
		return nil, err
	}
	executors := []workscheduler.Executor{executor}
	harnessExecutor, err := buildConfiguredClaudeCodeWorkExecutor(cfg, ledger)
	if err != nil {
		return nil, err
	}
	if harnessExecutor != nil {
		executors = append(executors, harnessExecutor)
	}
	remoteExecutor, err := buildConfiguredRemoteWorkExecutor(cfg, workerController, remoteRunner)
	if err != nil {
		return nil, err
	}
	if remoteExecutor != nil {
		executors = append(executors, remoteExecutor)
	}
	a2aExecutor, err := buildA2AWorkExecutor(context.Background(), cfg, ledger, &http.Client{Timeout: 15 * time.Second})
	if err != nil {
		return nil, err
	}
	if a2aExecutor != nil {
		executors = append(executors, a2aExecutor)
	}
	return workscheduler.New(workscheduler.Options{
		Store: ledger, WorkspaceID: defaultWorkspaceID, WorkerID: workerID, ActorID: "tars-work-scheduler",
		LeaseDuration:     time.Duration(cfg.WorkLedger.SchedulerLeaseSeconds) * time.Second,
		HeartbeatInterval: time.Duration(cfg.WorkLedger.SchedulerHeartbeatSeconds) * time.Second,
		PollInterval:      time.Duration(cfg.WorkLedger.SchedulerPollMilliseconds) * time.Millisecond,
		MaxWorkers:        cfg.WorkLedger.SchedulerMaxWorkers,
		Executors:         executors,
		OnError: func(err error) {
			logger.Error().Err(err).Msg("durable work scheduler operation failed")
		},
	})
}

func buildConfiguredClaudeCodeWorkExecutor(cfg config.Config, ledger *workstore.Store) (*executionplane.LifecycleExecutor, error) {
	configPath := strings.TrimSpace(cfg.WorkLedger.SchedulerExternalHarnessConfigPath)
	if configPath == "" {
		return nil, nil
	}
	workspaceDir := strings.TrimSpace(cfg.WorkspaceDir)
	dataDir := strings.TrimSpace(cfg.WorkLedger.SchedulerExecutionDataDir)
	if workspaceDir == "" || dataDir == "" || pathsOverlap(workspaceDir, dataDir) {
		return nil, fmt.Errorf("configured external harness requires private execution data outside the workspace")
	}
	canonicalWorkspace, err := filepath.EvalSymlinks(workspaceDir)
	if err != nil {
		return nil, fmt.Errorf("resolve external harness workspace: %w", err)
	}
	canonicalConfig, err := filepath.EvalSymlinks(configPath)
	if err != nil {
		return nil, fmt.Errorf("resolve external harness config: %w", err)
	}
	if pathContains(canonicalWorkspace, canonicalConfig) {
		return nil, fmt.Errorf("external harness config must be outside the workspace")
	}
	worker, err := executionplane.OpenConfiguredClaudeCodeWorker(configPath)
	if err != nil {
		return nil, err
	}
	harnessRoot := filepath.Join(dataDir, "external-harness", "claude-code")
	provider, err := executionplane.NewManagedWorktreeProvider(workspaceDir, filepath.Join(harnessRoot, "worktrees"))
	if err != nil {
		return nil, err
	}
	stateStore, err := executionplane.NewFileStateStore(filepath.Join(harnessRoot, "state"))
	if err != nil {
		return nil, err
	}
	sink, err := executionplane.NewWorkLedgerSink(ledger, "tars-claude-code-harness")
	if err != nil {
		return nil, err
	}
	collector, err := executionplane.NewFileArtifactCollector(executionplane.ArtifactCollectorOptions{
		RootDir: filepath.Join(harnessRoot, "artifacts"), Paths: cfg.WorkLedger.SchedulerArtifactPaths,
		IncludeTranscript: true, IncludeGitPatch: true,
	})
	if err != nil {
		return nil, err
	}
	return executionplane.NewLifecycleExecutor(executionplane.Options{
		Adapter: worker.Name(), SourceDir: workspaceDir, Provider: provider, Worker: worker,
		ArtifactCollector: collector, ArtifactSink: sink, StateStore: stateStore, EventSink: sink,
	})
}

func buildConfiguredRemoteWorkExecutor(
	cfg config.Config,
	controller *workerprotocol.Controller,
	runner workerprotocol.ProcessRunner,
) (*workerprotocol.SchedulerExecutor, error) {
	configPath := strings.TrimSpace(cfg.WorkLedger.SchedulerRemoteWorkersGatewayConfigPath)
	if configPath == "" {
		return nil, nil
	}
	if !cfg.WorkLedger.SchedulerRemoteWorkersEnabled {
		return nil, fmt.Errorf("configured remote scheduler worker requires remote_workers.enabled")
	}
	if controller == nil {
		return nil, fmt.Errorf("configured remote scheduler worker requires the shared worker controller")
	}
	dataDir := strings.TrimSpace(cfg.WorkLedger.SchedulerExecutionDataDir)
	if dataDir == "" || pathsOverlap(cfg.WorkspaceDir, dataDir) {
		return nil, fmt.Errorf("configured remote scheduler state must be outside the workspace")
	}
	return workerprotocol.OpenConfiguredSchedulerExecutor(workerprotocol.ConfiguredSchedulerExecutorOptions{
		ConfigPath: configPath, SourceDir: cfg.WorkspaceDir,
		DataDir:    filepath.Join(dataDir, "remote-workers", "scheduler"),
		Controller: controller, Runner: runner,
	})
}

func buildA2AWorkExecutor(ctx context.Context, cfg config.Config, ledger *workstore.Store, httpClient *http.Client) (*a2a.Executor, error) {
	if !cfg.WorkLedger.SchedulerA2AEnabled {
		return nil, nil
	}
	if ledger == nil || strings.TrimSpace(cfg.WorkLedger.SchedulerA2ADiscoveryURL) == "" {
		return nil, fmt.Errorf("A2A work executor requires the Work Ledger and a discovery URL")
	}
	card, endpoint, err := a2a.Discover(ctx, cfg.WorkLedger.SchedulerA2ADiscoveryURL, a2a.DiscoveryOptions{
		HTTPClient:        httpClient,
		AllowLoopbackHTTP: cfg.WorkLedger.SchedulerA2AAllowInsecureLoopback,
		AllowPrivateHosts: cfg.WorkLedger.SchedulerA2AAllowPrivateHosts,
		AllowedHosts:      cfg.WorkLedger.SchedulerA2AAllowedHosts,
	})
	if err != nil {
		return nil, fmt.Errorf("discover configured A2A agent: %w", err)
	}
	token := strings.TrimSpace(cfg.WorkLedger.SchedulerA2ABearerToken)
	if agentCardRequiresCredential(card) && token == "" {
		return nil, fmt.Errorf("configured A2A agent requires a gateway credential")
	}
	var tokenProvider a2a.TokenProvider
	if token != "" {
		tokenProvider = a2a.TokenProviderFunc(func(context.Context) (string, error) {
			return token, nil
		})
	}
	client, err := a2a.NewClient(endpoint, a2a.ClientOptions{
		HTTPClient:        httpClient,
		AllowLoopbackHTTP: cfg.WorkLedger.SchedulerA2AAllowInsecureLoopback,
		AllowPrivateHosts: cfg.WorkLedger.SchedulerA2AAllowPrivateHosts,
		AllowedHosts:      cfg.WorkLedger.SchedulerA2AAllowedHosts,
		TokenProvider:     tokenProvider,
	})
	if err != nil {
		return nil, err
	}
	journal, err := a2a.NewWorkLedgerJournal(ledger, "tars-a2a-gateway")
	if err != nil {
		return nil, err
	}
	return a2a.NewExecutor(a2a.ExecutorOptions{
		Client: client, Journal: journal,
		PollInterval:    time.Duration(cfg.WorkLedger.SchedulerA2APollMilliseconds) * time.Millisecond,
		MaxPollDuration: time.Duration(cfg.WorkLedger.SchedulerA2AMaxPollSeconds) * time.Second,
		AcceptedModes:   append([]string(nil), card.DefaultOutputModes...),
	})
}

func agentCardRequiresCredential(card a2a.AgentCard) bool {
	if len(card.SecurityRequirements) == 0 {
		return false
	}
	for _, requirement := range card.SecurityRequirements {
		if len(requirement.Schemes) == 0 {
			return false
		}
	}
	return true
}

func buildNativeWorkExecutionPlane(cfg config.Config, ledger *workstore.Store, runtime *agentruntime.Runtime) (*executionplane.LifecycleExecutor, error) {
	workspaceDir := strings.TrimSpace(cfg.WorkspaceDir)
	dataDir := strings.TrimSpace(cfg.WorkLedger.SchedulerExecutionDataDir)
	if workspaceDir == "" || dataDir == "" {
		return nil, fmt.Errorf("durable execution plane requires workspace and execution data directories")
	}
	if pathsOverlap(workspaceDir, dataDir) {
		return nil, fmt.Errorf("durable execution data directory must be outside the workspace")
	}

	var provider executionplane.EnvironmentProvider
	var err error
	switch mode := strings.ToLower(strings.TrimSpace(cfg.WorkLedger.SchedulerExecutionEnvironment)); mode {
	case "", "local":
		provider, err = executionplane.NewLocalEnvironmentProvider(workspaceDir)
	case "managed-worktree":
		provider, err = executionplane.NewManagedWorktreeProvider(workspaceDir, filepath.Join(dataDir, "worktrees"))
	default:
		return nil, fmt.Errorf("durable native execution environment %q is unsupported", mode)
	}
	if err != nil {
		return nil, err
	}
	stateStore, err := executionplane.NewFileStateStore(filepath.Join(dataDir, "state", "native-agentruntime"))
	if err != nil {
		return nil, err
	}
	sink, err := executionplane.NewWorkLedgerSink(ledger, "tars-execution-plane")
	if err != nil {
		return nil, err
	}
	nativeExecutor := tool.NewAgentRuntimeWorkExecutor(runtime, ledger)
	worker, err := executionplane.NewSchedulerWorkerClient("native-agentruntime", nativeExecutor, true)
	if err != nil {
		return nil, err
	}
	var collector executionplane.ArtifactCollector
	if len(cfg.WorkLedger.SchedulerArtifactPaths) > 0 {
		collector, err = executionplane.NewFileArtifactCollector(executionplane.ArtifactCollectorOptions{
			RootDir: filepath.Join(dataDir, "artifacts"), Paths: cfg.WorkLedger.SchedulerArtifactPaths,
			IncludeTranscript: true,
		})
		if err != nil {
			return nil, err
		}
	}
	return executionplane.NewLifecycleExecutor(executionplane.Options{
		Adapter: "agentruntime", SourceDir: workspaceDir, Provider: provider, Worker: worker,
		ArtifactCollector: collector, ArtifactSink: sink, StateStore: stateStore, EventSink: sink,
	})
}

func pathsOverlap(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(strings.TrimSpace(left))
	rightAbs, rightErr := filepath.Abs(strings.TrimSpace(right))
	if leftErr != nil || rightErr != nil {
		return true
	}
	return pathContains(leftAbs, rightAbs) || pathContains(rightAbs, leftAbs)
}

func pathContains(root, candidate string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
