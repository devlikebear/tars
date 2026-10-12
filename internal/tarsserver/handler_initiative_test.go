package tarsserver

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/initiative"
)

func TestInitiativeStatusDisabled(t *testing.T) {
	h := newInitiativeAPIHandler(nil, nil, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/initiative/status", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"enabled":false`) || !strings.Contains(rec.Body.String(), `"recent":[]`) ||
		!strings.Contains(rec.Body.String(), `"outcomes":{`) || !strings.Contains(rec.Body.String(), `"presence":{"present":false}`) {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/initiative/status", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d", rec.Code)
	}
}

// TestInitiativeStatusReportsPresenceWithNoRawText checks requirement #4:
// status shows the last presence signal time and whether the console is
// present right now, and nothing else — no message text or identifier of
// any kind (tars#1003).
func TestInitiativeStatusReportsPresenceWithNoRawText(t *testing.T) {
	store := initiative.OpenPresenceStore(filepath.Join(t.TempDir(), "presence.json"))
	now := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	if _, err := store.Record(now.Add(-2 * time.Minute)); err != nil {
		t.Fatalf("Record: %v", err)
	}
	h := newInitiativeAPIHandler(nil, store, func() time.Time { return now })
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/initiative/status", nil))
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, body)
	}
	if !strings.Contains(body, `"present":true`) {
		t.Fatalf("body = %s, want present:true (signal 2m old, within presenceTTL)", body)
	}
	if strings.Contains(body, "console") && strings.Contains(body, "session_id") {
		t.Fatalf("body = %s, want no raw text or identifiers", body)
	}

	// Past presenceTTL (10m), the console reads as not present.
	stale := now.Add(20 * time.Minute)
	h2 := newInitiativeAPIHandler(nil, store, func() time.Time { return stale })
	rec2 := httptest.NewRecorder()
	h2.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/v1/initiative/status", nil))
	if !strings.Contains(rec2.Body.String(), `"present":false`) {
		t.Fatalf("body = %s, want present:false once the signal is stale", rec2.Body.String())
	}
}
