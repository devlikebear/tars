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

func TestExecuteCommand_OpsAndApprove(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/ops/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"disk_used_percent": 80.0, "process_count": 333, "disk_free_bytes": 1000})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/ops/cleanup/plan":
			_ = json.NewEncoder(w).Encode(map[string]any{"approval_id": "apr_1", "total_bytes": 10, "candidates": []map[string]any{{"path": "/tmp/a", "size_bytes": 10}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runtime := runtimeClient{serverURL: server.URL}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	if _, _, err := executeCommand(context.Background(), runtime, "/ops status", "", stdout, stderr); err != nil {
		t.Fatalf("/ops status: %v", err)
	}
	if !strings.Contains(stdout.String(), "process_count=333") {
		t.Fatalf("unexpected /ops status output: %q", stdout.String())
	}

	stdout.Reset()
	if _, _, err := executeCommand(context.Background(), runtime, "/ops cleanup plan", "", stdout, stderr); err != nil {
		t.Fatalf("/ops cleanup plan: %v", err)
	}
	if !strings.Contains(stdout.String(), "approval_id=apr_1") {
		t.Fatalf("unexpected /ops cleanup plan output: %q", stdout.String())
	}

	stdout.Reset()
	if _, _, err := executeCommand(context.Background(), runtime, "/approve list", "", stdout, stderr); err != nil {
		t.Fatalf("/approve list: %v", err)
	}
	if !strings.Contains(stdout.String(), "legacy TUI no longer handles /approve") || !strings.Contains(stdout.String(), "tars approve") {
		t.Fatalf("unexpected /approve list output: %q", stdout.String())
	}

	stdout.Reset()
	if _, _, err := executeCommand(context.Background(), runtime, "/approve run apr_1", "", stdout, stderr); err != nil {
		t.Fatalf("/approve run: %v", err)
	}
	if !strings.Contains(stdout.String(), "legacy TUI no longer handles /approve") {
		t.Fatalf("unexpected /approve run output: %q", stdout.String())
	}

	stdout.Reset()
	if _, _, err := executeCommand(context.Background(), runtime, "/approve reject apr_1", "", stdout, stderr); err != nil {
		t.Fatalf("/approve reject: %v", err)
	}
	if !strings.Contains(stdout.String(), "legacy TUI no longer handles /approve") {
		t.Fatalf("unexpected /approve reject output: %q", stdout.String())
	}
}
