package tarsserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInitiativeStatusDisabled(t *testing.T) {
	h := newInitiativeAPIHandler(nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/initiative/status", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"enabled":false`) || !strings.Contains(rec.Body.String(), `"recent":[]`) {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/initiative/status", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d", rec.Code)
	}
}
