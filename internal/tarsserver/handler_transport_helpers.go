package tarsserver

import (
	"net/http"

	"github.com/devlikebear/tars/internal/httpapi"
)

// The request and response helpers live in internal/httpapi so a handler can
// move out of this package without taking the server with it (#1204). These
// names stay for the handlers that are still here.

const defaultJSONBodyLimitBytes = httpapi.DefaultJSONBodyLimitBytes

func requireMethod(w http.ResponseWriter, r *http.Request, allowed ...string) bool {
	return httpapi.RequireMethod(w, r, allowed...)
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	return httpapi.DecodeJSONBody(w, r, dst)
}

func decodeOptionalJSONBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	return httpapi.DecodeOptionalJSONBody(w, r, dst)
}

func decodeJSONBodyWithLimit(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64, allowEOF bool) bool {
	return httpapi.DecodeJSONBodyWithLimit(w, r, dst, maxBytes, allowEOF)
}

func parsePositiveLimit(w http.ResponseWriter, r *http.Request, defaultLimit int) (int, bool) {
	return httpapi.ParsePositiveLimit(w, r, defaultLimit)
}

func writeUnavailable(w http.ResponseWriter, message string) {
	httpapi.WriteUnavailable(w, message)
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	httpapi.WriteJSON(w, code, body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	httpapi.WriteError(w, status, code, message)
}

func writeMethodNotAllowed(w http.ResponseWriter) {
	httpapi.WriteMethodNotAllowed(w)
}
