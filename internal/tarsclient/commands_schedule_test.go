package tarsclient

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExecuteCommand_ScheduleCommands(t *testing.T) {
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

	runtime := runtimeClient{serverURL: server.URL}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	if _, _, err := executeCommand(context.Background(), runtime, "/schedule add 매주 월요일 9시 팀 동기화", "", stdout, stderr); err != nil {
		t.Fatalf("/schedule add: %v", err)
	}
	if !strings.Contains(stdout.String(), "schedule created") {
		t.Fatalf("unexpected /schedule add output: %q", stdout.String())
	}

	stdout.Reset()
	if _, _, err := executeCommand(context.Background(), runtime, "/schedule list", "", stdout, stderr); err != nil {
		t.Fatalf("/schedule list: %v", err)
	}
	if !strings.Contains(stdout.String(), "sch_1") {
		t.Fatalf("unexpected /schedule list output: %q", stdout.String())
	}

	stdout.Reset()
	if _, _, err := executeCommand(context.Background(), runtime, "/schedule done sch_1", "", stdout, stderr); err != nil {
		t.Fatalf("/schedule done: %v", err)
	}
	if !strings.Contains(stdout.String(), "completed") {
		t.Fatalf("unexpected /schedule done output: %q", stdout.String())
	}

	stdout.Reset()
	if _, _, err := executeCommand(context.Background(), runtime, "/schedule remove sch_1", "", stdout, stderr); err != nil {
		t.Fatalf("/schedule remove: %v", err)
	}
	if !strings.Contains(stdout.String(), "schedule removed") {
		t.Fatalf("unexpected /schedule remove output: %q", stdout.String())
	}
}
