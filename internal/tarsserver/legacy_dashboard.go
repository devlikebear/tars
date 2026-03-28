package tarsserver

import (
	"net/http"
	"strings"
)

func newLegacyDashboardRedirectHandler(console http.Handler, dashboard http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSpace(r.URL.Path)
		switch {
		case path == "/dashboards" || path == "/dashboards/":
			redirectToConsole(w, r, "/console")
			return
		case strings.HasPrefix(path, "/ui/projects/"):
			if strings.HasSuffix(path, "/stream") && dashboard != nil {
				dashboard.ServeHTTP(w, r)
				return
			}
			redirectPath := strings.TrimPrefix(path, "/ui/projects")
			if strings.TrimSpace(redirectPath) == "" || redirectPath == "/" {
				redirectPath = ""
			}
			redirectToConsole(w, r, "/console/projects"+redirectPath)
			return
		default:
			if dashboard != nil {
				dashboard.ServeHTTP(w, r)
				return
			}
			http.NotFound(w, r)
		}
	})
}

func redirectToConsole(w http.ResponseWriter, r *http.Request, targetPath string) {
	target := strings.TrimSpace(targetPath)
	if target == "" {
		target = "/console"
	}
	if raw := strings.TrimSpace(r.URL.RawQuery); raw != "" {
		target += "?" + raw
	}
	http.Redirect(w, r, target, http.StatusFound)
}
