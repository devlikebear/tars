package gateway

import (
	"fmt"
	"strings"
	"time"
)

func (r *Runtime) Status() GatewayStatus {
	if r == nil {
		return GatewayStatus{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	active := 0
	for _, state := range r.runs {
		if state.run.Status == RunStatusAccepted || state.run.Status == RunStatusRunning {
			active++
		}
	}
	status := GatewayStatus{
		Enabled:                    r.opts.Enabled,
		Version:                    r.version,
		RunsTotal:                  len(r.runs),
		RunsActive:                 active,
		AgentsCount:                len(r.executors),
		AgentsWatchEnabled:         r.agentsWatchEnabled,
		AgentsReloadVersion:        r.agentsReloadVersion,
		ChannelsLocal:              r.opts.ChannelsLocalEnabled,
		ChannelsWebhook:            r.opts.ChannelsWebhookEnabled,
		ChannelsTelegram:           r.opts.ChannelsTelegramEnabled,
		PersistenceEnabled:         r.opts.GatewayPersistenceEnabled,
		RunsPersistenceEnabled:     r.opts.GatewayRunsPersistenceEnabled,
		ChannelsPersistenceEnabled: r.opts.GatewayChannelsPersistenceEnabled,
		RestoreOnStartup:           r.opts.GatewayRestoreOnStartup,
		PersistenceDir:             strings.TrimSpace(r.opts.GatewayPersistenceDir),
		RunsRestored:               r.runsRestored,
		ChannelsRestored:           r.channelsRestored,
		LastRestoreError:           strings.TrimSpace(r.lastRestoreError),
		Browser:                    r.browser,
		Nodes:                      defaultNodes(),
	}
	if !r.lastPersistAt.IsZero() {
		status.LastPersistAt = r.lastPersistAt.UTC().Format(time.RFC3339)
	}
	if !r.lastRestoreAt.IsZero() {
		status.LastRestoreAt = r.lastRestoreAt.UTC().Format(time.RFC3339)
	}
	if !r.agentsLastReload.IsZero() {
		status.AgentsLastReloadAt = r.agentsLastReload.UTC().Format(time.RFC3339)
	}
	if !r.lastReload.IsZero() {
		status.LastReloadAt = r.lastReload.UTC().Format(time.RFC3339)
	}
	if !r.lastRestart.IsZero() {
		status.LastRestartAt = r.lastRestart.UTC().Format(time.RFC3339)
	}
	return status
}

func (r *Runtime) ReportsSummary() (ReportSummary, error) {
	return r.ReportsSummaryByWorkspace(defaultWorkspaceID)
}

func (r *Runtime) ReportsSummaryByWorkspace(workspaceID string) (ReportSummary, error) {
	if r == nil || !r.opts.Enabled {
		return ReportSummary{}, fmt.Errorf("gateway runtime is disabled")
	}
	targetWorkspaceID := normalizeWorkspaceID(workspaceID)
	r.mu.RLock()
	defer r.mu.RUnlock()
	report := ReportSummary{
		GeneratedAt:      r.nowFn().UTC().Format(time.RFC3339),
		SummaryEnabled:   r.opts.GatewayReportSummaryEnabled,
		ArchiveEnabled:   r.opts.GatewayArchiveEnabled,
		RunsByStatus:     map[string]int{},
		MessagesBySource: map[string]int{},
	}
	for _, state := range r.runs {
		if state == nil {
			continue
		}
		if normalizeWorkspaceID(state.run.WorkspaceID) != targetWorkspaceID {
			continue
		}
		report.RunsTotal++
		key := strings.TrimSpace(string(state.run.Status))
		if key == "" {
			key = string(RunStatusFailed)
		}
		report.RunsByStatus[key]++
		if state.run.Status == RunStatusAccepted || state.run.Status == RunStatusRunning {
			report.RunsActive++
		}
	}
	for _, messages := range r.channelMsgs {
		workspaceMessages := 0
		for _, msg := range messages {
			if normalizeWorkspaceID(msg.WorkspaceID) != targetWorkspaceID {
				continue
			}
			workspaceMessages++
			report.MessagesTotal++
			source := strings.TrimSpace(msg.Source)
			if source == "" {
				source = "unknown"
			}
			report.MessagesBySource[source]++
		}
		if workspaceMessages > 0 {
			report.ChannelsTotal++
		}
	}
	return report, nil
}

func (r *Runtime) ReportsRuns(limit int) (ReportRuns, error) {
	return r.ReportsRunsByWorkspace(defaultWorkspaceID, limit)
}

func (r *Runtime) ReportsRunsByWorkspace(workspaceID string, limit int) (ReportRuns, error) {
	if r == nil || !r.opts.Enabled {
		return ReportRuns{}, fmt.Errorf("gateway runtime is disabled")
	}
	if !r.opts.GatewayArchiveEnabled {
		return ReportRuns{}, fmt.Errorf("gateway archive report is disabled")
	}
	if limit <= 0 {
		limit = 50
	}
	runs := r.ListByWorkspace(workspaceID, limit)
	return ReportRuns{
		GeneratedAt:    r.nowFn().UTC().Format(time.RFC3339),
		ArchiveEnabled: true,
		Count:          len(runs),
		Runs:           runs,
	}, nil
}

func (r *Runtime) ReportsChannels(limit int) (ReportChannels, error) {
	return r.ReportsChannelsByWorkspace(defaultWorkspaceID, limit)
}

func (r *Runtime) ReportsChannelsByWorkspace(workspaceID string, limit int) (ReportChannels, error) {
	if r == nil || !r.opts.Enabled {
		return ReportChannels{}, fmt.Errorf("gateway runtime is disabled")
	}
	if !r.opts.GatewayArchiveEnabled {
		return ReportChannels{}, fmt.Errorf("gateway archive report is disabled")
	}
	if limit <= 0 {
		limit = 50
	}
	targetWorkspaceID := normalizeWorkspaceID(workspaceID)
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string][]ChannelMessage, len(r.channelMsgs))
	for _, messages := range r.channelMsgs {
		filtered := make([]ChannelMessage, 0, len(messages))
		channelID := ""
		for _, msg := range messages {
			if normalizeWorkspaceID(msg.WorkspaceID) != targetWorkspaceID {
				continue
			}
			channelID = strings.TrimSpace(msg.ChannelID)
			filtered = append(filtered, msg)
		}
		if len(filtered) == 0 {
			continue
		}
		if len(filtered) > limit {
			filtered = filtered[len(filtered)-limit:]
		}
		if channelID == "" {
			channelID = "unknown"
		}
		out[channelID] = filtered
	}
	return ReportChannels{
		GeneratedAt:    r.nowFn().UTC().Format(time.RFC3339),
		ArchiveEnabled: true,
		Count:          len(out),
		Messages:       out,
	}, nil
}

func (r *Runtime) Reload() GatewayStatus {
	if r == nil {
		return GatewayStatus{}
	}
	r.mu.Lock()
	r.version++
	r.lastReload = r.nowFn().UTC()
	r.stateVersion++
	r.mu.Unlock()
	r.persistSnapshot()
	return r.Status()
}

func (r *Runtime) Restart() GatewayStatus {
	if r == nil {
		return GatewayStatus{}
	}
	r.mu.Lock()
	for _, state := range r.runs {
		if state.run.Status == RunStatusAccepted || state.run.Status == RunStatusRunning {
			if state.cancel != nil {
				state.cancel()
			}
			now := r.nowFn().UTC().Format(time.RFC3339)
			state.run.Status = RunStatusCanceled
			state.run.Error = "canceled by gateway restart"
			state.run.CompletedAt = now
			state.run.UpdatedAt = now
			r.closeRunDoneLocked(state)
		}
	}
	if r.browserService != nil {
		r.browser = toGatewayBrowserState(r.browserService.Stop())
	} else {
		r.browser = BrowserState{}
	}
	r.trimRunHistoryLocked()
	r.version++
	r.lastRestart = r.nowFn().UTC()
	r.stateVersion++
	r.mu.Unlock()
	r.persistSnapshot()
	return r.Status()
}
