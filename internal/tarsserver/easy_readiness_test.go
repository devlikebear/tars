package tarsserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/easyproject"
)

func TestEasyReadinessReadsTheEngineAndKeepsTheAnswerBriefly(t *testing.T) {
	now := time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC)
	auth := `{"loggedIn": false}`
	runs := 0
	h := &easyReadiness{
		kind:    func() string { return "Claude-Code-CLI " },
		findCLI: func(kind string) (string, error) { return "/bin/claude", nil },
		runCLI: func(_ context.Context, path string, args ...string) ([]byte, error) {
			runs++
			if path != "/bin/claude" || len(args) != 3 || args[0] != "auth" {
				t.Fatalf("ran %s %v", path, args)
			}
			// A logged-out CLI exits non-zero and still prints its answer.
			return []byte(auth), errors.New("exit status 1")
		},
		toolsReady: func() error { return nil },
		now:        func() time.Time { return now },
	}
	read := func() easyproject.Readiness {
		rec := httptest.NewRecorder()
		h.serve(rec, httptest.NewRequest(http.MethodGet, "/v1/easy/readiness", nil))
		var body struct {
			Readiness easyproject.Readiness `json:"readiness"`
		}
		decodeInto(t, rec, &body)
		return body.Readiness
	}
	if got := read(); got.Next != easyproject.NextSignIn || got.Ready {
		t.Fatalf("signed out = %+v", got)
	}
	auth = `{"loggedIn": true, "authMethod": "claude.ai"}`
	if got := read(); got.Next != easyproject.NextSignIn || runs != 1 {
		t.Fatalf("within the TTL the answer is kept: %+v, %d runs", got, runs)
	}
	now = now.Add(easyReadinessTTL)
	if got := read(); !got.Ready || runs != 2 {
		t.Fatalf("after the TTL = %+v, %d runs", got, runs)
	}
}

func TestEasyReadinessWithoutTheEngineOrTools(t *testing.T) {
	h := &easyReadiness{
		kind:       func() string { return "claude-code-cli" },
		findCLI:    func(string) (string, error) { return "", errors.New("not found") },
		runCLI:     func(context.Context, string, ...string) ([]byte, error) { panic("nothing to run") },
		toolsReady: func() error { return easyproject.ErrNodeMissing },
		now:        time.Now,
	}
	got := h.read(context.Background())
	if got.Engine != easyproject.CheckMissing || got.Tools != easyproject.CheckMissing || got.Next != easyproject.NextInstallEngine {
		t.Fatalf("got %+v", got)
	}
}

func TestEasyReadinessRoute(t *testing.T) {
	t.Setenv(easyproject.ScaffoldPathEnv, "/no/such/scaffold")
	h := newEasyReadinessHandler(config.Config{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/easy/readiness", nil))
	var body struct {
		Readiness easyproject.Readiness `json:"readiness"`
	}
	decodeInto(t, rec, &body)
	if rec.Code != http.StatusOK || body.Readiness.Tools != easyproject.CheckMissing {
		t.Fatalf("%d %+v", rec.Code, body.Readiness)
	}
}

func TestParseClaudeAuthStatus(t *testing.T) {
	if in, method := parseClaudeAuthStatus([]byte(" {\"loggedIn\": true, \"authMethod\": \" claude.ai \"}\n")); in == nil || !*in || method != "claude.ai" {
		t.Fatalf("logged in = %v %q", in, method)
	}
	for _, out := range []string{"", "error: unknown command 'auth'", `{"other": 1}`} {
		if in, _ := parseClaudeAuthStatus([]byte(out)); in != nil {
			t.Fatalf("%q = %v", out, *in)
		}
	}
}
