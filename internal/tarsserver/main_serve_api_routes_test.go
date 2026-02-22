package tarsserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterBrowserRoutes_IncludesRelayRoute(t *testing.T) {
	mux := http.NewServeMux()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	registerBrowserRoutes(mux, handler)

	paths := []string{
		"/v1/browser/status",
		"/v1/browser/profiles",
		"/v1/browser/relay",
		"/v1/browser/login",
		"/v1/browser/check",
		"/v1/browser/run",
		"/v1/vault/status",
	}
	for _, path := range paths {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		mux.ServeHTTP(rec, req)
		if rec.Code == http.StatusNotFound {
			t.Fatalf("expected registered route, got 404 for %s", path)
		}
	}
}
