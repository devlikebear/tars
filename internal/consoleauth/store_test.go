package consoleauth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreSetAndVerifyPasswords(t *testing.T) {
	now := time.Date(2026, 5, 6, 9, 0, 0, 0, time.UTC)
	store := NewStore(t.TempDir(), WithNow(func() time.Time { return now }))

	if err := store.SetPassword(RoleAdmin, "correct horse battery staple"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	ok, err := store.VerifyPassword(RoleAdmin, "correct horse battery staple")
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Fatalf("expected admin password to verify")
	}
	ok, err = store.VerifyPassword(RoleAdmin, "wrong password")
	if err != nil {
		t.Fatalf("VerifyPassword wrong: %v", err)
	}
	if ok {
		t.Fatalf("wrong password must not verify")
	}
	ok, err = store.HasPassword(RoleUser)
	if err != nil {
		t.Fatalf("HasPassword user: %v", err)
	}
	if ok {
		t.Fatalf("user password should not be configured")
	}

	raw, err := os.ReadFile(filepath.Join(store.authDir(), "users.json"))
	if err != nil {
		t.Fatalf("read users.json: %v", err)
	}
	var users usersFile
	if err := json.Unmarshal(raw, &users); err != nil {
		t.Fatalf("decode users.json: %v", err)
	}
	if users.Admin == nil || !strings.HasPrefix(users.Admin.Hash, "$argon2id$") {
		t.Fatalf("expected argon2id admin hash, got %+v", users.Admin)
	}
	if !users.Admin.CreatedAt.Equal(now) {
		t.Fatalf("expected created_at %s, got %s", now, users.Admin.CreatedAt)
	}
}

func TestStorePasswordChangeRevokesRoleSessions(t *testing.T) {
	now := time.Date(2026, 5, 6, 9, 0, 0, 0, time.UTC)
	store := NewStore(t.TempDir(), WithNow(func() time.Time { return now }))
	if err := store.SetPassword(RoleAdmin, "old admin password"); err != nil {
		t.Fatalf("set admin password: %v", err)
	}
	if err := store.SetPassword(RoleUser, "user password"); err != nil {
		t.Fatalf("set user password: %v", err)
	}
	adminSession, err := store.CreateSession(RoleAdmin, "admin browser", time.Hour)
	if err != nil {
		t.Fatalf("create admin session: %v", err)
	}
	userSession, err := store.CreateSession(RoleUser, "iphone safari", time.Hour)
	if err != nil {
		t.Fatalf("create user session: %v", err)
	}

	if err := store.SetPassword(RoleAdmin, "new admin password"); err != nil {
		t.Fatalf("change admin password: %v", err)
	}

	if _, ok, err := store.ValidateSession(adminSession.ID); err != nil || ok {
		t.Fatalf("expected admin session revoked, ok=%v err=%v", ok, err)
	}
	if got, ok, err := store.ValidateSession(userSession.ID); err != nil || !ok || got.Role != RoleUser {
		t.Fatalf("expected user session to survive, got=%+v ok=%v err=%v", got, ok, err)
	}
}

func TestStoreSessionLifecycle(t *testing.T) {
	now := time.Date(2026, 5, 6, 9, 0, 0, 0, time.UTC)
	store := NewStore(t.TempDir(), WithNow(func() time.Time { return now }))

	session, err := store.CreateSession(RoleUser, "iphone safari", time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if session.ID == "" {
		t.Fatalf("expected generated session id")
	}
	if session.Role != RoleUser || session.UserAgentHint != "iphone safari" {
		t.Fatalf("unexpected session: %+v", session)
	}
	if !session.ExpiresAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("expected expires_at %s, got %s", now.Add(time.Hour), session.ExpiresAt)
	}

	got, ok, err := store.ValidateSession(session.ID)
	if err != nil {
		t.Fatalf("ValidateSession: %v", err)
	}
	if !ok || got.ID != session.ID {
		t.Fatalf("expected active session, got=%+v ok=%v", got, ok)
	}

	store = NewStore(store.workspaceDir, WithNow(func() time.Time { return now.Add(2 * time.Hour) }))
	if _, ok, err := store.ValidateSession(session.ID); err != nil || ok {
		t.Fatalf("expected expired session to be invalid, ok=%v err=%v", ok, err)
	}
}

func TestStoreRevokeSession(t *testing.T) {
	store := NewStore(t.TempDir())
	session, err := store.CreateSession(RoleUser, "iphone safari", time.Hour)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if err := store.RevokeSession(session.ID); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}
	if _, ok, err := store.ValidateSession(session.ID); err != nil || ok {
		t.Fatalf("expected revoked session to be invalid, ok=%v err=%v", ok, err)
	}
}

func TestStorePairingCodeIsOneTimeAndExpires(t *testing.T) {
	now := time.Date(2026, 5, 6, 9, 0, 0, 0, time.UTC)
	store := NewStore(t.TempDir(), WithNow(func() time.Time { return now }))

	code, err := store.CreatePairingCode(RoleUser, 5*time.Minute)
	if err != nil {
		t.Fatalf("CreatePairingCode: %v", err)
	}
	if len(code.Code) != 6 {
		t.Fatalf("expected 6-digit pairing code, got %q", code.Code)
	}
	if code.Role != RoleUser || !code.ExpiresAt.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("unexpected pairing code: %+v", code)
	}

	used, ok, err := store.ConsumePairingCode(code.Code)
	if err != nil {
		t.Fatalf("ConsumePairingCode: %v", err)
	}
	if !ok || used.Code != code.Code || used.UsedAt == nil || !used.UsedAt.Equal(now) {
		t.Fatalf("expected pairing code to be consumed, used=%+v ok=%v", used, ok)
	}
	if _, ok, err := store.ConsumePairingCode(code.Code); err != nil || ok {
		t.Fatalf("expected consumed code to be one-time, ok=%v err=%v", ok, err)
	}

	expired, err := store.CreatePairingCode(RoleUser, time.Minute)
	if err != nil {
		t.Fatalf("CreatePairingCode expired seed: %v", err)
	}
	store = NewStore(store.workspaceDir, WithNow(func() time.Time { return now.Add(2 * time.Minute) }))
	if _, ok, err := store.ConsumePairingCode(expired.Code); err != nil || ok {
		t.Fatalf("expected expired code to be rejected, ok=%v err=%v", ok, err)
	}
}

func TestStoreRejectsInvalidRoles(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.SetPassword("owner", "password"); err == nil {
		t.Fatalf("expected SetPassword to reject invalid role")
	}
	if _, err := store.CreateSession("owner", "", time.Hour); err == nil {
		t.Fatalf("expected CreateSession to reject invalid role")
	}
	if _, err := store.CreatePairingCode("owner", time.Minute); err == nil {
		t.Fatalf("expected CreatePairingCode to reject invalid role")
	}
}
