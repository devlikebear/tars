package tarsserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
)

const defaultJSONBodyLimitBytes int64 = 10 << 20

func requireMethod(w http.ResponseWriter, r *http.Request, allowed ...string) bool {
	if r == nil {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	for _, method := range allowed {
		if r.Method == method {
			return true
		}
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	return false
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	return decodeJSONBodyWithLimit(w, r, dst, defaultJSONBodyLimitBytes, false)
}

func decodeOptionalJSONBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	return decodeJSONBodyWithLimit(w, r, dst, defaultJSONBodyLimitBytes, true)
}

func decodeJSONBodyWithLimit(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64, allowEOF bool) bool {
	body := r.Body
	if maxBytes > 0 {
		body = http.MaxBytesReader(w, r.Body, maxBytes)
	}
	if err := json.NewDecoder(body).Decode(dst); err != nil {
		if allowEOF && errors.Is(err, io.EOF) {
			return true
		}
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "request body too large"})
			return false
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return false
	}
	return true
}

func parsePositiveLimit(w http.ResponseWriter, r *http.Request, defaultLimit int) (int, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return defaultLimit, true
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be a positive integer"})
		return 0, false
	}
	return limit, true
}

func writeUnavailable(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": strings.TrimSpace(message)})
}
