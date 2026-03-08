package tarsserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/cron"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func TestRequireMethod_WritesMethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/test", nil)
	rec := httptest.NewRecorder()

	ok := requireMethod(rec, req, http.MethodGet)

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

	ok := decodeJSONBody(rec, req, &payload)

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

func TestParsePositiveLimit_WritesValidationError(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/test?limit=0", nil)
	rec := httptest.NewRecorder()

	limit, ok := parsePositiveLimit(rec, req, 50)

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

func TestSessionAPIHandler_CreateIsUnsupportedInSingleMainMode(t *testing.T) {
	handler := newSessionAPIHandler(session.NewStore(t.TempDir()), zerolog.New(io.Discard))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader("{"))

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%q", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["error"] != "single-main-session mode is enabled" {
		t.Fatalf("unexpected body: %+v", body)
	}
}

func TestCronAPIHandler_CreateRejectsInvalidBody(t *testing.T) {
	handler := newCronAPIHandlerWithRunner(cron.NewStore(t.TempDir()), nil, zerolog.New(io.Discard))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/cron/jobs", strings.NewReader("{"))

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%q", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["error"] != "invalid request body" {
		t.Fatalf("unexpected body: %+v", body)
	}
}
