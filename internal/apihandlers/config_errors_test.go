package apihandlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/rs/zerolog"
)

func TestConfigAPI_WithoutAConfigFile(t *testing.T) {
	handler := NewConfigHandler("", config.Config{}, "", nil, zerolog.Nop())
	const noPath = "no config file path configured"
	runErrorCases(t, handler, []errorCase{
		{"reading it gives an empty file", http.MethodGet, "/v1/admin/config", "", http.StatusOK, `"content":""`},
		{"saving it", http.MethodPut, "/v1/admin/config", `{"content":"x: 1\n"}`, http.StatusBadRequest, noPath},
		{"patching values", http.MethodPatch, "/v1/admin/config/values", `{"updates":{"a":"b"}}`, http.StatusBadRequest, noPath},
		{"resetting a workspace that is not configured", http.MethodPost, "/v1/admin/reset/workspace", "", http.StatusBadRequest, "workspace directory not configured"},
		{"restart with the wrong method", http.MethodGet, "/v1/admin/restart", "", http.StatusMethodNotAllowed, "method not allowed"},
	})
}

func TestConfigAPI_ReadsAndSavesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("api_addr: 127.0.0.1:43180\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := NewConfigHandler(path, config.Config{}, "", nil, zerolog.Nop())
	runErrorCases(t, handler, []errorCase{
		{"read", http.MethodGet, "/v1/admin/config", "", http.StatusOK, "api_addr: 127.0.0.1:43180"},
		{"save with a body that is not JSON", http.MethodPut, "/v1/admin/config", "not json", http.StatusBadRequest, "invalid request body"},
		{"patch with a body that is not JSON", http.MethodPatch, "/v1/admin/config/values", "not json", http.StatusBadRequest, "invalid request body"},
		{"patch with nothing to change", http.MethodPatch, "/v1/admin/config/values", `{"updates":{}}`, http.StatusBadRequest, "no updates provided"},
		{"save", http.MethodPut, "/v1/admin/config", `{"content":"api_addr: 127.0.0.1:43181\n"}`, http.StatusOK, `"ok":"true"`},
	})
	saved, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(saved), "43181") {
		t.Fatalf("saved config = %q, %v", saved, err)
	}

	// A path that cannot be read as a file is an error, not an empty config.
	unreadable := NewConfigHandler(t.TempDir(), config.Config{}, "", nil, zerolog.Nop())
	runErrorCases(t, unreadable, []errorCase{
		{"read a folder as the config file", http.MethodGet, "/v1/admin/config", "", http.StatusInternalServerError, "failed to read config file"},
	})
}

func TestConfigAPI_ResetWorkspace(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "notes.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := NewConfigHandler("", config.Config{}, workspace, nil, zerolog.Nop())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/admin/reset/workspace", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("reset: %d %q", rec.Code, rec.Body.String())
	}

	missing := NewConfigHandler("", config.Config{}, filepath.Join(t.TempDir(), "gone"), nil, zerolog.Nop())
	runErrorCases(t, missing, []errorCase{
		{"a workspace folder that does not exist", http.MethodPost, "/v1/admin/reset/workspace", "", http.StatusInternalServerError, "read workspace directory failed"},
	})
}

// Outside a launchd service the restart route answers first and then hands
// the running executable to the restart function it was given.
func TestConfigAPI_RestartCallsTheRestartFunction(t *testing.T) {
	prevGOOS := restartRuntimeGOOS
	restartRuntimeGOOS = "linux"
	t.Cleanup(func() { restartRuntimeGOOS = prevGOOS })

	called := make(chan string, 1)
	restart := func(exe string, _ []string, _ []string) error {
		called <- exe
		return nil
	}
	handler := NewConfigHandler("", config.Config{}, "", restart, zerolog.Nop())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/admin/restart", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok":"true"`) {
		t.Fatalf("restart: %d %q", rec.Code, rec.Body.String())
	}
	select {
	case exe := <-called:
		if exe == "" {
			t.Fatal("restart was called without an executable path")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the restart function was not called")
	}
}

func TestLogsAPI_RejectsWhatItCannotRead(t *testing.T) {
	workspace := t.TempDir()
	runtimeLog := filepath.Join(workspace, "logs", "runtime.log")
	if err := os.MkdirAll(filepath.Dir(runtimeLog), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runtimeLog, []byte(`{"level":"info","message":"started"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := NewLogsHandler(workspace, runtimeLog, zerolog.Nop())
	runErrorCases(t, handler, []errorCase{
		{"lines that are not a number", http.MethodGet, "/v1/admin/logs?file=runtime&lines=many", "", http.StatusBadRequest, "lines must be a positive integer"},
		{"a level that does not exist", http.MethodGet, "/v1/admin/logs?file=runtime&level=LOUD", "", http.StatusBadRequest, "level must be ALL, TRACE, DEBUG, INFO, WARN, or ERROR"},
	})
}

func TestNormalizeRuntimeLogFilePath(t *testing.T) {
	for in, want := range map[string]string{
		"":                  "",
		"  ":                "",
		"logs/tars.log":     "logs/tars.log",
		" logs/ ":           "logs/tars.log",
		"/var/log/tars/":    "/var/log/tars/tars.log",
		"/var/log/tars.log": "/var/log/tars.log",
	} {
		if got := NormalizeRuntimeLogFilePath(in); got != want {
			t.Errorf("NormalizeRuntimeLogFilePath(%q) = %q, want %q", in, got, want)
		}
	}
}
