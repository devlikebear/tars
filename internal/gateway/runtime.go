package gateway

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/browser"
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
		agentsWatchEnabled: opts.GatewayAgentsWatchEnabled,
		version:            1,
		persistStore:       newSnapshotStore(opts.GatewayPersistenceDir),
		stateVersion:       1,
	}
	rt.browserService = opts.BrowserService
	if rt.browserService == nil {
		rt.browserService = browser.NewService(browser.Config{
			WorkspaceDir:           strings.TrimSpace(opts.WorkspaceDir),
			DefaultProfile:         strings.TrimSpace(opts.BrowserDefaultProfile),
			ManagedUserDataDir:     strings.TrimSpace(opts.BrowserManagedUserDataDir),
			SiteFlowsDir:           strings.TrimSpace(opts.BrowserSiteFlowsDir),
			AutoLoginSiteAllowlist: append([]string(nil), opts.BrowserAutoLoginSiteAllowlist...),
			Vault:                  opts.BrowserVaultReader,
		})
	}
	if rt.browserService != nil {
		rt.browser = toGatewayBrowserState(rt.browserService.Status())
	}
	rt.initExecutors()
	rt.restoreSnapshotOnStartup()
	return rt
}

func (r *Runtime) Enabled() bool {
	return r != nil && r.opts.Enabled
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
