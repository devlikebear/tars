package tarsserver

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	notificationHistoryMax          = 1000
	defaultNotificationHistoryLimit = 100
	pulseNotificationCoalesceWindow = 30 * time.Minute
)

type notificationStore struct {
	mu               sync.Mutex
	path             string
	max              int
	nextID           int64
	items            []notificationEvent
	readCursorByRole map[string]int64
}

type notificationHistoryView struct {
	Items       []notificationEvent
	UnreadCount int
	ReadCursor  int64
	LastID      int64
}

type notificationReadView struct {
	ReadCursor  int64
	UnreadCount int
}

type notificationAppendResult struct {
	Event     notificationEvent
	Coalesced bool
}

func newNotificationStore(path string, max int) (*notificationStore, error) {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return nil, fmt.Errorf("notification store path is required")
	}
	if max <= 0 {
		max = notificationHistoryMax
	}
	store := &notificationStore{
		path:             trimmedPath,
		max:              max,
		items:            make([]notificationEvent, 0, max),
		readCursorByRole: map[string]int64{},
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *notificationStore) append(evt notificationEvent) (notificationAppendResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if merged, ok := s.coalescePulseNotificationLocked(evt); ok {
		if err := s.persist(); err != nil {
			return notificationAppendResult{}, err
		}
		return notificationAppendResult{Event: merged, Coalesced: true}, nil
	}

	s.nextID++
	evt.ID = s.nextID
	s.items = append(s.items, evt)
	if len(s.items) > s.max {
		s.items = s.items[len(s.items)-s.max:]
	}
	// Keep immediate persistence for crash-safe durability. If event volume grows,
	// this can be optimized with a debounced/batched flush strategy.
	if err := s.persist(); err != nil {
		return notificationAppendResult{}, err
	}
	return notificationAppendResult{Event: evt}, nil
}

func (s *notificationStore) coalescePulseNotificationLocked(evt notificationEvent) (notificationEvent, bool) {
	if !isPulseNotification(evt) {
		return notificationEvent{}, false
	}
	evtTime, ok := notificationEventTime(evt)
	if !ok {
		return notificationEvent{}, false
	}
	key := pulseNotificationCoalesceKey(evt)
	if key == "" {
		return notificationEvent{}, false
	}
	for i := len(s.items) - 1; i >= 0; i-- {
		current := s.items[i]
		if pulseNotificationCoalesceKey(current) != key {
			continue
		}
		currentTime, ok := notificationEventTime(current)
		if !ok {
			return notificationEvent{}, false
		}
		if evtTime.Before(currentTime) || evtTime.Sub(currentTime) > pulseNotificationCoalesceWindow {
			return notificationEvent{}, false
		}
		merged := current
		if merged.Occurrences <= 0 {
			merged.Occurrences = 1
		}
		merged.Occurrences++
		merged.Message = strings.TrimSpace(evt.Message)
		merged.Timestamp = strings.TrimSpace(evt.Timestamp)
		merged.LastSeen = strings.TrimSpace(evt.Timestamp)
		merged.Type = notificationEventType
		s.items[i] = merged
		if i != len(s.items)-1 {
			copy(s.items[i:], s.items[i+1:])
			s.items[len(s.items)-1] = merged
		}
		return merged, true
	}
	return notificationEvent{}, false
}

func (s *notificationStore) history(role string, limit int) (notificationHistoryView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 {
		limit = defaultNotificationHistoryLimit
	}
	if limit > s.max {
		limit = s.max
	}
	start := 0
	if len(s.items) > limit {
		start = len(s.items) - limit
	}
	items := append([]notificationEvent(nil), s.items[start:]...)
	normalizedRole := normalizeNotificationRoleKey(role)
	readCursor := s.readCursorByRole[normalizedRole]
	lastID := s.lastIDLocked()

	// unread_count is calculated over all retained notifications in the store,
	// not only over the paged `items` slice returned by `limit`.
	unread := 0
	for _, item := range s.items {
		if item.ID > readCursor {
			unread++
		}
	}
	return notificationHistoryView{
		Items:       items,
		UnreadCount: unread,
		ReadCursor:  readCursor,
		LastID:      lastID,
	}, nil
}

func (s *notificationStore) markRead(role string, lastID int64) (notificationReadView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalizedRole := normalizeNotificationRoleKey(role)
	current := s.readCursorByRole[normalizedRole]
	clamped := lastID
	if clamped < 0 {
		clamped = 0
	}
	last := s.lastIDLocked()
	if clamped > last {
		clamped = last
	}
	if clamped < current {
		clamped = current
	}
	s.readCursorByRole[normalizedRole] = clamped
	if err := s.persist(); err != nil {
		return notificationReadView{}, err
	}
	unread := 0
	for _, item := range s.items {
		if item.ID > clamped {
			unread++
		}
	}
	return notificationReadView{
		ReadCursor:  clamped,
		UnreadCount: unread,
	}, nil
}

func (s *notificationStore) load() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create notification store directory: %w", err)
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read notification store: %w", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil
	}
	var payload struct {
		NextID           int64               `json:"next_id"`
		Items            []notificationEvent `json:"items"`
		ReadCursorByRole map[string]int64    `json:"read_cursor_by_role"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("decode notification store: %w", err)
	}
	s.nextID = payload.NextID
	if payload.ReadCursorByRole != nil {
		for rawRole, cursor := range payload.ReadCursorByRole {
			s.readCursorByRole[normalizeNotificationRoleKey(rawRole)] = cursor
		}
	}
	for _, item := range payload.Items {
		s.items = append(s.items, item)
		if item.ID > s.nextID {
			s.nextID = item.ID
		}
	}
	if len(s.items) > s.max {
		s.items = s.items[len(s.items)-s.max:]
	}
	return nil
}

func (s *notificationStore) persist() error {
	payload := struct {
		NextID           int64               `json:"next_id"`
		Items            []notificationEvent `json:"items"`
		ReadCursorByRole map[string]int64    `json:"read_cursor_by_role"`
	}{
		NextID:           s.nextID,
		Items:            append([]notificationEvent(nil), s.items...),
		ReadCursorByRole: map[string]int64{},
	}
	for role, cursor := range s.readCursorByRole {
		payload.ReadCursorByRole[role] = cursor
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("encode notification store: %w", err)
	}
	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write notification store temp: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("replace notification store: %w", err)
	}
	return nil
}

func (s *notificationStore) lastIDLocked() int64 {
	var last int64
	for _, item := range s.items {
		if item.ID > last {
			last = item.ID
		}
	}
	return last
}

func isPulseNotification(evt notificationEvent) bool {
	return strings.EqualFold(strings.TrimSpace(evt.Category), "pulse")
}

func pulseNotificationCoalesceKey(evt notificationEvent) string {
	if !isPulseNotification(evt) {
		return ""
	}
	family := pulseNotificationFamilyKey(evt)
	parts := []string{
		strings.ToLower(strings.TrimSpace(evt.Category)),
		strings.ToLower(strings.TrimSpace(evt.Severity)),
		family,
		strings.TrimSpace(evt.SessionID),
		strings.TrimSpace(evt.JobID),
		strings.TrimSpace(evt.OpenPath),
	}
	if parts[2] == "" {
		return ""
	}
	return strings.Join(parts, "\x00")
}

func pulseNotificationFamilyKey(evt notificationEvent) string {
	title := strings.ToLower(strings.TrimSpace(evt.Title))
	message := strings.ToLower(strings.TrimSpace(evt.Message))
	text := title + " " + message
	if strings.Contains(text, "chat") {
		if strings.Contains(text, "stalled") ||
			strings.Contains(text, "halted") ||
			strings.Contains(text, "failed") ||
			strings.Contains(text, "auto-resume") ||
			strings.Contains(text, "auto-retry") {
			return "chat_attention"
		}
	}
	if strings.Contains(text, "cron") {
		return "cron"
	}
	if strings.Contains(text, "disk") {
		return "disk"
	}
	if strings.Contains(text, "telegram") || strings.Contains(text, "delivery") {
		return "delivery"
	}
	if strings.Contains(text, "reflection") {
		return "reflection"
	}
	if strings.Contains(text, "agent") || strings.Contains(text, "run") || strings.Contains(text, "stuck") {
		return "agent_runtime"
	}
	return title
}

func notificationEventTime(evt notificationEvent) (time.Time, bool) {
	raw := strings.TrimSpace(evt.LastSeen)
	if raw == "" {
		raw = strings.TrimSpace(evt.Timestamp)
	}
	if raw == "" {
		return time.Time{}, false
	}
	ts, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, false
	}
	return ts, true
}

func normalizeNotificationRoleKey(raw string) string {
	role := strings.TrimSpace(strings.ToLower(raw))
	switch role {
	case "user", "admin":
		return role
	default:
		return "anonymous"
	}
}
