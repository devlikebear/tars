package gateway

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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
	ID          string    `json:"run_id"`
	SessionID   string    `json:"session_id,omitempty"`
	Agent       string    `json:"agent,omitempty"`
	Prompt      string    `json:"prompt,omitempty"`
	Status      RunStatus `json:"status"`
	Accepted    bool      `json:"accepted"`
	Response    string    `json:"response,omitempty"`
	Error       string    `json:"error,omitempty"`
	CreatedAt   string    `json:"created_at"`
	StartedAt   string    `json:"started_at,omitempty"`
	CompletedAt string    `json:"completed_at,omitempty"`
	UpdatedAt   string    `json:"updated_at"`
}

type SpawnRequest struct {
	SessionID string
	Title     string
	Prompt    string
	Agent     string
}

type ChannelMessage struct {
	ID        string         `json:"id"`
	ChannelID string         `json:"channel_id"`
	ThreadID  string         `json:"thread_id,omitempty"`
	Direction string         `json:"direction"`
	Source    string         `json:"source"`
	Text      string         `json:"text"`
	Payload   map[string]any `json:"payload,omitempty"`
	Timestamp string         `json:"timestamp"`
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
	Enabled          bool         `json:"enabled"`
	Version          int64        `json:"version"`
	RunsTotal        int          `json:"runs_total"`
	RunsActive       int          `json:"runs_active"`
	ChannelsLocal    bool         `json:"channels_local_enabled"`
	ChannelsWebhook  bool         `json:"channels_webhook_enabled"`
	ChannelsTelegram bool         `json:"channels_telegram_enabled"`
	LastReloadAt     string       `json:"last_reload_at,omitempty"`
	LastRestartAt    string       `json:"last_restart_at,omitempty"`
	Browser          BrowserState `json:"browser"`
	Nodes            []NodeInfo   `json:"nodes"`
}

type RuntimeOptions struct {
	Enabled                 bool
	WorkspaceDir            string
	SessionStore            *session.Store
	RunPrompt               func(ctx context.Context, runLabel string, prompt string) (string, error)
	Executors               []AgentExecutor
	DefaultAgent            string
	ChannelsLocalEnabled    bool
	ChannelsWebhookEnabled  bool
	ChannelsTelegramEnabled bool
	Now                     func() time.Time
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

	mu           sync.RWMutex
	runs         map[string]*runState
	runOrder     []string
	closed       bool
	runSeq       atomic.Uint64
	messageSeq   atomic.Uint64
	channelMsgs  map[string][]ChannelMessage
	executors    map[string]AgentExecutor
	defaultAgent string
	browser      BrowserState
	version      int64
	lastReload   time.Time
	lastRestart  time.Time
	runWG        sync.WaitGroup
}

func NewRuntime(opts RuntimeOptions) *Runtime {
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	rt := &Runtime{
		opts:        opts,
		nowFn:       nowFn,
		runs:        map[string]*runState{},
		channelMsgs: map[string][]ChannelMessage{},
		executors:   map[string]AgentExecutor{},
		version:     1,
	}
	rt.initExecutors()
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
		}
		states = append(states, state)
	}
	r.mu.Unlock()

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
}

func (r *Runtime) SetExecutors(executors []AgentExecutor, defaultAgent string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.opts.Executors = append([]AgentExecutor(nil), executors...)
	r.opts.DefaultAgent = strings.TrimSpace(defaultAgent)
	r.applyExecutorsLocked(r.opts.Executors, r.opts.DefaultAgent)
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
				RunPrompt:   r.opts.RunPrompt,
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
		out = append(out, map[string]any{
			"name":        info.Name,
			"description": info.Description,
			"enabled":     r.opts.Enabled && info.Enabled,
			"kind":        info.Kind,
			"source":      info.Source,
			"entry":       info.Entry,
			"default":     info.Name == r.defaultAgent,
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
	if r.opts.SessionStore == nil {
		return Run{}, fmt.Errorf("session store is not configured")
	}
	selectedAgent, executor, err := r.resolveExecutor(req.Agent)
	if err != nil {
		return Run{}, err
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		title := strings.TrimSpace(req.Title)
		if title == "" {
			title = "chat"
		}
		s, err := r.opts.SessionStore.Create(title)
		if err != nil {
			return Run{}, fmt.Errorf("create session: %w", err)
		}
		sessionID = s.ID
	} else {
		if _, err := r.opts.SessionStore.Get(sessionID); err != nil {
			return Run{}, fmt.Errorf("get session: %w", err)
		}
	}

	now := r.nowFn().UTC()
	runID := fmt.Sprintf("run_%d", r.runSeq.Add(1))
	runCtx, cancel := context.WithCancel(context.Background())
	run := Run{
		ID:        runID,
		SessionID: sessionID,
		Agent:     selectedAgent,
		Prompt:    prompt,
		Status:    RunStatusAccepted,
		Accepted:  true,
		CreatedAt: now.Format(time.RFC3339),
		UpdatedAt: now.Format(time.RFC3339),
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
	r.mu.Unlock()

	go func() {
		defer r.runWG.Done()
		r.executeRun(runCtx, runID)
	}()
	return run, nil
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
	r.mu.Unlock()

	_ = r.appendSessionMessage(state.run.SessionID, "user", state.run.Prompt, r.nowFn().UTC())
	var (
		resp string
		err  error
	)
	if executor == nil {
		err = fmt.Errorf("agent executor is not configured")
	} else {
		resp, err = executor.Execute(ctx, ExecuteRequest{
			RunID:     state.run.ID,
			SessionID: state.run.SessionID,
			Prompt:    state.run.Prompt,
		})
	}
	if err == nil && ctx.Err() == nil {
		assistant := strings.TrimSpace(resp)
		if assistant != "" {
			_ = r.appendSessionMessage(state.run.SessionID, "assistant", assistant, r.nowFn().UTC())
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if state.run.Status == RunStatusCanceled {
		r.closeRunDoneLocked(state)
		return
	}
	finishedAt := r.nowFn().UTC().Format(time.RFC3339)
	state.run.CompletedAt = finishedAt
	state.run.UpdatedAt = finishedAt
	if err != nil {
		state.run.Status = RunStatusFailed
		state.run.Error = strings.TrimSpace(err.Error())
		r.closeRunDoneLocked(state)
		return
	}
	state.run.Status = RunStatusCompleted
	state.run.Response = strings.TrimSpace(resp)
	r.closeRunDoneLocked(state)
}

func (r *Runtime) appendSessionMessage(sessionID, role, content string, ts time.Time) error {
	if r == nil || r.opts.SessionStore == nil {
		return nil
	}
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(content) == "" {
		return nil
	}
	path := r.opts.SessionStore.TranscriptPath(sessionID)
	if err := session.AppendMessage(path, session.Message{Role: role, Content: content, Timestamp: ts.UTC()}); err != nil {
		return err
	}
	return r.opts.SessionStore.Touch(sessionID, ts.UTC())
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

func (r *Runtime) closeRunDoneLocked(state *runState) {
	if state.closed {
		return
	}
	close(state.done)
	state.closed = true
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

func (r *Runtime) Cancel(runID string) (Run, error) {
	if r == nil {
		return Run{}, fmt.Errorf("gateway runtime is disabled")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state, ok := r.runs[strings.TrimSpace(runID)]
	if !ok {
		return Run{}, fmt.Errorf("run not found: %s", strings.TrimSpace(runID))
	}
	if state.run.Status == RunStatusCompleted || state.run.Status == RunStatusFailed || state.run.Status == RunStatusCanceled {
		return state.run, nil
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
	return state.run, nil
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
		Enabled:          r.opts.Enabled,
		Version:          r.version,
		RunsTotal:        len(r.runs),
		RunsActive:       active,
		ChannelsLocal:    r.opts.ChannelsLocalEnabled,
		ChannelsWebhook:  r.opts.ChannelsWebhookEnabled,
		ChannelsTelegram: r.opts.ChannelsTelegramEnabled,
		Browser:          r.browser,
		Nodes:            defaultNodes(),
	}
	if !r.lastReload.IsZero() {
		status.LastReloadAt = r.lastReload.UTC().Format(time.RFC3339)
	}
	if !r.lastRestart.IsZero() {
		status.LastRestartAt = r.lastRestart.UTC().Format(time.RFC3339)
	}
	return status
}

func (r *Runtime) Reload() GatewayStatus {
	if r == nil {
		return GatewayStatus{}
	}
	r.mu.Lock()
	r.version++
	r.lastReload = r.nowFn().UTC()
	r.mu.Unlock()
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
	r.version++
	r.lastRestart = r.nowFn().UTC()
	r.mu.Unlock()
	return r.Status()
}

func (r *Runtime) MessageSend(channelID, threadID, text string) (ChannelMessage, error) {
	if r == nil || !r.opts.Enabled {
		return ChannelMessage{}, fmt.Errorf("gateway runtime is disabled")
	}
	if !r.opts.ChannelsLocalEnabled {
		return ChannelMessage{}, fmt.Errorf("local channels are disabled")
	}
	return r.appendChannelMessage(channelID, threadID, text, "outbound", "local", nil)
}

func (r *Runtime) MessageRead(channelID string, limit int) ([]ChannelMessage, error) {
	if r == nil || !r.opts.Enabled {
		return nil, fmt.Errorf("gateway runtime is disabled")
	}
	key := strings.TrimSpace(channelID)
	if key == "" {
		return nil, fmt.Errorf("channel_id is required")
	}
	if limit <= 0 {
		limit = 20
	}
	r.mu.RLock()
	items := append([]ChannelMessage(nil), r.channelMsgs[key]...)
	r.mu.RUnlock()
	if len(items) > limit {
		items = items[len(items)-limit:]
	}
	return items, nil
}

func (r *Runtime) ThreadReply(channelID, threadID, text string) (ChannelMessage, error) {
	if strings.TrimSpace(threadID) == "" {
		return ChannelMessage{}, fmt.Errorf("thread_id is required")
	}
	return r.MessageSend(channelID, threadID, text)
}

func (r *Runtime) InboundWebhook(channelID, threadID, text string, payload map[string]any) (ChannelMessage, error) {
	if r == nil || !r.opts.Enabled {
		return ChannelMessage{}, fmt.Errorf("gateway runtime is disabled")
	}
	if !r.opts.ChannelsWebhookEnabled {
		return ChannelMessage{}, fmt.Errorf("webhook channels are disabled")
	}
	return r.appendChannelMessage(channelID, threadID, text, "inbound", "webhook", payload)
}

func (r *Runtime) InboundTelegram(botID, threadID, text string, payload map[string]any) (ChannelMessage, error) {
	if r == nil || !r.opts.Enabled {
		return ChannelMessage{}, fmt.Errorf("gateway runtime is disabled")
	}
	if !r.opts.ChannelsTelegramEnabled {
		return ChannelMessage{}, fmt.Errorf("telegram channels are disabled")
	}
	channelID := strings.TrimSpace(botID)
	if channelID == "" {
		channelID = "telegram"
	}
	return r.appendChannelMessage(channelID, threadID, text, "inbound", "telegram", payload)
}

func (r *Runtime) appendChannelMessage(channelID, threadID, text, direction, source string, payload map[string]any) (ChannelMessage, error) {
	key := strings.TrimSpace(channelID)
	if key == "" {
		return ChannelMessage{}, fmt.Errorf("channel_id is required")
	}
	body := strings.TrimSpace(text)
	if body == "" {
		return ChannelMessage{}, fmt.Errorf("text is required")
	}
	now := r.nowFn().UTC()
	msg := ChannelMessage{
		ID:        fmt.Sprintf("msg_%d", r.messageSeq.Add(1)),
		ChannelID: key,
		ThreadID:  strings.TrimSpace(threadID),
		Direction: strings.TrimSpace(direction),
		Source:    strings.TrimSpace(source),
		Text:      body,
		Timestamp: now.Format(time.RFC3339),
	}
	if len(payload) > 0 {
		msg.Payload = payload
	}
	r.mu.Lock()
	r.channelMsgs[key] = append(r.channelMsgs[key], msg)
	r.mu.Unlock()
	return msg, nil
}

func (r *Runtime) BrowserStatus() BrowserState {
	if r == nil {
		return BrowserState{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.browser
}

func (r *Runtime) BrowserStart() BrowserState {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.browser.Running = true
	return r.browser
}

func (r *Runtime) BrowserStop() BrowserState {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.browser.Running = false
	return r.browser
}

func (r *Runtime) BrowserOpen(url string) (BrowserState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.browser.Running {
		return r.browser, fmt.Errorf("browser is not running")
	}
	url = strings.TrimSpace(url)
	if url == "" {
		return r.browser, fmt.Errorf("url is required")
	}
	r.browser.CurrentURL = url
	r.browser.LastAction = "open"
	return r.browser, nil
}

func (r *Runtime) BrowserSnapshot() (BrowserState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.browser.Running {
		return r.browser, fmt.Errorf("browser is not running")
	}
	if strings.TrimSpace(r.browser.CurrentURL) == "" {
		r.browser.LastSnapshot = "no page opened"
	} else {
		r.browser.LastSnapshot = fmt.Sprintf("snapshot captured for %s", r.browser.CurrentURL)
	}
	r.browser.LastAction = "snapshot"
	return r.browser, nil
}

func (r *Runtime) BrowserAct(action string, target string, value string) (BrowserState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.browser.Running {
		return r.browser, fmt.Errorf("browser is not running")
	}
	action = strings.TrimSpace(action)
	if action == "" {
		return r.browser, fmt.Errorf("action is required")
	}
	r.browser.LastAction = fmt.Sprintf("%s target=%s value=%s", action, strings.TrimSpace(target), strings.TrimSpace(value))
	return r.browser, nil
}

func (r *Runtime) BrowserScreenshot(name string) (BrowserState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.browser.Running {
		return r.browser, fmt.Errorf("browser is not running")
	}
	base := strings.TrimSpace(name)
	if base == "" {
		base = fmt.Sprintf("shot_%d.txt", r.nowFn().UnixNano())
	}
	file := base
	if r.opts.WorkspaceDir != "" {
		dir := filepath.Join(r.opts.WorkspaceDir, "_shared", "browser")
		if err := os.MkdirAll(dir, 0o755); err == nil {
			file = filepath.Join(dir, base)
			_ = os.WriteFile(file, []byte("browser screenshot placeholder\nurl="+r.browser.CurrentURL+"\n"), 0o644)
		}
	}
	r.browser.LastScreenshot = file
	r.browser.LastAction = "screenshot"
	return r.browser, nil
}

func (r *Runtime) Nodes() []NodeInfo {
	return defaultNodes()
}

func (r *Runtime) NodeDescribe(name string) (NodeInfo, error) {
	key := strings.TrimSpace(name)
	if key == "" {
		return NodeInfo{}, fmt.Errorf("name is required")
	}
	for _, node := range defaultNodes() {
		if node.Name == key {
			return node, nil
		}
	}
	return NodeInfo{}, fmt.Errorf("node not found: %s", key)
}

func (r *Runtime) NodeInvoke(name string, args map[string]any) (map[string]any, error) {
	key := strings.TrimSpace(name)
	switch key {
	case "echo":
		return map[string]any{"node": key, "output": args}, nil
	case "clock.now":
		return map[string]any{"node": key, "now": r.nowFn().UTC().Format(time.RFC3339)}, nil
	case "sessions.latest":
		if r.opts.SessionStore == nil {
			return nil, fmt.Errorf("session store is not configured")
		}
		latest, err := r.opts.SessionStore.Latest()
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"node":       key,
			"session_id": latest.ID,
			"title":      latest.Title,
			"updated_at": latest.UpdatedAt.UTC().Format(time.RFC3339),
		}, nil
	default:
		return nil, fmt.Errorf("node not found: %s", key)
	}
}

func defaultNodes() []NodeInfo {
	nodes := []NodeInfo{
		{Name: "echo", Description: "Return given input payload."},
		{Name: "clock.now", Description: "Return current UTC timestamp."},
		{Name: "sessions.latest", Description: "Return latest session metadata."},
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Name < nodes[j].Name
	})
	return nodes
}
