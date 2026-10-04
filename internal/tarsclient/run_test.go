package tarsclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_OneShot(t *testing.T) {
	for _, verbose := range []bool{false, true} {
		t.Run(fmt.Sprintf("verbose=%t", verbose), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/v1/chat" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
					return
				}
				if r.Header.Get("Authorization") != "Bearer user-token" {
					t.Errorf("unexpected authorization header")
				}
				var req chatRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Errorf("decode request: %v", err)
				}
				if req.Message != "hello" || req.SessionID != "s-1" {
					t.Errorf("unexpected request: %+v", req)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"type\":\"status\",\"message\":\"working\"}\n\n")
				fmt.Fprint(w, "data: {\"type\":\"delta\",\"text\":\"hello \"}\n\n")
				fmt.Fprint(w, "data: {\"type\":\"delta\",\"text\":\"world\"}\n\n")
				fmt.Fprint(w, "data: {\"type\":\"done\",\"session_id\":\"s-1\"}\n\n")
			}))
			defer server.Close()
			var stdout, stderr bytes.Buffer
			err := Run(context.Background(), nil, &stdout, &stderr, Options{
				ServerURL: server.URL, APIToken: "user-token", SessionID: " s-1 ", Message: "hello", Verbose: verbose,
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if got := stdout.String(); got != "TARS > hello world\n" {
				t.Fatalf("unexpected stdout: %q", got)
			}
			want := "session=s-1\n"
			if verbose {
				want = "status: working\n" + want
			}
			if got := stderr.String(); got != want {
				t.Fatalf("unexpected stderr: %q; want %q", got, want)
			}
		})
	}
}

func TestRun_WithoutMessage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), strings.NewReader("/help\n"), &stdout, &stderr, Options{Message: " "})
	if err == nil || !strings.Contains(err.Error(), "interactive terminal UI has been removed") {
		t.Fatalf("expected removed UI error, got %v", err)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("unexpected output: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}
