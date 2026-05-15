package tarsserver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/sessionoverride"
)

// TestEffectiveClaudeCodePermissionMode_OverrideWinsOverFallback verifies a
// session-cwd `.tars/settings.json` with claude_code_cli_permission_mode set
// is honored over the global fallback passed by the handler. This is the
// primary value of follow-up #4 — per-project permission policy without
// touching the global config.
func TestEffectiveClaudeCodePermissionMode_OverrideWinsOverFallback(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	cwd := sess.WorkDirs[0] // artifact dir is the default current cwd
	if err := os.MkdirAll(filepath.Join(cwd, ".tars"), 0o755); err != nil {
		t.Fatalf("mkdir .tars: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cwd, ".tars", "settings.json"), []byte(
		`{"claude_code_cli_permission_mode":"plan"}`,
	), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	svc := sessionoverride.NewService(store)
	reloaded, err := store.Get(sess.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	got := effectiveClaudeCodePermissionMode(svc, reloaded, "auto")
	if got != "plan" {
		t.Fatalf("expected session override 'plan' to win over fallback 'auto', got %q", got)
	}
}

// TestEffectiveClaudeCodePermissionMode_FallbackWhenNoOverride verifies the
// global config value (passed as fallback) is returned when no
// `.tars/settings*.json` set the field.
func TestEffectiveClaudeCodePermissionMode_FallbackWhenNoOverride(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	svc := sessionoverride.NewService(store)
	got := effectiveClaudeCodePermissionMode(svc, sess, "acceptEdits")
	if got != "acceptEdits" {
		t.Fatalf("expected fallback 'acceptEdits', got %q", got)
	}
}

// TestEffectiveClaudeCodePermissionMode_NilServiceUsesFallback verifies the
// helper is safe when no override service is configured (boot-time scenarios
// before session storage is fully wired).
func TestEffectiveClaudeCodePermissionMode_NilServiceUsesFallback(t *testing.T) {
	got := effectiveClaudeCodePermissionMode(nil, session.Session{}, "plan")
	if got != "plan" {
		t.Fatalf("expected fallback 'plan' when svc nil, got %q", got)
	}
}

// TestEffectiveClaudeCodePermissionMode_EmptyOverrideFallsBack verifies that
// an override file present but with an empty string value is treated as "not
// set" — the trimmed empty falls through to the fallback rather than
// silently clearing the permission mode. This matches the schema-level
// semantic where empty maps to "auto" in the provider, but at this layer we
// prefer the explicit caller fallback over an empty override.
func TestEffectiveClaudeCodePermissionMode_EmptyOverrideFallsBack(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	cwd := sess.WorkDirs[0]
	if err := os.MkdirAll(filepath.Join(cwd, ".tars"), 0o755); err != nil {
		t.Fatalf("mkdir .tars: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cwd, ".tars", "settings.json"), []byte(
		`{"claude_code_cli_permission_mode":"   "}`,
	), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	svc := sessionoverride.NewService(store)
	reloaded, err := store.Get(sess.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	got := effectiveClaudeCodePermissionMode(svc, reloaded, "acceptEdits")
	if got != "acceptEdits" {
		t.Fatalf("expected fallback when override is whitespace-only, got %q", got)
	}
}
