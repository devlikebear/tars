package notification

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

	"github.com/devlikebear/tars/internal/httpapi"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/shellexec"
	"github.com/rs/zerolog"
)

const notificationEventType = "notification"
const keepaliveEventType = "keepalive"

// companionEventCategory marks an event meant for CASE's face and bubble
// (#1192), not for the general notification list. Emit never persists or
// desktop-notifies these — they only ever reach subscribers of the live
// broker (see Emit below). The event that actually sends one (initiative,
// #1000) is out of scope here; this is just the wire format and the
// dispatcher's special-case.
const companionEventCategory = "companion"

var notificationAppleScriptPath = "/usr/bin/osascript"

type Event struct {
	ID          int64  `json:"id,omitempty"`
	Type        string `json:"type"`
	Category    string `json:"category"`
	Severity    string `json:"severity"`
	Title       string `json:"title"`
	Message     string `json:"message"`
	Timestamp   string `json:"timestamp"`
	Occurrences int    `json:"occurrences,omitempty"`
	LastSeen    string `json:"last_seen,omitempty"`
	Coalesced   bool   `json:"coalesced,omitempty"`
	JobID       string `json:"job_id,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	OpenPath    string `json:"open_path,omitempty"`
	// RequestID names the permission request an "approval" notification is
	// about (see chat_activity.go).
	RequestID string `json:"request_id,omitempty"`
	// Expression names one of CASE's faces (lib/companion.ts
	// COMPANION_EXPRESSIONS) for a companionEventCategory event. The
	// console ignores the whole event when this is set to a value it does
	// not recognize (#1192).
	Expression string `json:"expression,omitempty"`
}

func NewEvent(category, severity, title, message string) Event {
	return Event{
		Type:      notificationEventType,
		Category:  strings.TrimSpace(category),
		Severity:  strings.TrimSpace(severity),
		Title:     strings.TrimSpace(title),
		Message:   strings.TrimSpace(message),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}

type Broker struct {
	mu     sync.RWMutex
	nextID int
	subs   map[int]eventSubscription
}

type eventSubscription struct {
	ch chan Event
}

func NewBroker() *Broker {
	return &Broker{
		subs: map[int]eventSubscription{},
	}
}

func (b *Broker) Subscribe() (int, <-chan Event, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	id := b.nextID
	ch := make(chan Event, 32)
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

func (b *Broker) Publish(evt Event) {
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

func (b *Broker) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs)
}

type DesktopNotifier interface {
	Notify(ctx context.Context, evt Event) error
}

type commandNotifier struct {
	command string
	logger  zerolog.Logger
}

func NewCommandNotifier(command string, logger zerolog.Logger) DesktopNotifier {
	return &commandNotifier{
		command: strings.TrimSpace(command),
		logger:  logger,
	}
}

func (n *commandNotifier) Notify(ctx context.Context, evt Event) error {
	title := strings.TrimSpace(evt.Title)
	message := strings.TrimSpace(evt.Message)
	if title == "" || message == "" {
		return nil
	}
	if n.command != "" {
		shell, err := shellexec.Executable()
		if err != nil {
			return fmt.Errorf("notify command: %w", err)
		}
		cmd := exec.CommandContext(ctx, shell, "-lc", n.command)
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

func (n *commandNotifier) notifyAuto(ctx context.Context, evt Event) error {
	return n.notifyAutoForGOOS(ctx, evt, runtime.GOOS)
}

func (n *commandNotifier) notifyAutoForGOOS(ctx context.Context, evt Event, goos string) error {
	title := strings.TrimSpace(evt.Title)
	message := strings.TrimSpace(evt.Message)
	switch goos {
	case "darwin":
		if notifierPath, err := exec.LookPath("terminal-notifier"); err == nil {
			return exec.CommandContext(ctx, notifierPath, buildTerminalNotifierArgs(evt)...).Run()
		}
		script := fmt.Sprintf("display notification %q with title %q", message, title)
		return exec.CommandContext(ctx, notificationAppleScriptPath, "-e", script).Run()
	case "linux":
		notifySendPath, err := exec.LookPath("notify-send")
		if err != nil {
			return err
		}
		return exec.CommandContext(ctx, notifySendPath, title, message).Run()
	default:
		return fmt.Errorf("desktop notification is not supported on %s", goos)
	}
}

func buildTerminalNotifierArgs(evt Event) []string {
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

func notificationGroupID(evt Event) string {
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

type Dispatcher struct {
	broker                  *Broker
	store                   *Store
	notifier                DesktopNotifier
	notifyWhenNoSubscribers bool
	logger                  zerolog.Logger
}

func NewDispatcher(
	broker *Broker,
	notifier DesktopNotifier,
	notifyWhenNoSubscribers bool,
	logger zerolog.Logger,
) *Dispatcher {
	return &Dispatcher{
		broker:                  broker,
		notifier:                notifier,
		notifyWhenNoSubscribers: notifyWhenNoSubscribers,
		logger:                  logger,
	}
}

// SetStore makes the dispatcher record every event it emits in store. Call
// it before the dispatcher is shared; it is not safe to call while Emit runs.
func (d *Dispatcher) SetStore(store *Store) {
	d.store = store
}

func (d *Dispatcher) Emit(ctx context.Context, evt Event) {
	if d == nil {
		return
	}
	evt.Type = notificationEventType
	if strings.TrimSpace(evt.Timestamp) == "" {
		evt.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	// A companion event (#1192) is CASE's live face/bubble, not something
	// to list in /v1/events/history, count toward unread, or push as a
	// desktop notification — it is not the general alert the rest of this
	// method is built for. It only ever reaches whoever is subscribed to
	// the broker right now.
	if strings.EqualFold(strings.TrimSpace(evt.Category), companionEventCategory) {
		if d.broker != nil {
			d.broker.Publish(evt)
		}
		return
	}
	coalesced := false
	if d.store != nil {
		stored, err := d.store.append(evt)
		if err != nil {
			d.logger.Debug().Err(err).Msg("notification persistence failed; continuing without persistence")
		} else {
			evt = stored.Event
			coalesced = stored.Coalesced
			evt.Coalesced = coalesced
		}
	}
	if d.broker != nil {
		d.broker.Publish(evt)
	}
	if coalesced {
		return
	}
	if !d.notifyWhenNoSubscribers || d.notifier == nil {
		return
	}
	if d.broker != nil && d.broker.SubscriberCount() > 0 && !strings.EqualFold(strings.TrimSpace(evt.Category), "cron") {
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

func newEventStreamHandler(broker *Broker, logger zerolog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !httpapi.RequireMethod(w, r, http.MethodGet) {
			return
		}
		if broker == nil {
			httpapi.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "event broker is not configured"})
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

		_, ch, unsubscribe := broker.Subscribe()
		defer unsubscribe()

		ping := time.NewTicker(10 * time.Second)
		defer ping.Stop()

		writeEvent := func(evt Event) error {
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
		connected := NewEvent("system", "info", "event stream connected", "subscribed to runtime notifications")
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

func NewEventsAPIHandler(broker *Broker, store *Store, logger zerolog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/v1/events/stream", newEventStreamHandler(broker, logger))

	mux.HandleFunc("/v1/events/history", func(w http.ResponseWriter, r *http.Request) {
		if !httpapi.RequireMethod(w, r, http.MethodGet) {
			return
		}
		if store == nil {
			httpapi.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "notification store is not configured"})
			return
		}
		limit := defaultNotificationHistoryLimit
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			v, err := strconv.Atoi(raw)
			if err != nil || v <= 0 {
				httpapi.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be a positive integer"})
				return
			}
			limit = v
		}
		role := normalizeNotificationRoleKey(serverauth.RoleFromRequest(r))
		view, err := store.history(role, limit)
		if err != nil {
			logger.Error().Err(err).Msg("load notification history failed")
			httpapi.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "load notification history failed"})
			return
		}
		httpapi.WriteJSON(w, http.StatusOK, map[string]any{
			"items":        view.Items,
			"unread_count": view.UnreadCount,
			"read_cursor":  view.ReadCursor,
			"last_id":      view.LastID,
		})
	})

	mux.HandleFunc("/v1/events/read", func(w http.ResponseWriter, r *http.Request) {
		if !httpapi.RequireMethod(w, r, http.MethodPost) {
			return
		}
		if store == nil {
			httpapi.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "notification store is not configured"})
			return
		}
		var req struct {
			LastID int64 `json:"last_id"`
		}
		if !httpapi.DecodeJSONBody(w, r, &req) {
			return
		}
		role := normalizeNotificationRoleKey(serverauth.RoleFromRequest(r))
		view, err := store.markRead(role, req.LastID)
		if err != nil {
			logger.Error().Err(err).Msg("mark notifications read failed")
			httpapi.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "mark notifications read failed"})
			return
		}
		httpapi.WriteJSON(w, http.StatusOK, map[string]any{
			"acknowledged": true,
			"read_cursor":  view.ReadCursor,
			"unread_count": view.UnreadCount,
		})
	})

	return mux
}
