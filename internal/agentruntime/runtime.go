package agentruntime

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

func NewRuntime(opts RuntimeOptions) *Runtime {
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	if opts.AgentRuntimeRunsMaxRecords <= 0 {
		opts.AgentRuntimeRunsMaxRecords = 2000
	}
	if opts.AgentRuntimeChannelsMaxMessagesPerChannel <= 0 {
		opts.AgentRuntimeChannelsMaxMessagesPerChannel = 500
	}
	if opts.AgentRuntimeSubagentsMaxThreads <= 0 {
		opts.AgentRuntimeSubagentsMaxThreads = 4
	}
	if opts.AgentRuntimeSubagentsMaxDepth <= 0 {
		opts.AgentRuntimeSubagentsMaxDepth = 1
	}
	if opts.AgentRuntimeConsensusMaxFanout <= 0 {
		opts.AgentRuntimeConsensusMaxFanout = 3
	}
	if opts.AgentRuntimeConsensusBudgetTokens <= 0 {
		opts.AgentRuntimeConsensusBudgetTokens = 20000
	}
	if opts.AgentRuntimeConsensusBudgetUSD <= 0 {
		opts.AgentRuntimeConsensusBudgetUSD = 0.50
	}
	if opts.AgentRuntimeConsensusTimeoutSeconds <= 0 {
		opts.AgentRuntimeConsensusTimeoutSeconds = 120
	}
	if opts.AgentRuntimeConsensusConcurrentRuns <= 0 {
		opts.AgentRuntimeConsensusConcurrentRuns = 1
	}
	if strings.TrimSpace(opts.AgentRuntimePersistenceDir) == "" {
		opts.AgentRuntimePersistenceDir = filepath.Join(strings.TrimSpace(opts.WorkspaceDir), "_shared", "agentruntime")
	}
	if strings.TrimSpace(opts.AgentRuntimeArchiveDir) == "" {
		opts.AgentRuntimeArchiveDir = filepath.Join(strings.TrimSpace(opts.WorkspaceDir), "_shared", "agentruntime", "archive")
	}
	if opts.AgentRuntimeArchiveRetentionDays <= 0 {
		opts.AgentRuntimeArchiveRetentionDays = 30
	}
	if opts.AgentRuntimeArchiveMaxFileBytes <= 0 {
		opts.AgentRuntimeArchiveMaxFileBytes = 10485760
	}
	rt := &Runtime{
		opts:               opts,
		nowFn:              nowFn,
		runs:               map[string]*runState{},
		channelMsgs:        map[string][]ChannelMessage{},
		executors:          map[string]AgentExecutor{},
		executionSem:       newExecutionSemaphore(opts.AgentRuntimeSubagentsMaxThreads),
		agentsWatchEnabled: opts.AgentRuntimeAgentsWatchEnabled,
		version:            1,
		persistStore:       newSnapshotStore(opts.AgentRuntimePersistenceDir),
		stateVersion:       1,
		runEvents:          newRunEventBroker(),
		subagentPool:       newWeightedSemaphore(opts.AgentRuntimeSubagentsMaxThreads),
		consensusRuns:      newWeightedSemaphore(opts.AgentRuntimeConsensusConcurrentRuns),
		consensusPool:      newWeightedSemaphore(opts.AgentRuntimeConsensusMaxFanout * opts.AgentRuntimeConsensusConcurrentRuns),
	}
	rt.initExecutors()
	rt.restoreSnapshotOnStartup()
	return rt
}

func (r *Runtime) Enabled() bool {
	return r != nil && r.opts.Enabled
}

func (r *Runtime) requireEnabled() error {
	if !r.Enabled() {
		return fmt.Errorf("agent runtime is disabled")
	}
	return nil
}

func (r *Runtime) Close(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	states := make([]*runState, 0, len(r.runs))
	canceledAt := r.nowFn().UTC().Format(time.RFC3339)
	mutated := false
	for _, state := range r.runs {
		if state == nil {
			continue
		}
		if state.run.Status == RunStatusAccepted || state.run.Status == RunStatusRunning {
			state.run.Status = RunStatusCanceled
			if state.run.CompletedAt == "" {
				state.run.CompletedAt = canceledAt
			}
			state.run.UpdatedAt = canceledAt
			r.closeRunDoneLocked(state)
			mutated = true
		}
		states = append(states, state)
	}
	r.trimRunHistoryLocked()
	if mutated {
		r.stateVersion++
	}
	r.mu.Unlock()
	r.persistSnapshot()

	for _, state := range states {
		if state != nil && state.cancel != nil {
			state.cancel()
		}
	}

	done := make(chan struct{})
	go func() {
		r.runWG.Wait()
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}
