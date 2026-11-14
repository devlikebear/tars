package serverauth

import (
	"net/http"
	"strings"
)

// DefaultAppOrigins are the origins the mobile app's web view runs on
// (docs/decisions/mobile-remote.md §5): Capacitor serves its bundle from
// capacitor://localhost on iOS and https://localhost on Android.
var DefaultAppOrigins = []string{"capacitor://localhost", "https://localhost"}

const appCORSMaxAge = "600"

// NewAppCORS answers CORS for the mobile app only: an allowed origin, on a
// route a paired device may call (focusEndpointRules) or the pairing route.
// It never allows credentials — the app sends a bearer token, never the
// console's cookie — and leaves every other request exactly as it was, so
// the console's own same-origin requests and other origins are unaffected.
// It sits outside the auth middleware: a preflight carries no token.
func NewAppCORS(origins []string) func(http.Handler) http.Handler {
	allowed := map[string]struct{}{}
	for _, origin := range origins {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			allowed[origin] = struct{}{}
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := strings.TrimSpace(r.Header.Get("Origin"))
			if _, ok := allowed[origin]; !ok || origin == "" {
				next.ServeHTTP(w, r)
				return
			}
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				method := strings.ToUpper(strings.TrimSpace(r.Header.Get("Access-Control-Request-Method")))
				if !appRouteAllowed(method, r.URL.Path) {
					next.ServeHTTP(w, r)
					return
				}
				h := w.Header()
				h.Add("Vary", "Origin")
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Allow-Methods", method)
				h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				h.Set("Access-Control-Max-Age", appCORSMaxAge)
				w.WriteHeader(http.StatusNoContent)
				return
			}
			if appRouteAllowed(r.Method, r.URL.Path) {
				w.Header().Add("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// appRouteAllowed reports whether the app may call method path across
// origins: a route a paired device reaches, or trading a pairing code for
// its token.
func appRouteAllowed(method, path string) bool {
	if method == http.MethodPost && normalizeEndpointPath(path) == "/v1/auth/device-pair" {
		return true
	}
	return focusEndpointAllows(method, path)
}
