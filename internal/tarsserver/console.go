package tarsserver

import (
	"io/fs"
	"mime"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/devlikebear/tars/internal/tarsserver/consoleassets"
	"github.com/rs/zerolog"
)

const consoleDevProxyEnv = "TARS_CONSOLE_DEV_URL"

const consolePlaceholderHTML = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>TARS Console</title>
  </head>
  <body>
    <div id="app">tars console placeholder</div>
  </body>
</html>
`

func newConsoleHandler(logger zerolog.Logger) (http.Handler, error) {
	if proxyURL := strings.TrimSpace(os.Getenv(consoleDevProxyEnv)); proxyURL != "" {
		target, err := url.Parse(proxyURL)
		if err != nil {
			return nil, err
		}
		return newConsoleDevProxy(target), nil
	}

	distFS := consoleassets.DistFS()
	_ = logger

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isConsolePath(r.URL.Path) {
			http.NotFound(w, r)
			return
		}

		assetPath := strings.TrimPrefix(strings.TrimSpace(r.URL.Path), "/console")
		assetPath = strings.TrimPrefix(assetPath, "/")
		if assetPath == "" {
			assetPath = "index.html"
		}
		if hasAsset(distFS, assetPath) {
			serveConsoleAsset(w, distFS, assetPath)
			return
		}

		if !hasAsset(distFS, "index.html") {
			serveConsolePlaceholder(w)
			return
		}

		serveConsoleAsset(w, distFS, "index.html")
	}), nil
}

func newConsoleDevProxy(target *url.URL) http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.URL.Path = strings.TrimPrefix(req.URL.Path, "/console")
		if req.URL.Path == "" {
			req.URL.Path = "/"
		}
		if !strings.HasPrefix(req.URL.Path, "/") {
			req.URL.Path = "/" + req.URL.Path
		}
	}
	return proxy
}

func isConsolePath(requestPath string) bool {
	pathValue := strings.TrimSpace(requestPath)
	return pathValue == "/console" || pathValue == "/console/" || strings.HasPrefix(pathValue, "/console/")
}

func hasAsset(root fs.FS, assetPath string) bool {
	cleanPath := path.Clean(strings.TrimSpace(assetPath))
	cleanPath = strings.TrimPrefix(cleanPath, "/")
	if cleanPath == "." || cleanPath == "" {
		cleanPath = "index.html"
	}
	info, err := fs.Stat(root, cleanPath)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func serveConsoleAsset(w http.ResponseWriter, root fs.FS, assetPath string) {
	cleanPath := path.Clean(strings.TrimPrefix(strings.TrimSpace(assetPath), "/"))
	if cleanPath == "." || cleanPath == "" {
		cleanPath = "index.html"
	}
	content, err := fs.ReadFile(root, cleanPath)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
	if contentType := mime.TypeByExtension(path.Ext(cleanPath)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	if strings.HasSuffix(cleanPath, ".html") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	_, _ = w.Write(content)
}

func serveConsolePlaceholder(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(consolePlaceholderHTML))
}
