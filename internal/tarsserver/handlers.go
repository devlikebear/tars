package tarsserver

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/rs/zerolog"
)

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

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := r.ResponseWriter.(http.Hijacker); ok {
		if r.status == 0 {
			r.status = http.StatusSwitchingProtocols
		}
		return hijacker.Hijack()
	}
	return nil, nil, errors.New("response writer does not support hijacking")
}

func requestDebugMiddleware(logger zerolog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		logger.Debug().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Msg("http request started")
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
