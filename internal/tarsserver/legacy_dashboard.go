package tarsserver

import (
	"net/http"
	"strings"
)

func newLegacyDashboardRedirectHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSpace(r.URL.Path)
		switch {
		case path == "/dashboards" || path == "/dashboards/":
			redirectToConsole(w, r, "/console")
		default:
			redirectToConsole(w, r, "/console")
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
