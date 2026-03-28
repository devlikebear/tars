package tarsserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/project"
	"github.com/rs/zerolog"
)

type projectDashboardEvent struct {
	Type      string `json:"type"`
	ProjectID string `json:"project_id"`
	Kind      string `json:"kind"`
	Timestamp string `json:"timestamp"`
}

type projectDashboardBroker struct {
	mu     sync.RWMutex
	nextID int
	subs   map[int]chan projectDashboardEvent
}

func newProjectDashboardBroker() *projectDashboardBroker {
	return &projectDashboardBroker{subs: map[int]chan projectDashboardEvent{}}
}

func newProjectDashboardEvent(projectID, kind string) projectDashboardEvent {
	return projectDashboardEvent{
		Type:      "project_dashboard",
		ProjectID: strings.TrimSpace(projectID),
		Kind:      strings.TrimSpace(kind),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}

func (b *projectDashboardBroker) subscribe() (<-chan projectDashboardEvent, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	id := b.nextID
	ch := make(chan projectDashboardEvent, 32)
	b.subs[id] = ch
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if current, ok := b.subs[id]; ok {
			delete(b.subs, id)
			close(current)
		}
	}
}

func (b *projectDashboardBroker) publish(evt projectDashboardEvent) {
	if b == nil {
		return
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, sub := range b.subs {
		select {
		case sub <- evt:
		default:
		}
	}
}

func newProjectDashboardHandler(_ *project.Store, _ project.PhaseEngine, broker *projectDashboardBroker, logger zerolog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		projectID, ok := parseProjectDashboardStreamPath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		serveProjectDashboardStream(w, r, projectID, broker, logger)
	})
}

func serveProjectDashboardStream(w http.ResponseWriter, r *http.Request, projectID string, broker *projectDashboardBroker, logger zerolog.Logger) {
	if broker == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "dashboard broker is not configured"})
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

	ch, unsubscribe := broker.subscribe()
	defer unsubscribe()

	writeEvent := func(evt projectDashboardEvent) error {
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
	_ = writeEvent(newProjectDashboardEvent(projectID, "connected"))

	ping := time.NewTicker(10 * time.Second)
	defer ping.Stop()

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
			if evt.ProjectID != projectID {
				continue
			}
			if err := writeEvent(evt); err != nil {
				logger.Debug().Err(err).Msg("dashboard stream write failed")
				return
			}
		}
	}
}

func parseProjectDashboardStreamPath(path string) (string, bool) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(path, "/ui/projects/"))
	if trimmed == "" {
		return "", false
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 2 && strings.TrimSpace(parts[1]) == "stream" {
		projectID := strings.TrimSpace(parts[0])
		if projectID == "" {
			return "", false
		}
		return projectID, true
	}
	return "", false
}
