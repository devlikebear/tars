package main

import (
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/consoleauth"
)

func runAuthCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), err
}

func TestAuthPairingCodeCreatesFocusCode(t *testing.T) {
	workspace := t.TempDir()
	out, err := runAuthCLI(t, "auth", "pairing-code", "--workspace-dir", workspace, "--role", "focus")
	if err != nil {
		t.Fatalf("auth pairing-code --role focus: %v", err)
	}
	code := regexp.MustCompile(`\b\d{6}\b`).FindString(out)
	if code == "" || !strings.Contains(out, "focus") {
		t.Fatalf("expected a focus code in %q", out)
	}
	if _, ok, err := consoleauth.NewStore(workspace).ConsumePairingCodeForRole(code, consoleauth.RoleFocus); err != nil || !ok {
		t.Fatalf("the code must pair a device: ok=%v err=%v", ok, err)
	}
}

func TestAuthPairingCodeRejectsUnknownRole(t *testing.T) {
	if _, err := runAuthCLI(t, "auth", "pairing-code", "--workspace-dir", t.TempDir(), "--role", "robot"); err == nil || !strings.Contains(err.Error(), "focus") {
		t.Fatalf("expected an unsupported role error naming focus, got %v", err)
	}
}

func TestAuthDevicesListAndRevoke(t *testing.T) {
	workspace := t.TempDir()

	out, err := runAuthCLI(t, "auth", "devices", "--workspace-dir", workspace)
	if err != nil || !strings.Contains(out, "no paired devices") {
		t.Fatalf("empty list: out=%q err=%v", out, err)
	}

	device, _, err := consoleauth.NewStore(workspace).CreateDevice("Pocket phone", consoleauth.RoleFocus)
	if err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}
	out, err = runAuthCLI(t, "auth", "devices", "--workspace-dir", workspace)
	if err != nil || !strings.Contains(out, device.ID) || !strings.Contains(out, "Pocket phone") || !strings.Contains(out, "active") {
		t.Fatalf("list: out=%q err=%v", out, err)
	}

	out, err = runAuthCLI(t, "auth", "devices", "revoke", device.ID, "--workspace-dir", workspace)
	if err != nil || !strings.Contains(out, "revoked") {
		t.Fatalf("revoke: out=%q err=%v", out, err)
	}
	out, err = runAuthCLI(t, "auth", "devices", "--workspace-dir", workspace)
	if err != nil || !strings.Contains(out, "revoked ") {
		t.Fatalf("list after revoke: out=%q err=%v", out, err)
	}

	if _, err := runAuthCLI(t, "auth", "devices", "revoke", "missing", "--workspace-dir", workspace); err == nil || !strings.Contains(err.Error(), "no paired device") {
		t.Fatalf("revoke missing: %v", err)
	}
}
