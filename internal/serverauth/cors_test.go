package serverauth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func corsHandler() (http.Handler, *int) {
	reached := 0
	h := NewAppCORS(append([]string{" "}, DefaultAppOrigins...))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached++
		w.WriteHeader(http.StatusTeapot)
	}))
	return h, &reached
}

func corsRequest(h http.Handler, method, path, origin, preflightMethod string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if preflightMethod != "" {
		req.Header.Set("Access-Control-Request-Method", preflightMethod)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAppCORSPreflight(t *testing.T) {
	h, reached := corsHandler()
	rec := corsRequest(h, http.MethodOptions, "/v1/focus/pipelines/s1/gates/plan", "capacitor://localhost", http.MethodPost)
	if rec.Code != http.StatusNoContent || *reached != 0 {
		t.Fatalf("preflight: code=%d reached=%d", rec.Code, *reached)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "capacitor://localhost" {
		t.Fatalf("allow origin = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != http.MethodPost {
		t.Fatalf("allow methods = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Authorization, Content-Type" {
		t.Fatalf("allow headers = %q", got)
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatalf("credentials must never be allowed")
	}

	// Pairing is the one route outside the device table.
	if rec := corsRequest(h, http.MethodOptions, "/v1/auth/device-pair", "https://localhost", http.MethodPost); rec.Code != http.StatusNoContent {
		t.Fatalf("pairing preflight: %d", rec.Code)
	}
}

func TestAppCORSLeavesOtherRequestsAlone(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, origin, preflight string
	}{
		{"unknown origin preflight", http.MethodOptions, "/v1/focus/pipelines", "https://evil.example", http.MethodGet},
		{"route outside the device table", http.MethodOptions, "/v1/chat", "capacitor://localhost", http.MethodPost},
		{"method outside the device table", http.MethodOptions, "/v1/focus/pipelines/s1", "capacitor://localhost", http.MethodDelete},
		{"no origin", http.MethodGet, "/v1/focus/pipelines", "", ""},
		{"plain OPTIONS without a preflight method", http.MethodOptions, "/v1/focus/pipelines", "capacitor://localhost", ""},
	} {
		h, reached := corsHandler()
		rec := corsRequest(h, tc.method, tc.path, tc.origin, tc.preflight)
		if *reached != 1 || rec.Code != http.StatusTeapot {
			t.Errorf("%s: must reach the next handler unchanged (code=%d reached=%d)", tc.name, rec.Code, *reached)
		}
		if rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("%s: must not add CORS headers", tc.name)
		}
	}
}

func TestAppCORSActualRequests(t *testing.T) {
	h, reached := corsHandler()
	rec := corsRequest(h, http.MethodGet, "/v1/focus/pipelines", "https://localhost", "")
	if *reached != 1 || rec.Header().Get("Access-Control-Allow-Origin") != "https://localhost" || rec.Header().Get("Vary") != "Origin" {
		t.Fatalf("allowed request: reached=%d headers=%v", *reached, rec.Header())
	}

	h, reached = corsHandler()
	rec = corsRequest(h, http.MethodGet, "/v1/status", "capacitor://localhost", "")
	if *reached != 1 || rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("route outside the table must not get CORS headers: %v", rec.Header())
	}
}
