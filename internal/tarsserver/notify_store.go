package tarsserver

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	notificationHistoryMax          = 1000
	defaultNotificationHistoryLimit = 100
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

func (s *notificationStore) append(evt notificationEvent) (notificationEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++
	evt.ID = s.nextID
	s.items = append(s.items, evt)
	if len(s.items) > s.max {
		s.items = s.items[len(s.items)-s.max:]
	}
	// Keep immediate persistence for crash-safe durability. If event volume grows,
	// this can be optimized with a debounced/batched flush strategy.
	if err := s.persist(); err != nil {
		return notificationEvent{}, err
	}
	return evt, nil
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
	if len(s.items) == 0 {
		return 0
	}
	return s.items[len(s.items)-1].ID
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
