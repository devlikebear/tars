package apihandlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/devlikebear/tars/internal/consoleauth"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func newRemoteChatFixture(t *testing.T) (http.Handler, *session.Store, *consoleauth.Store) {
	t.Helper()
	dir := t.TempDir()
	sessions := session.NewStore(dir)
	auth := consoleauth.NewStore(dir)
	handler := NewRemoteChatHandler(sessions, auth, zerolog.Nop())
	return handler, sessions, auth
}

func deviceRequest(method, path, deviceID string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	if deviceID != "" {
		req = req.WithContext(serverauth.WithDeviceIDContext(req.Context(), deviceID))
	}
	return req
}

func TestRemoteChatSessionEnsureCreatesAndReuses(t *testing.T) {
	handler, sessions, auth := newRemoteChatFixture(t)
	if _, _, err := auth.CreateDevice("Jaeyoon's iPhone", consoleauth.RoleFocus); err != nil {
		t.Fatal(err)
	}
	devices, err := auth.ListDevices()
	if err != nil || len(devices) != 1 {
		t.Fatalf("ListDevices: %v %v", devices, err)
	}
	deviceID := devices[0].ID

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, deviceRequest(http.MethodPost, "/v1/remote/chat/session", deviceID))
	if rec.Code != http.StatusOK {
		t.Fatalf("first ensure: got %d body=%q", rec.Code, rec.Body.String())
	}
	var first remoteChatSessionView
	if err := json.Unmarshal(rec.Body.Bytes(), &first); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if first.SessionID == "" {
		t.Fatalf("expected a session id")
	}
	sess, err := sessions.Get(first.SessionID)
	if err != nil {
		t.Fatalf("Get created session: %v", err)
	}
	if sess.Title != "Phone · Jaeyoon's iPhone" {
		t.Fatalf("title = %q", sess.Title)
	}
	if sess.PermissionMode != "manual" {
		t.Fatalf("permission mode = %q, want manual", sess.PermissionMode)
	}

	// A second call for the same device returns the same session.
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, deviceRequest(http.MethodPost, "/v1/remote/chat/session", deviceID))
	var second remoteChatSessionView
	if err := json.Unmarshal(rec2.Body.Bytes(), &second); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if second.SessionID != first.SessionID {
		t.Fatalf("second call made a new session: %q != %q", second.SessionID, first.SessionID)
	}
}

func TestRemoteChatSessionRecreatesWhenDeletedOrArchived(t *testing.T) {
	handler, sessions, auth := newRemoteChatFixture(t)
	if err := auth.SetDeviceChatSession("dev-1", "missing-session"); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, deviceRequest(http.MethodPost, "/v1/remote/chat/session", "dev-1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d body=%q", rec.Code, rec.Body.String())
	}
	var created remoteChatSessionView
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.SessionID == "missing-session" {
		t.Fatalf("must not reuse a session id that does not exist")
	}

	// Archiving the newly-created session must also trigger a replacement.
	if _, err := sessions.SetArchived(created.SessionID, true); err != nil {
		t.Fatal(err)
	}
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, deviceRequest(http.MethodPost, "/v1/remote/chat/session", "dev-1"))
	var second remoteChatSessionView
	if err := json.Unmarshal(rec2.Body.Bytes(), &second); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if second.SessionID == created.SessionID {
		t.Fatalf("an archived session must not be reused")
	}
}

func TestRemoteChatHistoryReturnsOnlyThatDeviceSession(t *testing.T) {
	handler, sessions, auth := newRemoteChatFixture(t)
	if _, _, err := auth.CreateDevice("phone", consoleauth.RoleFocus); err != nil {
		t.Fatal(err)
	}
	devices, _ := auth.ListDevices()
	deviceID := devices[0].ID

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, deviceRequest(http.MethodPost, "/v1/remote/chat/session", deviceID))
	var created remoteChatSessionView
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if err := session.AppendMessage(sessions.TranscriptPath(created.SessionID), session.Message{Role: "user", Content: "hi"}); err != nil {
		t.Fatal(err)
	}

	histRec := httptest.NewRecorder()
	handler.ServeHTTP(histRec, deviceRequest(http.MethodGet, "/v1/remote/chat/history", deviceID))
	if histRec.Code != http.StatusOK {
		t.Fatalf("history: got %d body=%q", histRec.Code, histRec.Body.String())
	}
	var body struct {
		SessionID string            `json:"session_id"`
		Messages  []session.Message `json:"messages"`
	}
	if err := json.Unmarshal(histRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.SessionID != created.SessionID {
		t.Fatalf("session id = %q, want %q", body.SessionID, created.SessionID)
	}
	if len(body.Messages) != 1 || body.Messages[0].Content != "hi" {
		t.Fatalf("messages = %+v", body.Messages)
	}
}

func TestRemoteChatRequiresDeviceID(t *testing.T) {
	handler, _, _ := newRemoteChatFixture(t)
	for _, tc := range []struct {
		method, path string
	}{
		{http.MethodPost, "/v1/remote/chat/session"},
		{http.MethodGet, "/v1/remote/chat/history"},
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, deviceRequest(tc.method, tc.path, ""))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s without a device id: got %d, want 403", tc.method, tc.path, rec.Code)
		}
	}
}
