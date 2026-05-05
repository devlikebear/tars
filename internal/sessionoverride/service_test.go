package sessionoverride

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/session"
)

func newServiceWithSession(t *testing.T) (*Service, session.Session, string) {
	t.Helper()
	root := t.TempDir()
	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := store.SetToolConfig(sess.ID, &session.SessionToolConfig{
		ToolsCustom:  true,
		ToolsEnabled: []string{"read_file"},
	}); err != nil {
		t.Fatalf("set tool config: %v", err)
	}
	if err := store.SetPromptOverride(sess.ID, "base prompt"); err != nil {
		t.Fatalf("set prompt: %v", err)
	}
	got, err := store.Get(sess.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	return NewService(store), got, got.WorkDirs[0] // artifact dir
}

func TestService_Resolve_BaseOnly(t *testing.T) {
	svc, sess, _ := newServiceWithSession(t)

	res, changed, err := svc.Resolve(sess.ID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !changed {
		t.Fatal("first resolve should report changed=true")
	}
	if res.Effective.PromptOverride != "base prompt" {
		t.Fatalf("prompt mismatch: %q", res.Effective.PromptOverride)
	}
	if !reflect.DeepEqual(res.Effective.ToolConfig.ToolsEnabled, []string{"read_file"}) {
		t.Fatalf("tools_enabled mismatch: %+v", res.Effective.ToolConfig)
	}
	if res.Sources["prompt_override"] != SourceBase {
		t.Fatalf("source should be base, got %q", res.Sources["prompt_override"])
	}
}

func TestService_Resolve_PicksUpSettingsFile(t *testing.T) {
	svc, sess, cwd := newServiceWithSession(t)

	if _, _, err := svc.Resolve(sess.ID); err != nil {
		t.Fatalf("first resolve: %v", err)
	}

	// drop a settings.json in the active cwd
	dir := filepath.Join(cwd, ".tars")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"),
		[]byte(`{"prompt_override":"shared override"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	res, changed, err := svc.Resolve(sess.ID)
	if err != nil {
		t.Fatalf("re-resolve: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true after new settings file appeared")
	}
	if res.Effective.PromptOverride != "shared override" {
		t.Fatalf("prompt should come from shared, got %q", res.Effective.PromptOverride)
	}
	if res.Sources["prompt_override"] != SourceShared {
		t.Fatalf("source should be shared")
	}
}

func TestService_Resolve_CachedWhenUnchanged(t *testing.T) {
	svc, sess, cwd := newServiceWithSession(t)
	dir := filepath.Join(cwd, ".tars")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	settingsPath := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"prompt_override":"v1"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, changed, _ := svc.Resolve(sess.ID); !changed {
		t.Fatal("first resolve should report changed")
	}
	if _, changed, _ := svc.Resolve(sess.ID); changed {
		t.Fatal("second resolve should be cached (changed=false)")
	}
}

func TestService_Resolve_ReloadsOnMtimeBump(t *testing.T) {
	svc, sess, cwd := newServiceWithSession(t)
	dir := filepath.Join(cwd, ".tars")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	settingsPath := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"prompt_override":"v1"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, _, err := svc.Resolve(sess.ID); err != nil {
		t.Fatalf("first resolve: %v", err)
	}

	// rewrite + nudge mtime
	if err := os.WriteFile(settingsPath, []byte(`{"prompt_override":"v2"}`), 0o600); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(settingsPath, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	res, changed, err := svc.Resolve(sess.ID)
	if err != nil {
		t.Fatalf("re-resolve: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true after mtime bump")
	}
	if res.Effective.PromptOverride != "v2" {
		t.Fatalf("prompt should be v2 after reload, got %q", res.Effective.PromptOverride)
	}
}

func TestService_Resolve_BrokenJSONReturnsError(t *testing.T) {
	svc, sess, cwd := newServiceWithSession(t)
	dir := filepath.Join(cwd, ".tars")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{not json`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, _, err := svc.Resolve(sess.ID); err == nil {
		t.Fatal("expected error from broken settings.json")
	}
}

func TestService_Invalidate_ForcesReload(t *testing.T) {
	svc, sess, _ := newServiceWithSession(t)
	if _, _, err := svc.Resolve(sess.ID); err != nil {
		t.Fatalf("first: %v", err)
	}
	svc.Invalidate(sess.ID)
	_, changed, err := svc.Resolve(sess.ID)
	if err != nil {
		t.Fatalf("post-invalidate: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true after invalidate")
	}
}

func TestService_Resolve_NilStore(t *testing.T) {
	svc := NewService(nil)
	if _, _, err := svc.Resolve("any"); err == nil {
		t.Fatal("expected error with nil store")
	}
}

func TestService_Resolve_UnknownSession(t *testing.T) {
	svc := NewService(session.NewStore(t.TempDir()))
	if _, _, err := svc.Resolve("does-not-exist"); err == nil {
		t.Fatal("expected error for unknown session")
	}
}
