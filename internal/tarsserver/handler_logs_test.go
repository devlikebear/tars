package tarsserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
)

func TestLogsAPI_TailsAndFiltersRuntimeLog(t *testing.T) {
	workspaceDir := t.TempDir()
	runtimeLogPath := filepath.Join(workspaceDir, "logs", "runtime.log")
	if err := os.MkdirAll(filepath.Dir(runtimeLogPath), 0o755); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	content := "" +
		`{"level":"debug","component":"pulse","time":"2026-05-01T00:00:00Z","message":"pulse tick"}` + "\n" +
		`{"level":"warn","component":"cron","time":"2026-05-01T00:01:00Z","message":"job retry"}` + "\n" +
		`{"level":"error","component":"agentruntime","time":"2026-05-01T00:02:00Z","message":"snapshot failed","run_id":"run-1"}` + "\n"
	if err := os.WriteFile(runtimeLogPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	handler := newLogsAPIHandler(workspaceDir, runtimeLogPath, zerolog.Nop())
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/logs?file=runtime&lines=2&level=error&component=runtime", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		SelectedFile string `json:"selected_file"`
		Lines        []struct {
			Raw       string         `json:"raw"`
			Level     string         `json:"level"`
			Component string         `json:"component"`
			Message   string         `json:"message"`
			Time      string         `json:"time"`
			Fields    map[string]any `json:"fields"`
		} `json:"lines"`
		Files []struct {
			ID     string `json:"id"`
			Label  string `json:"label"`
			Exists bool   `json:"exists"`
		} `json:"files"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.SelectedFile != "runtime" {
		t.Fatalf("unexpected selected file %q", out.SelectedFile)
	}
	if len(out.Lines) != 1 {
		t.Fatalf("expected one filtered line, got %+v", out.Lines)
	}
	line := out.Lines[0]
	if line.Level != "error" || line.Component != "agentruntime" || line.Message != "snapshot failed" {
		t.Fatalf("unexpected parsed line: %+v", line)
	}
	if got := line.Fields["run_id"]; got != "run-1" {
		t.Fatalf("expected run_id field, got %+v", line.Fields)
	}
	if len(out.Files) == 0 || out.Files[0].ID != "runtime" || !out.Files[0].Exists {
		t.Fatalf("expected runtime file option first, got %+v", out.Files)
	}
}

func TestLogsAPI_RejectsUnknownFileID(t *testing.T) {
	handler := newLogsAPIHandler(t.TempDir(), "", zerolog.Nop())
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/logs?file=../secret&lines=50", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}
