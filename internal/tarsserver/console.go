package tarsserver

import (
	"fmt"
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
const consoleBuildHint = "cd frontend/console && npm install && npm run build"

func newConsoleHandler(logger zerolog.Logger) (http.Handler, error) {
	if proxyURL := strings.TrimSpace(os.Getenv(consoleDevProxyEnv)); proxyURL != "" {
		target, err := url.Parse(proxyURL)
		if err != nil {
			return nil, err
		}
		return newConsoleDevProxy(target), nil
	}

	distFS := consoleassets.DistFS()
	return newConsoleStaticHandler(logger, distFS, consoleHasBuiltAssets(distFS)), nil
}

// newConsoleDevViteHandler returns a passthrough proxy for Vite dev server
// paths (/@vite/, /src/, /@fs/, /node_modules/) that are requested at the
// root level by Vite's HMR client. Returns nil when not in dev proxy mode.
func newConsoleDevViteHandler() http.Handler {
	proxyURL := strings.TrimSpace(os.Getenv(consoleDevProxyEnv))
	if proxyURL == "" {
		return nil
	}
	target, err := url.Parse(proxyURL)
	if err != nil {
		return nil
	}
	return httputil.NewSingleHostReverseProxy(target)
}

func newConsoleStaticHandler(logger zerolog.Logger, distFS fs.FS, builtAssets bool) http.Handler {
	if !builtAssets {
		logger.Warn().
			Str("hint", consoleBuildHint).
			Str("dev_proxy_env", consoleDevProxyEnv).
			Msg("console assets are not built; serving placeholder console")
	}

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

		if !builtAssets {
			serveConsolePlaceholder(w)
			return
		}

		serveConsoleAsset(w, distFS, "index.html")
	})
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

func consoleHasBuiltAssets(root fs.FS) bool {
	return hasAsset(root, "index.html")
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
	_, _ = w.Write([]byte(consolePlaceholderHTML()))
}

func consolePlaceholderHTML() string {
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>TARS Console</title>
    <style>
      :root { color-scheme: dark; font-family: ui-sans-serif, system-ui, sans-serif; }
      body { margin: 0; background: #0b1020; color: #e5edf5; }
      main { max-width: 820px; margin: 0 auto; padding: 48px 24px 72px; }
      h1 { margin: 0 0 12px; font-size: 32px; }
      p, li { line-height: 1.6; color: #b5c3d1; }
      code, pre { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
      pre { background: #121a30; border: 1px solid #26314d; border-radius: 12px; padding: 16px; overflow-x: auto; }
      .callout { background: #121a30; border: 1px solid #26314d; border-radius: 12px; padding: 18px; margin: 24px 0; }
      .muted { color: #8ea0b5; }
    </style>
  </head>
  <body>
    <main>
      <h1>TARS Console build required</h1>
      <p>The Go server is running, but the embedded Svelte console assets have not been built in this checkout yet.</p>
      <div class="callout">
        <p><strong>Build the console assets:</strong></p>
        <pre>%s</pre>
        <p class="muted">Then restart <code>tars serve</code> and refresh <code>/console</code>.</p>
      </div>
      <div class="callout">
        <p><strong>Or use the Vite dev server:</strong></p>
        <pre>cd frontend/console
npm install
npm run dev</pre>
        <p class="muted">Start the Go server with <code>%s=http://127.0.0.1:5173</code> to proxy the live frontend during development.</p>
      </div>
    </main>
  </body>
</html>
`, consoleBuildHint, consoleDevProxyEnv)
}
