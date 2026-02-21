package tarsserver

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tarsncase/internal/config"
)

func TestTelegramPairingStore_PathFollowsGatewayPersistenceDir(t *testing.T) {
	cfg := config.Config{
		GatewayPersistenceDir: filepath.Join(t.TempDir(), "gateway-state"),
	}
	got := telegramPairingStorePath(cfg)
	want := filepath.Join(cfg.GatewayPersistenceDir, "telegram_pairings.json")
	if got != want {
		t.Fatalf("expected pairing store path %q, got %q", want, got)
	}
}

func TestTelegramPairingStore_IssueApproveAndRestore(t *testing.T) {
	now := time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)
	nowFn := func() time.Time { return now }
	path := filepath.Join(t.TempDir(), "telegram_pairings.json")

	store, err := newTelegramPairingStore(path, nowFn)
	if err != nil {
		t.Fatalf("newTelegramPairingStore: %v", err)
	}
	pairing, reused, err := store.issue(telegramPairingIdentity{
		UserID:   11,
		ChatID:   "101",
		Username: "alice",
	}, telegramPairingTTL)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if reused {
		t.Fatalf("first issue must not be reused")
	}
	if pairing.Code == "" {
		t.Fatalf("expected issued code")
	}

	approved, err := store.approve(pairing.Code)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.UserID != 11 || approved.ChatID != "101" {
		t.Fatalf("unexpected approved identity: %+v", approved)
	}
	if !store.isAllowed(11) {
		t.Fatalf("expected user to be allowed after approval")
	}

	if err := store.bindSession(11, "sess-1"); err != nil {
		t.Fatalf("bindSession: %v", err)
	}
	if got := store.sessionID(11); got != "sess-1" {
		t.Fatalf("expected session binding sess-1, got %q", got)
	}

	restored, err := newTelegramPairingStore(path, nowFn)
	if err != nil {
		t.Fatalf("restore newTelegramPairingStore: %v", err)
	}
	if !restored.isAllowed(11) {
		t.Fatalf("expected restored store to keep allowed user")
	}
	if got := restored.sessionID(11); got != "sess-1" {
		t.Fatalf("expected restored session binding sess-1, got %q", got)
	}
}

func TestTelegramPairingStore_LastUpdateIDRestore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "telegram_pairings.json")
	store, err := newTelegramPairingStore(path, nil)
	if err != nil {
		t.Fatalf("newTelegramPairingStore: %v", err)
	}
	if err := store.setLastUpdateID(19); err != nil {
		t.Fatalf("setLastUpdateID: %v", err)
	}
	restored, err := newTelegramPairingStore(path, nil)
	if err != nil {
		t.Fatalf("restore newTelegramPairingStore: %v", err)
	}
	if got := restored.lastUpdateIDValue(); got != 19 {
		t.Fatalf("expected restored last_update_id=19, got %d", got)
	}
}

func TestTelegramPairingStore_ResolveDefaultChatID(t *testing.T) {
	now := time.Date(2026, 2, 21, 12, 0, 0, 0, time.UTC)
	nowFn := func() time.Time { return now }
	path := filepath.Join(t.TempDir(), "telegram_pairings.json")
	store, err := newTelegramPairingStore(path, nowFn)
	if err != nil {
		t.Fatalf("newTelegramPairingStore: %v", err)
	}

	chatID, err := store.resolveDefaultChatID()
	if err != nil {
		t.Fatalf("resolveDefaultChatID without allowed users: %v", err)
	}
	if chatID != "" {
		t.Fatalf("expected empty default chat id, got %q", chatID)
	}

	issued, _, err := store.issue(telegramPairingIdentity{
		UserID:   11,
		ChatID:   "101",
		Username: "alice",
	}, telegramPairingTTL)
	if err != nil {
		t.Fatalf("issue first user: %v", err)
	}
	if _, err := store.approve(issued.Code); err != nil {
		t.Fatalf("approve first user: %v", err)
	}

	chatID, err = store.resolveDefaultChatID()
	if err != nil {
		t.Fatalf("resolveDefaultChatID one allowed user: %v", err)
	}
	if chatID != "101" {
		t.Fatalf("expected default chat id 101, got %q", chatID)
	}

	now = now.Add(1 * time.Minute)
	issued2, _, err := store.issue(telegramPairingIdentity{
		UserID:   22,
		ChatID:   "202",
		Username: "bob",
	}, telegramPairingTTL)
	if err != nil {
		t.Fatalf("issue second user: %v", err)
	}
	if _, err := store.approve(issued2.Code); err != nil {
		t.Fatalf("approve second user: %v", err)
	}

	chatID, err = store.resolveDefaultChatID()
	if err == nil || !strings.Contains(err.Error(), "multiple paired telegram chats") {
		t.Fatalf("expected multiple chat id error, got chat_id=%q err=%v", chatID, err)
	}
}
