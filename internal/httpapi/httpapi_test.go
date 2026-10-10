package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequireMethod_WritesMethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/test", nil)
	rec := httptest.NewRecorder()

	ok := RequireMethod(rec, req, http.MethodGet)

	if ok {
		t.Fatal("expected requireMethod to reject disallowed method")
	}
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
	if rec.Body.String() != "method not allowed\n" {
		t.Fatalf("expected plain text method-not-allowed body, got %q", rec.Body.String())
	}
}

func TestDecodeJSONBody_WritesInvalidRequestBodyError(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/test", strings.NewReader("{"))
	rec := httptest.NewRecorder()
	var payload struct {
		Name string `json:"name"`
	}

	ok := DecodeJSONBody(rec, req, &payload)

	if ok {
		t.Fatal("expected decodeJSONBody to fail for invalid JSON")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["error"] != "invalid request body" {
		t.Fatalf("expected invalid request body error, got %+v", body)
	}
}

func TestDecodeJSONBodyWithLimit_WritesRequestEntityTooLarge(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/test", strings.NewReader(`{"name":"too-long"}`))
	rec := httptest.NewRecorder()
	var payload struct {
		Name string `json:"name"`
	}

	ok := DecodeJSONBodyWithLimit(rec, req, &payload, 8, false)

	if ok {
		t.Fatal("expected decodeJSONBodyWithLimit to reject oversized body")
	}
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["error"] != "request body too large" {
		t.Fatalf("expected request body too large error, got %+v", body)
	}
}

func TestDecodeJSONBodyWithLimit_AllowsOptionalEOF(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/test", strings.NewReader(""))
	rec := httptest.NewRecorder()
	var payload struct {
		Name string `json:"name"`
	}

	ok := DecodeJSONBodyWithLimit(rec, req, &payload, 8, true)

	if !ok {
		t.Fatal("expected decodeJSONBodyWithLimit to allow empty body when EOF is permitted")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected recorder to stay untouched, got %d", rec.Code)
	}
}

func TestParsePositiveLimit_WritesValidationError(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/test?limit=0", nil)
	rec := httptest.NewRecorder()

	limit, ok := ParsePositiveLimit(rec, req, 50)

	if ok {
		t.Fatal("expected parsePositiveLimit to reject non-positive limit")
	}
	if limit != 0 {
		t.Fatalf("expected limit=0 on failure, got %d", limit)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["error"] != "limit must be a positive integer" {
		t.Fatalf("unexpected body: %+v", body)
	}
}

func TestWriteError_FillsCodeAndMessage(t *testing.T) {
	for _, tc := range []struct {
		name          string
		status        int
		code, message string
		wantCode      string
		wantMessage   string
	}{
		{"both given", http.StatusBadRequest, "bad_goal", "goal is required", "bad_goal", "goal is required"},
		{"code from status text", http.StatusNotFound, " ", "no such session", "not_found", "no such session"},
		{"message from code", http.StatusConflict, "busy", "", "busy", "busy"},
		{"both from status text", http.StatusServiceUnavailable, "", "", "service_unavailable", "service_unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteError(rec, tc.status, tc.code, tc.message)
			if rec.Code != tc.status || rec.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("status = %d, content type = %q", rec.Code, rec.Header().Get("Content-Type"))
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body["code"] != tc.wantCode || body["error"] != tc.wantMessage {
				t.Fatalf("body = %v", body)
			}
		})
	}
}

func TestWriteMethodNotAllowedAndUnavailable(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteMethodNotAllowed(rec)
	if rec.Code != http.StatusMethodNotAllowed || !strings.Contains(rec.Body.String(), `"code":"method_not_allowed"`) {
		t.Fatalf("method not allowed: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	WriteUnavailable(rec, "  ledger is off  ")
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"error":"ledger is off"`) {
		t.Fatalf("unavailable: %d %s", rec.Code, rec.Body.String())
	}
}

func TestRequireMethod_AllowsListedMethodAndRejectsNilRequest(t *testing.T) {
	rec := httptest.NewRecorder()
	if !RequireMethod(rec, httptest.NewRequest(http.MethodPut, "/v1/test", nil), http.MethodGet, http.MethodPut) {
		t.Fatal("a listed method was rejected")
	}
	rec = httptest.NewRecorder()
	if RequireMethod(rec, nil, http.MethodGet) || rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("nil request: code = %d", rec.Code)
	}
}

func TestParsePositiveLimit_DefaultAndValue(t *testing.T) {
	rec := httptest.NewRecorder()
	if got, ok := ParsePositiveLimit(rec, httptest.NewRequest(http.MethodGet, "/v1/test", nil), 25); !ok || got != 25 {
		t.Fatalf("missing limit = %d, %v", got, ok)
	}
	if got, ok := ParsePositiveLimit(rec, httptest.NewRequest(http.MethodGet, "/v1/test?limit=7", nil), 25); !ok || got != 7 {
		t.Fatalf("limit=7 gave %d, %v", got, ok)
	}
}
