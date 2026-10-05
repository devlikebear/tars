package tarsserver

import (
	"fmt"
	"os"
	"time"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/apptool"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/workscheduler"
	"github.com/devlikebear/tars/internal/workstore"
	"github.com/rs/zerolog"
)

func buildWorkSchedulerIfEnabled(cfg config.Config, ledger *workstore.Store, runtime *agentruntime.Runtime, logger zerolog.Logger) (*workscheduler.Scheduler, error) {
	if !cfg.WorkLedger.SchedulerEnabled {
		return nil, nil
	}
	// The scheduler is on by default, so a server that has what it needs
	// switched off runs without it rather than refusing to start: subagent
	// orchestration then stays on the request-bound path.
	if !cfg.WorkLedger.Enabled || ledger == nil {
		logger.Warn().Msg("durable work scheduler is off: it requires work_ledger.enabled")
		return nil, nil
	}
	if runtime == nil || !runtime.Enabled() {
		logger.Warn().Msg("durable work scheduler is off: it requires agentruntime.enabled")
		return nil, nil
	}
	workerID := fmt.Sprintf("tarsd-%d-%d", os.Getpid(), time.Now().UnixNano())
	return workscheduler.New(workscheduler.Options{
		Store: ledger, WorkspaceID: defaultWorkspaceID, WorkerID: workerID, ActorID: "tars-work-scheduler",
		LeaseDuration:     time.Duration(cfg.WorkLedger.SchedulerLeaseSeconds) * time.Second,
		HeartbeatInterval: time.Duration(cfg.WorkLedger.SchedulerHeartbeatSeconds) * time.Second,
		PollInterval:      time.Duration(cfg.WorkLedger.SchedulerPollMilliseconds) * time.Millisecond,
		MaxWorkers:        cfg.WorkLedger.SchedulerMaxWorkers,
		Executors:         []workscheduler.Executor{apptool.NewAgentRuntimeWorkExecutor(runtime, ledger)},
		OnError: func(err error) {
			logger.Error().Err(err).Msg("durable work scheduler operation failed")
		},
	})
}
