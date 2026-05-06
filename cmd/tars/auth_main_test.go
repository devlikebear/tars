package main

import (
	"io"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/consoleauth"
)

func TestRootCommand_IncludesAuthSubcommand(t *testing.T) {
	cmd := newRootCommand(strings.NewReader(""), io.Discard, io.Discard)
	if subcmd, _, err := cmd.Find([]string{"auth"}); err != nil || subcmd == nil || subcmd.Name() != "auth" {
		t.Fatalf("expected auth subcommand, got subcmd=%v err=%v", subcmd, err)
	}
}

func TestAuthInitCreatesAdminPasswordFromFlag(t *testing.T) {
	workspace := t.TempDir()
	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"auth", "init", "--workspace-dir", workspace, "--password", "admin secret"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth init: %v", err)
	}
	if !strings.Contains(stdout.String(), "admin account initialized") {
		t.Fatalf("expected init success output, got %q", stdout.String())
	}
	ok, err := consoleauth.NewStore(workspace).VerifyPassword(consoleauth.RoleAdmin, "admin secret")
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Fatalf("expected stored admin password to verify")
	}
}

func TestAuthInitUsesInitialAdminPasswordEnv(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("TARS_INITIAL_ADMIN_PASSWORD", "env admin secret")
	cmd := newRootCommand(strings.NewReader(""), io.Discard, io.Discard)
	cmd.SetArgs([]string{"auth", "init", "--workspace-dir", workspace})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth init: %v", err)
	}
	ok, err := consoleauth.NewStore(workspace).VerifyPassword(consoleauth.RoleAdmin, "env admin secret")
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Fatalf("expected env admin password to verify")
	}
}

func TestAuthPasswdChangesUserPassword(t *testing.T) {
	workspace := t.TempDir()
	cmd := newRootCommand(strings.NewReader(""), io.Discard, io.Discard)
	cmd.SetArgs([]string{"auth", "passwd", "user", "--workspace-dir", workspace, "--password", "user secret"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth passwd: %v", err)
	}
	ok, err := consoleauth.NewStore(workspace).VerifyPassword(consoleauth.RoleUser, "user secret")
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Fatalf("expected stored user password to verify")
	}
}

func TestAuthPairingCodeCreatesOneTimeUserCode(t *testing.T) {
	workspace := t.TempDir()
	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"auth", "pairing-code", "--workspace-dir", workspace, "--role", "user", "--ttl", "5m"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth pairing-code: %v", err)
	}
	code := regexp.MustCompile(`\b\d{6}\b`).FindString(stdout.String())
	if code == "" {
		t.Fatalf("expected 6-digit code in output, got %q", stdout.String())
	}
	used, ok, err := consoleauth.NewStore(workspace, consoleauth.WithNow(func() time.Time {
		return time.Now().UTC()
	})).ConsumePairingCode(code)
	if err != nil {
		t.Fatalf("ConsumePairingCode: %v", err)
	}
	if !ok || used.Role != consoleauth.RoleUser {
		t.Fatalf("expected user pairing code, got=%+v ok=%v", used, ok)
	}
}
