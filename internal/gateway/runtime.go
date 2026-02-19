package gateway

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devlikebear/tarsncase/internal/serverauth"
	"github.com/devlikebear/tarsncase/internal/session"
)

type RunStatus string

const (
	RunStatusAccepted  RunStatus = "accepted"
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
	RunStatusCanceled  RunStatus = "canceled"
)

type Run struct {
	ID                 string    `json:"run_id"`
	WorkspaceID        string    `json:"workspace_id,omitempty"`
	SessionID          string    `json:"session_id,omitempty"`
	Agent              string    `json:"agent,omitempty"`
	Prompt             string    `json:"prompt,omitempty"`
	Status             RunStatus `json:"status"`
	Accepted           bool      `json:"accepted"`
	Response           string    `json:"response,omitempty"`
	Error              string    `json:"error,omitempty"`
	DiagnosticCode     string    `json:"diagnostic_code,omitempty"`
	DiagnosticReason   string    `json:"diagnostic_reason,omitempty"`
	PolicyBlockedTool  string    `json:"policy_blocked_tool,omitempty"`
	PolicyAllowedTools []string  `json:"policy_allowed_tools,omitempty"`
	CreatedAt          string    `json:"created_at"`
	StartedAt          string    `json:"started_at,omitempty"`
	CompletedAt        string    `json:"completed_at,omitempty"`
	UpdatedAt          string    `json:"updated_at"`
}

type SpawnRequest struct {
	WorkspaceID string
	SessionID   string
	Title       string
	Prompt      string
	Agent       string
}

type ChannelMessage struct {
	ID          string         `json:"id"`
	WorkspaceID string         `json:"workspace_id,omitempty"`
	ChannelID   string         `json:"channel_id"`
	ThreadID    string         `json:"thread_id,omitempty"`
	Direction   string         `json:"direction"`
	Source      string         `json:"source"`
	Text        string         `json:"text"`
	Payload     map[string]any `json:"payload,omitempty"`
	Timestamp   string         `json:"timestamp"`
}

type BrowserState struct {
	Running        bool   `json:"running"`
	CurrentURL     string `json:"current_url,omitempty"`
	LastSnapshot   string `json:"last_snapshot,omitempty"`
	LastAction     string `json:"last_action,omitempty"`
	LastScreenshot string `json:"last_screenshot,omitempty"`
}

type NodeInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type GatewayStatus struct {
	Enabled                    bool         `json:"enabled"`
	Version                    int64        `json:"version"`
	RunsTotal                  int          `json:"runs_total"`
	RunsActive                 int          `json:"runs_active"`
	AgentsCount                int          `json:"agents_count"`
	AgentsWatchEnabled         bool         `json:"agents_watch_enabled"`
	AgentsReloadVersion        int64        `json:"agents_reload_version"`
	AgentsLastReloadAt         string       `json:"agents_last_reload_at,omitempty"`
	ChannelsLocal              bool         `json:"channels_local_enabled"`
	ChannelsWebhook            bool         `json:"channels_webhook_enabled"`
	ChannelsTelegram           bool         `json:"channels_telegram_enabled"`
	PersistenceEnabled         bool         `json:"persistence_enabled"`
	RunsPersistenceEnabled     bool         `json:"runs_persistence_enabled"`
	ChannelsPersistenceEnabled bool         `json:"channels_persistence_enabled"`
	RestoreOnStartup           bool         `json:"restore_on_startup"`
	PersistenceDir             string       `json:"persistence_dir,omitempty"`
	RunsRestored               int          `json:"runs_restored"`
	ChannelsRestored           int          `json:"channels_restored"`
	LastPersistAt              string       `json:"last_persist_at,omitempty"`
	LastRestoreAt              string       `json:"last_restore_at,omitempty"`
	LastRestoreError           string       `json:"last_restore_error,omitempty"`
	LastReloadAt               string       `json:"last_reload_at,omitempty"`
	LastRestartAt              string       `json:"last_restart_at,omitempty"`
	Browser                    BrowserState `json:"browser"`
	Nodes                      []NodeInfo   `json:"nodes"`
}

type ReportSummary struct {
	GeneratedAt      string         `json:"generated_at"`
	SummaryEnabled   bool           `json:"summary_enabled"`
	ArchiveEnabled   bool           `json:"archive_enabled"`
	RunsTotal        int            `json:"runs_total"`
	RunsActive       int            `json:"runs_active"`
	RunsByStatus     map[string]int `json:"runs_by_status"`
	ChannelsTotal    int            `json:"channels_total"`
	MessagesTotal    int            `json:"messages_total"`
	MessagesBySource map[string]int `json:"messages_by_source"`
}

type ReportRuns struct {
	GeneratedAt    string `json:"generated_at"`
	ArchiveEnabled bool   `json:"archive_enabled"`
	Count          int    `json:"count"`
	Runs           []Run  `json:"runs"`
}

type ReportChannels struct {
	GeneratedAt    string                      `json:"generated_at"`
	ArchiveEnabled bool                        `json:"archive_enabled"`
	Count          int                         `json:"count"`
	Messages       map[string][]ChannelMessage `json:"messages"`
}

type RuntimeOptions struct {
	Enabled                              bool
	WorkspaceDir                         string
	SessionStore                         *session.Store
	SessionStoreForWorkspace             func(workspaceID string) *session.Store
	RunPrompt                            func(ctx context.Context, runLabel string, prompt string) (string, error)
	Executors                            []AgentExecutor
	DefaultAgent                         string
	GatewayAgentsWatchEnabled            bool
	ChannelsLocalEnabled                 bool
	ChannelsWebhookEnabled               bool
	ChannelsTelegramEnabled              bool
	GatewayPersistenceEnabled            bool
	GatewayRunsPersistenceEnabled        bool
	GatewayChannelsPersistenceEnabled    bool
	GatewayRunsMaxRecords                int
	GatewayChannelsMaxMessagesPerChannel int
	GatewayPersistenceDir                string
	GatewayRestoreOnStartup              bool
	GatewayReportSummaryEnabled          bool
	GatewayArchiveEnabled                bool
	GatewayArchiveDir                    string
	GatewayArchiveRetentionDays          int
	GatewayArchiveMaxFileBytes           int
	Now                                  func() time.Time
}

type runState struct {
	run      Run
	executor AgentExecutor
	cancel   context.CancelFunc
	done     chan struct{}
	closed   bool
}

type Runtime struct {
	opts RuntimeOptions

	nowFn func() time.Time

	mu                  sync.RWMutex
	runs                map[string]*runState
	runOrder            []string
	closed              bool
	runSeq              atomic.Uint64
	messageSeq          atomic.Uint64
	channelMsgs         map[string][]ChannelMessage
	executors           map[string]AgentExecutor
	defaultAgent        string
	agentsWatchEnabled  bool
	agentsReloadVersion int64
	agentsLastReload    time.Time
	browser             BrowserState
	version             int64
	lastReload          time.Time
	lastRestart         time.Time
	runWG               sync.WaitGroup
	stateVersion        uint64
	persistStore        snapshotStore
	lastPersistAt       time.Time
	lastRestoreAt       time.Time
	lastRestoreError    string
	runsRestored        int
	channelsRestored    int
}

const defaultWorkspaceID = "default"

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

func (r *Runtime) initExecutors() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.applyExecutorsLocked(r.opts.Executors, r.opts.DefaultAgent)
	r.markAgentsReloadLocked()
	r.stateVersion++
}

func (r *Runtime) SetExecutors(executors []AgentExecutor, defaultAgent string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.opts.Executors = append([]AgentExecutor(nil), executors...)
	r.opts.DefaultAgent = strings.TrimSpace(defaultAgent)
	r.applyExecutorsLocked(r.opts.Executors, r.opts.DefaultAgent)
	r.markAgentsReloadLocked()
	r.stateVersion++
	r.mu.Unlock()
	r.persistSnapshot()
}

func (r *Runtime) SetAgentsWatchEnabled(enabled bool) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.agentsWatchEnabled = enabled
	r.mu.Unlock()
}

func (r *Runtime) applyExecutorsLocked(executors []AgentExecutor, requestedDefault string) {
	r.executors = map[string]AgentExecutor{}
	r.defaultAgent = ""

	registered := false
	for _, executor := range executors {
		if r.registerExecutorLocked(executor) {
			registered = true
		}
	}
	if r.opts.RunPrompt != nil {
		if _, exists := r.executors["default"]; !exists {
			if ex, err := NewPromptExecutorWithOptions(PromptExecutorOptions{
				Name:        "default",
				Description: "Default in-process agent loop",
				Source:      "in-process",
				Entry:       "llm-loop",
				RunPrompt: func(ctx context.Context, runLabel string, prompt string, _ []string) (string, error) {
					return r.opts.RunPrompt(ctx, runLabel, prompt)
				},
			}); err == nil && ex != nil {
				r.executors["default"] = ex
				registered = true
			}
		}
	}

	requested := strings.TrimSpace(requestedDefault)
	if requested != "" {
		if _, ok := r.executors[requested]; ok {
			r.defaultAgent = requested
		}
	}
	if r.defaultAgent == "" {
		if _, ok := r.executors["default"]; ok {
			r.defaultAgent = "default"
		}
	}
	if r.defaultAgent == "" && registered {
		names := r.executorNamesLocked()
		if len(names) > 0 {
			r.defaultAgent = names[0]
		}
	}
}

func (r *Runtime) markAgentsReloadLocked() {
	r.agentsReloadVersion++
	r.agentsLastReload = r.nowFn().UTC()
}

func (r *Runtime) registerExecutorLocked(executor AgentExecutor) bool {
	if executor == nil {
		return false
	}
	info := executor.Info()
	name := strings.TrimSpace(info.Name)
	if name == "" {
		return false
	}
	r.executors[name] = executor
	return true
}

func (r *Runtime) executorNamesLocked() []string {
	names := make([]string, 0, len(r.executors))
	for name := range r.executors {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r *Runtime) resolveExecutor(agentName string) (string, AgentExecutor, error) {
	if r == nil {
		return "", nil, fmt.Errorf("gateway runtime is disabled")
	}
	requested := strings.TrimSpace(agentName)

	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.executors) == 0 {
		return "", nil, fmt.Errorf("no agent executors are configured")
	}
	if requested == "" {
		requested = strings.TrimSpace(r.defaultAgent)
	}
	if requested == "" {
		return "", nil, fmt.Errorf("default agent is not configured")
	}
	executor, ok := r.executors[requested]
	if ok {
		return requested, executor, nil
	}
	names := r.executorNamesLocked()
	return "", nil, fmt.Errorf("unknown agent %q (available: %s)", requested, strings.Join(names, ", "))
}

func (r *Runtime) Agents() []map[string]any {
	if r == nil {
		return []map[string]any{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := r.executorNamesLocked()
	out := make([]map[string]any, 0, len(names))
	for _, name := range names {
		executor := r.executors[name]
		if executor == nil {
			continue
		}
		info := executor.Info()
		toolsAllow := append([]string{}, info.ToolsAllow...)
		out = append(out, map[string]any{
			"name":                 info.Name,
			"description":          info.Description,
			"enabled":              r.opts.Enabled && info.Enabled,
			"kind":                 info.Kind,
			"source":               info.Source,
			"entry":                info.Entry,
			"default":              info.Name == r.defaultAgent,
			"policy_mode":          info.PolicyMode,
			"tools_allow":          toolsAllow,
			"tools_allow_count":    info.ToolsAllowCount,
			"tools_deny":           append([]string{}, info.ToolsDeny...),
			"tools_deny_count":     info.ToolsDenyCount,
			"tools_risk_max":       info.ToolsRiskMax,
			"tools_allow_groups":   append([]string{}, info.ToolsAllowGroups...),
			"tools_allow_patterns": append([]string{}, info.ToolsAllowPatterns...),
			"session_routing_mode": normalizeSessionRoutingMode(info.SessionRoutingMode),
			"session_fixed_id":     strings.TrimSpace(info.SessionFixedID),
		})
	}
	return out
}

func (r *Runtime) Spawn(ctx context.Context, req SpawnRequest) (Run, error) {
	if r == nil || !r.opts.Enabled {
		return Run{}, fmt.Errorf("gateway runtime is disabled")
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return Run{}, fmt.Errorf("prompt is required")
	}
	sessionStore := r.sessionStoreForWorkspace(req.WorkspaceID)
	if sessionStore == nil {
		return Run{}, fmt.Errorf("session store is not configured")
	}
	selectedAgent, executor, err := r.resolveExecutor(req.Agent)
	if err != nil {
		return Run{}, err
	}
	executorInfo := gatewayAgentInfo(executor)
	sessionID := strings.TrimSpace(req.SessionID)
	switch normalizeSessionRoutingMode(executorInfo.SessionRoutingMode) {
	case "new":
		sessionID = ""
	case "fixed":
		sessionID = strings.TrimSpace(executorInfo.SessionFixedID)
		if sessionID == "" {
			return Run{}, fmt.Errorf("agent %q is configured with fixed session routing but session_fixed_id is empty", selectedAgent)
		}
	}
	if sessionID == "" {
		title := strings.TrimSpace(req.Title)
		if title == "" {
			title = "chat"
		}
		s, err := sessionStore.Create(title)
		if err != nil {
			return Run{}, fmt.Errorf("create session: %w", err)
		}
		sessionID = s.ID
	} else {
		if _, err := sessionStore.Get(sessionID); err != nil {
			return Run{}, fmt.Errorf("get session: %w", err)
		}
	}

	now := r.nowFn().UTC()
	runID := fmt.Sprintf("run_%d", r.runSeq.Add(1))
	runCtx, cancel := context.WithCancel(context.Background())
	workspaceID := normalizeWorkspaceID(req.WorkspaceID)
	run := Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		SessionID:   sessionID,
		Agent:       selectedAgent,
		Prompt:      prompt,
		Status:      RunStatusAccepted,
		Accepted:    true,
		CreatedAt:   now.Format(time.RFC3339),
		UpdatedAt:   now.Format(time.RFC3339),
	}
	state := &runState{run: run, executor: executor, cancel: cancel, done: make(chan struct{})}

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		cancel()
		return Run{}, fmt.Errorf("gateway runtime is closed")
	}
	r.runs[runID] = state
	r.runOrder = append(r.runOrder, runID)
	r.runWG.Add(1)
	r.trimRunHistoryLocked()
	r.stateVersion++
	r.mu.Unlock()
	r.persistSnapshot()

	go func() {
		defer r.runWG.Done()
		r.executeRun(runCtx, runID)
	}()
	return run, nil
}

func gatewayAgentInfo(executor AgentExecutor) AgentInfo {
	if executor == nil {
		return AgentInfo{}
	}
	return executor.Info()
}

func (r *Runtime) executeRun(ctx context.Context, runID string) {
	state, ok := r.getRunState(runID)
	if !ok {
		return
	}

	r.mu.Lock()
	if state.run.Status == RunStatusCanceled {
		r.closeRunDoneLocked(state)
		r.mu.Unlock()
		return
	}
	executor := state.executor
	now := r.nowFn().UTC().Format(time.RFC3339)
	state.run.Status = RunStatusRunning
	state.run.StartedAt = now
	state.run.UpdatedAt = now
	r.stateVersion++
	r.mu.Unlock()
	r.persistSnapshot()

	_ = r.appendSessionMessage(state.run.WorkspaceID, state.run.SessionID, "user", state.run.Prompt, r.nowFn().UTC())
	var (
		resp string
		err  error
	)
	if executor == nil {
		err = fmt.Errorf("agent executor is not configured")
	} else {
		execCtx := serverauth.WithWorkspaceID(ctx, state.run.WorkspaceID)
		resp, err = executor.Execute(execCtx, ExecuteRequest{
			RunID:       state.run.ID,
			WorkspaceID: state.run.WorkspaceID,
			SessionID:   state.run.SessionID,
			Prompt:      state.run.Prompt,
		})
	}
	if err == nil && ctx.Err() == nil {
		assistant := strings.TrimSpace(resp)
		if assistant != "" {
			_ = r.appendSessionMessage(state.run.WorkspaceID, state.run.SessionID, "assistant", assistant, r.nowFn().UTC())
		}
	}

	r.mu.Lock()
	if state.run.Status == RunStatusCanceled {
		r.closeRunDoneLocked(state)
		r.trimRunHistoryLocked()
		r.stateVersion++
		r.mu.Unlock()
		r.persistSnapshot()
		return
	}
	finishedAt := r.nowFn().UTC().Format(time.RFC3339)
	state.run.CompletedAt = finishedAt
	state.run.UpdatedAt = finishedAt
	if err != nil {
		state.run.Status = RunStatusFailed
		state.run.Error = strings.TrimSpace(err.Error())
		state.run.DiagnosticCode, state.run.DiagnosticReason = classifyRunDiagnostic(err)
		if state.run.DiagnosticCode == "policy_tool_blocked" {
			state.run.PolicyBlockedTool = blockedToolNameFromReason(state.run.DiagnosticReason)
			info := gatewayAgentInfo(executor)
			if len(info.ToolsAllow) > 0 {
				state.run.PolicyAllowedTools = append([]string(nil), info.ToolsAllow...)
			}
		}
		r.closeRunDoneLocked(state)
		r.trimRunHistoryLocked()
		r.stateVersion++
		r.mu.Unlock()
		r.persistSnapshot()
		return
	}
	state.run.Status = RunStatusCompleted
	state.run.Response = strings.TrimSpace(resp)
	r.closeRunDoneLocked(state)
	r.trimRunHistoryLocked()
	r.stateVersion++
	r.mu.Unlock()
	r.persistSnapshot()
}

func blockedToolNameFromReason(reason string) string {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return ""
	}
	const prefix = "tool not injected for this request:"
	lower := strings.ToLower(trimmed)
	idx := strings.Index(lower, prefix)
	if idx == -1 {
		return ""
	}
	toolName := strings.TrimSpace(trimmed[idx+len(prefix):])
	if toolName == "" {
		return ""
	}
	return toolName
}

func (r *Runtime) appendSessionMessage(workspaceID, sessionID, role, content string, ts time.Time) error {
	sessionStore := r.sessionStoreForWorkspace(workspaceID)
	if r == nil || sessionStore == nil {
		return nil
	}
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(content) == "" {
		return nil
	}
	path := sessionStore.TranscriptPath(sessionID)
	if err := session.AppendMessage(path, session.Message{Role: role, Content: content, Timestamp: ts.UTC()}); err != nil {
		return err
	}
	return sessionStore.Touch(sessionID, ts.UTC())
}

func (r *Runtime) getRunState(runID string) (*runState, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	state, ok := r.runs[strings.TrimSpace(runID)]
	return state, ok
}

func (r *Runtime) sessionStoreForWorkspace(workspaceID string) *session.Store {
	if r == nil {
		return nil
	}
	if r.opts.SessionStoreForWorkspace != nil {
		if resolved := r.opts.SessionStoreForWorkspace(normalizeWorkspaceID(workspaceID)); resolved != nil {
			return resolved
		}
	}
	return r.opts.SessionStore
}

func (r *Runtime) closeRunDoneLocked(state *runState) {
	if state.closed {
		return
	}
	close(state.done)
	state.closed = true
}

func (r *Runtime) trimRunHistoryLocked() {
	max := r.opts.GatewayRunsMaxRecords
	if max <= 0 {
		return
	}
	for len(r.runOrder) > max {
		id := r.runOrder[0]
		state := r.runs[id]
		if state != nil {
			if state.run.Status == RunStatusAccepted || state.run.Status == RunStatusRunning {
				return
			}
		}
		delete(r.runs, id)
		r.runOrder = r.runOrder[1:]
	}
}

func (r *Runtime) Wait(ctx context.Context, runID string) (Run, error) {
	state, ok := r.getRunState(runID)
	if !ok {
		return Run{}, fmt.Errorf("run not found: %s", strings.TrimSpace(runID))
	}
	select {
	case <-ctx.Done():
		return Run{}, ctx.Err()
	case <-state.done:
		return r.GetOrZero(runID), nil
	}
}

func (r *Runtime) GetOrZero(runID string) Run {
	run, _ := r.Get(runID)
	return run
}

func (r *Runtime) Get(runID string) (Run, bool) {
	if r == nil {
		return Run{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	state, ok := r.runs[strings.TrimSpace(runID)]
	if !ok {
		return Run{}, false
	}
	return state.run, true
}

func (r *Runtime) GetByWorkspace(workspaceID, runID string) (Run, bool) {
	if r == nil {
		return Run{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	state, ok := r.runs[strings.TrimSpace(runID)]
	if !ok {
		return Run{}, false
	}
	if normalizeWorkspaceID(state.run.WorkspaceID) != normalizeWorkspaceID(workspaceID) {
		return Run{}, false
	}
	return state.run, true
}

func (r *Runtime) List(limit int) []Run {
	if r == nil {
		return []Run{}
	}
	if limit <= 0 {
		limit = 50
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Run, 0, len(r.runOrder))
	for i := len(r.runOrder) - 1; i >= 0; i-- {
		id := r.runOrder[i]
		state := r.runs[id]
		if state == nil {
			continue
		}
		out = append(out, state.run)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (r *Runtime) ListByWorkspace(workspaceID string, limit int) []Run {
	if r == nil {
		return []Run{}
	}
	if limit <= 0 {
		limit = 50
	}
	targetWorkspaceID := normalizeWorkspaceID(workspaceID)
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Run, 0, len(r.runOrder))
	for i := len(r.runOrder) - 1; i >= 0; i-- {
		id := r.runOrder[i]
		state := r.runs[id]
		if state == nil {
			continue
		}
		if normalizeWorkspaceID(state.run.WorkspaceID) != targetWorkspaceID {
			continue
		}
		out = append(out, state.run)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (r *Runtime) Cancel(runID string) (Run, error) {
	if r == nil {
		return Run{}, fmt.Errorf("gateway runtime is disabled")
	}
	r.mu.Lock()
	state, ok := r.runs[strings.TrimSpace(runID)]
	if !ok {
		r.mu.Unlock()
		return Run{}, fmt.Errorf("run not found: %s", strings.TrimSpace(runID))
	}
	if state.run.Status == RunStatusCompleted || state.run.Status == RunStatusFailed || state.run.Status == RunStatusCanceled {
		run := state.run
		r.mu.Unlock()
		return run, nil
	}
	if state.cancel != nil {
		state.cancel()
	}
	now := r.nowFn().UTC().Format(time.RFC3339)
	state.run.Status = RunStatusCanceled
	state.run.Error = "canceled by user"
	state.run.CompletedAt = now
	state.run.UpdatedAt = now
	r.closeRunDoneLocked(state)
	r.trimRunHistoryLocked()
	r.stateVersion++
	run := state.run
	r.mu.Unlock()
	r.persistSnapshot()
	return run, nil
}

func (r *Runtime) CancelByWorkspace(workspaceID, runID string) (Run, error) {
	if r == nil {
		return Run{}, fmt.Errorf("gateway runtime is disabled")
	}
	targetWorkspaceID := normalizeWorkspaceID(workspaceID)
	r.mu.Lock()
	state, ok := r.runs[strings.TrimSpace(runID)]
	if !ok || normalizeWorkspaceID(state.run.WorkspaceID) != targetWorkspaceID {
		r.mu.Unlock()
		return Run{}, fmt.Errorf("run not found: %s", strings.TrimSpace(runID))
	}
	if state.run.Status == RunStatusCompleted || state.run.Status == RunStatusFailed || state.run.Status == RunStatusCanceled {
		run := state.run
		r.mu.Unlock()
		return run, nil
	}
	if state.cancel != nil {
		state.cancel()
	}
	now := r.nowFn().UTC().Format(time.RFC3339)
	state.run.Status = RunStatusCanceled
	state.run.Error = "canceled by user"
	state.run.CompletedAt = now
	state.run.UpdatedAt = now
	r.closeRunDoneLocked(state)
	r.trimRunHistoryLocked()
	r.stateVersion++
	run := state.run
	r.mu.Unlock()
	r.persistSnapshot()
	return run, nil
}

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

func classifyRunDiagnostic(err error) (string, string) {
	if err == nil {
		return "", ""
	}
	reason := strings.TrimSpace(err.Error())
	if reason == "" {
		return "", ""
	}
	lower := strings.ToLower(reason)
	switch {
	case strings.Contains(lower, "tool not injected for this request"):
		return "policy_tool_blocked", reason
	case strings.Contains(lower, "repeated tool call pattern"):
		return "agent_loop_guard", reason
	case strings.Contains(lower, "context canceled"), strings.Contains(lower, "canceled"):
		return "run_canceled", reason
	default:
		return "run_failed", reason
	}
}

func normalizeWorkspaceID(workspaceID string) string {
	trimmed := strings.TrimSpace(workspaceID)
	if trimmed == "" {
		return defaultWorkspaceID
	}
	return trimmed
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
	r.browser = BrowserState{}
	r.trimRunHistoryLocked()
	r.version++
	r.lastRestart = r.nowFn().UTC()
	r.stateVersion++
	r.mu.Unlock()
	r.persistSnapshot()
	return r.Status()
}
