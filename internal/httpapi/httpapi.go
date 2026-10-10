// Package httpapi holds the request and response helpers every TARS HTTP
// handler uses: write a JSON body or error, check the method, decode a JSON
// request body within a size limit.
//
// It imports only the standard library so that a handler can live in any
// package. It was split out of internal/tarsserver (#1204): twenty-odd
// handler groups there depended on nothing else in the package.
package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// DefaultJSONBodyLimitBytes is the largest JSON request body DecodeJSONBody
// and DecodeOptionalJSONBody accept.
const DefaultJSONBodyLimitBytes int64 = 10 << 20

// WriteJSON writes body as JSON with the given status code.
func WriteJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

// WriteError writes {"error": message, "code": code}. An empty code becomes
// the status text in snake case; an empty message becomes the code.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	normalizedCode := strings.TrimSpace(code)
	if normalizedCode == "" {
		normalizedCode = strings.ToLower(strings.ReplaceAll(http.StatusText(status), " ", "_"))
	}
	normalizedMessage := strings.TrimSpace(message)
	if normalizedMessage == "" {
		normalizedMessage = normalizedCode
	}
	WriteJSON(w, status, map[string]string{
		"error": normalizedMessage,
		"code":  normalizedCode,
	})
}

// WriteMethodNotAllowed writes the JSON 405 error.
func WriteMethodNotAllowed(w http.ResponseWriter) {
	WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
}

// WriteUnavailable writes a 503 with {"error": message}.
func WriteUnavailable(w http.ResponseWriter, message string) {
	WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": strings.TrimSpace(message)})
}

// RequireMethod reports whether the request uses one of the allowed methods,
// and answers 405 in plain text when it does not.
func RequireMethod(w http.ResponseWriter, r *http.Request, allowed ...string) bool {
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

// DecodeJSONBody decodes the request body into dst. On failure it writes the
// error response and returns false.
func DecodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	return DecodeJSONBodyWithLimit(w, r, dst, DefaultJSONBodyLimitBytes, false)
}

// DecodeOptionalJSONBody is DecodeJSONBody for a body that may be empty.
func DecodeOptionalJSONBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	return DecodeJSONBodyWithLimit(w, r, dst, DefaultJSONBodyLimitBytes, true)
}

// DecodeJSONBodyWithLimit decodes at most maxBytes of the request body into
// dst (no limit when maxBytes is not positive). A body over the limit is
// answered with 413, an unreadable one with 400; allowEOF accepts an empty
// body.
func DecodeJSONBodyWithLimit(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64, allowEOF bool) bool {
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
			WriteJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "request body too large"})
			return false
		}
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return false
	}
	return true
}

// ParsePositiveLimit reads the "limit" query parameter. A missing one is
// defaultLimit; one that is not a positive integer is answered with 400.
func ParsePositiveLimit(w http.ResponseWriter, r *http.Request, defaultLimit int) (int, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return defaultLimit, true
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be a positive integer"})
		return 0, false
	}
	return limit, true
}
