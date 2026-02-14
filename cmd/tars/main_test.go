package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_Help(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := run([]string{"--help"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}

	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestRun_DefaultShowsHelp(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := run([]string{}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}

	if !strings.Contains(stdout.String(), "CLI client for TARS") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestRun_HeartbeatRunOnce(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/v1/heartbeat/run-once" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":"hello from server"}`))
	}))
	defer server.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := run([]string{"heartbeat", "run-once", "--server-url", server.URL}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "hello from server") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestRun_ChatMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/v1/chat" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		var req struct {
			SessionID string `json:"session_id"`
			Message   string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if req.SessionID != "sess-1" {
			t.Fatalf("expected session_id sess-1, got %q", req.SessionID)
		}
		if req.Message != "hello" {
			t.Fatalf("expected message hello, got %q", req.Message)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"status\",\"phase\":\"before_llm\",\"message\":\"calling llm\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"delta\",\"text\":\"Hello \"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"delta\",\"text\":\"from TARS\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"done\",\"session_id\":\"sess-1\"}\n\n"))
	}))
	defer server.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := run([]string{"chat", "-m", "hello", "--session", "sess-1", "--server-url", server.URL}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Hello from TARS") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "[status] calling llm") {
		t.Fatalf("expected status stream in stderr, got %q", stderr.String())
	}
}

func TestRun_VerboseLogFile(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "logs", "tars-debug.log")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := runWithIO([]string{"--verbose", "--log-file", logPath, "--help"}, strings.NewReader(""), stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(raw), "verbose logging enabled") {
		t.Fatalf("expected verbose log record in log file, got %q", string(raw))
	}
	if !strings.Contains(stderr.String(), "[debug] verbose logs ->") {
		t.Fatalf("expected log-file notice in stderr, got %q", stderr.String())
	}
}

func TestRun_ChatMessage_Pretty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"delta\",\"text\":\"Hello from TARS\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"done\",\"session_id\":\"sess-p\"}\n\n"))
	}))
	defer server.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := run([]string{"chat", "-m", "hello", "--pretty", "--server-url", server.URL}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "You > hello") {
		t.Fatalf("expected pretty user line, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "TARS > Hello from TARS") {
		t.Fatalf("expected pretty assistant line, got %q", stdout.String())
	}
}

func TestRun_ChatREPL(t *testing.T) {
	callCount := 0
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := strings.NewReader("hello\n/quit\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"delta\",\"text\":\"reply\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"done\",\"session_id\":\"sess-repl\"}\n\n"))
	}))
	defer server.Close()

	code := runWithIO([]string{"chat", "--server-url", server.URL}, stdin, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}
	if callCount != 1 {
		t.Fatalf("expected one chat api call, got %d", callCount)
	}
	if !strings.Contains(stdout.String(), "Entering chat REPL") {
		t.Fatalf("expected repl banner, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "reply") {
		t.Fatalf("expected repl response in stdout, got %q", stdout.String())
	}
}

func TestRun_ChatREPL_ReusesSessionID(t *testing.T) {
	callCount := 0
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := strings.NewReader("first\nsecond\n/exit\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req struct {
			SessionID string `json:"session_id"`
			Message   string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		if callCount == 1 {
			if req.SessionID != "" {
				t.Fatalf("expected empty session_id for first message, got %q", req.SessionID)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"type\":\"delta\",\"text\":\"first-reply\"}\n\n"))
			_, _ = w.Write([]byte("data: {\"type\":\"done\",\"session_id\":\"sess-42\"}\n\n"))
			return
		}

		if req.SessionID != "sess-42" {
			t.Fatalf("expected session_id sess-42 for second message, got %q", req.SessionID)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"delta\",\"text\":\"second-reply\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"done\",\"session_id\":\"sess-42\"}\n\n"))
	}))
	defer server.Close()

	code := runWithIO([]string{"chat", "--server-url", server.URL}, stdin, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}
	if callCount != 2 {
		t.Fatalf("expected two chat api calls, got %d", callCount)
	}
	if !strings.Contains(stdout.String(), "first-reply") || !strings.Contains(stdout.String(), "second-reply") {
		t.Fatalf("expected both replies in stdout, got %q", stdout.String())
	}
}

func TestRun_ChatREPL_SlashCommands(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := strings.NewReader("/sessions\n/new project session\n/history\n/export\n/search alpha\n/status\n/compact\n/quit\n")

	var historyPathRequested bool
	var exportPathRequested bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id":"sess-a","title":"alpha"}]`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"sess-new","title":"project session"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions/sess-new/history":
			historyPathRequested = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"role":"user","content":"hello","timestamp":"2026-02-14T12:00:00Z"}]`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions/sess-new/export":
			exportPathRequested = true
			w.Header().Set("Content-Type", "text/markdown")
			_, _ = w.Write([]byte("# Session: project session\n\n**user**: hello"))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions/search":
			if got := strings.TrimSpace(r.URL.Query().Get("q")); got != "alpha" {
				t.Fatalf("expected search keyword alpha, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id":"sess-s","title":"alpha result"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/status":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"workspace_dir":"./workspace","session_count":3}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/compact":
			var req struct {
				SessionID string `json:"session_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode compact request: %v", err)
			}
			if req.SessionID != "sess-new" {
				t.Fatalf("expected compact session_id sess-new, got %q", req.SessionID)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"message":"compaction complete (session=sess-new compacted=3 final=4)"}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	code := runWithIO([]string{"chat", "--server-url", server.URL}, stdin, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}
	if !historyPathRequested {
		t.Fatalf("expected history endpoint call for created session")
	}
	if !exportPathRequested {
		t.Fatalf("expected export endpoint call for created session")
	}
	out := stdout.String()
	if !strings.Contains(out, "sess-a\talpha") {
		t.Fatalf("expected sessions output, got %q", out)
	}
	if !strings.Contains(out, "active session: sess-new") {
		t.Fatalf("expected active session output, got %q", out)
	}
	if !strings.Contains(out, "[user] hello") {
		t.Fatalf("expected history output, got %q", out)
	}
	if !strings.Contains(out, "# Session: project session") {
		t.Fatalf("expected export output, got %q", out)
	}
	if !strings.Contains(out, "sess-s\talpha result") {
		t.Fatalf("expected search output, got %q", out)
	}
	if !strings.Contains(out, "workspace=./workspace sessions=3") {
		t.Fatalf("expected status output, got %q", out)
	}
	if !strings.Contains(out, "compaction complete (session=sess-new compacted=3 final=4)") {
		t.Fatalf("expected compact output, got %q", out)
	}
}

func TestRun_ChatREPL_ResumeThenChat(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := strings.NewReader("/resume sess-r\nhello\n/quit\n")

	chatCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions/sess-r":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"sess-r","title":"resume target"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/chat":
			chatCalls++
			var req struct {
				SessionID string `json:"session_id"`
				Message   string `json:"message"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode chat request: %v", err)
			}
			if req.SessionID != "sess-r" {
				t.Fatalf("expected resumed session_id sess-r, got %q", req.SessionID)
			}
			if req.Message != "hello" {
				t.Fatalf("expected chat message hello, got %q", req.Message)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"type\":\"delta\",\"text\":\"ok\"}\n\n"))
			_, _ = w.Write([]byte("data: {\"type\":\"done\",\"session_id\":\"sess-r\"}\n\n"))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	code := runWithIO([]string{"chat", "--server-url", server.URL}, stdin, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}
	if chatCalls != 1 {
		t.Fatalf("expected exactly one chat request, got %d", chatCalls)
	}
	out := stdout.String()
	if !strings.Contains(out, "resumed session: sess-r") {
		t.Fatalf("expected resume output, got %q", out)
	}
	if !strings.Contains(out, "ok") {
		t.Fatalf("expected chat output, got %q", out)
	}
}

func TestRun_ChatREPL_ResumeSelectByNumber(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := strings.NewReader("/resume\n2\nhello\n/quit\n")

	chatCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id":"sess-a","title":"alpha"},{"id":"sess-b","title":"beta"}]`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/chat":
			chatCalls++
			var req struct {
				SessionID string `json:"session_id"`
				Message   string `json:"message"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode chat request: %v", err)
			}
			if req.SessionID != "sess-b" {
				t.Fatalf("expected selected session_id sess-b, got %q", req.SessionID)
			}
			if req.Message != "hello" {
				t.Fatalf("expected chat message hello, got %q", req.Message)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"type\":\"delta\",\"text\":\"ok\"}\n\n"))
			_, _ = w.Write([]byte("data: {\"type\":\"done\",\"session_id\":\"sess-b\"}\n\n"))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	code := runWithIO([]string{"chat", "--server-url", server.URL}, stdin, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}
	if chatCalls != 1 {
		t.Fatalf("expected exactly one chat request, got %d", chatCalls)
	}
	out := stdout.String()
	if !strings.Contains(out, "Select session:") {
		t.Fatalf("expected session selection prompt, got %q", out)
	}
	if !strings.Contains(out, "2) sess-b\tbeta") {
		t.Fatalf("expected numbered sessions list, got %q", out)
	}
	if !strings.Contains(out, "resumed session: sess-b") {
		t.Fatalf("expected resumed output, got %q", out)
	}
	if !strings.Contains(out, "ok") {
		t.Fatalf("expected chat output, got %q", out)
	}
}

func TestRun_ChatREPL_ResumeWithWonPrefix(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := strings.NewReader("₩resume sess-kor\nhello\n/quit\n")

	chatCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions/sess-kor":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"sess-kor","title":"korean session"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/chat":
			chatCalls++
			var req struct {
				SessionID string `json:"session_id"`
				Message   string `json:"message"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode chat request: %v", err)
			}
			if req.SessionID != "sess-kor" {
				t.Fatalf("expected resumed session_id sess-kor, got %q", req.SessionID)
			}
			if req.Message != "hello" {
				t.Fatalf("expected chat message hello, got %q", req.Message)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"type\":\"delta\",\"text\":\"ok\"}\n\n"))
			_, _ = w.Write([]byte("data: {\"type\":\"done\",\"session_id\":\"sess-kor\"}\n\n"))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	code := runWithIO([]string{"chat", "--server-url", server.URL}, stdin, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}
	if chatCalls != 1 {
		t.Fatalf("expected exactly one chat request, got %d", chatCalls)
	}
	out := stdout.String()
	if !strings.Contains(out, "resumed session: sess-kor") {
		t.Fatalf("expected resume output, got %q", out)
	}
	if !strings.Contains(out, "ok") {
		t.Fatalf("expected chat output, got %q", out)
	}
}

func TestNormalizeREPLInput_StripsControlSequences(t *testing.T) {
	if got := normalizeREPLInput("\x1b[A"); got != "" {
		t.Fatalf("expected empty input for arrow sequence, got %q", got)
	}
	if got := normalizeREPLInput("\x1b[A/resume sess-1"); got != "/resume sess-1" {
		t.Fatalf("expected normalized command, got %q", got)
	}
}
