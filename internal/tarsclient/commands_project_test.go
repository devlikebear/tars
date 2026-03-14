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

func TestExecuteCommand_ProjectWorkflowCommands(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/projects/proj_1/board":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"project_id": "proj_1",
				"updated_at": "2026-03-14T06:30:00Z",
				"columns":    []string{"todo", "in_progress", "review", "done"},
				"tasks": []map[string]any{
					{
						"id":              "task-1",
						"title":           "Implement dashboard",
						"status":          "todo",
						"assignee":        "dev-1",
						"role":            "developer",
						"review_required": true,
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/projects/proj_1/activity":
			if got := r.URL.Query().Get("limit"); got != "2" {
				t.Fatalf("expected activity limit=2, got %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"count": 1,
				"items": []map[string]any{
					{
						"id":         "act_1",
						"project_id": "proj_1",
						"task_id":    "task-1",
						"source":     "system",
						"agent":      "dev-1",
						"kind":       "task_status",
						"status":     "review",
						"message":    "Task dispatched",
						"timestamp":  "2026-03-14T06:31:00Z",
					},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/projects/proj_1/dispatch":
			var req map[string]string
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode dispatch request: %v", err)
			}
			if req["stage"] != "todo" {
				t.Fatalf("expected todo dispatch stage, got %+v", req)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"project_id": "proj_1",
				"runs": []map[string]any{
					{"id": "run_1", "task_id": "task-1", "agent": "dev-1", "worker_kind": "codex-cli", "status": "completed"},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/projects/proj_1/autopilot":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"project_id": "proj_1",
				"run_id":     "auto_1",
				"status":     "running",
				"message":    "Autopilot started",
				"iterations": 0,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/projects/proj_1/autopilot":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"project_id": "proj_1",
				"run_id":     "auto_1",
				"status":     "blocked",
				"message":    "Waiting on review",
				"iterations": 2,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runtime := runtimeClient{serverURL: server.URL}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	if _, _, err := executeCommand(context.Background(), runtime, "/project board proj_1", "", stdout, stderr); err != nil {
		t.Fatalf("/project board: %v", err)
	}
	if out := stdout.String(); !strings.Contains(out, "project board proj_1") || !strings.Contains(out, "task-1") || !strings.Contains(out, "status=todo") {
		t.Fatalf("unexpected /project board output: %q", out)
	}

	stdout.Reset()
	if _, _, err := executeCommand(context.Background(), runtime, "/project activity proj_1 2", "", stdout, stderr); err != nil {
		t.Fatalf("/project activity: %v", err)
	}
	if out := stdout.String(); !strings.Contains(out, "project activity proj_1") || !strings.Contains(out, "task_status") || !strings.Contains(out, "Task dispatched") {
		t.Fatalf("unexpected /project activity output: %q", out)
	}

	stdout.Reset()
	if _, _, err := executeCommand(context.Background(), runtime, "/project dispatch proj_1 todo", "", stdout, stderr); err != nil {
		t.Fatalf("/project dispatch: %v", err)
	}
	if out := stdout.String(); !strings.Contains(out, "project dispatch proj_1") || !strings.Contains(out, "run_1") || !strings.Contains(out, "stage=todo") {
		t.Fatalf("unexpected /project dispatch output: %q", out)
	}

	stdout.Reset()
	if _, _, err := executeCommand(context.Background(), runtime, "/project autopilot start proj_1", "", stdout, stderr); err != nil {
		t.Fatalf("/project autopilot start: %v", err)
	}
	if out := stdout.String(); !strings.Contains(out, "autopilot started") || !strings.Contains(out, "run_id=auto_1") {
		t.Fatalf("unexpected /project autopilot start output: %q", out)
	}

	stdout.Reset()
	if _, _, err := executeCommand(context.Background(), runtime, "/project autopilot status proj_1", "", stdout, stderr); err != nil {
		t.Fatalf("/project autopilot status: %v", err)
	}
	if out := stdout.String(); !strings.Contains(out, "project autopilot proj_1") || !strings.Contains(out, "status=blocked") || !strings.Contains(out, "iterations=2") {
		t.Fatalf("unexpected /project autopilot status output: %q", out)
	}
}
