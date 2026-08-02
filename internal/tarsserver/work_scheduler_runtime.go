package tarsserver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/executionplane"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/devlikebear/tars/internal/workscheduler"
	"github.com/devlikebear/tars/internal/workstore"
	"github.com/rs/zerolog"
)

func buildWorkSchedulerIfEnabled(cfg config.Config, ledger *workstore.Store, runtime *agentruntime.Runtime, logger zerolog.Logger) (*workscheduler.Scheduler, error) {
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
	return workscheduler.New(workscheduler.Options{
		Store: ledger, WorkspaceID: defaultWorkspaceID, WorkerID: workerID, ActorID: "tars-work-scheduler",
		LeaseDuration:     time.Duration(cfg.WorkLedger.SchedulerLeaseSeconds) * time.Second,
		HeartbeatInterval: time.Duration(cfg.WorkLedger.SchedulerHeartbeatSeconds) * time.Second,
		PollInterval:      time.Duration(cfg.WorkLedger.SchedulerPollMilliseconds) * time.Millisecond,
		MaxWorkers:        cfg.WorkLedger.SchedulerMaxWorkers,
		Executors:         []workscheduler.Executor{executor},
		OnError: func(err error) {
			logger.Error().Err(err).Msg("durable work scheduler operation failed")
		},
	})
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
