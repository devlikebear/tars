package tarsserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func adminCriticRequest(t *testing.T, handler http.Handler, method, sessionID, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, "/v1/admin/sessions/"+sessionID+"/critic", reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.RemoteAddr = "127.0.0.1:1"
	req.Header.Set("Tars-Debug-Auth-Role", "admin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestSessionCriticAPI_PutGetDeleteRoundTrip(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	handler := newSessionAPIHandler(store, zerolog.New(io.Discard))

	rec := adminCriticRequest(t, handler, http.MethodPut, main.ID, `{"enabled":true,"max_iterations":3}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("put expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	var putResp struct {
		Critic *session.SessionCritic `json:"critic"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &putResp); err != nil {
		t.Fatalf("put decode: %v", err)
	}
	if putResp.Critic == nil || !putResp.Critic.Enabled || putResp.Critic.MaxIterations != 3 {
		t.Fatalf("unexpected put critic: %+v", putResp.Critic)
	}
	if putResp.Critic.Status != session.SessionCriticStatusIdle {
		t.Fatalf("status not idle: %q", putResp.Critic.Status)
	}

	rec = adminCriticRequest(t, handler, http.MethodGet, main.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	var getResp struct {
		Critic *session.SessionCritic `json:"critic"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("get decode: %v", err)
	}
	if getResp.Critic == nil || !getResp.Critic.Enabled {
		t.Fatalf("get critic mismatch: %+v", getResp.Critic)
	}

	rec = adminCriticRequest(t, handler, http.MethodDelete, main.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	rec = adminCriticRequest(t, handler, http.MethodGet, main.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get-after-delete expected 200, got %d", rec.Code)
	}
	var postDelete struct {
		Critic *session.SessionCritic `json:"critic"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &postDelete); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if postDelete.Critic != nil {
		t.Fatalf("expected nil critic after delete, got %+v", postDelete.Critic)
	}
}

func TestSessionCriticAPI_AcceptsWorkerSession(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	worker, err := store.EnsureWorker("p1")
	if err != nil {
		t.Fatalf("ensure worker: %v", err)
	}
	handler := newSessionAPIHandler(store, zerolog.New(io.Discard))

	rec := adminCriticRequest(t, handler, http.MethodPut, worker.ID, `{"enabled":true,"max_iterations":2}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for worker session, got %d body=%q", rec.Code, rec.Body.String())
	}
	var resp struct {
		Critic *session.SessionCritic `json:"critic"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Critic.IsEnabled() || resp.Critic.MaxIterations != 2 {
		t.Fatalf("unexpected worker critic: %+v", resp.Critic)
	}
}

func TestSessionCriticAPI_MethodNotAllowed(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	handler := newSessionAPIHandler(store, zerolog.New(io.Discard))
	rec := adminCriticRequest(t, handler, http.MethodPost, main.ID, "{}")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestSessionCriticAPI_NotFound(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	handler := newSessionAPIHandler(store, zerolog.New(io.Discard))
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		body := ""
		if method == http.MethodPut {
			body = `{"enabled":true}`
		}
		rec := adminCriticRequest(t, handler, method, "no-such-session", body)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("method %s expected 404, got %d body=%q", method, rec.Code, rec.Body.String())
		}
	}
}

func TestSessionCriticAPI_BadJSON(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	handler := newSessionAPIHandler(store, zerolog.New(io.Discard))
	rec := adminCriticRequest(t, handler, http.MethodPut, main.ID, `not json`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on bad JSON, got %d", rec.Code)
	}
}

func TestSessionCriticAPI_ClampsMaxIterations(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	handler := newSessionAPIHandler(store, zerolog.New(io.Discard))

	rec := adminCriticRequest(t, handler, http.MethodPut, main.ID, `{"enabled":true,"max_iterations":99}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("put expected 200, got %d", rec.Code)
	}
	var resp struct {
		Critic *session.SessionCritic `json:"critic"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Critic.MaxIterations != session.MaxCriticMaxIterations {
		t.Fatalf("expected clamp to %d, got %d", session.MaxCriticMaxIterations, resp.Critic.MaxIterations)
	}
}
