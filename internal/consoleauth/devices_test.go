package consoleauth

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDeviceTokenLifecycle(t *testing.T) {
	now := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
	store := NewStore(t.TempDir(), WithNow(func() time.Time { return now }))

	device, token, err := store.CreateDevice("  my   iPhone ", RoleFocus)
	if err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}
	if device.Name != "my iPhone" || device.Role != RoleFocus || !device.CreatedAt.Equal(now) || device.ID == "" {
		t.Fatalf("unexpected device: %+v", device)
	}
	if token == "" || device.TokenHash == "" || strings.Contains(device.TokenHash, token) {
		t.Fatalf("token must be returned and only its hash kept: token=%q hash=%q", token, device.TokenHash)
	}

	raw, err := os.ReadFile(store.devicesPath())
	if err != nil {
		t.Fatalf("read devices file: %v", err)
	}
	if strings.Contains(string(raw), token) {
		t.Fatalf("devices file must not hold the token")
	}

	got, ok, err := store.ValidateDeviceToken(token)
	if err != nil || !ok || got.ID != device.ID {
		t.Fatalf("ValidateDeviceToken = %+v, %v, %v", got, ok, err)
	}
	if _, ok, err := store.ValidateDeviceToken(token + "x"); err != nil || ok {
		t.Fatalf("a wrong token must not validate: ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.ValidateDeviceToken("  "); err != nil || ok {
		t.Fatalf("an empty token must not validate: ok=%v err=%v", ok, err)
	}

	if err := store.RevokeDevice(device.ID); err != nil {
		t.Fatalf("RevokeDevice: %v", err)
	}
	if _, ok, err := store.ValidateDeviceToken(token); err != nil || ok {
		t.Fatalf("a revoked token must not validate: ok=%v err=%v", ok, err)
	}
	if err := store.RevokeDevice(device.ID); err != nil {
		t.Fatalf("revoking twice must not fail: %v", err)
	}
	if err := store.RevokeDevice("missing"); !errors.Is(err, ErrDeviceNotFound) {
		t.Fatalf("RevokeDevice(missing) = %v, want ErrDeviceNotFound", err)
	}

	devices, err := store.ListDevices()
	if err != nil || len(devices) != 1 || devices[0].Active() {
		t.Fatalf("ListDevices = %+v, %v", devices, err)
	}
}

func TestCreateDeviceRejectsBadInput(t *testing.T) {
	store := NewStore(t.TempDir())
	if _, _, err := store.CreateDevice("phone", RoleUser); err == nil {
		t.Fatalf("only the focus role may be a device")
	}
	if _, _, err := store.CreateDevice("   ", RoleFocus); err == nil {
		t.Fatalf("an empty name must be rejected")
	}
	if _, _, err := store.CreateDevice("bad\x00name", RoleFocus); err == nil {
		t.Fatalf("a name with control characters must be rejected")
	}
	device, _, err := store.CreateDevice(strings.Repeat("가", 100), RoleFocus)
	if err != nil {
		t.Fatalf("long name: %v", err)
	}
	if got := len([]rune(device.Name)); got != maxDeviceNameRune {
		t.Fatalf("name has %d runes, want %d", got, maxDeviceNameRune)
	}
}

func TestDeviceTokensAreDistinct(t *testing.T) {
	store := NewStore(t.TempDir())
	first, firstToken, err := store.CreateDevice("phone", RoleFocus)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, secondToken, err := store.CreateDevice("tablet", RoleFocus)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first.ID == second.ID || firstToken == secondToken {
		t.Fatalf("devices must get their own id and token")
	}
	if err := store.RevokeDevice(first.ID); err != nil {
		t.Fatalf("revoke first: %v", err)
	}
	if got, ok, err := store.ValidateDeviceToken(secondToken); err != nil || !ok || got.ID != second.ID {
		t.Fatalf("revoking one device must not revoke another: %+v %v %v", got, ok, err)
	}
}

func TestConsumePairingCodeForRole(t *testing.T) {
	now := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
	store := NewStore(t.TempDir(), WithNow(func() time.Time { return now }))

	focusCode, err := store.CreatePairingCode(RoleFocus, 10*time.Minute)
	if err != nil {
		t.Fatalf("CreatePairingCode(focus): %v", err)
	}
	if focusCode.Role != RoleFocus {
		t.Fatalf("pairing code role = %q", focusCode.Role)
	}
	userCode, err := store.CreatePairingCode(RoleUser, 10*time.Minute)
	if err != nil {
		t.Fatalf("CreatePairingCode(user): %v", err)
	}

	if _, ok, err := store.ConsumePairingCodeForRole(userCode.Code, RoleFocus); err != nil || ok {
		t.Fatalf("a user code must not pair a device: ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.ConsumePairingCode(userCode.Code); err != nil || !ok {
		t.Fatalf("the user code must still be usable after a wrong-role try: ok=%v err=%v", ok, err)
	}

	got, ok, err := store.ConsumePairingCodeForRole(focusCode.Code, RoleFocus)
	if err != nil || !ok || got.UsedAt == nil {
		t.Fatalf("ConsumePairingCodeForRole = %+v, %v, %v", got, ok, err)
	}
	if _, ok, err := store.ConsumePairingCodeForRole(focusCode.Code, RoleFocus); err != nil || ok {
		t.Fatalf("a code is single-use: ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.ConsumePairingCodeForRole("", RoleFocus); err != nil || ok {
		t.Fatalf("an empty code must not match: ok=%v err=%v", ok, err)
	}

	expired, err := store.CreatePairingCode(RoleFocus, time.Minute)
	if err != nil {
		t.Fatalf("CreatePairingCode(expired): %v", err)
	}
	now = now.Add(2 * time.Minute)
	if _, ok, err := store.ConsumePairingCodeForRole(expired.Code, RoleFocus); err != nil || ok {
		t.Fatalf("an expired code must not match: ok=%v err=%v", ok, err)
	}
}

func TestFocusRoleCannotHavePasswordOrSession(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.SetPassword(RoleFocus, "a long enough password"); err == nil {
		t.Fatalf("focus must not get a password")
	}
	if _, err := store.CreateSession(RoleFocus, "phone", time.Hour); err == nil {
		t.Fatalf("focus must not get a browser session")
	}
}
