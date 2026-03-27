package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type FailureMode string

const (
	FailureModeNone    FailureMode = "none"
	FailureModeTimeout FailureMode = "timeout"
	FailureModeHTTP500 FailureMode = "http500"
)

type Service struct {
	mu         sync.RWMutex
	failure    FailureMode
	errorCount int
	logWriter  io.Writer
	nowFn      func() time.Time
	handler    http.Handler
}

func New(logWriter io.Writer, nowFn func() time.Time) *Service {
	if logWriter == nil {
		logWriter = io.Discard
	}
	if nowFn == nil {
		nowFn = time.Now
	}
	svc := &Service{
		failure:   FailureModeNone,
		logWriter: logWriter,
		nowFn:     nowFn,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", svc.handleIndex)
	mux.HandleFunc("/healthz", svc.handleHealth)
	mux.HandleFunc("/admin/failure", svc.handleFailure)
	mux.HandleFunc("/admin/failure/clear", svc.handleClearFailure)
	svc.handler = mux
	return svc
}

func (s *Service) Handler() http.Handler {
	if s == nil || s.handler == nil {
		return http.NewServeMux()
	}
	return s.handler
}

func (s *Service) FailureMode() FailureMode {
	if s == nil {
		return FailureModeNone
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.failure
}

func (s *Service) SetFailureMode(mode FailureMode) error {
	normalized, err := normalizeFailureMode(mode)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.failure = normalized
	s.errorCount = 0
	s.mu.Unlock()
	s.log("info", "failure_mode_updated", "failure mode updated", map[string]any{
		"failure_mode": normalized,
	})
	return nil
}

func (s *Service) RunSyntheticCheck() {
	if s == nil {
		return
	}
	mode := s.FailureMode()
	if mode == FailureModeNone {
		s.log("info", "synthetic_probe_ok", "synthetic probe succeeded", map[string]any{
			"failure_mode": mode,
		})
		return
	}

	s.mu.Lock()
	s.errorCount++
	errorCount := s.errorCount
	s.mu.Unlock()

	s.log("error", "synthetic_probe_failed", failureMessage(mode), map[string]any{
		"failure_mode": mode,
		"error_count":  errorCount,
	})
}

func (s *Service) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	mode, errors := s.snapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"service":      "ops-service-demo",
		"status":       statusForFailureMode(mode),
		"failure_mode": mode,
		"error_count":  errors,
		"checked_at":   s.nowFn().UTC().Format(time.RFC3339),
	})
}

func (s *Service) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	mode, errors := s.snapshot()
	code := http.StatusOK
	if mode != FailureModeNone {
		code = http.StatusServiceUnavailable
	}
	writeJSON(w, code, map[string]any{
		"service":      "ops-service-demo",
		"status":       statusForFailureMode(mode),
		"failure_mode": mode,
		"error_count":  errors,
		"checked_at":   s.nowFn().UTC().Format(time.RFC3339),
	})
}

func (s *Service) handleFailure(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	mode := FailureMode(strings.TrimSpace(r.URL.Query().Get("mode")))
	if err := s.SetFailureMode(mode); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"service":      "ops-service-demo",
		"status":       statusForFailureMode(mode),
		"failure_mode": mode,
		"checked_at":   s.nowFn().UTC().Format(time.RFC3339),
	})
}

func (s *Service) handleClearFailure(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.SetFailureMode(FailureModeNone); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"service":      "ops-service-demo",
		"status":       statusForFailureMode(FailureModeNone),
		"failure_mode": FailureModeNone,
		"checked_at":   s.nowFn().UTC().Format(time.RFC3339),
	})
}

func (s *Service) snapshot() (FailureMode, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.failure, s.errorCount
}

func (s *Service) log(level, event, message string, extra map[string]any) {
	entry := map[string]any{
		"ts":      s.nowFn().UTC().Format(time.RFC3339),
		"level":   strings.TrimSpace(level),
		"event":   strings.TrimSpace(event),
		"service": "ops-service-demo",
		"message": strings.TrimSpace(message),
	}
	for key, value := range extra {
		entry[key] = value
	}
	data, err := json.Marshal(entry)
	if err != nil {
		_, _ = fmt.Fprintf(s.logWriter, "{\"level\":\"error\",\"event\":\"log_encode_failed\",\"message\":%q}\n", err.Error())
		return
	}
	_, _ = fmt.Fprintf(s.logWriter, "%s\n", data)
}

func normalizeFailureMode(mode FailureMode) (FailureMode, error) {
	value := FailureMode(strings.ToLower(strings.TrimSpace(string(mode))))
	switch value {
	case "", FailureModeNone:
		return FailureModeNone, nil
	case FailureModeTimeout:
		return FailureModeTimeout, nil
	case FailureModeHTTP500:
		return FailureModeHTTP500, nil
	default:
		return "", fmt.Errorf("unsupported failure mode %q", mode)
	}
}

func statusForFailureMode(mode FailureMode) string {
	if mode == FailureModeNone {
		return "ok"
	}
	return "degraded"
}

func failureMessage(mode FailureMode) string {
	switch mode {
	case FailureModeTimeout:
		return "upstream timeout while processing synthetic probe"
	case FailureModeHTTP500:
		return "downstream 500 while processing synthetic probe"
	default:
		return "synthetic probe succeeded"
	}
}

func writeJSON(w http.ResponseWriter, status int, body map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
