package tarsserver

import (
	"fmt"
	"os"
	"time"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/config"
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
	return workscheduler.New(workscheduler.Options{
		Store: ledger, WorkspaceID: defaultWorkspaceID, WorkerID: workerID, ActorID: "tars-work-scheduler",
		LeaseDuration:     time.Duration(cfg.WorkLedger.SchedulerLeaseSeconds) * time.Second,
		HeartbeatInterval: time.Duration(cfg.WorkLedger.SchedulerHeartbeatSeconds) * time.Second,
		PollInterval:      time.Duration(cfg.WorkLedger.SchedulerPollMilliseconds) * time.Millisecond,
		MaxWorkers:        cfg.WorkLedger.SchedulerMaxWorkers,
		Executors:         []workscheduler.Executor{tool.NewAgentRuntimeWorkExecutor(runtime, ledger)},
		OnError: func(err error) {
			logger.Error().Err(err).Msg("durable work scheduler operation failed")
		},
	})
}
