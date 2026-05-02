package tarsserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
