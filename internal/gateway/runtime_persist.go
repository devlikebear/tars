package gateway

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

func (r *Runtime) persistenceEnabled() bool {
	return r != nil && r.opts.Enabled && r.opts.GatewayPersistenceEnabled
}

func (r *Runtime) restoreSnapshotOnStartup() {
	if r == nil || !r.persistenceEnabled() || !r.opts.GatewayRestoreOnStartup {
		return
	}
	var (
		runs     []Run
		channels map[string][]ChannelMessage
		errText  []string
	)
	if r.opts.GatewayRunsPersistenceEnabled {
		loadedRuns, err := r.persistStore.readRuns()
		if err != nil {
			errText = append(errText, err.Error())
		} else {
			runs = loadedRuns
		}
	}
	if r.opts.GatewayChannelsPersistenceEnabled {
		loadedChannels, err := r.persistStore.readChannels()
		if err != nil {
			errText = append(errText, err.Error())
		} else {
			channels = loadedChannels
		}
	}

	recoveredAt := r.nowFn().UTC().Format(time.RFC3339)
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(runs) > 0 {
		sort.SliceStable(runs, func(i, j int) bool {
			if runs[i].CreatedAt == runs[j].CreatedAt {
				return runs[i].ID < runs[j].ID
			}
			return runs[i].CreatedAt < runs[j].CreatedAt
		})
		runs = trimRuns(runs, r.opts.GatewayRunsMaxRecords)
		r.runs = make(map[string]*runState, len(runs))
		r.runOrder = make([]string, 0, len(runs))
		for _, item := range runs {
			run := item
			if strings.TrimSpace(run.ID) == "" {
				continue
			}
			run.WorkspaceID = normalizeWorkspaceID(run.WorkspaceID)
			if run.Status == RunStatusAccepted || run.Status == RunStatusRunning {
				run.Status = RunStatusCanceled
				run.Error = "canceled by restart recovery"
				if strings.TrimSpace(run.CompletedAt) == "" {
					run.CompletedAt = recoveredAt
				}
				run.UpdatedAt = recoveredAt
			}
			state := &runState{
				run:    run,
				done:   make(chan struct{}),
				closed: true,
			}
			close(state.done)
			r.runs[run.ID] = state
			r.runOrder = append(r.runOrder, run.ID)
			if seq := parseIDSequence(run.ID, "run_"); seq > 0 {
				current := r.runSeq.Load()
				if seq > current {
					r.runSeq.Store(seq)
				}
			}
		}
		r.runsRestored = len(r.runOrder)
	}

	if len(channels) > 0 {
		normalizedChannels := make(map[string][]ChannelMessage, len(channels))
		for rawKey, messages := range channels {
			workspaceIDFromKey, channelIDFromKey := splitWorkspaceChannelKey(rawKey)
			for _, item := range messages {
				msg := item
				msg.WorkspaceID = normalizeWorkspaceID(firstNonEmpty(msg.WorkspaceID, workspaceIDFromKey))
				msg.ChannelID = strings.TrimSpace(firstNonEmpty(msg.ChannelID, channelIDFromKey))
				if msg.ChannelID == "" {
					continue
				}
				internalKey := workspaceChannelKey(msg.WorkspaceID, msg.ChannelID)
				normalizedChannels[internalKey] = append(normalizedChannels[internalKey], msg)
			}
		}
		r.channelMsgs = trimChannels(normalizedChannels, r.opts.GatewayChannelsMaxMessagesPerChannel)
		for _, messages := range r.channelMsgs {
			for _, msg := range messages {
				if seq := parseIDSequence(msg.ID, "msg_"); seq > 0 {
					current := r.messageSeq.Load()
					if seq > current {
						r.messageSeq.Store(seq)
					}
				}
			}
		}
		r.channelsRestored = len(r.channelMsgs)
	}

	r.lastRestoreAt = r.nowFn().UTC()
	if len(errText) > 0 {
		r.lastRestoreError = strings.Join(errText, "; ")
	}
	r.stateVersion++
}

func splitWorkspaceChannelKey(value string) (workspaceID string, channelID string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return defaultWorkspaceID, ""
	}
	ws, ch, ok := strings.Cut(trimmed, ":")
	if !ok {
		return defaultWorkspaceID, trimmed
	}
	return normalizeWorkspaceID(ws), strings.TrimSpace(ch)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func parseIDSequence(value, prefix string) uint64 {
	if !strings.HasPrefix(value, prefix) {
		return 0
	}
	seq, err := strconv.ParseUint(strings.TrimPrefix(value, prefix), 10, 64)
	if err != nil {
		return 0
	}
	return seq
}

func (r *Runtime) snapshotForPersistence() ([]Run, map[string][]ChannelMessage, uint64) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	runs := make([]Run, 0, len(r.runOrder))
	for _, id := range r.runOrder {
		state := r.runs[id]
		if state == nil {
			continue
		}
		run := state.run
		run.WorkspaceID = normalizeWorkspaceID(run.WorkspaceID)
		runs = append(runs, run)
	}
	channels := trimChannels(r.channelMsgs, r.opts.GatewayChannelsMaxMessagesPerChannel)
	return runs, channels, r.stateVersion
}

func (r *Runtime) persistSnapshot() {
	if r == nil || !r.persistenceEnabled() {
		return
	}
	for attempt := 0; attempt < 2; attempt++ {
		runs, channels, snapshotVersion := r.snapshotForPersistence()
		runs = trimRuns(runs, r.opts.GatewayRunsMaxRecords)
		writeErr := ""
		if r.opts.GatewayRunsPersistenceEnabled {
			if err := r.persistStore.writeRuns(runs); err != nil {
				writeErr = err.Error()
			}
		}
		if writeErr == "" && r.opts.GatewayChannelsPersistenceEnabled {
			if err := r.persistStore.writeChannels(channels); err != nil {
				writeErr = err.Error()
			}
		}
		if writeErr == "" && r.opts.GatewayArchiveEnabled {
			_ = r.persistArchiveSnapshot(runs, channels)
		}
		r.mu.Lock()
		currentVersion := r.stateVersion
		if strings.TrimSpace(writeErr) != "" {
			r.mu.Unlock()
			return
		}
		if currentVersion == snapshotVersion || attempt == 1 {
			r.lastPersistAt = r.nowFn().UTC()
			r.mu.Unlock()
			return
		}
		r.mu.Unlock()
	}
}
