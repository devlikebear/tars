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

func adminGoalRequest(t *testing.T, handler http.Handler, method, sessionID, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, "/v1/admin/sessions/"+sessionID+"/goal", reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.RemoteAddr = "127.0.0.1:1"
	req.Header.Set("Tars-Debug-Auth-Role", "admin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestSessionGoalAPI_PutGetDeleteRoundTrip(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	handler := newSessionAPIHandler(store, zerolog.New(io.Discard))

	rec := adminGoalRequest(t, handler, http.MethodPut, main.ID, `{"description":"ship it","max_auto_continues":5}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("put expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	var putResp struct {
		Goal *session.SessionGoal `json:"goal"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &putResp); err != nil {
		t.Fatalf("put decode: %v", err)
	}
	if putResp.Goal == nil || putResp.Goal.Description != "ship it" {
		t.Fatalf("unexpected put goal: %+v", putResp.Goal)
	}
	if putResp.Goal.MaxAutoContinues != 5 {
		t.Fatalf("max not honored: %d", putResp.Goal.MaxAutoContinues)
	}
	if putResp.Goal.Status != session.SessionGoalStatusActive {
		t.Fatalf("status not active: %q", putResp.Goal.Status)
	}

	rec = adminGoalRequest(t, handler, http.MethodGet, main.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	var getResp struct {
		Goal *session.SessionGoal `json:"goal"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("get decode: %v", err)
	}
	if getResp.Goal == nil || getResp.Goal.Description != "ship it" {
		t.Fatalf("get goal mismatch: %+v", getResp.Goal)
	}

	rec = adminGoalRequest(t, handler, http.MethodDelete, main.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	rec = adminGoalRequest(t, handler, http.MethodGet, main.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get-after-delete expected 200, got %d", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if getResp.Goal != nil {
		t.Fatalf("expected goal nil after delete, got %+v", getResp.Goal)
	}
}

func TestSessionGoalAPI_NonMainRejected(t *testing.T) {
	store := session.NewStore(t.TempDir())
	sess, err := store.Create("regular")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	handler := newSessionAPIHandler(store, zerolog.New(io.Discard))

	rec := adminGoalRequest(t, handler, http.MethodPut, sess.ID, `{"description":"x"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-main, got %d body=%q", rec.Code, rec.Body.String())
	}
}

func TestSessionGoalAPI_UnknownSession(t *testing.T) {
	store := session.NewStore(t.TempDir())
	handler := newSessionAPIHandler(store, zerolog.New(io.Discard))

	rec := adminGoalRequest(t, handler, http.MethodPut, "doesnotexist", `{"description":"x"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%q", rec.Code, rec.Body.String())
	}
}

func TestSessionGoalAPI_MethodNotAllowed(t *testing.T) {
	store := session.NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	handler := newSessionAPIHandler(store, zerolog.New(io.Discard))
	rec := adminGoalRequest(t, handler, http.MethodPost, main.ID, `{}`)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for POST, got %d body=%q", rec.Code, rec.Body.String())
	}
}

func TestSessionGoalAPI_PutEmptyDescriptionClears(t *testing.T) {
	store := session.NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	handler := newSessionAPIHandler(store, zerolog.New(io.Discard))

	if rec := adminGoalRequest(t, handler, http.MethodPut, main.ID, `{"description":"first"}`); rec.Code != http.StatusOK {
		t.Fatalf("put first failed: %d %s", rec.Code, rec.Body.String())
	}
	rec := adminGoalRequest(t, handler, http.MethodPut, main.ID, `{"description":"   "}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("put empty expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	var resp struct {
		Goal *session.SessionGoal `json:"goal"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Goal != nil {
		t.Fatalf("expected nil goal after empty put, got %+v", resp.Goal)
	}
}
