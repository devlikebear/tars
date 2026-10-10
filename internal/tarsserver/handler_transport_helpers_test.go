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
