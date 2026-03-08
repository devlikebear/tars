package tarsserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/cron"
	"github.com/devlikebear/tars/internal/schedule"
	"github.com/rs/zerolog"
)

func TestScheduleAPI_CRUD(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	store := schedule.NewStore(workspace, cron.NewStore(workspace), schedule.Options{Timezone: "Asia/Seoul"})
	handler := newScheduleAPIHandler(store, zerolog.New(io.Discard))

	createReq := httptest.NewRequest(http.MethodPost, "/v1/schedules", strings.NewReader(`{"natural":"내일 오후 3시에 회의 준비 알려줘"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("expected 200 create, got %d body=%q", createRec.Code, createRec.Body.String())
	}
	var created schedule.Item
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created schedule: %v", err)
	}
	relativeReq := httptest.NewRequest(http.MethodPost, "/v1/schedules", strings.NewReader(`{"natural":"1분뒤 테스트 알림"}`))
	relativeReq.Header.Set("Content-Type", "application/json")
	relativeRec := httptest.NewRecorder()
	handler.ServeHTTP(relativeRec, relativeReq)
	if relativeRec.Code != http.StatusOK {
		t.Fatalf("expected 200 relative create, got %d body=%q", relativeRec.Code, relativeRec.Body.String())
	}
	var relative schedule.Item
	if err := json.Unmarshal(relativeRec.Body.Bytes(), &relative); err != nil {
		t.Fatalf("decode relative created schedule: %v", err)
	}
	if !strings.HasPrefix(relative.Schedule, "at:") {
		t.Fatalf("expected relative schedule as at:, got %q", relative.Schedule)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/schedules", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 list, got %d body=%q", listRec.Code, listRec.Body.String())
	}

	patchReq := httptest.NewRequest(http.MethodPatch, "/v1/schedules/"+created.ID, strings.NewReader(`{"status":"completed"}`))
	patchReq.Header.Set("Content-Type", "application/json")
	patchRec := httptest.NewRecorder()
	handler.ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("expected 200 patch, got %d body=%q", patchRec.Code, patchRec.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/schedules/"+created.ID, nil)
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 delete, got %d body=%q", deleteRec.Code, deleteRec.Body.String())
	}
}
