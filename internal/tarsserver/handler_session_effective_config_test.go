package tarsserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/sessionoverride"
	"github.com/rs/zerolog"
)

func TestEffectiveConfigAPI_ReturnsBaseAndDiagnostics(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.SetPromptOverride(sess.ID, "base prompt"); err != nil {
		t.Fatalf("set prompt: %v", err)
	}

	// Drop a settings.json with a blocked field so we can verify diagnostics.
	cwd := sess.WorkDirs[0] // artifact dir is current cwd
	dir := filepath.Join(cwd, ".tars")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{
		"llm_providers": {"x": {"api_key": "secret"}},
		"prompt_override": "shared override"
	}`), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	// Re-fetch to make sure the SetPromptOverride wrote
	if got, err := store.Get(sess.ID); err != nil || got.PromptOverride != "base prompt" {
		t.Fatalf("base prompt didn't stick: %+v err=%v", got, err)
	}

	svc := sessionoverride.NewService(store)

	var emitCount int32
	notify := func(_ context.Context, _ notificationEvent) { atomic.AddInt32(&emitCount, 1) }

	handler := newSessionAPIHandlerFull(store, zerolog.New(io.Discard), nil, sessionStyleValues{}, notify, svc)

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/sessions/"+sess.ID+"/effective-config", nil)
	req.Header.Set("Tars-Debug-Auth-Role", "admin")
	req.RemoteAddr = "127.0.0.1:1"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var payload sessionoverride.Resolution
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Effective.PromptOverride != "shared override" {
		t.Fatalf("expected shared prompt, got %q", payload.Effective.PromptOverride)
	}
	if payload.Sources["prompt_override"] != sessionoverride.SourceShared {
		t.Fatalf("source should be shared")
	}
	if len(payload.Diagnostics) == 0 {
		t.Fatalf("expected diagnostics for llm_providers, got none")
	}
	// First request reports a change => emits SSE
	if atomic.LoadInt32(&emitCount) != 1 {
		t.Fatalf("expected 1 SSE emit on first resolve, got %d", atomic.LoadInt32(&emitCount))
	}
}

func TestEffectiveConfigAPI_NoServiceReturns503(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	handler := newSessionAPIHandler(store, zerolog.New(io.Discard))

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/sessions/"+sess.ID+"/effective-config", nil)
	req.Header.Set("Tars-Debug-Auth-Role", "admin")
	req.RemoteAddr = "127.0.0.1:1"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when service is nil, got %d", rec.Code)
	}
}

func TestEffectiveConfigAPI_CwdPutInvalidatesCache(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	extra := filepath.Join(root, "projects", "alpha")
	if err := os.MkdirAll(extra, 0o755); err != nil {
		t.Fatalf("mkdir extra: %v", err)
	}
	if err := store.SetWorkDirs(sess.ID, []string{extra}, ""); err != nil {
		t.Fatalf("set work dirs: %v", err)
	}

	svc := sessionoverride.NewService(store)
	handler := newSessionAPIHandlerFull(store, zerolog.New(io.Discard), nil, sessionStyleValues{}, nil, svc)

	// Initial resolve (base only) populates cache
	if _, _, err := svc.Resolve(sess.ID); err != nil {
		t.Fatalf("warm cache: %v", err)
	}

	// PUT a new cwd (the extra dir). Service should be invalidated.
	body := `{"current":"` + extra + `"}`
	req := httptest.NewRequest(http.MethodPut, "/v1/admin/sessions/"+sess.ID+"/cwd", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Tars-Debug-Auth-Role", "admin")
	req.RemoteAddr = "127.0.0.1:1"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cwd PUT expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	// Next Resolve must see the new cwd → changed=true
	res, changed, err := svc.Resolve(sess.ID)
	if err != nil {
		t.Fatalf("resolve after cwd change: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true after cwd transition invalidated cache")
	}
	if !strings.HasSuffix(res.Cwd, filepath.Join("projects", "alpha")) {
		t.Fatalf("resolution cwd mismatch: got %q want suffix projects/alpha", res.Cwd)
	}
}
