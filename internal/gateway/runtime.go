package gateway

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
	if opts.GatewayRunsMaxRecords <= 0 {
		opts.GatewayRunsMaxRecords = 2000
	}
	if opts.GatewayChannelsMaxMessagesPerChannel <= 0 {
		opts.GatewayChannelsMaxMessagesPerChannel = 500
	}
	if opts.GatewaySubagentsMaxThreads <= 0 {
		opts.GatewaySubagentsMaxThreads = 4
	}
	if opts.GatewaySubagentsMaxDepth <= 0 {
		opts.GatewaySubagentsMaxDepth = 1
	}
	if opts.GatewayConsensusMaxFanout <= 0 {
		opts.GatewayConsensusMaxFanout = 3
	}
	if opts.GatewayConsensusBudgetTokens <= 0 {
		opts.GatewayConsensusBudgetTokens = 20000
	}
	if opts.GatewayConsensusBudgetUSD <= 0 {
		opts.GatewayConsensusBudgetUSD = 0.50
	}
	if opts.GatewayConsensusTimeoutSeconds <= 0 {
		opts.GatewayConsensusTimeoutSeconds = 120
	}
	if opts.GatewayConsensusConcurrentRuns <= 0 {
		opts.GatewayConsensusConcurrentRuns = 1
	}
	if strings.TrimSpace(opts.GatewayPersistenceDir) == "" {
		opts.GatewayPersistenceDir = filepath.Join(strings.TrimSpace(opts.WorkspaceDir), "_shared", "gateway")
	}
	if strings.TrimSpace(opts.GatewayArchiveDir) == "" {
		opts.GatewayArchiveDir = filepath.Join(strings.TrimSpace(opts.WorkspaceDir), "_shared", "gateway", "archive")
	}
	if opts.GatewayArchiveRetentionDays <= 0 {
		opts.GatewayArchiveRetentionDays = 30
	}
	if opts.GatewayArchiveMaxFileBytes <= 0 {
		opts.GatewayArchiveMaxFileBytes = 10485760
	}
	rt := &Runtime{
		opts:               opts,
		nowFn:              nowFn,
		runs:               map[string]*runState{},
		channelMsgs:        map[string][]ChannelMessage{},
		executors:          map[string]AgentExecutor{},
		executionSem:       newExecutionSemaphore(opts.GatewaySubagentsMaxThreads),
		agentsWatchEnabled: opts.GatewayAgentsWatchEnabled,
		version:            1,
		persistStore:       newSnapshotStore(opts.GatewayPersistenceDir),
		stateVersion:       1,
		runEvents:          newRunEventBroker(),
		subagentPool:       newWeightedSemaphore(opts.GatewaySubagentsMaxThreads),
		consensusRuns:      newWeightedSemaphore(opts.GatewayConsensusConcurrentRuns),
		consensusPool:      newWeightedSemaphore(opts.GatewayConsensusMaxFanout * opts.GatewayConsensusConcurrentRuns),
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
		return fmt.Errorf("gateway runtime is disabled")
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
