package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/devlikebear/tarsncase/internal/llm"
	"github.com/devlikebear/tarsncase/internal/memory"
	"github.com/devlikebear/tarsncase/internal/session"
)

func TestRun_DefaultConfig(t *testing.T) {
	workspaceDir := filepath.Join(t.TempDir(), "workspace")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := run([]string{"--workspace-dir", workspaceDir}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}

	if !strings.Contains(stdout.String(), "tarsd starting in standalone mode") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}

	if !strings.Contains(stderr.String(), `"level":"info"`) {
		t.Fatalf("expected info log in stderr, got %q", stderr.String())
	}
}

func TestRun_FlagOverridesEnvAndYAML(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	content := "mode: service\nworkspace_dir: ./tenant-workspace\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("TARSD_MODE", "service")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	workspaceDir := filepath.Join(t.TempDir(), "workspace")

	code := run([]string{"--config", configPath, "--mode", "standalone", "--workspace-dir", workspaceDir}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}

	if !strings.Contains(stdout.String(), "tarsd starting in standalone mode") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestRun_InvalidConfigPathReturnsError(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := run([]string{"--config", "./not-found.yaml", "--workspace-dir", filepath.Join(t.TempDir(), "workspace")}, stdout, stderr)
	if code == 0 {
		t.Fatalf("expected non-zero exit code, stdout=%q", stdout.String())
	}

	if !strings.Contains(stderr.String(), "failed to load config") {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}

	if !strings.Contains(stderr.String(), `"level":"error"`) {
		t.Fatalf("expected error log in stderr, got %q", stderr.String())
	}
}

func TestRun_CreatesWorkspaceAndDailyLog(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := run([]string{"--workspace-dir", root}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}

	if _, err := os.Stat(filepath.Join(root, "HEARTBEAT.md")); err != nil {
		t.Fatalf("expected HEARTBEAT.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "MEMORY.md")); err != nil {
		t.Fatalf("expected MEMORY.md: %v", err)
	}
}

func TestRun_RunOnceAppendsHeartbeatLog(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	server := newBifrostTestServer(t)
	defer server.Close()
	t.Setenv("BIFROST_BASE_URL", server.URL+"/v1")
	t.Setenv("BIFROST_API_KEY", "test-key")
	t.Setenv("BIFROST_MODEL", "test-model")

	code := run([]string{"--workspace-dir", root, "--run-once"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}

	logPath := filepath.Join(root, "memory", time.Now().Format("2006-01-02")+".md")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read daily log: %v", err)
	}
	if !strings.Contains(string(data), "heartbeat tick") {
		t.Fatalf("expected heartbeat tick entry in %s, got %q", logPath, string(data))
	}
}

func TestRun_MutuallyExclusiveRunFlags(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	workspaceDir := filepath.Join(t.TempDir(), "workspace")

	code := run([]string{"--workspace-dir", workspaceDir, "--run-once", "--run-loop"}, stdout, stderr)
	if code != 2 {
		t.Fatalf("expected exit code 2 for mutually exclusive flags, got %d", code)
	}
}

func TestRun_HelpReturnsZero(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := run([]string{"--help"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}

	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("expected help usage output, got %q", stdout.String())
	}
}

func TestRun_RunLoopAppendsHeartbeatLog(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	server := newBifrostTestServer(t)
	defer server.Close()
	t.Setenv("BIFROST_BASE_URL", server.URL+"/v1")
	t.Setenv("BIFROST_API_KEY", "test-key")
	t.Setenv("BIFROST_MODEL", "test-model")

	code := run([]string{
		"--workspace-dir", root,
		"--run-loop",
		"--heartbeat-interval", "5ms",
		"--max-heartbeats", "2",
	}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}

	logPath := filepath.Join(root, "memory", time.Now().Format("2006-01-02")+".md")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read daily log: %v", err)
	}
	content := string(data)
	if strings.Count(content, "heartbeat tick") < 2 {
		t.Fatalf("expected at least 2 heartbeat ticks in %s, got %q", logPath, content)
	}
}

func newBifrostTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"next step"}}]}`))
	}))
}

func TestHeartbeatAPI_RunOnce(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "HEARTBEAT.md"), []byte("api-test"), 0o644); err != nil {
		t.Fatalf("write heartbeat: %v", err)
	}

	now := time.Date(2026, 2, 13, 10, 0, 0, 0, time.UTC)
	handler := newHeartbeatAPIHandler(
		root,
		func() time.Time { return now },
		func(_ context.Context, prompt string) (string, error) {
			if !strings.Contains(prompt, "HEARTBEAT:") {
				t.Fatalf("unexpected prompt: %q", prompt)
			}
			return "next action from api", nil
		},
		zerolog.New(io.Discard),
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/heartbeat/run-once", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var body struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Response != "next action from api" {
		t.Fatalf("unexpected response: %q", body.Response)
	}
}

func TestChatAPI(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	logger := zerolog.New(io.Discard)
	store := session.NewStore(root)

	mockClient := &mockLLMClient{
		response: llm.ChatResponse{
			Message: llm.ChatMessage{
				Role:    "assistant",
				Content: "Hello from TARS!",
			},
			Usage: llm.Usage{
				InputTokens:  10,
				OutputTokens: 5,
			},
			StopReason: "end_turn",
		},
	}

	handler := newChatAPIHandler(root, store, mockClient, logger)

	reqBody := `{"message": "hi"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %q", ct)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `"type":"delta"`) {
		t.Fatalf("expected delta event in SSE, got %q", body)
	}
	if !strings.Contains(body, `"type":"done"`) {
		t.Fatalf("expected done event in SSE, got %q", body)
	}
	if !strings.Contains(body, "Hello from TARS!") {
		t.Fatalf("expected response text in SSE, got %q", body)
	}
}

func TestChatAPI_WithSessionID(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	logger := zerolog.New(io.Discard)
	store := session.NewStore(root)

	sess, err := store.Create("test session")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	mockClient := &mockLLMClient{
		response: llm.ChatResponse{
			Message: llm.ChatMessage{Role: "assistant", Content: "reply"},
		},
	}

	handler := newChatAPIHandler(root, store, mockClient, logger)

	reqBody := fmt.Sprintf(`{"session_id": "%s", "message": "hello"}`, sess.ID)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	msgs, err := session.ReadMessages(store.TranscriptPath(sess.ID))
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	if len(msgs) < 2 {
		t.Fatalf("expected at least 2 messages in transcript, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "hello" {
		t.Fatalf("unexpected first message: %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "reply" {
		t.Fatalf("unexpected second message: %+v", msgs[1])
	}
}

type mockLLMClient struct {
	response llm.ChatResponse
}

func (m *mockLLMClient) Ask(ctx context.Context, prompt string) (string, error) {
	return m.response.Message.Content, nil
}

func (m *mockLLMClient) Chat(ctx context.Context, messages []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	if opts.OnDelta != nil {
		opts.OnDelta(m.response.Message.Content)
	}
	return m.response, nil
}

