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
	"github.com/devlikebear/tars/internal/testutil"
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

	body, err := json.Marshal(map[string]string{"current": extra})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, "/v1/admin/sessions/"+sess.ID+"/cwd", strings.NewReader(string(body)))
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

func putSessionCwd(t *testing.T, handler http.Handler, sessionID, current string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"current": current})
	req := httptest.NewRequest(http.MethodPut, "/v1/admin/sessions/"+sessionID+"/cwd", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:1"
	req.Header.Set("Tars-Debug-Auth-Role", "admin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestSessionCwdAPI_PutAddsExistingDirectory(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	repo := filepath.Join(root, "some", "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	canonical, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatalf("eval symlinks: %v", err)
	}

	var emitted []notificationEvent
	notify := func(_ context.Context, evt notificationEvent) { emitted = append(emitted, evt) }
	handler := newSessionAPIHandlerWithNotifier(store, zerolog.New(io.Discard), nil, sessionStyleValues{}, notify)

	rec := putSessionCwd(t, handler, sess.ID, repo)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	var resp struct {
		Current string `json:"current"`
		Added   bool   `json:"added"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Current != canonical || !resp.Added {
		t.Fatalf("unexpected response %+v", resp)
	}
	eligible, _ := store.EligibleCwds(sess.ID)
	if len(eligible) != 2 || eligible[1] != canonical {
		t.Fatalf("expected repo registered as a candidate, got %v", eligible)
	}
	if len(emitted) != 1 || emitted[0].Message != canonical {
		t.Fatalf("expected one cwd event for %q, got %+v", canonical, emitted)
	}
}

func TestSessionCwdAPI_PutRejectsMissingOrNonDirectory(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	file := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	var emitCount int32
	notify := func(_ context.Context, _ notificationEvent) {
		atomic.AddInt32(&emitCount, 1)
	}
	handler := newSessionAPIHandlerWithNotifier(store, zerolog.New(io.Discard), nil, sessionStyleValues{}, notify)

	cases := map[string]struct {
		target string
		want   string
	}{
		"missing":  {filepath.Join(root, "elsewhere"), "does not exist"},
		"file":     {file, "not a directory"},
		"relative": {"some/repo", "absolute path"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rec := putSessionCwd(t, handler, sess.ID, tc.target)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d body=%q", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.want) {
				t.Fatalf("expected error mentioning %q, got %q", tc.want, rec.Body.String())
			}
		})
	}
	if atomic.LoadInt32(&emitCount) != 0 {
		t.Fatalf("expected no SSE emit on rejection, got %d", atomic.LoadInt32(&emitCount))
	}
	if eligible, _ := store.EligibleCwds(sess.ID); len(eligible) != 1 {
		t.Fatalf("rejections must not register candidates, got %v", eligible)
	}
}

func TestExpandCwdHome(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	cases := map[string]string{
		"~":              home,
		"~/src/repo":     filepath.Join(home, "src", "repo"),
		"/abs/path":      "/abs/path",
		"~other/project": "~other/project",
	}
	for in, want := range cases {
		if got := expandCwdHome(in); got != want {
			t.Errorf("expandCwdHome(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSessionCwdAPI_PutUnknownSessionReturns404(t *testing.T) {
	store := session.NewStore(t.TempDir())
	handler := newSessionAPIHandlerWithNotifier(store, zerolog.New(io.Discard), nil, sessionStyleValues{}, nil)

	rec := putSessionCwd(t, handler, "does-not-exist", t.TempDir())
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%q", rec.Code, rec.Body.String())
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
