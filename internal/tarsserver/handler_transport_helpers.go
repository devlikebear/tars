package tarsserver

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

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
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
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
