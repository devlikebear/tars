package tarsserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func TestSessionAPIArchivePinAndArchivedFilters(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	keep, err := store.Create("keep visible")
	if err != nil {
		t.Fatalf("create keep: %v", err)
	}
	candidate, err := store.Create("New Chat")
	if err != nil {
		t.Fatalf("create candidate: %v", err)
	}
	if err := session.AppendMessage(store.TranscriptPath(candidate.ID), session.Message{Role: "user", Content: "still valuable"}); err != nil {
		t.Fatalf("append transcript: %v", err)
	}
	handler := newSessionAPIHandler(store, zerolog.Nop())

	patchSession(t, handler, keep.ID, `{"pinned":true}`)
	pinned := getSessionForTest(t, handler, keep.ID)
	if pinned.PinnedAt == nil {
		t.Fatalf("expected pinned_at after pin, got %+v", pinned)
	}

	patchSession(t, handler, candidate.ID, `{"archived":true}`)
	archived := getSessionForTest(t, handler, candidate.ID)
	if archived.ArchivedAt == nil {
		t.Fatalf("expected archived_at after archive, got %+v", archived)
	}
	if _, err := session.ReadMessages(store.TranscriptPath(candidate.ID)); err != nil {
		t.Fatalf("expected archive to preserve transcript: %v", err)
	}

	active := listSessionsForTest(t, handler, "/v1/admin/sessions?hidden=1")
	if containsSession(active, candidate.ID) {
		t.Fatalf("default admin list should hide archived session, got %+v", active)
	}
	if !containsSession(active, keep.ID) {
		t.Fatalf("default admin list should keep active pinned session, got %+v", active)
	}

	withArchived := listSessionsForTest(t, handler, "/v1/admin/sessions?hidden=1&archived=include")
	if !containsSession(withArchived, candidate.ID) || !containsSession(withArchived, keep.ID) {
		t.Fatalf("include archived list should contain both sessions, got %+v", withArchived)
	}

	onlyArchived := listSessionsForTest(t, handler, "/v1/admin/sessions?hidden=1&archived=only")
	if len(onlyArchived) != 1 || onlyArchived[0].ID != candidate.ID {
		t.Fatalf("archived-only list should contain only archived candidate, got %+v", onlyArchived)
	}
}

func TestSessionAPIPatchTitleNoopAndOrganizationErrors(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	sess, err := store.Create("Original")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	handler := newSessionAPIHandler(store, zerolog.Nop())

	renamed := patchSessionJSON(t, handler, sess.ID, `{"title":"Renamed thread"}`)
	if renamed.Title != "Renamed thread" {
		t.Fatalf("expected renamed session response, got %+v", renamed)
	}

	unchanged := patchSessionJSON(t, handler, sess.ID, `{}`)
	if unchanged.ID != sess.ID || unchanged.Title != "Renamed thread" {
		t.Fatalf("expected no-op patch to return current session, got %+v", unchanged)
	}

	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "title", body: `{"title":"Missing"}`},
		{name: "archive", body: `{"archived":true}`},
		{name: "pin", body: `{"pinned":true}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, body := patchSessionRaw(t, handler, "missing", tc.body)
			if status != http.StatusBadRequest {
				t.Fatalf("expected missing session patch to fail with 400, got %d body=%q", status, string(body))
			}
			if !strings.Contains(string(body), "session not found") {
				t.Fatalf("expected session not found error, got %q", string(body))
			}
		})
	}
}

func patchSession(t *testing.T, handler http.Handler, id string, body string) {
	t.Helper()
	status, responseBody := patchSessionRaw(t, handler, id, body)
	if status != http.StatusOK {
		t.Fatalf("expected patch 200, got %d body=%q", status, string(responseBody))
	}
}

func patchSessionJSON(t *testing.T, handler http.Handler, id string, body string) session.Session {
	t.Helper()
	status, responseBody := patchSessionRaw(t, handler, id, body)
	if status != http.StatusOK {
		t.Fatalf("expected patch 200, got %d body=%q", status, string(responseBody))
	}
	var sess session.Session
	if err := json.Unmarshal(responseBody, &sess); err != nil {
		t.Fatalf("decode patched session: %v", err)
	}
	return sess
}

func patchSessionRaw(t *testing.T, handler http.Handler, id string, body string) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/v1/admin/sessions/"+id, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Tars-Debug-Auth-Role", "admin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

func getSessionForTest(t *testing.T, handler http.Handler, id string) session.Session {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/sessions/"+id, nil)
	req.Header.Set("Tars-Debug-Auth-Role", "admin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected get 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	var sess session.Session
	if err := json.Unmarshal(rec.Body.Bytes(), &sess); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	return sess
}

func listSessionsForTest(t *testing.T, handler http.Handler, path string) []session.Session {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Tars-Debug-Auth-Role", "admin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	var sessions []session.Session
	if err := json.Unmarshal(body, &sessions); err != nil {
		t.Fatalf("decode sessions: %v", err)
	}
	return sessions
}

func containsSession(sessions []session.Session, id string) bool {
	for _, sess := range sessions {
		if sess.ID == id {
			return true
		}
	}
	return false
}
