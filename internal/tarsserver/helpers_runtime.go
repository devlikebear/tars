package tarsserver

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devlikebear/tarsncase/internal/heartbeat"
	"github.com/devlikebear/tarsncase/internal/memory"
	"github.com/devlikebear/tarsncase/internal/session"
	"github.com/devlikebear/tarsncase/internal/tool"
)

type runtimeActivity struct {
	chatInFlight atomic.Int64
}

type gatewayPromptRunner func(ctx context.Context, runLabel string, promptText string, allowedTools []string) (string, error)

func (a *runtimeActivity) beginChat() func() {
	if a == nil {
		return func() {}
	}
	a.chatInFlight.Add(1)
	return func() {
		a.chatInFlight.Add(-1)
	}
}

func (a *runtimeActivity) isChatBusy() bool {
	if a == nil {
		return false
	}
	return a.chatInFlight.Load() > 0
}

type heartbeatRuntimeState struct {
	mu        sync.RWMutex
	hasRun    bool
	lastRunAt time.Time
	lastErr   string
	last      heartbeat.RunResult
}

type heartbeatWorkspaceState struct {
	mu    sync.RWMutex
	items map[string]*heartbeatRuntimeState
}

func newHeartbeatWorkspaceState() *heartbeatWorkspaceState {
	return &heartbeatWorkspaceState{
		items: map[string]*heartbeatRuntimeState{},
	}
}

func (s *heartbeatWorkspaceState) getOrCreate(workspaceID string) *heartbeatRuntimeState {
	if s == nil {
		return nil
	}
	normalizedWorkspaceID := normalizeWorkspaceID(workspaceID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.items == nil {
		s.items = map[string]*heartbeatRuntimeState{}
	}
	existing, ok := s.items[normalizedWorkspaceID]
	if ok && existing != nil {
		return existing
	}
	created := &heartbeatRuntimeState{}
	s.items[normalizedWorkspaceID] = created
	return created
}

func (s *heartbeatWorkspaceState) get(workspaceID string) *heartbeatRuntimeState {
	if s == nil {
		return nil
	}
	normalizedWorkspaceID := normalizeWorkspaceID(workspaceID)
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.items[normalizedWorkspaceID]
}

func (s *heartbeatWorkspaceState) record(workspaceID string, ranAt time.Time, result heartbeat.RunResult, runErr error) {
	state := s.getOrCreate(workspaceID)
	if state == nil {
		return
	}
	state.record(ranAt, result, runErr)
}

func (s *heartbeatWorkspaceState) snapshot(
	workspaceID string,
	configured bool,
	activeHours, timezone string,
	chatBusy bool,
) tool.HeartbeatStatus {
	state := s.get(workspaceID)
	if state == nil {
		return tool.HeartbeatStatus{
			Configured:  configured,
			ActiveHours: strings.TrimSpace(activeHours),
			Timezone:    strings.TrimSpace(timezone),
			ChatBusy:    chatBusy,
		}
	}
	return state.snapshot(configured, activeHours, timezone, chatBusy)
}

func (s *heartbeatRuntimeState) record(ranAt time.Time, result heartbeat.RunResult, runErr error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hasRun = true
	s.lastRunAt = ranAt.UTC()
	s.last = result
	if runErr != nil {
		s.lastErr = strings.TrimSpace(runErr.Error())
	} else {
		s.lastErr = ""
	}
}

func (s *heartbeatRuntimeState) snapshot(configured bool, activeHours, timezone string, chatBusy bool) tool.HeartbeatStatus {
	status := tool.HeartbeatStatus{
		Configured:  configured,
		ActiveHours: strings.TrimSpace(activeHours),
		Timezone:    strings.TrimSpace(timezone),
		ChatBusy:    chatBusy,
	}
	if s == nil {
		return status
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.hasRun {
		return status
	}
	status.LastRunAt = s.lastRunAt.Format(time.RFC3339)
	status.LastSkipped = s.last.Skipped
	status.LastSkipReason = s.last.SkipReason
	status.LastLogged = s.last.Logged
	status.LastAcknowledged = s.last.Acknowledged
	status.LastResponse = s.last.Response
	status.LastError = s.lastErr
	return status
}

func newHeartbeatRunner(
	workspaceDir string,
	nowFn func() time.Time,
	ask heartbeat.AskFunc,
	policy heartbeat.Policy,
	state *heartbeatRuntimeState,
) func(ctx context.Context) (heartbeat.RunResult, error) {
	return newHeartbeatRunnerWithNotify(workspaceDir, nowFn, ask, policy, state, nil)
}

func newHeartbeatRunnerWithNotify(
	workspaceDir string,
	nowFn func() time.Time,
	ask heartbeat.AskFunc,
	policy heartbeat.Policy,
	state *heartbeatRuntimeState,
	emit func(ctx context.Context, evt notificationEvent),
) func(ctx context.Context) (heartbeat.RunResult, error) {
	return newSerializedSupervisorRunner(serializedSupervisorOptions[heartbeat.RunResult]{
		nowFn:   nowFn,
		timeout: 30 * time.Second,
		run: func(ctx context.Context, ranAt time.Time) (heartbeat.RunResult, error) {
			return heartbeat.RunOnceWithLLMResultWithPolicy(ctx, workspaceDir, ranAt, ask, policy)
		},
		record: func(ranAt time.Time, result heartbeat.RunResult, runErr error) {
			if state != nil {
				state.record(ranAt, result, runErr)
			}
		},
		emit: func(ctx context.Context, result heartbeat.RunResult, runErr error) {
			if emit == nil {
				return
			}
			if runErr != nil {
				evt := newNotificationEvent("heartbeat", "error", "Heartbeat failed", trimForMemory(runErr.Error(), 240))
				emit(ctx, evt)
			} else if result.Skipped {
				evt := newNotificationEvent("heartbeat", "info", "Heartbeat skipped", trimForMemory(result.SkipReason, 240))
				emit(ctx, evt)
			} else if result.Acknowledged {
				evt := newNotificationEvent("heartbeat", "info", "Heartbeat acknowledged", "no follow-up action")
				emit(ctx, evt)
			} else {
				evt := newNotificationEvent("heartbeat", "info", "Heartbeat action", trimForMemory(result.Response, 280))
				emit(ctx, evt)
			}
		},
	})
}

func newWorkspaceHeartbeatRunnerWithNotify(
	baseWorkspaceDir string,
	nowFn func() time.Time,
	ask heartbeat.AskFunc,
	policyForWorkspace func(workspaceID string) heartbeat.Policy,
	state *heartbeatWorkspaceState,
	emit func(ctx context.Context, evt notificationEvent),
) func(ctx context.Context) (heartbeat.RunResult, error) {
	return newSerializedSupervisorRunner(serializedSupervisorOptions[heartbeat.RunResult]{
		nowFn:   nowFn,
		timeout: 30 * time.Second,
		run: func(ctx context.Context, ranAt time.Time) (heartbeat.RunResult, error) {
			workspaceID := defaultWorkspaceID
			workspaceDir := resolveWorkspaceDir(baseWorkspaceDir, workspaceID)
			if err := memory.EnsureWorkspace(workspaceDir); err != nil {
				return heartbeat.RunResult{}, err
			}

			policy := heartbeat.Policy{}
			if policyForWorkspace != nil {
				policy = policyForWorkspace(workspaceID)
			}
			return heartbeat.RunOnceWithLLMResultWithPolicy(ctx, workspaceDir, ranAt, ask, policy)
		},
		record: func(ranAt time.Time, result heartbeat.RunResult, runErr error) {
			if state != nil {
				state.record(defaultWorkspaceID, ranAt, result, runErr)
			}
		},
		emit: func(ctx context.Context, result heartbeat.RunResult, runErr error) {
			if emit == nil {
				return
			}
			switch {
			case runErr != nil:
				evt := newNotificationEvent("heartbeat", "error", "Heartbeat failed", trimForMemory(runErr.Error(), 240))
				emit(ctx, evt)
			case result.Skipped:
				evt := newNotificationEvent("heartbeat", "info", "Heartbeat skipped", trimForMemory(result.SkipReason, 240))
				emit(ctx, evt)
			case result.Acknowledged:
				evt := newNotificationEvent("heartbeat", "info", "Heartbeat acknowledged", "no follow-up action")
				emit(ctx, evt)
			default:
				evt := newNotificationEvent("heartbeat", "info", "Heartbeat action", trimForMemory(result.Response, 280))
				emit(ctx, evt)
			}
		},
	})
}

func buildHeartbeatPolicy(
	store *session.Store,
	activeHours string,
	timezone string,
	activity *runtimeActivity,
) heartbeat.Policy {
	return heartbeat.Policy{
		ActiveHours: strings.TrimSpace(activeHours),
		Timezone:    strings.TrimSpace(timezone),
		ShouldRun: func(_ context.Context, _ time.Time) (bool, string) {
			if activity != nil && activity.isChatBusy() {
				return false, "chat queue is busy"
			}
			return true, ""
		},
		LoadSessionContext: func(_ context.Context, _ time.Time) (string, error) {
			return latestSessionContext(store, 6)
		},
		AppendSessionTurn: func(_ context.Context, turn heartbeat.TurnRecord) error {
			return appendHeartbeatTurnToLatestSession(store, turn)
		},
	}
}

func latestSessionContext(store *session.Store, maxMessages int) (string, error) {
	if store == nil {
		return "", nil
	}
	latest, err := store.Latest()
	if err != nil {
		if strings.Contains(err.Error(), "session not found") {
			return "", nil
		}
		return "", err
	}
	messages, err := session.ReadMessages(store.TranscriptPath(latest.ID))
	if err != nil {
		return "", err
	}
	if len(messages) == 0 {
		return "", nil
	}
	if maxMessages <= 0 {
		maxMessages = 6
	}
	start := len(messages) - maxMessages
	if start < 0 {
		start = 0
	}
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "session_id=%s\nsession_title=%s\n", latest.ID, latest.Title)
	for _, msg := range messages[start:] {
		_, _ = fmt.Fprintf(&b, "- [%s] %s\n", msg.Role, trimForMemory(msg.Content, 180))
	}
	return strings.TrimSpace(b.String()), nil
}

func appendHeartbeatTurnToLatestSession(store *session.Store, turn heartbeat.TurnRecord) error {
	if store == nil {
		return nil
	}
	latest, err := store.Latest()
	if err != nil {
		if strings.Contains(err.Error(), "session not found") {
			return nil
		}
		return err
	}
	if strings.TrimSpace(turn.Response) == "" {
		return nil
	}
	path := store.TranscriptPath(latest.ID)
	content := fmt.Sprintf(
		"[HEARTBEAT]\nprompt: %s\nresponse: %s",
		trimForMemory(turn.Prompt, 220),
		trimForMemory(turn.Response, 320),
	)
	if err := session.AppendMessage(path, session.Message{
		Role:      "system",
		Content:   content,
		Timestamp: turn.OccurredAt.UTC(),
	}); err != nil {
		return err
	}
	return store.Touch(latest.ID, turn.OccurredAt.UTC())
}
