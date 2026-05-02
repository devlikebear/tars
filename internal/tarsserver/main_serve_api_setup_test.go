package tarsserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/rs/zerolog"
)

func TestRegisterSetupOnlyRoutes_ServesAllowedEndpoints(t *testing.T) {
	mux := http.NewServeMux()
	stub := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	registerSetupOnlyRoutes(mux, setupOnlyHandlers{
		healthz: stub,
		setup:   stub,
		config:  stub,
		auth:    stub,
		events:  stub,
		console: stub,
	})

	allowed := []string{
		"/v1/healthz",
		"/v1/setup/status",
		"/v1/admin/config",
		"/v1/admin/config/values",
		"/v1/admin/config/schema",
		"/v1/admin/restart",
		"/v1/auth/whoami",
		"/v1/events/stream",
		"/console",
		"/console/",
	}
	for _, path := range allowed {
		t.Run("allow "+path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, path, nil)
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusNoContent {
				t.Fatalf("expected 204 from stub for %s, got %d body=%q", path, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestRegisterSetupOnlyRoutes_FallbackOnUnknownV1(t *testing.T) {
	mux := http.NewServeMux()
	registerSetupOnlyRoutes(mux, setupOnlyHandlers{
		healthz: noContentHandler(),
		setup:   noContentHandler(),
		config:  noContentHandler(),
		auth:    noContentHandler(),
	})

	for _, path := range []string{"/v1/chat", "/v1/agentruntime/runs", "/v1/sessions", "/v1/cron/jobs", "/v1/pulse/status"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, path, nil)
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("expected 503 for %s, got %d body=%q", path, rec.Code, rec.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode fallback body: %v", err)
			}
			if needs, _ := body["needs_setup"].(bool); !needs {
				t.Fatalf("expected needs_setup=true in fallback body, got %+v", body)
			}
			if got, _ := body["path"].(string); got != path {
				t.Fatalf("expected path=%q, got %q", path, got)
			}
		})
	}
}

func TestRegisterSetupOnlyRoutes_RootRedirectsToConsole(t *testing.T) {
	mux := http.NewServeMux()
	registerSetupOnlyRoutes(mux, setupOnlyHandlers{
		healthz: noContentHandler(),
		setup:   noContentHandler(),
		config:  noContentHandler(),
		auth:    noContentHandler(),
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/console/" {
		t.Fatalf("expected redirect to /console/, got %q", got)
	}
}

func TestRegisterSetupOnlyRoutes_UnknownNonV1Returns404(t *testing.T) {
	mux := http.NewServeMux()
	registerSetupOnlyRoutes(mux, setupOnlyHandlers{
		healthz: noContentHandler(),
		setup:   noContentHandler(),
		config:  noContentHandler(),
		auth:    noContentHandler(),
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/some/random/path", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%q", rec.Code, rec.Body.String())
	}
}

func TestRegisterSetupOnlyRoutes_OmitsEventsAndConsoleWhenNil(t *testing.T) {
	mux := http.NewServeMux()
	registerSetupOnlyRoutes(mux, setupOnlyHandlers{
		healthz: noContentHandler(),
		setup:   noContentHandler(),
		config:  noContentHandler(),
		auth:    noContentHandler(),
		// events and console intentionally nil
	})

	for _, path := range []string{"/v1/events/stream", "/console", "/console/x"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, path, nil)
			mux.ServeHTTP(rec, req)
			// /v1/events/stream falls into the /v1/ catch-all → 503
			// /console paths fall into the / catch-all → 404
			if path == "/v1/events/stream" && rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("expected 503 for events when nil, got %d", rec.Code)
			}
			if path != "/v1/events/stream" && rec.Code != http.StatusNotFound {
				t.Fatalf("expected 404 for %s when console nil, got %d", path, rec.Code)
			}
		})
	}
}

func noContentHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
}

func TestBuildAPIMux_RoutesToSetupOnlyWhenLLMNotReady(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{
		RuntimeConfig: config.RuntimeConfig{WorkspaceDir: filepath.Join(dir, "workspace")},
		APIConfig: config.APIConfig{
			APIAuthMode:               "off",
			APIAllowInsecureLocalAuth: true,
		},
	}
	logger := zerolog.New(io.Discard)
	base, err := buildBaseDeps(&options{}, cfg, time.Now, logger)
	if err != nil {
		t.Fatalf("buildBaseDeps: %v", err)
	}
	// LLMReady=false (default) — no llm router populated.

	opts := &options{APIAddr: "127.0.0.1:0", ConfigPath: ""}
	apiRuntime, err := buildAPIMux(opts, base, time.Now, logger, io.Discard)
	if err != nil {
		t.Fatalf("buildAPIMux setup-only: %v", err)
	}
	if apiRuntime.server == nil {
		t.Fatalf("expected server constructed")
	}
	if apiRuntime.agentRuntime != nil {
		t.Fatalf("expected agentRuntime nil in setup-only mode")
	}
	if apiRuntime.cronManager != nil {
		t.Fatalf("expected cronManager nil in setup-only mode")
	}
	if apiRuntime.pulseRuntime != nil {
		t.Fatalf("expected pulseRuntime nil in setup-only mode")
	}
	if apiRuntime.reflectionRuntime != nil {
		t.Fatalf("expected reflectionRuntime nil in setup-only mode")
	}

	// Probe routes through the actual server handler (so middleware runs too).
	for _, tc := range []struct {
		name       string
		path       string
		wantStatus int
	}{
		{"healthz allowed", "/v1/healthz", http.StatusOK},
		{"setup status allowed", "/v1/setup/status", http.StatusOK},
		{"chat blocked with 503", "/v1/chat", http.StatusServiceUnavailable},
		{"agentruntime blocked with 503", "/v1/agentruntime/runs", http.StatusServiceUnavailable},
		{"pulse blocked with 503", "/v1/pulse/status", http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.RemoteAddr = "127.0.0.1:5555"
			apiRuntime.server.Handler.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("expected %d for %s, got %d body=%q", tc.wantStatus, tc.path, rec.Code, rec.Body.String())
			}
		})
	}

	// healthz body should expose needs_setup=true since cfg has no providers.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/healthz", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	apiRuntime.server.Handler.ServeHTTP(rec, req)
	var body struct {
		NeedsSetup bool `json:"needs_setup"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode healthz: %v", err)
	}
	if !body.NeedsSetup {
		t.Fatalf("expected needs_setup=true in setup-only mode, got body %s", rec.Body.String())
	}
}
