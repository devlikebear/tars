package apihandlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/remoteaccess"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/usage"
	"github.com/rs/zerolog"
)

// The answers a route gives when it cannot do its work: nothing configured,
// a request it cannot read, a backend that fails.

type errorCase struct {
	name, method, target, body string
	want                       int
	wantIn                     string
}

func runErrorCases(t *testing.T, handler http.Handler, cases []errorCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.target, strings.NewReader(tc.body)))
			if rec.Code != tc.want || !strings.Contains(rec.Body.String(), tc.wantIn) {
				t.Fatalf("got %d %q, want %d containing %q", rec.Code, rec.Body.String(), tc.want, tc.wantIn)
			}
		})
	}
}

func TestUsageAPI_WithoutATracker(t *testing.T) {
	handler := NewUsageHandler(nil, "off", zerolog.Nop())
	const missing = "usage tracker is not configured"
	runErrorCases(t, handler, []errorCase{
		{"summary", http.MethodGet, "/v1/usage/summary", "", http.StatusInternalServerError, missing},
		{"limits", http.MethodGet, "/v1/usage/limits", "", http.StatusInternalServerError, missing},
		{"today", http.MethodGet, "/v1/admin/usage/today", "", http.StatusInternalServerError, missing},
		{"analytics", http.MethodGet, "/v1/admin/analytics", "", http.StatusInternalServerError, missing},
		{"signals", http.MethodGet, "/v1/usage/signals", "", http.StatusInternalServerError, missing},
	})
}

func TestUsageAPI_RejectsWhatItCannotRead(t *testing.T) {
	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	tracker, err := usage.NewTracker(t.TempDir(), usage.TrackerOptions{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}
	handler := NewUsageHandler(tracker, "off", zerolog.Nop())
	const days = "days must be one of 7, 30, or 90"
	runErrorCases(t, handler, []errorCase{
		{"a period that does not exist", http.MethodGet, "/v1/usage/summary?period=fortnight", "", http.StatusBadRequest, "error"},
		{"days that are not a number", http.MethodGet, "/v1/admin/analytics?days=many", "", http.StatusBadRequest, days},
		{"days outside the three windows", http.MethodGet, "/v1/admin/analytics?days=5", "", http.StatusBadRequest, days},
		{"signals for a period that does not exist", http.MethodGet, "/v1/usage/signals?period=fortnight", "", http.StatusBadRequest, "error"},
	})

	// With auth on, only the admin role may change the limits.
	guarded := NewUsageHandler(tracker, "required", zerolog.Nop())
	runErrorCases(t, guarded, []errorCase{
		{"limits changed without the admin role", http.MethodPatch, "/v1/usage/limits", `{"daily_usd":1}`, http.StatusForbidden, "forbidden"},
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/usage/limits", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "daily_usd") {
		t.Fatalf("limits: %d %q", rec.Code, rec.Body.String())
	}
}

func newRemoteAccessErrorHandler(t *testing.T, outputs map[string]string) http.Handler {
	t.Helper()
	return NewRemoteAccessHandler(RemoteAccessOptions{
		Config: config.Config{
			RuntimeConfig: config.RuntimeConfig{WorkspaceDir: t.TempDir()},
			APIConfig:     config.APIConfig{APIAuthMode: "off"},
			RemoteAccessConfig: config.RemoteAccessConfig{
				RemoteAccessTailscaleServeHTTPSPort: remoteaccess.DefaultHTTPSPort,
			},
		},
		ConfigPath: t.TempDir() + "/config.yaml",
		Logger:     zerolog.Nop(),
		Runner:     newRemoteAccessTestRunner(outputs),
	})
}

func TestRemoteAccessAPI_StatusAndUnreadableRequests(t *testing.T) {
	handler := newRemoteAccessErrorHandler(t, map[string]string{
		"tailscale status --json":       `{"BackendState":"Running","Self":{"HostName":"mac","DNSName":"mac.tail.ts.net."}}`,
		"tailscale serve status --json": `{}`,
	})
	const invalid = "invalid_remote_access_request"
	runErrorCases(t, handler, []errorCase{
		{"status", http.MethodGet, "/v1/admin/remote-access/status", "", http.StatusOK, "checks"},
		{"status with the wrong method", http.MethodPost, "/v1/admin/remote-access/status", "", http.StatusMethodNotAllowed, "method not allowed"},
		{"enable with a body that is not JSON", http.MethodPost, "/v1/admin/remote-access/enable", "not json", http.StatusBadRequest, invalid},
		{"disable with a body that is not JSON", http.MethodPost, "/v1/admin/remote-access/disable", "not json", http.StatusBadRequest, invalid},
	})
}

// With no tailscale to answer, every route reports the failed status read
// instead of acting on a state it could not see.
func TestRemoteAccessAPI_WhenTailscaleCannotBeRead(t *testing.T) {
	handler := newRemoteAccessErrorHandler(t, map[string]string{})
	const failed = "remote_access_status_failed"
	runErrorCases(t, handler, []errorCase{
		{"status", http.MethodGet, "/v1/admin/remote-access/status", "", http.StatusBadGateway, failed},
		{"enable", http.MethodPost, "/v1/admin/remote-access/enable", "{}", http.StatusBadGateway, failed},
		{"disable", http.MethodPost, "/v1/admin/remote-access/disable", "{}", http.StatusBadGateway, failed},
	})
}

func TestGitAPI_ErrorAnswers(t *testing.T) {
	workspace := t.TempDir()
	notARepo := t.TempDir()
	store := session.NewStore(workspace)
	sess, err := store.Create("git")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := store.SetWorkDirs(sess.ID, []string{notARepo}, notARepo); err != nil {
		t.Fatalf("set workdirs: %v", err)
	}
	handler := NewGitHandler(workspace, store, nil, zerolog.New(io.Discard))
	runErrorCases(t, handler, []errorCase{
		{"log of a folder that is not a repository", http.MethodGet, "/v1/git/log?session_id=" + sess.ID, "", http.StatusNotFound, "not a git repository"},
		{"a mutation with no approval queue", http.MethodPost, "/v1/git/mutations", "{}", http.StatusInternalServerError, "ops manager is not configured"},
	})
}
