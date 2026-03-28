package tarsserver

import (
	"bytes"
	"io/fs"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/rs/zerolog"
)

func TestConsoleHandler_ServesPlaceholderGuidanceWhenBuiltAssetsMissing(t *testing.T) {
	var logBuffer bytes.Buffer
	logger := zerolog.New(&logBuffer)
	handler := newConsoleStaticHandler(logger, fstest.MapFS{}, false)

	for _, route := range []string{"/console", "/console/", "/console/projects/demo"} {
		t.Run(route, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, route, nil)
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
				t.Fatalf("expected text/html content type, got %q", got)
			}
			body := rec.Body.String()
			for _, needle := range []string{
				"TARS Console build required",
				"npm install",
				"npm run build",
				consoleDevProxyEnv,
			} {
				if !strings.Contains(body, needle) {
					t.Fatalf("expected placeholder body to contain %q, got %q", needle, body)
				}
			}
		})
	}

	if got := logBuffer.String(); !strings.Contains(got, "console assets are not built") || !strings.Contains(got, "npm run build") {
		t.Fatalf("expected startup warning about placeholder assets, got %q", got)
	}
}

func TestConsoleHandler_ServesBuiltAssetsForConsoleRoutes(t *testing.T) {
	handler := newConsoleStaticHandler(zerolog.New(io.Discard), fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<!doctype html><div id=\"app\">console</div>")},
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log('ok')")},
	}, true)

	for _, route := range []string{"/console", "/console/", "/console/projects/demo"} {
		t.Run(route, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, route, nil)
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

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/console/assets/app.js", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for built asset, got %d body=%q", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "console.log") {
		t.Fatalf("expected js asset body, got %q", body)
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

func TestConsoleHasBuiltAssets(t *testing.T) {
	tests := []struct {
		name string
		fs   fs.FS
		want bool
	}{
		{
			name: "missing index",
			fs:   fstest.MapFS{},
			want: false,
		},
		{
			name: "present index",
			fs: fstest.MapFS{
				"index.html": &fstest.MapFile{Data: []byte("<!doctype html>")},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := consoleHasBuiltAssets(tt.fs); got != tt.want {
				t.Fatalf("consoleHasBuiltAssets()=%v want %v", got, tt.want)
			}
		})
	}
}
