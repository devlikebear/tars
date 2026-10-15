//go:build e2e

package tarsserver

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterE2EConsoleBuildRoute_OverridesState(t *testing.T) {
	mux := http.NewServeMux()
	state := newConsoleBuildState("original")
	registerE2EConsoleBuildRoute(mux, e2eRouteDeps{ConsoleBuild: state})

	body := []byte(`{"build":"newbuild0001"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/e2e/console-build", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := state.Get(); got != "newbuild0001" {
		t.Fatalf("expected state to be overridden, got %q", got)
	}
}

func TestRegisterE2EConsoleBuildRoute_RejectsNonPost(t *testing.T) {
	mux := http.NewServeMux()
	registerE2EConsoleBuildRoute(mux, e2eRouteDeps{ConsoleBuild: newConsoleBuildState("x")})

	req := httptest.NewRequest(http.MethodGet, "/v1/e2e/console-build", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestRegisterE2EConsoleBuildRoute_NoOpWithoutState(t *testing.T) {
	mux := http.NewServeMux()
	registerE2EConsoleBuildRoute(mux, e2eRouteDeps{})

	req := httptest.NewRequest(http.MethodPost, "/v1/e2e/console-build", bytes.NewReader([]byte(`{"build":"x"}`)))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected the route to not be registered (404), got %d", rec.Code)
	}
}
