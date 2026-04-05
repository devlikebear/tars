package tarsserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/rs/zerolog"
)

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	normalizedCode := strings.TrimSpace(code)
	if normalizedCode == "" {
		normalizedCode = strings.ToLower(strings.ReplaceAll(http.StatusText(status), " ", "_"))
	}
	normalizedMessage := strings.TrimSpace(message)
	if normalizedMessage == "" {
		normalizedMessage = normalizedCode
	}
	writeJSON(w, status, map[string]string{
		"error": normalizedMessage,
		"code":  normalizedCode,
	})
}

func writeMethodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(p)
	r.bytes += n
	return n, err
}

func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func requestDebugMiddleware(logger zerolog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		evt := logger.Debug().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", rec.status).
			Int("bytes", rec.bytes).
			Dur("latency", time.Since(start))
		if role := serverauth.RoleFromRequest(r); role != "" {
			evt = evt.Str("auth_role", role)
		}
		evt.Msg("http request")
	})
}
