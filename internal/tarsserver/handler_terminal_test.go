package tarsserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func TestTerminalAPI_OpenUsesSessionCurrentDir(t *testing.T) {
	root, store, sess := newTerminalTestSession(t)
	projectDir := testCanonicalPath(t, filepath.Join(root, "projects", "demo"))
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	if err := store.SetWorkDirs(sess.ID, []string{projectDir}, projectDir); err != nil {
		t.Fatalf("set work dirs: %v", err)
	}

	var openedCWD string
	handler := newTerminalAPIHandlerWithOpener(root, store, func(_ context.Context, cwd string) (terminalOpenResult, error) {
		openedCWD = cwd
		return terminalOpenResult{App: "Terminal", CWD: cwd, Message: "opened"}, nil
	}, zerolog.New(io.Discard))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newTerminalOpenRequest(t, sess.ID, ""))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if openedCWD != projectDir {
		t.Fatalf("expected opener cwd %q, got %q", projectDir, openedCWD)
	}
}

func TestTerminalAPI_OpenSupportsMainAlias(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	mainSession, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main session: %v", err)
	}

	var openedCWD string
	handler := newTerminalAPIHandlerWithOpener(root, store, func(_ context.Context, cwd string) (terminalOpenResult, error) {
		openedCWD = cwd
		return terminalOpenResult{App: "Terminal", CWD: cwd}, nil
	}, zerolog.New(io.Discard))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newTerminalOpenRequest(t, "main", ""))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if openedCWD != mainSession.CurrentDir {
		t.Fatalf("expected opener cwd %q, got %q", mainSession.CurrentDir, openedCWD)
	}
}

func TestTerminalAPI_OpenRejectsPathOutsideSessionWorkDirs(t *testing.T) {
	root, store, sess := newTerminalTestSession(t)
	projectDir := testCanonicalPath(t, filepath.Join(root, "projects", "demo"))
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	if err := store.SetWorkDirs(sess.ID, []string{projectDir}, projectDir); err != nil {
		t.Fatalf("set work dirs: %v", err)
	}
	outsideDir := testCanonicalPath(t, filepath.Join(t.TempDir(), "outside"))
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("mkdir outside dir: %v", err)
	}

	called := false
	handler := newTerminalAPIHandlerWithOpener(root, store, func(_ context.Context, cwd string) (terminalOpenResult, error) {
		called = true
		return terminalOpenResult{App: "Terminal", CWD: cwd}, nil
	}, zerolog.New(io.Discard))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newTerminalOpenRequest(t, sess.ID, outsideDir))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%q", rec.Code, rec.Body.String())
	}
	if called {
		t.Fatalf("opener should not be called for paths outside session workdirs")
	}
}

func TestTerminalAPI_OpenRejectsRelativeTraversalOutsideCurrentDir(t *testing.T) {
	root, store, sess := newTerminalTestSession(t)
	projectDir := testCanonicalPath(t, filepath.Join(root, "projects", "demo"))
	siblingDir := testCanonicalPath(t, filepath.Join(root, "projects", "outside"))
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	if err := os.MkdirAll(siblingDir, 0o755); err != nil {
		t.Fatalf("mkdir sibling dir: %v", err)
	}
	if err := store.SetWorkDirs(sess.ID, []string{projectDir}, projectDir); err != nil {
		t.Fatalf("set work dirs: %v", err)
	}

	called := false
	handler := newTerminalAPIHandlerWithOpener(root, store, func(_ context.Context, cwd string) (terminalOpenResult, error) {
		called = true
		return terminalOpenResult{App: "Terminal", CWD: cwd}, nil
	}, zerolog.New(io.Discard))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newTerminalOpenRequest(t, sess.ID, "../outside"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%q", rec.Code, rec.Body.String())
	}
	if called {
		t.Fatalf("opener should not be called after relative traversal outside current_dir")
	}
}

func TestTerminalAPI_OpenRejectsNonAdminRole(t *testing.T) {
	root, store, sess := newTerminalTestSession(t)
	called := false
	handler := newTerminalAPIHandlerWithOpener(root, store, func(_ context.Context, cwd string) (terminalOpenResult, error) {
		called = true
		return terminalOpenResult{App: "Terminal", CWD: cwd}, nil
	}, zerolog.New(io.Discard))

	req := newTerminalOpenRequest(t, sess.ID, "")
	req.Header.Set("Tars-Debug-Auth-Role", "user")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%q", rec.Code, rec.Body.String())
	}
	if called {
		t.Fatalf("opener should not be called for non-admin requests")
	}
}

func newTerminalTestSession(t *testing.T) (string, *session.Store, session.Session) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	sess, err := store.Create("chat")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return root, store, sess
}

func newTerminalOpenRequest(t *testing.T, sessionID string, cwd string) *http.Request {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"session_id": sessionID,
		"cwd":        cwd,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/terminal/open", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Tars-Debug-Auth-Role", "admin")
	return req
}
