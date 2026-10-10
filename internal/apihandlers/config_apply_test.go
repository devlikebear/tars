package apihandlers

import (
	"encoding/json"
	"github.com/devlikebear/tars/internal/config"
	"github.com/rs/zerolog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigRuntimeStatusAndSecretMask(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("log_level: info\nllm:\n  providers:\n    p:\n      kind: openai\n      api_key: secret-provider-value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	h := NewConfigHandler(path, cfg, "", nil, zerolog.Nop())
	patch := httptest.NewRecorder()
	h.ServeHTTP(patch, httptest.NewRequest(http.MethodPatch, "/v1/admin/config/values", strings.NewReader(`{"updates":{"log_level":"debug"}}`)))
	if patch.Code != 200 {
		t.Fatal(patch.Body.String())
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/admin/config/schema", nil))
	if strings.Contains(response.Body.String(), "secret-provider-value") {
		t.Fatal("provider secret exposed")
	}
	var status configSchemaResponse
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Values["log_level"] != "debug" || status.RuntimeValues["log_level"] != "info" || len(status.PendingRestartKeys) == 0 || status.RuntimeStartedAt == "" {
		t.Fatalf("invalid runtime status: %+v", status)
	}
	invalid := httptest.NewRecorder()
	h.ServeHTTP(invalid, httptest.NewRequest(http.MethodPatch, "/v1/admin/config/values", strings.NewReader(`{"updates":{"log_level":true}}`)))
	if invalid.Code != 400 {
		t.Fatalf("want 400, got %d", invalid.Code)
	}
}
