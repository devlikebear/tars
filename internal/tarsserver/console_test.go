package tarsserver

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestConsoleHandler_ServesEmbeddedIndexForConsoleRoutes(t *testing.T) {
	handler, err := newConsoleHandler(zerolog.New(io.Discard))
	if err != nil {
		t.Fatalf("new console handler: %v", err)
	}

	for _, path := range []string{"/console", "/console/", "/console/projects/demo"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, path, nil)
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
				t.Fatalf("expected text/html content type, got %q", got)
			}
			if body := rec.Body.String(); !strings.Contains(body, "id=\"app\"") {
				t.Fatalf("expected console html shell, got %q", body)
			}
		})
	}
}

func TestConsoleHandler_ProxiesDevServerWhenConfigured(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("proxied " + r.URL.Path))
	}))
	t.Cleanup(target.Close)
	t.Setenv(consoleDevProxyEnv, target.URL)

	handler, err := newConsoleHandler(zerolog.New(io.Discard))
	if err != nil {
		t.Fatalf("new console handler: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/console/projects/demo", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); body != "proxied /projects/demo" {
		t.Fatalf("expected stripped proxied path, got %q", body)
	}
}
