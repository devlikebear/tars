package tarsserver

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tarsncase/internal/config"
)

const (
	telegramPollTimeout      = 30 * time.Second
	telegramPairingTTL       = 60 * time.Minute
	telegramPairingCodeLen   = 8
	telegramPairingStoreFile = "telegram_pairings.json"
)

var telegramPairingCodeAlphabet = []byte("ABCDEFGHJKLMNPQRSTUVWXYZ23456789")

type telegramPairingIdentity struct {
	UserID   int64
	ChatID   string
	Username string
}

type telegramPairingEntry struct {
	Code      string `json:"code"`
	UserID    int64  `json:"user_id"`
	ChatID    string `json:"chat_id"`
	Username  string `json:"username,omitempty"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
}

type telegramAllowedUser struct {
	UserID     int64  `json:"user_id"`
	ChatID     string `json:"chat_id"`
	Username   string `json:"username,omitempty"`
	ApprovedAt string `json:"approved_at"`
}

type telegramPairingSnapshot struct {
	DMPolicy       string                 `json:"dm_policy"`
	PollingEnabled bool                   `json:"polling_enabled"`
	Pending        []telegramPairingEntry `json:"pending"`
	Allowed        []telegramAllowedUser  `json:"allowed"`
}

type telegramPairingStore struct {
	mu            sync.Mutex
	path          string
	nowFn         func() time.Time
	pendingByCode map[string]telegramPairingEntry
	allowedByUser map[int64]telegramAllowedUser
	sessionByUser map[int64]string
	lastUpdateID  int64
}

func telegramPairingStorePath(cfg config.Config) string {
	return filepath.Join(strings.TrimSpace(cfg.GatewayPersistenceDir), telegramPairingStoreFile)
}

func newTelegramPairingStore(path string, nowFn func() time.Time) (*telegramPairingStore, error) {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return nil, fmt.Errorf("telegram pairing store path is required")
	}
	if nowFn == nil {
		nowFn = time.Now
	}
	store := &telegramPairingStore{
		path:          trimmedPath,
		nowFn:         nowFn,
		pendingByCode: map[string]telegramPairingEntry{},
		allowedByUser: map[int64]telegramAllowedUser{},
		sessionByUser: map[int64]string{},
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *telegramPairingStore) issue(identity telegramPairingIdentity, ttl time.Duration) (telegramPairingEntry, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if identity.UserID <= 0 {
		return telegramPairingEntry{}, false, fmt.Errorf("user_id is required")
	}
	chatID := strings.TrimSpace(identity.ChatID)
	if chatID == "" {
		return telegramPairingEntry{}, false, fmt.Errorf("chat_id is required")
	}
	if ttl <= 0 {
		ttl = telegramPairingTTL
	}
	now := s.nowFn().UTC()
	_ = s.pruneExpiredLocked(now)
	for _, pending := range s.pendingByCode {
		if pending.UserID == identity.UserID {
			return pending, true, nil
		}
	}
	code, err := s.newCodeLocked()
	if err != nil {
		return telegramPairingEntry{}, false, err
	}
	entry := telegramPairingEntry{
		Code:      code,
		UserID:    identity.UserID,
		ChatID:    chatID,
		Username:  strings.TrimSpace(identity.Username),
		CreatedAt: now.Format(time.RFC3339),
		ExpiresAt: now.Add(ttl).Format(time.RFC3339),
	}
	s.pendingByCode[entry.Code] = entry
	if err := s.persistLocked(); err != nil {
		return telegramPairingEntry{}, false, err
	}
	return entry, false, nil
}

func (s *telegramPairingStore) approve(code string) (telegramAllowedUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	normalizedCode := normalizeTelegramPairingCode(code)
	if normalizedCode == "" {
		return telegramAllowedUser{}, fmt.Errorf("pairing code is required")
	}
	now := s.nowFn().UTC()
	_ = s.pruneExpiredLocked(now)
	entry, ok := s.pendingByCode[normalizedCode]
	if !ok {
		return telegramAllowedUser{}, fmt.Errorf("pairing code not found")
	}
	delete(s.pendingByCode, normalizedCode)
	allowed := telegramAllowedUser{
		UserID:     entry.UserID,
		ChatID:     entry.ChatID,
		Username:   entry.Username,
		ApprovedAt: now.Format(time.RFC3339),
	}
	s.allowedByUser[allowed.UserID] = allowed
	if err := s.persistLocked(); err != nil {
		return telegramAllowedUser{}, err
	}
	return allowed, nil
}

func (s *telegramPairingStore) isAllowed(userID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if userID <= 0 {
		return false
	}
	_, ok := s.allowedByUser[userID]
	return ok
}

func (s *telegramPairingStore) bindSession(userID int64, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if userID <= 0 {
		return fmt.Errorf("user_id is required")
	}
	trimmedSessionID := strings.TrimSpace(sessionID)
	if trimmedSessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if s.sessionByUser[userID] == trimmedSessionID {
		return nil
	}
	s.sessionByUser[userID] = trimmedSessionID
	return s.persistLocked()
}

func (s *telegramPairingStore) sessionID(userID int64) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.TrimSpace(s.sessionByUser[userID])
}

func (s *telegramPairingStore) snapshot(dmPolicy string, pollingEnabled bool) telegramPairingSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.nowFn().UTC()
	changed := s.pruneExpiredLocked(now)
	if changed {
		_ = s.persistLocked()
	}
	pending := make([]telegramPairingEntry, 0, len(s.pendingByCode))
	for _, item := range s.pendingByCode {
		pending = append(pending, item)
	}
	sort.Slice(pending, func(i, j int) bool {
		return pending[i].CreatedAt < pending[j].CreatedAt
	})

	allowed := make([]telegramAllowedUser, 0, len(s.allowedByUser))
	for _, item := range s.allowedByUser {
		allowed = append(allowed, item)
	}
	sort.Slice(allowed, func(i, j int) bool {
		return allowed[i].ApprovedAt < allowed[j].ApprovedAt
	})
	return telegramPairingSnapshot{
		DMPolicy:       strings.TrimSpace(dmPolicy),
		PollingEnabled: pollingEnabled,
		Pending:        pending,
		Allowed:        allowed,
	}
}

func (s *telegramPairingStore) lastUpdateIDValue() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastUpdateID < 0 {
		return 0
	}
	return s.lastUpdateID
}

func (s *telegramPairingStore) setLastUpdateID(updateID int64) error {
	if updateID <= 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if updateID <= s.lastUpdateID {
		return nil
	}
	s.lastUpdateID = updateID
	return s.persistLocked()
}

func (s *telegramPairingStore) allowedIdentity(userID int64) (telegramAllowedUser, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.allowedByUser[userID]
	return item, ok
}

func (s *telegramPairingStore) resolveDefaultChatID() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.nowFn().UTC()
	if s.pruneExpiredLocked(now) {
		_ = s.persistLocked()
	}
	allowed := make([]telegramAllowedUser, 0, len(s.allowedByUser))
	for _, item := range s.allowedByUser {
		if strings.TrimSpace(item.ChatID) == "" {
			continue
		}
		allowed = append(allowed, item)
	}
	if len(allowed) == 0 {
		return "", nil
	}
	if len(allowed) == 1 {
		return strings.TrimSpace(allowed[0].ChatID), nil
	}
	sort.Slice(allowed, func(i, j int) bool {
		return strings.TrimSpace(allowed[i].ApprovedAt) > strings.TrimSpace(allowed[j].ApprovedAt)
	})
	return "", fmt.Errorf("chat_id is required: multiple paired telegram chats found")
}

func (s *telegramPairingStore) load() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create telegram pairing store directory: %w", err)
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read telegram pairing store: %w", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil
	}
	var payload struct {
		Pending       []telegramPairingEntry `json:"pending"`
		Allowed       []telegramAllowedUser  `json:"allowed"`
		SessionByUser map[string]string      `json:"session_by_user"`
		LastUpdateID  int64                  `json:"last_update_id"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("decode telegram pairing store: %w", err)
	}
	for _, item := range payload.Pending {
		code := normalizeTelegramPairingCode(item.Code)
		if code == "" || item.UserID <= 0 {
			continue
		}
		item.Code = code
		item.ChatID = strings.TrimSpace(item.ChatID)
		item.Username = strings.TrimSpace(item.Username)
		s.pendingByCode[code] = item
	}
	for _, item := range payload.Allowed {
		if item.UserID <= 0 {
			continue
		}
		item.ChatID = strings.TrimSpace(item.ChatID)
		item.Username = strings.TrimSpace(item.Username)
		s.allowedByUser[item.UserID] = item
	}
	for rawUserID, sessionID := range payload.SessionByUser {
		userID, err := strconv.ParseInt(strings.TrimSpace(rawUserID), 10, 64)
		if err != nil || userID <= 0 {
			continue
		}
		trimmedSessionID := strings.TrimSpace(sessionID)
		if trimmedSessionID == "" {
			continue
		}
		s.sessionByUser[userID] = trimmedSessionID
	}
	if payload.LastUpdateID > 0 {
		s.lastUpdateID = payload.LastUpdateID
	}
	now := s.nowFn().UTC()
	if s.pruneExpiredLocked(now) {
		return s.persistLocked()
	}
	return nil
}

func (s *telegramPairingStore) persistLocked() error {
	payload := struct {
		Pending       []telegramPairingEntry `json:"pending"`
		Allowed       []telegramAllowedUser  `json:"allowed"`
		SessionByUser map[string]string      `json:"session_by_user,omitempty"`
		LastUpdateID  int64                  `json:"last_update_id,omitempty"`
	}{
		Pending:       make([]telegramPairingEntry, 0, len(s.pendingByCode)),
		Allowed:       make([]telegramAllowedUser, 0, len(s.allowedByUser)),
		SessionByUser: map[string]string{},
		LastUpdateID:  s.lastUpdateID,
	}
	for _, item := range s.pendingByCode {
		payload.Pending = append(payload.Pending, item)
	}
	for _, item := range s.allowedByUser {
		payload.Allowed = append(payload.Allowed, item)
	}
	sort.Slice(payload.Pending, func(i, j int) bool {
		return payload.Pending[i].CreatedAt < payload.Pending[j].CreatedAt
	})
	sort.Slice(payload.Allowed, func(i, j int) bool {
		return payload.Allowed[i].ApprovedAt < payload.Allowed[j].ApprovedAt
	})
	for userID, sessionID := range s.sessionByUser {
		trimmedSessionID := strings.TrimSpace(sessionID)
		if trimmedSessionID == "" {
			continue
		}
		payload.SessionByUser[strconv.FormatInt(userID, 10)] = trimmedSessionID
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("encode telegram pairing store: %w", err)
	}
	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write telegram pairing store temp: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("replace telegram pairing store: %w", err)
	}
	return nil
}

func (s *telegramPairingStore) pruneExpiredLocked(now time.Time) bool {
	changed := false
	for code, pending := range s.pendingByCode {
		expiresAt := strings.TrimSpace(pending.ExpiresAt)
		if expiresAt == "" {
			continue
		}
		ts, err := time.Parse(time.RFC3339, expiresAt)
		if err != nil || ts.After(now) {
			continue
		}
		delete(s.pendingByCode, code)
		changed = true
	}
	return changed
}

func (s *telegramPairingStore) newCodeLocked() (string, error) {
	for i := 0; i < 16; i++ {
		code, err := generateTelegramPairingCode()
		if err != nil {
			return "", err
		}
		if _, exists := s.pendingByCode[code]; exists {
			continue
		}
		return code, nil
	}
	return "", fmt.Errorf("failed to generate unique pairing code")
}

func generateTelegramPairingCode() (string, error) {
	raw := make([]byte, telegramPairingCodeLen)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate pairing code: %w", err)
	}
	buf := make([]byte, telegramPairingCodeLen)
	for i := range raw {
		buf[i] = telegramPairingCodeAlphabet[int(raw[i])%len(telegramPairingCodeAlphabet)]
	}
	return string(buf), nil
}

func normalizeTelegramPairingCode(raw string) string {
	code := strings.ToUpper(strings.TrimSpace(raw))
	code = strings.ReplaceAll(code, "-", "")
	code = strings.ReplaceAll(code, " ", "")
	return code
}
