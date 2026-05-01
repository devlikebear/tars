package tarsserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func TestPlanArchiveAPIListsGlobalAndSessionArchives(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	mainSession, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main session: %v", err)
	}
	otherSession, err := store.Create("Other")
	if err != nil {
		t.Fatalf("create other session: %v", err)
	}
	archivedAt := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	createdAt := "2026-04-30T08:00:00Z"
	if err := memory.AppendMemoryNote(root, archivedAt, "[archived plan] session="+mainSession.ID+"\nPlan: Main goal (created: "+createdAt+")\nTasks: 2 total, 1 completed, 1 cancelled, 0 pending\n  [x] 1: Done\n  [~] 2: Skipped"); err != nil {
		t.Fatalf("append main archive: %v", err)
	}
	if err := memory.AppendMemoryNote(root, archivedAt.Add(time.Minute), "[archived plan] session="+otherSession.ID+"\nPlan: Other goal\nTasks: 1 total"); err != nil {
		t.Fatalf("append other archive: %v", err)
	}
	if err := memory.AppendMemoryNote(root, archivedAt.Add(2*time.Minute), "[archived plan]\nPlan: Legacy goal\nTasks: 1 total"); err != nil {
		t.Fatalf("append legacy archive: %v", err)
	}

	handler := newSessionAPIHandler(store, zerolog.Nop())
	globalReq := httptest.NewRequest(http.MethodGet, "/v1/admin/plans/archive", nil)
	globalReq.Header.Set("Tars-Debug-Auth-Role", "admin")
	globalRec := httptest.NewRecorder()
	handler.ServeHTTP(globalRec, globalReq)
	if globalRec.Code != http.StatusOK {
		t.Fatalf("global archive expected 200, got %d body=%q", globalRec.Code, globalRec.Body.String())
	}
	var global struct {
		Items []planArchiveItem `json:"items"`
	}
	if err := json.Unmarshal(globalRec.Body.Bytes(), &global); err != nil {
		t.Fatalf("decode global archive: %v", err)
	}
	if len(global.Items) != 3 {
		t.Fatalf("expected three global archive items, got %+v", global.Items)
	}
	if global.Items[2].SessionID != mainSession.ID || global.Items[2].Goal != "Main goal" || global.Items[2].CreatedAt != createdAt {
		t.Fatalf("unexpected parsed main archive: %+v", global.Items[2])
	}
	if global.Items[2].Summary == "" {
		t.Fatalf("expected read-only summary body, got %+v", global.Items[2])
	}

	sessionReq := httptest.NewRequest(http.MethodGet, "/v1/admin/sessions/"+mainSession.ID+"/plans/archive", nil)
	sessionReq.Header.Set("Tars-Debug-Auth-Role", "admin")
	sessionRec := httptest.NewRecorder()
	handler.ServeHTTP(sessionRec, sessionReq)
	if sessionRec.Code != http.StatusOK {
		t.Fatalf("session archive expected 200, got %d body=%q", sessionRec.Code, sessionRec.Body.String())
	}
	var scoped struct {
		Items []planArchiveItem `json:"items"`
	}
	if err := json.Unmarshal(sessionRec.Body.Bytes(), &scoped); err != nil {
		t.Fatalf("decode session archive: %v", err)
	}
	if len(scoped.Items) != 1 || scoped.Items[0].SessionID != mainSession.ID || scoped.Items[0].Goal != "Main goal" {
		t.Fatalf("expected only main session archive, got %+v", scoped.Items)
	}
}

func TestPlanArchiveAPIFiltersSessionBeforeLimit(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	mainSession, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main session: %v", err)
	}
	otherSession, err := store.Create("Other")
	if err != nil {
		t.Fatalf("create other session: %v", err)
	}
	archivedAt := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	if err := memory.AppendMemoryNote(root, archivedAt, "[archived plan] session="+mainSession.ID+"\nPlan: Main goal\nTasks: 1 total"); err != nil {
		t.Fatalf("append main archive: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := memory.AppendMemoryNote(root, archivedAt.Add(time.Duration(i+1)*time.Minute), "[archived plan] session="+otherSession.ID+"\nPlan: Other goal\nTasks: 1 total"); err != nil {
			t.Fatalf("append other archive: %v", err)
		}
	}

	handler := newSessionAPIHandler(store, zerolog.Nop())
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/sessions/"+mainSession.ID+"/plans/archive?limit=1", nil)
	req.Header.Set("Tars-Debug-Auth-Role", "admin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("session archive expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	var scoped struct {
		Items []planArchiveItem `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &scoped); err != nil {
		t.Fatalf("decode session archive: %v", err)
	}
	if len(scoped.Items) != 1 || scoped.Items[0].SessionID != mainSession.ID || scoped.Items[0].Goal != "Main goal" {
		t.Fatalf("expected main session archive after filtering before limit, got %+v", scoped.Items)
	}
}
