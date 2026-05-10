package tarsserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

func TestNormalizeTerminalSizeClampsBounds(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	tests := []struct {
		name string
		cols int
		rows int
		want terminalSize
	}{
		{name: "negative uses defaults", cols: -1, rows: -1, want: terminalSize{Cols: defaultTerminalCols, Rows: defaultTerminalRows}},
		{name: "zero uses defaults", cols: 0, rows: 0, want: terminalSize{Cols: defaultTerminalCols, Rows: defaultTerminalRows}},
		{name: "below minimum clamps up", cols: 1, rows: 1, want: terminalSize{Cols: minTerminalCols, Rows: minTerminalRows}},
		{name: "huge clamps down", cols: maxInt, rows: maxInt, want: terminalSize{Cols: maxTerminalCols, Rows: maxTerminalRows}},
		{name: "max accepted unchanged", cols: maxTerminalCols, rows: maxTerminalRows, want: terminalSize{Cols: maxTerminalCols, Rows: maxTerminalRows}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeTerminalSize(tt.cols, tt.rows)
			if got != tt.want {
				t.Fatalf("normalizeTerminalSize(%d, %d) = %+v, want %+v", tt.cols, tt.rows, got, tt.want)
			}
			winSize := terminalWinsize(terminalSize{Cols: tt.cols, Rows: tt.rows})
			if winSize.Cols != uint16(tt.want.Cols) || winSize.Rows != uint16(tt.want.Rows) {
				t.Fatalf("terminalWinsize(%d, %d) = cols=%d rows=%d, want cols=%d rows=%d", tt.cols, tt.rows, winSize.Cols, winSize.Rows, tt.want.Cols, tt.want.Rows)
			}
		})
	}
}

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

func TestTerminalAPI_WebSocketRelaysInputOutputAndResize(t *testing.T) {
	root, store, sess := newTerminalTestSession(t)
	projectDir := testCanonicalPath(t, filepath.Join(root, "projects", "demo"))
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	if err := store.SetWorkDirs(sess.ID, []string{projectDir}, projectDir); err != nil {
		t.Fatalf("set work dirs: %v", err)
	}

	fake := newFakeTerminalSession()
	var startedCWD string
	var startedSize terminalSize
	handler := newTerminalAPIHandlerWithDeps(root, store, terminalHandlerDeps{
		OpenExternal: func(_ context.Context, cwd string) (terminalOpenResult, error) {
			return terminalOpenResult{App: "Terminal", CWD: cwd}, nil
		},
		StartSession: func(_ context.Context, cwd string, size terminalSize) (terminalSession, error) {
			startedCWD = cwd
			startedSize = size
			return fake, nil
		},
	}, zerolog.New(io.Discard))
	server := httptest.NewServer(handler)
	defer server.Close()

	conn := dialTerminalWebSocket(t, server.URL, sess.ID, "", 100, 32, "admin")
	defer conn.Close()

	var ready terminalWSMessage
	if err := conn.ReadJSON(&ready); err != nil {
		t.Fatalf("read ready message: %v", err)
	}
	if ready.Type != "ready" || ready.CWD != projectDir {
		t.Fatalf("unexpected ready message: %+v", ready)
	}
	if startedCWD != projectDir {
		t.Fatalf("expected started cwd %q, got %q", projectDir, startedCWD)
	}
	if startedSize.Cols != 100 || startedSize.Rows != 32 {
		t.Fatalf("expected initial size 100x32, got %+v", startedSize)
	}

	if err := conn.WriteJSON(terminalWSMessage{Type: "input", Data: encodeTerminalTestData("echo hi\r")}); err != nil {
		t.Fatalf("write input: %v", err)
	}
	if got := fake.waitForWrite(t); got != "echo hi\r" {
		t.Fatalf("expected terminal input %q, got %q", "echo hi\r", got)
	}

	if err := conn.WriteJSON(terminalWSMessage{Type: "resize", Cols: 120, Rows: 40}); err != nil {
		t.Fatalf("write resize: %v", err)
	}
	if got := fake.waitForResize(t); got.Cols != 120 || got.Rows != 40 {
		t.Fatalf("expected resize 120x40, got %+v", got)
	}

	fake.sendOutput("hi\r\n")
	var output terminalWSMessage
	if err := conn.ReadJSON(&output); err != nil {
		t.Fatalf("read output: %v", err)
	}
	if output.Type != "output" {
		t.Fatalf("expected output message, got %+v", output)
	}
	decoded, err := decodeTerminalWSData(output.Data)
	if err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if string(decoded) != "hi\r\n" {
		t.Fatalf("expected output %q, got %q", "hi\r\n", string(decoded))
	}
}

func TestTerminalAPI_WebSocketRejectsPathOutsideSessionWorkDirs(t *testing.T) {
	root, store, sess := newTerminalTestSession(t)
	projectDir := testCanonicalPath(t, filepath.Join(root, "projects", "demo"))
	outsideDir := testCanonicalPath(t, filepath.Join(t.TempDir(), "outside"))
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("mkdir outside dir: %v", err)
	}
	if err := store.SetWorkDirs(sess.ID, []string{projectDir}, projectDir); err != nil {
		t.Fatalf("set work dirs: %v", err)
	}

	called := false
	handler := newTerminalAPIHandlerWithDeps(root, store, terminalHandlerDeps{
		StartSession: func(_ context.Context, cwd string, size terminalSize) (terminalSession, error) {
			called = true
			return newFakeTerminalSession(), nil
		},
	}, zerolog.New(io.Discard))
	server := httptest.NewServer(handler)
	defer server.Close()

	_, resp, err := websocket.DefaultDialer.Dial(terminalWSURL(server.URL, sess.ID, outsideDir, 80, 24), http.Header{
		"Tars-Debug-Auth-Role": []string{"admin"},
	})
	if err == nil {
		t.Fatalf("expected websocket dial to fail")
	}
	if resp == nil || resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got resp=%v err=%v", resp, err)
	}
	if called {
		t.Fatalf("terminal session should not start for outside cwd")
	}
}

func TestTerminalAPI_WebSocketRejectsNonAdminRole(t *testing.T) {
	root, store, sess := newTerminalTestSession(t)
	called := false
	handler := newTerminalAPIHandlerWithDeps(root, store, terminalHandlerDeps{
		StartSession: func(_ context.Context, cwd string, size terminalSize) (terminalSession, error) {
			called = true
			return newFakeTerminalSession(), nil
		},
	}, zerolog.New(io.Discard))
	server := httptest.NewServer(handler)
	defer server.Close()

	_, resp, err := websocket.DefaultDialer.Dial(terminalWSURL(server.URL, sess.ID, "", 80, 24), http.Header{
		"Tars-Debug-Auth-Role": []string{"user"},
	})
	if err == nil {
		t.Fatalf("expected websocket dial to fail")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got resp=%v err=%v", resp, err)
	}
	if called {
		t.Fatalf("terminal session should not start for non-admin request")
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

type fakeTerminalSession struct {
	readCh   chan []byte
	writeCh  chan []byte
	resizeCh chan terminalSize
	closed   chan struct{}
	once     sync.Once
}

func newFakeTerminalSession() *fakeTerminalSession {
	return &fakeTerminalSession{
		readCh:   make(chan []byte, 4),
		writeCh:  make(chan []byte, 4),
		resizeCh: make(chan terminalSize, 4),
		closed:   make(chan struct{}),
	}
}

func (f *fakeTerminalSession) Read(p []byte) (int, error) {
	select {
	case <-f.closed:
		return 0, io.EOF
	case data := <-f.readCh:
		return copy(p, data), nil
	}
}

func (f *fakeTerminalSession) Write(p []byte) (int, error) {
	select {
	case <-f.closed:
		return 0, io.ErrClosedPipe
	case f.writeCh <- append([]byte(nil), p...):
		return len(p), nil
	}
}

func (f *fakeTerminalSession) Resize(size terminalSize) error {
	select {
	case <-f.closed:
		return io.ErrClosedPipe
	case f.resizeCh <- size:
		return nil
	}
}

func (f *fakeTerminalSession) Close() error {
	f.once.Do(func() {
		close(f.closed)
	})
	return nil
}

func (f *fakeTerminalSession) sendOutput(value string) {
	f.readCh <- []byte(value)
}

func (f *fakeTerminalSession) waitForWrite(t *testing.T) string {
	t.Helper()
	select {
	case data := <-f.writeCh:
		return string(data)
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for terminal write")
		return ""
	}
}

func (f *fakeTerminalSession) waitForResize(t *testing.T) terminalSize {
	t.Helper()
	select {
	case size := <-f.resizeCh:
		return size
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for terminal resize")
		return terminalSize{}
	}
}

func dialTerminalWebSocket(t *testing.T, serverURL, sessionID, cwd string, cols, rows int, role string) *websocket.Conn {
	t.Helper()
	conn, resp, err := websocket.DefaultDialer.Dial(terminalWSURL(serverURL, sessionID, cwd, cols, rows), http.Header{
		"Tars-Debug-Auth-Role": []string{role},
	})
	if err != nil {
		status := ""
		if resp != nil {
			status = resp.Status
		}
		t.Fatalf("dial websocket failed status=%s err=%v", status, err)
	}
	return conn
}

func terminalWSURL(serverURL, sessionID, cwd string, cols, rows int) string {
	wsURL := "ws" + strings.TrimPrefix(serverURL, "http") + "/v1/terminal/ws"
	q := make([]string, 0, 5)
	q = append(q, "session_id="+url.QueryEscape(sessionID))
	if cwd != "" {
		q = append(q, "cwd="+url.QueryEscape(cwd))
	}
	if cols > 0 {
		q = append(q, "cols="+url.QueryEscape(fmt.Sprintf("%d", cols)))
	}
	if rows > 0 {
		q = append(q, "rows="+url.QueryEscape(fmt.Sprintf("%d", rows)))
	}
	return wsURL + "?" + strings.Join(q, "&")
}

func encodeTerminalTestData(value string) string {
	return base64.StdEncoding.EncodeToString([]byte(value))
}

func decodeTerminalWSData(value string) ([]byte, error) {
	if strings.TrimSpace(value) == "" {
		return nil, errors.New("data is empty")
	}
	return base64.StdEncoding.DecodeString(value)
}
