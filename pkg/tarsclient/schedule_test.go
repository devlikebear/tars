package tarsclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_ScheduleMethods(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/schedules":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "sch_1", "title": "회의", "schedule": "0 9 * * 1", "status": "active"}})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/schedules":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "sch_1", "title": "회의", "schedule": "0 9 * * 1", "status": "active"})
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/schedules/sch_1":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "sch_1", "title": "회의", "schedule": "0 9 * * 1", "status": "completed"})
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/schedules/sch_1":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(Config{ServerURL: server.URL})
	items, err := client.ListSchedules(context.Background())
	if err != nil {
		t.Fatalf("ListSchedules: %v", err)
	}
	if len(items) != 1 || items[0].ID != "sch_1" {
		t.Fatalf("unexpected schedules: %+v", items)
	}
	created, err := client.CreateSchedule(context.Background(), ScheduleCreateRequest{Natural: "매주 월요일 9시"})
	if err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}
	if created.ID != "sch_1" {
		t.Fatalf("unexpected created schedule: %+v", created)
	}
	updated, err := client.UpdateSchedule(context.Background(), "sch_1", ScheduleUpdateRequest{Status: ptrString("completed")})
	if err != nil {
		t.Fatalf("UpdateSchedule: %v", err)
	}
	if updated.Status != "completed" {
		t.Fatalf("unexpected updated schedule: %+v", updated)
	}
	if err := client.DeleteSchedule(context.Background(), "sch_1"); err != nil {
		t.Fatalf("DeleteSchedule: %v", err)
	}
}

func ptrString(v string) *string { return &v }
