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
	"github.com/rs/zerolog"
)

func TestSessionCwdAPI_GetReturnsCurrentAndEligible(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	extra := filepath.Join(root, "projects", "alpha")
	if err := os.MkdirAll(extra, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := store.SetWorkDirs(sess.ID, []string{extra}, extra); err != nil {
		t.Fatalf("set work dirs: %v", err)
	}

	handler := newSessionAPIHandler(store, zerolog.New(io.Discard))

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/sessions/"+sess.ID+"/cwd", nil)
	req.RemoteAddr = "127.0.0.1:1"
	req.Header.Set("Tars-Debug-Auth-Role", "admin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var payload struct {
		Current  string   `json:"current"`
		Eligible []string `json:"eligible"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.HasSuffix(payload.Current, filepath.Join("projects", "alpha")) {
		t.Fatalf("unexpected current dir: %q", payload.Current)
	}
	if len(payload.Eligible) < 2 {
		t.Fatalf("expected at least 2 eligible dirs, got %+v", payload.Eligible)
	}
}

func TestSessionCwdAPI_PutTransitionsAndEmits(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	extra := filepath.Join(root, "projects", "beta")
	if err := os.MkdirAll(extra, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := store.SetWorkDirs(sess.ID, []string{extra}, ""); err != nil {
		t.Fatalf("set work dirs: %v", err)
	}

	var emitCount int32
	var lastSession string
	notify := func(_ context.Context, evt notificationEvent) {
		atomic.AddInt32(&emitCount, 1)
		lastSession = evt.SessionID
	}

	handler := newSessionAPIHandlerWithNotifier(store, zerolog.New(io.Discard), nil, sessionStyleValues{}, notify)

	body := `{"current":"` + extra + `"}`
	req := httptest.NewRequest(http.MethodPut, "/v1/admin/sessions/"+sess.ID+"/cwd", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:1"
	req.Header.Set("Tars-Debug-Auth-Role", "admin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if atomic.LoadInt32(&emitCount) != 1 {
		t.Fatalf("expected 1 SSE emit, got %d", atomic.LoadInt32(&emitCount))
	}
	if lastSession != sess.ID {
		t.Fatalf("expected emit session %q, got %q", sess.ID, lastSession)
	}

	cur, err := store.GetCurrentDir(sess.ID)
	if err != nil {
		t.Fatalf("get current: %v", err)
	}
	if !strings.HasSuffix(cur, filepath.Join("projects", "beta")) {
		t.Fatalf("expected stored current to be beta, got %q", cur)
	}
}

func TestSessionCwdAPI_PutRejectsNonEligible(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	stranger := filepath.Join(root, "elsewhere")
	if err := os.MkdirAll(stranger, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	var emitCount int32
	notify := func(_ context.Context, _ notificationEvent) {
		atomic.AddInt32(&emitCount, 1)
	}
	handler := newSessionAPIHandlerWithNotifier(store, zerolog.New(io.Discard), nil, sessionStyleValues{}, notify)

	body := `{"current":"` + stranger + `"}`
	req := httptest.NewRequest(http.MethodPut, "/v1/admin/sessions/"+sess.ID+"/cwd", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:1"
	req.Header.Set("Tars-Debug-Auth-Role", "admin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%q", rec.Code, rec.Body.String())
	}
	if atomic.LoadInt32(&emitCount) != 0 {
		t.Fatalf("expected no SSE emit on rejection, got %d", atomic.LoadInt32(&emitCount))
	}
}

func TestSessionCwdAPI_GetUnknownSessionReturns404(t *testing.T) {
	store := session.NewStore(t.TempDir())
	handler := newSessionAPIHandler(store, zerolog.New(io.Discard))

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/sessions/does-not-exist/cwd", nil)
	req.RemoteAddr = "127.0.0.1:1"
	req.Header.Set("Tars-Debug-Auth-Role", "admin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%q", rec.Code, rec.Body.String())
	}
}
