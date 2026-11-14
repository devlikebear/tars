package apihandlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/remoteaccess"
)

func TestReadinessOf(t *testing.T) {
	serving := remoteaccess.Status{Installed: true, LoggedIn: true, TailnetURL: "mac.tail1.ts.net.", ServeActive: true, OwnedByTARS: true}
	states := func(checks []ReadinessCheck) [3]string {
		return [3]string{checks[0].State, checks[1].State, checks[2].State}
	}
	notFound := errors.New("claude not found")
	for _, tc := range []struct {
		name   string
		kind   string
		cliErr error
		gh     string
		ts     remoteaccess.Status
		tsErr  error
		want   [3]string
	}{
		{"all ready", "claude-code-cli", nil, "ok", serving, nil, [3]string{"ok", "ok", "ok"}},
		{"API provider needs no binary", "anthropic", notFound, "ok", serving, nil, [3]string{"ok", "ok", "ok"}},
		{"CLI provider without its binary", "claude-code-cli", notFound, "ok", serving, nil, [3]string{"missing", "ok", "ok"}},
		{"antigravity without agy", "antigravity-cli", notFound, "ok", serving, nil, [3]string{"missing", "ok", "ok"}},
		{"no chat tier", "", nil, "ok", serving, nil, [3]string{"missing", "ok", "ok"}},
		{"gh signed out", "anthropic", nil, "signed_out", serving, nil, [3]string{"ok", "signed_out", "ok"}},
		{"gh missing", "anthropic", nil, "missing", serving, nil, [3]string{"ok", "missing", "ok"}},
		{"gh says something else", "anthropic", nil, "weird", serving, nil, [3]string{"ok", "unknown", "ok"}},
		{"tailscale not installed", "anthropic", nil, "ok", remoteaccess.Status{}, nil, [3]string{"ok", "ok", "missing"}},
		{"tailscale signed out", "anthropic", nil, "ok", remoteaccess.Status{Installed: true}, nil, [3]string{"ok", "ok", "signed_out"}},
		{"serve off", "anthropic", nil, "ok", remoteaccess.Status{Installed: true, LoggedIn: true, TailnetURL: "mac.ts.net"}, nil, [3]string{"ok", "ok", "not_serving"}},
		{"serve for something else", "anthropic", nil, "ok", remoteaccess.Status{Installed: true, LoggedIn: true, TailnetURL: "mac.ts.net", ServeActive: true}, nil, [3]string{"ok", "ok", "not_serving"}},
		{"tailscale failed", "anthropic", nil, "ok", remoteaccess.Status{Installed: true}, errors.New("daemon down"), [3]string{"ok", "ok", "unknown"}},
	} {
		got := ReadinessOf(tc.kind, tc.cliErr, tc.gh, tc.ts, tc.tsErr)
		if states(got) != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, states(got), tc.want)
		}
		if got[0].ID != "model" || got[1].ID != "gh" || got[2].ID != "tailscale" {
			t.Errorf("%s: ids %+v", tc.name, got)
		}
	}
	if got := ReadinessOf(" Claude-Code-CLI ", nil, "ok", serving, nil); got[0].Kind != "claude-code-cli" {
		t.Errorf("kind = %q", got[0].Kind)
	}
}

func TestFocusReadinessHandlerCaches(t *testing.T) {
	now := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	calls := map[string]int{}
	h := NewFocusReadinessHandler(ReadinessProbes{
		ModelKind: func() string { calls["model"]++; return "claude-code-cli" },
		FindCLI:   func(string) error { calls["cli"]++; return nil },
		GH:        func(context.Context) string { calls["gh"]++; return "signed_out" },
		Tailscale: func(context.Context) (remoteaccess.Status, error) {
			calls["tailscale"]++
			return remoteaccess.Status{Installed: true, LoggedIn: true}, nil
		},
		Now: func() time.Time { return now },
	})
	read := func() readinessAnswer {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/focus/readiness", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		var answer readinessAnswer
		if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil {
			t.Fatal(err)
		}
		return answer
	}
	first := read()
	if len(first.Checks) != 3 || first.Checks[1].State != "signed_out" || first.Checks[2].State != "not_serving" || !first.CheckedAt.Equal(now) {
		t.Fatalf("answer = %+v", first)
	}
	now = now.Add(time.Minute)
	read()
	if calls["gh"] != 1 || calls["tailscale"] != 1 {
		t.Fatalf("read again inside the TTL: %v", calls)
	}
	now = now.Add(readinessTTL)
	if again := read(); !again.CheckedAt.Equal(now) || calls["gh"] != 2 {
		t.Fatalf("not read again after the TTL: %v %+v", calls, again)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/focus/readiness", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("post: %d", rec.Code)
	}
}

func TestFocusReadinessWithoutProbes(t *testing.T) {
	rec := httptest.NewRecorder()
	NewFocusReadinessHandler(ReadinessProbes{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/focus/readiness", nil))
	var answer readinessAnswer
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if got := [3]string{answer.Checks[0].State, answer.Checks[1].State, answer.Checks[2].State}; got != [3]string{"missing", "unknown", "unknown"} {
		t.Fatalf("no probes: %v", got)
	}
}
