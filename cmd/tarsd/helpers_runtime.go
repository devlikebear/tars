package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devlikebear/tarsncase/internal/heartbeat"
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
	var mu sync.Mutex
	return func(ctx context.Context) (heartbeat.RunResult, error) {
		mu.Lock()
		defer mu.Unlock()
		callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		ranAt := nowFn().UTC()
		result, err := heartbeat.RunOnceWithLLMResultWithPolicy(callCtx, workspaceDir, ranAt, ask, policy)
		if state != nil {
			state.record(ranAt, result, err)
		}
		if emit != nil {
			if err != nil {
				evt := newNotificationEvent("heartbeat", "error", "Heartbeat failed", trimForMemory(err.Error(), 240))
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
		}
		return result, err
	}
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
