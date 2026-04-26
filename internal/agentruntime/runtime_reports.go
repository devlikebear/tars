package agentruntime

import (
	"fmt"
	"strings"
	"time"
)

func (r *Runtime) Status() AgentRuntimeStatus {
	if r == nil {
		return AgentRuntimeStatus{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	active := 0
	for _, state := range r.runs {
		if state.run.Status == RunStatusAccepted || state.run.Status == RunStatusRunning {
			active++
		}
	}
	status := AgentRuntimeStatus{
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
		PersistenceEnabled:         r.opts.AgentRuntimePersistenceEnabled,
		RunsPersistenceEnabled:     r.opts.AgentRuntimeRunsPersistenceEnabled,
		ChannelsPersistenceEnabled: r.opts.AgentRuntimeChannelsPersistenceEnabled,
		RestoreOnStartup:           r.opts.AgentRuntimeRestoreOnStartup,
		PersistenceDir:             strings.TrimSpace(r.opts.AgentRuntimePersistenceDir),
		RunsRestored:               r.runsRestored,
		ChannelsRestored:           r.channelsRestored,
		LastRestoreError:           strings.TrimSpace(r.lastRestoreError),
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
	if err := r.requireEnabled(); err != nil {
		return ReportSummary{}, err
	}
	targetWorkspaceID := normalizeWorkspaceID(workspaceID)
	r.mu.RLock()
	defer r.mu.RUnlock()
	report := ReportSummary{
		GeneratedAt:      r.nowFn().UTC().Format(time.RFC3339),
		SummaryEnabled:   r.opts.AgentRuntimeReportSummaryEnabled,
		ArchiveEnabled:   r.opts.AgentRuntimeArchiveEnabled,
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

// ReportsRunsByWorkspace returns recent in-memory run summaries.
//
// Despite the name, the gating flag AgentRuntimeArchiveEnabled doubles as the
// "report endpoint visibility" switch — it controls both on-disk archive
// writes and whether this in-memory report endpoint serves data, even
// though the data itself is from r.runs (memory) and never touches the
// archive directory. Operators who want only the report endpoint without
// disk archives still have to enable archive_enabled. Splitting this
// into a dedicated AgentRuntimeReportEnabled flag is tracked in RF-057 as
// part of the broader config namespace migration (ID-005).
func (r *Runtime) ReportsRunsByWorkspace(workspaceID string, limit int) (ReportRuns, error) {
	if err := r.requireEnabled(); err != nil {
		return ReportRuns{}, err
	}
	if !r.opts.AgentRuntimeArchiveEnabled {
		return ReportRuns{}, fmt.Errorf("agent runtime archive report is disabled")
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

// ReportsChannelsByWorkspace returns recent in-memory channel messages.
// See ReportsRunsByWorkspace for why AgentRuntimeArchiveEnabled also gates
// this endpoint despite reading from in-memory state (RF-057, ID-005).
func (r *Runtime) ReportsChannelsByWorkspace(workspaceID string, limit int) (ReportChannels, error) {
	if err := r.requireEnabled(); err != nil {
		return ReportChannels{}, err
	}
	if !r.opts.AgentRuntimeArchiveEnabled {
		return ReportChannels{}, fmt.Errorf("agent runtime archive report is disabled")
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

func (r *Runtime) Reload() AgentRuntimeStatus {
	if r == nil {
		return AgentRuntimeStatus{}
	}
	r.mu.Lock()
	r.version++
	r.lastReload = r.nowFn().UTC()
	r.stateVersion++
	r.mu.Unlock()
	r.persistSnapshot()
	return r.Status()
}

func (r *Runtime) Restart() AgentRuntimeStatus {
	if r == nil {
		return AgentRuntimeStatus{}
	}
	r.mu.Lock()
	for _, state := range r.runs {
		if state.run.Status == RunStatusAccepted || state.run.Status == RunStatusRunning {
			if state.cancel != nil {
				state.cancel()
			}
			now := r.nowFn().UTC().Format(time.RFC3339)
			state.run.Status = RunStatusCanceled
			state.run.Error = "canceled by agent runtime restart"
			state.run.CompletedAt = now
			state.run.UpdatedAt = now
			r.closeRunDoneLocked(state)
		}
	}
	r.trimRunHistoryLocked()
	r.version++
	r.lastRestart = r.nowFn().UTC()
	r.stateVersion++
	r.mu.Unlock()
	r.persistSnapshot()
	return r.Status()
}
