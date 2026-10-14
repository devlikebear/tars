package tarsserver

import (
	"bytes"
	"io"
	"io/fs"
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
	handler := newConsoleStaticHandler(logger, fstest.MapFS{}, false, "")

	for _, route := range []string{"/console/", "/console/projects/demo"} {
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
		"index.html":    &fstest.MapFile{Data: []byte("<!doctype html><div id=\"app\">console</div>")},
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log('ok')")},
		"favicon.svg":   &fstest.MapFile{Data: []byte("<svg></svg>")},
	}, true, "")

	for _, route := range []string{"/console/", "/console/projects/demo"} {
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

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/console/favicon.svg", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for console favicon, got %d body=%q", rec.Code, rec.Body.String())
	}
}

func TestConsoleHandler_ServesPWAManifestWithCorrectContentType(t *testing.T) {
	manifestBody := `{"name":"TARS Console","start_url":"/console/"}`
	handler := newConsoleStaticHandler(zerolog.New(io.Discard), fstest.MapFS{
		"index.html":           &fstest.MapFile{Data: []byte("<!doctype html>")},
		"manifest.webmanifest": &fstest.MapFile{Data: []byte(manifestBody)},
		"pwa-icon-192.png":     &fstest.MapFile{Data: []byte("\x89PNG\r\n")},
	}, true, "")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/console/manifest.webmanifest", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/manifest+json" {
		t.Fatalf("expected application/manifest+json, got %q", got)
	}
	if body := rec.Body.String(); body != manifestBody {
		t.Fatalf("expected manifest body to be served verbatim, got %q", body)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/console/pwa-icon-192.png", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for icon, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "image/png") {
		t.Fatalf("expected image/png content type for icon, got %q", got)
	}
}

func TestConsoleHandler_RedirectsBareConsolePath(t *testing.T) {
	handler := newConsoleStaticHandler(zerolog.New(io.Discard), fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<!doctype html><div id=\"app\">console</div>")},
	}, true, "")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/console", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/console/" {
		t.Fatalf("expected redirect location /console/, got %q", got)
	}
}

func TestConsoleHandler_ProxiesDevServerWhenConfigured(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("proxied " + r.URL.Path))
	}))
	t.Cleanup(target.Close)
	t.Setenv(consoleDevProxyEnv, target.URL)

	handler, _, err := newConsoleHandler(zerolog.New(io.Discard))
	if err != nil {
		t.Fatalf("new console handler: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/console/projects/demo", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); body != "proxied /console/projects/demo" {
		t.Fatalf("expected console-mounted proxied path, got %q", body)
	}
}

func TestConsoleHandler_RedirectsBareConsolePathBeforeDevProxy(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	t.Cleanup(target.Close)
	t.Setenv(consoleDevProxyEnv, target.URL)

	handler, _, err := newConsoleHandler(zerolog.New(io.Discard))
	if err != nil {
		t.Fatalf("new console handler: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/console", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/console/" {
		t.Fatalf("expected redirect location /console/, got %q", got)
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

// TestConsoleIndexHash_StableAndContentSensitive is console_build's own
// coverage (tars#1257): same bytes -> same hash, different bytes ->
// different hash, missing index.html -> "".
func TestConsoleIndexHash_StableAndContentSensitive(t *testing.T) {
	fsA := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<!doctype html><title>a</title>")}}
	fsASame := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<!doctype html><title>a</title>")}}
	fsB := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<!doctype html><title>b</title>")}}

	hashA := consoleIndexHash(fsA)
	if hashA == "" {
		t.Fatal("expected a non-empty hash for a present index.html")
	}
	if len(hashA) != 12 {
		t.Fatalf("expected a 12-character hash, got %q (%d chars)", hashA, len(hashA))
	}
	if got := consoleIndexHash(fsASame); got != hashA {
		t.Fatalf("expected identical content to hash the same, got %q want %q", got, hashA)
	}
	if got := consoleIndexHash(fsB); got == hashA {
		t.Fatalf("expected different content to hash differently, both %q", got)
	}
	if got := consoleIndexHash(fstest.MapFS{}); got != "" {
		t.Fatalf("expected empty hash when index.html is missing, got %q", got)
	}
}

// TestInjectConsoleBuildMeta_InsertsBeforeHeadClose covers the string
// surgery that gets the console its own build value (tars#1257).
func TestInjectConsoleBuildMeta_InsertsBeforeHeadClose(t *testing.T) {
	html := []byte("<!doctype html><html><head><title>t</title></head><body></body></html>")
	out := injectConsoleBuildMeta(html, "abc123def456")

	got := string(out)
	if !strings.Contains(got, `<meta name="tars-console-build" content="abc123def456">`) {
		t.Fatalf("expected meta tag in output, got %q", got)
	}
	if !strings.Contains(got, `<meta name="tars-console-build" content="abc123def456"></head>`) {
		t.Fatalf("expected meta tag immediately before </head>, got %q", got)
	}

	// No "</head>" at all: returned unchanged, not corrupted.
	noHead := []byte("<!doctype html><body>no head here</body></html>")
	if got := string(injectConsoleBuildMeta(noHead, "abc123def456")); got != string(noHead) {
		t.Fatalf("expected html without </head> to pass through unchanged, got %q", got)
	}
}

// TestConsoleHandler_InjectsBuildMetaAndNoCacheOnIndex covers the static
// handler wiring: the meta tag and Cache-Control land on index.html (both
// the direct request and the SPA fallback), never on another asset.
func TestConsoleHandler_InjectsBuildMetaAndNoCacheOnIndex(t *testing.T) {
	handler := newConsoleStaticHandler(zerolog.New(io.Discard), fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("<!doctype html><head></head><div id=\"app\">console</div>")},
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log('ok')")},
	}, true, "deadbeef0001")

	for _, route := range []string{"/console/", "/console/projects/demo"} {
		t.Run(route, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, route, nil)
			handler.ServeHTTP(rec, req)

			if !strings.Contains(rec.Body.String(), `content="deadbeef0001"`) {
				t.Fatalf("expected build meta tag in index.html response, got %q", rec.Body.String())
			}
			if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
				t.Fatalf("expected Cache-Control: no-cache on index.html, got %q", got)
			}
		})
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/console/assets/app.js", nil)
	handler.ServeHTTP(rec, req)
	if strings.Contains(rec.Body.String(), "tars-console-build") {
		t.Fatalf("expected no build meta tag injected into a non-html asset, got %q", rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got == "no-cache" {
		t.Fatal("expected Cache-Control: no-cache to stay specific to index.html")
	}
}
