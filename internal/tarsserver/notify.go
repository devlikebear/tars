package tarsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/rs/zerolog"
)

const notificationEventType = "notification"
const keepaliveEventType = "keepalive"

type notificationEvent struct {
	ID        int64  `json:"id,omitempty"`
	Type      string `json:"type"`
	Category  string `json:"category"`
	Severity  string `json:"severity"`
	Title     string `json:"title"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
	JobID     string `json:"job_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	OpenPath  string `json:"open_path,omitempty"`
}

func newNotificationEvent(category, severity, title, message string) notificationEvent {
	return notificationEvent{
		Type:      notificationEventType,
		Category:  strings.TrimSpace(category),
		Severity:  strings.TrimSpace(severity),
		Title:     strings.TrimSpace(title),
		Message:   strings.TrimSpace(message),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}

type eventBroker struct {
	mu     sync.RWMutex
	nextID int
	subs   map[int]eventSubscription
}

type eventSubscription struct {
	ch chan notificationEvent
}

func newEventBroker() *eventBroker {
	return &eventBroker{
		subs: map[int]eventSubscription{},
	}
}

func (b *eventBroker) subscribe() (int, <-chan notificationEvent, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	id := b.nextID
	ch := make(chan notificationEvent, 32)
	b.subs[id] = eventSubscription{ch: ch}
	unsubscribe := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if current, ok := b.subs[id]; ok {
			delete(b.subs, id)
			close(current.ch)
		}
	}
	return id, ch, unsubscribe
}

func (b *eventBroker) publish(evt notificationEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, sub := range b.subs {
		select {
		case sub.ch <- evt:
		default:
			// Drop when consumer is too slow; this channel is best-effort realtime UI signal.
		}
	}
}

func (b *eventBroker) subscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs)
}

type desktopNotifier interface {
	Notify(ctx context.Context, evt notificationEvent) error
}

type commandNotifier struct {
	command string
	logger  zerolog.Logger
}

func newCommandNotifier(command string, logger zerolog.Logger) desktopNotifier {
	return &commandNotifier{
		command: strings.TrimSpace(command),
		logger:  logger,
	}
}

func (n *commandNotifier) Notify(ctx context.Context, evt notificationEvent) error {
	title := strings.TrimSpace(evt.Title)
	message := strings.TrimSpace(evt.Message)
	if title == "" || message == "" {
		return nil
	}
	if n.command != "" {
		cmd := exec.CommandContext(ctx, "sh", "-lc", n.command)
		cmd.Env = append(os.Environ(),
			"TARS_NOTIFY_TITLE="+title,
			"TARS_NOTIFY_MESSAGE="+message,
			"TARS_NOTIFY_CATEGORY="+strings.TrimSpace(evt.Category),
			"TARS_NOTIFY_SEVERITY="+strings.TrimSpace(evt.Severity),
			"TARS_NOTIFY_OPEN_PATH="+strings.TrimSpace(evt.OpenPath),
		)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("notify command failed: %w output=%q", err, strings.TrimSpace(string(output)))
		}
		return nil
	}
	return n.notifyAuto(ctx, evt)
}

func (n *commandNotifier) notifyAuto(ctx context.Context, evt notificationEvent) error {
	title := strings.TrimSpace(evt.Title)
	message := strings.TrimSpace(evt.Message)
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("terminal-notifier"); err == nil {
			return exec.CommandContext(ctx, "terminal-notifier", buildTerminalNotifierArgs(evt)...).Run()
		}
		if _, err := exec.LookPath("osascript"); err != nil {
			return err
		}
		script := fmt.Sprintf("display notification %q with title %q", message, title)
		return exec.CommandContext(ctx, "osascript", "-e", script).Run()
	case "linux":
		if _, err := exec.LookPath("notify-send"); err != nil {
			return err
		}
		return exec.CommandContext(ctx, "notify-send", title, message).Run()
	default:
		return fmt.Errorf("desktop notification is not supported on %s", runtime.GOOS)
	}
}

func buildTerminalNotifierArgs(evt notificationEvent) []string {
	args := []string{
		"-title", strings.TrimSpace(evt.Title),
		"-message", strings.TrimSpace(evt.Message),
		"-group", notificationGroupID(evt),
	}
	if openCommand := notificationOpenCommand(evt.OpenPath); openCommand != "" {
		args = append(args, "-execute", openCommand)
		return args
	}
	args = append(args, "-sender", "com.apple.Terminal")
	return args
}

func notificationGroupID(evt notificationEvent) string {
	category := sanitizeNotificationIDPart(evt.Category)
	if category == "" {
		category = "general"
	}
	jobID := sanitizeNotificationIDPart(evt.JobID)
	if jobID != "" {
		return "tars-" + category + "-" + jobID
	}
	return "tars-" + category
}

func sanitizeNotificationIDPart(raw string) string {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	if trimmed == "" {
		return ""
	}
	replacer := strings.NewReplacer(" ", "-", "/", "-", "_", "-", ":", "-", ".", "-")
	return replacer.Replace(trimmed)
}

func notificationOpenCommand(rawPath string) string {
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" {
		return ""
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return ""
	}
	escaped := strings.ReplaceAll(abs, "'", "'\\''")
	return "open '" + escaped + "'"
}

type notificationDispatcher struct {
	broker                  *eventBroker
	store                   *notificationStore
	notifier                desktopNotifier
	notifyWhenNoSubscribers bool
	logger                  zerolog.Logger
}

func newNotificationDispatcher(
	broker *eventBroker,
	notifier desktopNotifier,
	notifyWhenNoSubscribers bool,
	logger zerolog.Logger,
) *notificationDispatcher {
	return &notificationDispatcher{
		broker:                  broker,
		notifier:                notifier,
		notifyWhenNoSubscribers: notifyWhenNoSubscribers,
		logger:                  logger,
	}
}

func (d *notificationDispatcher) Emit(ctx context.Context, evt notificationEvent) {
	if d == nil {
		return
	}
	evt.Type = notificationEventType
	if strings.TrimSpace(evt.Timestamp) == "" {
		evt.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if d.store != nil {
		stored, err := d.store.append(evt)
		if err != nil {
			d.logger.Debug().Err(err).Msg("notification persistence failed; continuing without persistence")
		} else {
			evt = stored
		}
	}
	if d.broker != nil {
		d.broker.publish(evt)
	}
	if !d.notifyWhenNoSubscribers || d.notifier == nil {
		return
	}
	if d.broker != nil && d.broker.subscriberCount() > 0 && !strings.EqualFold(strings.TrimSpace(evt.Category), "cron") {
		return
	}
	notifyCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := d.notifier.Notify(notifyCtx, evt); err != nil {
		d.logger.Debug().Err(err).Str("title", evt.Title).Msg("desktop notification failed; retrying once")
		select {
		case <-notifyCtx.Done():
			d.logger.Debug().Err(notifyCtx.Err()).Str("title", evt.Title).Msg("desktop notification retry skipped")
			return
		case <-time.After(200 * time.Millisecond):
		}
		if retryErr := d.notifier.Notify(notifyCtx, evt); retryErr != nil {
			d.logger.Debug().Err(retryErr).Str("title", evt.Title).Msg("desktop notification skipped")
		}
	}
}

func newEventStreamHandler(broker *eventBroker, logger zerolog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if broker == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "event broker is not configured"})
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming is not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		_, ch, unsubscribe := broker.subscribe()
		defer unsubscribe()

		ping := time.NewTicker(10 * time.Second)
		defer ping.Stop()

		writeEvent := func(evt notificationEvent) error {
			payload, err := json.Marshal(evt)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
				return err
			}
			flusher.Flush()
			return nil
		}
		connected := newNotificationEvent("system", "info", "event stream connected", "subscribed to runtime notifications")
		_ = writeEvent(connected)

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ping.C:
				if _, err := fmt.Fprintf(w, "data: {\"type\":\"%s\"}\n\n", keepaliveEventType); err != nil {
					return
				}
				flusher.Flush()
			case evt, ok := <-ch:
				if !ok {
					return
				}
				if err := writeEvent(evt); err != nil {
					logger.Debug().Err(err).Msg("event stream write failed")
					return
				}
			}
		}
	})
}

func newEventsAPIHandler(broker *eventBroker, store *notificationStore, logger zerolog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/v1/events/stream", newEventStreamHandler(broker, logger))

	mux.HandleFunc("/v1/events/history", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if store == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "notification store is not configured"})
			return
		}
		limit := defaultNotificationHistoryLimit
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			v, err := strconv.Atoi(raw)
			if err != nil || v <= 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be a positive integer"})
				return
			}
			limit = v
		}
		role := normalizeNotificationRoleKey(serverauth.RoleFromRequest(r))
		view, err := store.history(role, limit)
		if err != nil {
			logger.Error().Err(err).Msg("load notification history failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "load notification history failed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items":        view.Items,
			"unread_count": view.UnreadCount,
			"read_cursor":  view.ReadCursor,
			"last_id":      view.LastID,
		})
	})

	mux.HandleFunc("/v1/events/read", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if store == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "notification store is not configured"})
			return
		}
		var req struct {
			LastID int64 `json:"last_id"`
		}
		if !decodeJSONBody(w, r, &req) {
			return
		}
		role := normalizeNotificationRoleKey(serverauth.RoleFromRequest(r))
		view, err := store.markRead(role, req.LastID)
		if err != nil {
			logger.Error().Err(err).Msg("mark notifications read failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "mark notifications read failed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"acknowledged": true,
			"read_cursor":  view.ReadCursor,
			"unread_count": view.UnreadCount,
		})
	})

	return mux
}
