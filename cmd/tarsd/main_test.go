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

	"github.com/devlikebear/tarsncase/internal/cron"
	"github.com/devlikebear/tarsncase/internal/llm"
	"github.com/devlikebear/tarsncase/internal/memory"
	"github.com/devlikebear/tarsncase/internal/session"
	"github.com/rs/zerolog"
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

	if !strings.Contains(stderr.String(), "tarsd startup complete") {
		t.Fatalf("expected startup log in stderr, got %q", stderr.String())
	}
}

func TestRun_LogFileWritesJSONLines(t *testing.T) {
	workspaceDir := filepath.Join(t.TempDir(), "workspace")
	logPath := filepath.Join(t.TempDir(), "tarsd.log")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := run([]string{"--workspace-dir", workspaceDir, "--log-file", logPath}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, `"level":"info"`) {
		t.Fatalf("expected json info log in file, got %q", content)
	}
	if !strings.Contains(content, `"message":"tarsd startup complete"`) {
		t.Fatalf("expected startup message in file, got %q", content)
	}

	if !strings.Contains(stderr.String(), "tarsd startup complete") {
		t.Fatalf("expected startup log in stderr, got %q", stderr.String())
	}
}

func TestRun_LogFileOpenFailureFallsBackToConsole(t *testing.T) {
	workspaceDir := filepath.Join(t.TempDir(), "workspace")
	badLogPath := t.TempDir()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := run([]string{"--workspace-dir", workspaceDir, "--log-file", badLogPath}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}

	if !strings.Contains(stderr.String(), "failed to open log file") {
		t.Fatalf("expected log file open error in stderr, got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "tarsd startup complete") {
		t.Fatalf("expected startup log in stderr, got %q", stderr.String())
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

func TestHeartbeatAPI_RunOnce_UsesAgentLoopToolFlow(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "HEARTBEAT.md"), []byte("check memory"), 0o644); err != nil {
		t.Fatalf("write heartbeat: %v", err)
	}

	mockClient := &mockLLMClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.ChatMessage{
					Role: "assistant",
					ToolCalls: []llm.ToolCall{
						{
							ID:        "call_hb_1",
							Name:      "read_file",
							Arguments: `{"path":"MEMORY.md"}`,
						},
					},
				},
			},
			{
				Message: llm.ChatMessage{
					Role:    "assistant",
					Content: "heartbeat tool flow done",
				},
			},
		},
	}

	now := time.Date(2026, 2, 13, 10, 0, 0, 0, time.UTC)
	ask := newAgentAskFunc(root, mockClient, 6, zerolog.New(io.Discard))
	handler := newHeartbeatAPIHandler(root, func() time.Time { return now }, ask, zerolog.New(io.Discard))

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
	if body.Response != "heartbeat tool flow done" {
		t.Fatalf("unexpected heartbeat response: %q", body.Response)
	}
	if mockClient.callCount != 2 {
		t.Fatalf("expected 2 llm calls for tool flow, got %d", mockClient.callCount)
	}
	if len(mockClient.seenToolCounts) == 0 || mockClient.seenToolCounts[0] == 0 {
		t.Fatalf("expected tool schemas in heartbeat call, got %+v", mockClient.seenToolCounts)
	}
}

func TestCronAPI_ListCreateRun(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	store := cron.NewStore(root)
	ranPrompts := make([]string, 0, 1)
	handler := newCronAPIHandler(
		store,
		func(_ context.Context, prompt string) (string, error) {
			ranPrompts = append(ranPrompts, prompt)
			return "cron job done", nil
		},
		zerolog.New(io.Discard),
	)

	listReq := httptest.NewRequest(http.MethodGet, "/v1/cron/jobs", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list status 200, got %d body=%q", listRec.Code, listRec.Body.String())
	}
	var initial []cron.Job
	if err := json.Unmarshal(listRec.Body.Bytes(), &initial); err != nil {
		t.Fatalf("decode initial list: %v", err)
	}
	if len(initial) != 0 {
		t.Fatalf("expected empty cron jobs initially, got %d", len(initial))
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/cron/jobs", strings.NewReader(`{"name":"morning","prompt":"check inbox"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK && createRec.Code != http.StatusCreated {
		t.Fatalf("expected create status 200/201, got %d body=%q", createRec.Code, createRec.Body.String())
	}
	var created cron.Job
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created job: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("expected created job id")
	}

	runReq := httptest.NewRequest(http.MethodPost, "/v1/cron/jobs/"+created.ID+"/run", nil)
	runRec := httptest.NewRecorder()
	handler.ServeHTTP(runRec, runReq)
	if runRec.Code != http.StatusOK {
		t.Fatalf("expected run status 200, got %d body=%q", runRec.Code, runRec.Body.String())
	}
	var runBody struct {
		JobID    string `json:"job_id"`
		Response string `json:"response"`
	}
	if err := json.Unmarshal(runRec.Body.Bytes(), &runBody); err != nil {
		t.Fatalf("decode run response: %v", err)
	}
	if runBody.JobID != created.ID {
		t.Fatalf("expected run job id %q, got %q", created.ID, runBody.JobID)
	}
	if runBody.Response != "cron job done" {
		t.Fatalf("unexpected cron run response: %q", runBody.Response)
	}
	if len(ranPrompts) != 1 || ranPrompts[0] != "check inbox" {
		t.Fatalf("unexpected run prompt capture: %+v", ranPrompts)
	}

	runsReq := httptest.NewRequest(http.MethodGet, "/v1/cron/jobs/"+created.ID+"/runs", nil)
	runsRec := httptest.NewRecorder()
	handler.ServeHTTP(runsRec, runsReq)
	if runsRec.Code != http.StatusOK {
		t.Fatalf("expected runs status 200, got %d body=%q", runsRec.Code, runsRec.Body.String())
	}
	var runs []cron.RunRecord
	if err := json.Unmarshal(runsRec.Body.Bytes(), &runs); err != nil {
		t.Fatalf("decode runs list: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 cron run record, got %d", len(runs))
	}
	if runs[0].JobID != created.ID {
		t.Fatalf("expected run job id %q, got %q", created.ID, runs[0].JobID)
	}
}

func TestCronAPI_UpdateDelete(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	store := cron.NewStore(root)
	handler := newCronAPIHandler(
		store,
		func(_ context.Context, prompt string) (string, error) { return "ok:" + prompt, nil },
		zerolog.New(io.Discard),
	)

	createReq := httptest.NewRequest(http.MethodPost, "/v1/cron/jobs", strings.NewReader(`{"name":"morning","prompt":"check inbox","schedule":"every:1h","enabled":true}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK && createRec.Code != http.StatusCreated {
		t.Fatalf("expected create status 200/201, got %d body=%q", createRec.Code, createRec.Body.String())
	}
	var created cron.Job
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created job: %v", err)
	}

	updateReq := httptest.NewRequest(http.MethodPut, "/v1/cron/jobs/"+created.ID, strings.NewReader(`{"name":"morning-updated","prompt":"check all","schedule":"every:30m","enabled":false,"delete_after_run":true}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected update status 200, got %d body=%q", updateRec.Code, updateRec.Body.String())
	}
	var updated cron.Job
	if err := json.Unmarshal(updateRec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated job: %v", err)
	}
	if updated.Name != "morning-updated" || updated.Prompt != "check all" {
		t.Fatalf("unexpected updated job payload: %+v", updated)
	}
	if updated.Schedule != "every:30m" {
		t.Fatalf("expected updated schedule every:30m, got %q", updated.Schedule)
	}
	if updated.Enabled {
		t.Fatalf("expected enabled=false after update")
	}
	if !updated.DeleteAfterRun {
		t.Fatalf("expected delete_after_run=true after update")
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/cron/jobs/"+created.ID, nil)
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent && deleteRec.Code != http.StatusOK {
		t.Fatalf("expected delete status 204/200, got %d body=%q", deleteRec.Code, deleteRec.Body.String())
	}

	runMissingReq := httptest.NewRequest(http.MethodPost, "/v1/cron/jobs/"+created.ID+"/run", nil)
	runMissingRec := httptest.NewRecorder()
	handler.ServeHTTP(runMissingRec, runMissingReq)
	if runMissingRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d body=%q", runMissingRec.Code, runMissingRec.Body.String())
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

func TestChatAPI_ToolCallMemorySearch(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "MEMORY.md"), []byte("I prefer black coffee.\n"), 0o644); err != nil {
		t.Fatalf("write memory file: %v", err)
	}

	logger := zerolog.New(io.Discard)
	store := session.NewStore(root)

	mockClient := &mockLLMClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.ChatMessage{
					Role: "assistant",
					ToolCalls: []llm.ToolCall{
						{
							ID:        "call_1",
							Name:      "memory_search",
							Arguments: `{"query":"coffee","limit":3}`,
						},
					},
				},
			},
			{
				Message: llm.ChatMessage{
					Role:    "assistant",
					Content: "Memory says: you prefer black coffee.",
				},
			},
		},
	}

	handler := newChatAPIHandler(root, store, mockClient, logger)

	reqBody := `{"message":"what coffee do i prefer?"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	if !strings.Contains(body, `"phase":"before_tool_call"`) || !strings.Contains(body, `"tool_name":"memory_search"`) {
		t.Fatalf("expected before_tool_call event for memory_search, got %q", body)
	}
	if !strings.Contains(body, `"phase":"after_tool_call"`) {
		t.Fatalf("expected after_tool_call event, got %q", body)
	}
	if !strings.Contains(body, "Memory says: you prefer black coffee.") {
		t.Fatalf("expected final assistant text in SSE, got %q", body)
	}
	if mockClient.callCount != 2 {
		t.Fatalf("expected 2 llm calls (tool + final), got %d", mockClient.callCount)
	}
	if len(mockClient.seenToolCounts) == 0 || mockClient.seenToolCounts[0] == 0 {
		t.Fatalf("expected tool schemas to be forwarded, got %+v", mockClient.seenToolCounts)
	}
}

func TestChatAPI_ToolCallReadFile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "README_LOCAL.txt"), []byte("workspace note"), 0o644); err != nil {
		t.Fatalf("write local read file: %v", err)
	}

	logger := zerolog.New(io.Discard)
	store := session.NewStore(root)
	mockClient := &mockLLMClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.ChatMessage{
					Role: "assistant",
					ToolCalls: []llm.ToolCall{
						{
							ID:        "call_read_1",
							Name:      "read_file",
							Arguments: `{"path":"README_LOCAL.txt"}`,
						},
					},
				},
			},
			{
				Message: llm.ChatMessage{
					Role:    "assistant",
					Content: "read complete",
				},
			},
		},
	}

	handler := newChatAPIHandler(root, store, mockClient, logger)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(`{"message":"read local file"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if mockClient.callCount != 2 {
		t.Fatalf("expected 2 llm calls (tool + final), got %d", mockClient.callCount)
	}
	if len(mockClient.seenMessages) < 2 || len(mockClient.seenMessages[1]) == 0 {
		t.Fatalf("expected captured second llm call messages")
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"tool_call_id":"call_read_1"`) {
		t.Fatalf("expected tool_call_id in status events, got %q", body)
	}
	if !strings.Contains(body, `"tool_args_preview":"{\"path\":\"README_LOCAL.txt\"}"`) {
		t.Fatalf("expected tool_args_preview in status events, got %q", body)
	}
	if !strings.Contains(body, `"tool_result_preview":"`) || !strings.Contains(body, `README_LOCAL.txt`) {
		t.Fatalf("expected tool_result_preview in status events, got %q", body)
	}
	last := mockClient.seenMessages[1][len(mockClient.seenMessages[1])-1]
	if last.Role != "tool" {
		t.Fatalf("expected tool role at second call tail, got %q", last.Role)
	}
	if !strings.Contains(last.Content, "workspace note") {
		t.Fatalf("expected tool result content to include file text, got %q", last.Content)
	}
}

func TestChatAPI_MemoryQueryForcesToolChoiceRequired(t *testing.T) {
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
				Content: "ok",
			},
		},
	}
	handler := newChatAPIHandler(root, store, mockClient, logger)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(`{"message":"what do you remember about me?"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if len(mockClient.seenToolChoices) == 0 || mockClient.seenToolChoices[0] != "required" {
		t.Fatalf("expected tool_choice required, got %+v", mockClient.seenToolChoices)
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

func TestChatAPI_AutoCompactsLargeTranscript(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	logger := zerolog.New(io.Discard)
	store := session.NewStore(root)
	sess, err := store.Create("large transcript")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	transcriptPath := store.TranscriptPath(sess.ID)
	largeContent := strings.Repeat("x", 2000)
	for i := 0; i < 260; i++ {
		if err := session.AppendMessage(transcriptPath, session.Message{
			Role:      "user",
			Content:   largeContent,
			Timestamp: time.Date(2026, 2, 14, 10, 0, i%60, 0, time.UTC),
		}); err != nil {
			t.Fatalf("append transcript message %d: %v", i, err)
		}
	}

	mockClient := &mockLLMClient{
		response: llm.ChatResponse{
			Message: llm.ChatMessage{
				Role:    "assistant",
				Content: "post-compaction reply",
			},
		},
	}

	handler := newChatAPIHandler(root, store, mockClient, logger)
	reqBody := fmt.Sprintf(`{"session_id":"%s","message":"hello after auto compact"}`, sess.ID)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	msgs, err := session.ReadMessages(transcriptPath)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatalf("expected compacted transcript to have messages")
	}
	if msgs[0].Role != "system" || !strings.Contains(msgs[0].Content, "[COMPACTION SUMMARY]") {
		t.Fatalf("expected compaction summary at transcript head, got %+v", msgs[0])
	}

	memoryData, err := os.ReadFile(filepath.Join(root, "MEMORY.md"))
	if err != nil {
		t.Fatalf("read memory file: %v", err)
	}
	if !strings.Contains(string(memoryData), "session "+sess.ID+" compacted") {
		t.Fatalf("expected compaction flush note in MEMORY.md, got %q", string(memoryData))
	}
}

func TestChatAPI_NonStreamingProviderStillEmitsDelta(t *testing.T) {
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
				Content: "non-streaming response",
			},
		},
		disableDelta: true,
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

	body := rec.Body.String()
	if !strings.Contains(body, `"type":"delta"`) {
		t.Fatalf("expected fallback delta event in SSE, got %q", body)
	}
	if !strings.Contains(body, "non-streaming response") {
		t.Fatalf("expected assistant content in SSE, got %q", body)
	}
	if !strings.Contains(body, `"type":"done"`) {
		t.Fatalf("expected done event in SSE, got %q", body)
	}
}

func TestChatAPI_WritesDailyAndLongTermMemory(t *testing.T) {
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
				Content: "알겠습니다. 기억하겠습니다.",
			},
		},
	}

	handler := newChatAPIHandler(root, store, mockClient, logger)
	reqBody := `{"message":"기억해: 나는 블랙커피를 좋아해"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	memoryData, err := os.ReadFile(filepath.Join(root, "MEMORY.md"))
	if err != nil {
		t.Fatalf("read memory file: %v", err)
	}
	if !strings.Contains(string(memoryData), "기억해: 나는 블랙커피를 좋아해") {
		t.Fatalf("expected promoted memory note, got %q", string(memoryData))
	}

	dailyPath := filepath.Join(root, "memory", time.Now().Format("2006-01-02")+".md")
	dailyData, err := os.ReadFile(dailyPath)
	if err != nil {
		t.Fatalf("read daily log: %v", err)
	}
	if !strings.Contains(string(dailyData), "chat session=") {
		t.Fatalf("expected chat daily log entry, got %q", string(dailyData))
	}
}

func TestChatAPI_UsesConfiguredMaxIterations(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	logger := zerolog.New(io.Discard)
	store := session.NewStore(root)

	mockClient := &mockLLMClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.ChatMessage{
					Role: "assistant",
					ToolCalls: []llm.ToolCall{
						{
							ID:        "call_1",
							Name:      "session_status",
							Arguments: `{}`,
						},
					},
				},
			},
		},
	}

	handler := newChatAPIHandlerWithOptions(root, store, mockClient, logger, 2)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(`{"message":"loop test"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "agent loop exceeded max iterations: 2") {
		t.Fatalf("expected max iteration error with configured value, got %q", rec.Body.String())
	}
}

type mockLLMClient struct {
	response        llm.ChatResponse
	responses       []llm.ChatResponse
	disableDelta    bool
	callCount       int
	seenMessages    [][]llm.ChatMessage
	seenToolCounts  []int
	seenToolChoices []string
}

func (m *mockLLMClient) Ask(ctx context.Context, prompt string) (string, error) {
	if len(m.responses) > 0 {
		return m.responses[0].Message.Content, nil
	}
	return m.response.Message.Content, nil
}

func (m *mockLLMClient) Chat(ctx context.Context, messages []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	m.callCount++
	msgCopy := append([]llm.ChatMessage(nil), messages...)
	m.seenMessages = append(m.seenMessages, msgCopy)
	m.seenToolCounts = append(m.seenToolCounts, len(opts.Tools))
	m.seenToolChoices = append(m.seenToolChoices, strings.TrimSpace(opts.ToolChoice))

	resp := m.response
	if len(m.responses) > 0 {
		idx := m.callCount - 1
		if idx >= len(m.responses) {
			idx = len(m.responses) - 1
		}
		resp = m.responses[idx]
	}

	if opts.OnDelta != nil && !m.disableDelta && resp.Message.Content != "" {
		opts.OnDelta(resp.Message.Content)
	}
	return resp, nil
}

func TestSessionAPIs(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	logger := zerolog.New(io.Discard)
	store := session.NewStore(root)
	handler := newSessionAPIHandler(store, logger)

	listReq := httptest.NewRequest(http.MethodGet, "/v1/sessions", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", listRec.Code, listRec.Body.String())
	}

	var sessions []session.Session
	if err := json.Unmarshal(listRec.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("decode sessions list: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected empty sessions list, got %d", len(sessions))
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(`{"title":"test session"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated && createRec.Code != http.StatusOK {
		t.Fatalf("expected 200 or 201, got %d body=%q", createRec.Code, createRec.Body.String())
	}

	var created session.Session
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created session: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("expected non-empty session id")
	}
	if created.Title != "test session" {
		t.Fatalf("expected title %q, got %q", "test session", created.Title)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+created.ID, nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", getRec.Code, getRec.Body.String())
	}

	var fetched session.Session
	if err := json.Unmarshal(getRec.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("decode fetched session: %v", err)
	}
	if fetched.ID != created.ID {
		t.Fatalf("expected id %q, got %q", created.ID, fetched.ID)
	}
	if fetched.Title != "test session" {
		t.Fatalf("expected title %q, got %q", "test session", fetched.Title)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/sessions/"+created.ID, nil)
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent && deleteRec.Code != http.StatusOK {
		t.Fatalf("expected 200 or 204, got %d body=%q", deleteRec.Code, deleteRec.Body.String())
	}

	listAfterDeleteReq := httptest.NewRequest(http.MethodGet, "/v1/sessions", nil)
	listAfterDeleteRec := httptest.NewRecorder()
	handler.ServeHTTP(listAfterDeleteRec, listAfterDeleteReq)
	if listAfterDeleteRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", listAfterDeleteRec.Code, listAfterDeleteRec.Body.String())
	}

	sessions = nil
	if err := json.Unmarshal(listAfterDeleteRec.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("decode sessions list after delete: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected empty sessions list after delete, got %d", len(sessions))
	}
}

func TestSessionAPI_History(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	logger := zerolog.New(io.Discard)
	store := session.NewStore(root)
	handler := newSessionAPIHandler(store, logger)

	sess, err := store.Create("history session")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	transcriptPath := store.TranscriptPath(sess.ID)
	msg1 := session.Message{Role: "user", Content: "hello", Timestamp: time.Now().UTC()}
	msg2 := session.Message{Role: "assistant", Content: "hi there", Timestamp: time.Now().UTC().Add(time.Second)}
	if err := session.AppendMessage(transcriptPath, msg1); err != nil {
		t.Fatalf("append first message: %v", err)
	}
	if err := session.AppendMessage(transcriptPath, msg2); err != nil {
		t.Fatalf("append second message: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+sess.ID+"/history", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var history []session.Message
	if err := json.Unmarshal(rec.Body.Bytes(), &history); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 history messages, got %d", len(history))
	}
	if history[0].Role != "user" || history[0].Content != "hello" {
		t.Fatalf("unexpected first history message: %+v", history[0])
	}
	if history[1].Role != "assistant" || history[1].Content != "hi there" {
		t.Fatalf("unexpected second history message: %+v", history[1])
	}
}

func TestSessionAPI_Export(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	logger := zerolog.New(io.Discard)
	store := session.NewStore(root)
	handler := newSessionAPIHandler(store, logger)

	sess, err := store.Create("export session")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	start := time.Date(2026, 2, 14, 9, 0, 0, 0, time.UTC)
	transcriptPath := store.TranscriptPath(sess.ID)
	if err := session.AppendMessage(transcriptPath, session.Message{
		Role:      "user",
		Content:   "What is 2+2?",
		Timestamp: start,
	}); err != nil {
		t.Fatalf("append user message: %v", err)
	}
	if err := session.AppendMessage(transcriptPath, session.Message{
		Role:      "assistant",
		Content:   "2+2 is 4.",
		Timestamp: start.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("append assistant message: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+sess.ID+"/export", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	bodyLower := strings.ToLower(body)
	if !strings.Contains(bodyLower, "user") {
		t.Fatalf("expected exported markdown to contain user label, got %q", body)
	}
	if !strings.Contains(bodyLower, "assistant") {
		t.Fatalf("expected exported markdown to contain assistant label, got %q", body)
	}
	if !strings.Contains(body, "What is 2+2?") {
		t.Fatalf("expected exported markdown to contain user content, got %q", body)
	}
	if !strings.Contains(body, "2+2 is 4.") {
		t.Fatalf("expected exported markdown to contain assistant content, got %q", body)
	}
}

func TestSessionAPI_Search(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	logger := zerolog.New(io.Discard)
	store := session.NewStore(root)
	handler := newSessionAPIHandler(store, logger)

	titles := []string{"apple pie", "banana split", "apple tart"}
	for _, title := range titles {
		if _, err := store.Create(title); err != nil {
			t.Fatalf("create session %q: %v", title, err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/search?q=apple", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var results []session.Session
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("decode search results: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(results))
	}

	foundTitles := map[string]bool{}
	for _, s := range results {
		foundTitles[s.Title] = true
	}
	if !foundTitles["apple pie"] || !foundTitles["apple tart"] {
		t.Fatalf("unexpected search results titles: %+v", foundTitles)
	}
}

func TestStatusAPI(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	logger := zerolog.New(io.Discard)
	store := session.NewStore(root)

	for _, title := range []string{"status one", "status two"} {
		if _, err := store.Create(title); err != nil {
			t.Fatalf("create session %q: %v", title, err)
		}
	}

	handler := newStatusAPIHandler(root, store, logger)

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var body struct {
		WorkspaceDir string `json:"workspace_dir"`
		SessionCount int    `json:"session_count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if body.WorkspaceDir != root {
		t.Fatalf("expected workspace_dir %q, got %q", root, body.WorkspaceDir)
	}
	if body.SessionCount != 2 {
		t.Fatalf("expected session_count 2, got %d", body.SessionCount)
	}
}

func TestCompactAPI(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	logger := zerolog.New(io.Discard)
	store := session.NewStore(root)
	sess, err := store.Create("compact target")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	transcriptPath := store.TranscriptPath(sess.ID)
	for i := 0; i < 12; i++ {
		if err := session.AppendMessage(transcriptPath, session.Message{
			Role:      "user",
			Content:   fmt.Sprintf("compact message %d", i),
			Timestamp: time.Date(2026, 2, 14, 12, 0, i, 0, time.UTC),
		}); err != nil {
			t.Fatalf("append message %d: %v", i, err)
		}
	}

	handler := newCompactAPIHandler(root, store, nil, logger)

	reqBody, err := json.Marshal(map[string]any{
		"session_id":  sess.ID,
		"keep_recent": 5,
	})
	if err != nil {
		t.Fatalf("marshal compact request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/compact", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var body struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode compact response: %v", err)
	}
	if !strings.Contains(body.Message, "compaction complete") {
		t.Fatalf("expected compaction completion message, got %q", body.Message)
	}

	messages, err := session.ReadMessages(transcriptPath)
	if err != nil {
		t.Fatalf("read compacted transcript: %v", err)
	}
	if len(messages) != 6 {
		t.Fatalf("expected 6 messages after compaction, got %d", len(messages))
	}
	if messages[0].Role != "system" || !strings.Contains(messages[0].Content, "[COMPACTION SUMMARY]") {
		t.Fatalf("expected summary message at first entry, got %+v", messages[0])
	}

	memoryData, err := os.ReadFile(filepath.Join(root, "MEMORY.md"))
	if err != nil {
		t.Fatalf("read memory file: %v", err)
	}
	if !strings.Contains(string(memoryData), "session "+sess.ID+" compacted") {
		t.Fatalf("expected compaction note in MEMORY.md, got %q", string(memoryData))
	}
}

func TestCompactAPI_UsesLLMSummaryWhenAvailable(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	logger := zerolog.New(io.Discard)
	store := session.NewStore(root)
	sess, err := store.Create("compact llm")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	transcriptPath := store.TranscriptPath(sess.ID)
	for i := 0; i < 12; i++ {
		if err := session.AppendMessage(transcriptPath, session.Message{
			Role:      "user",
			Content:   fmt.Sprintf("llm compact message %d", i),
			Timestamp: time.Date(2026, 2, 14, 12, 0, i, 0, time.UTC),
		}); err != nil {
			t.Fatalf("append message %d: %v", i, err)
		}
	}

	mockClient := &mockLLMClient{
		response: llm.ChatResponse{
			Message: llm.ChatMessage{
				Role:    "assistant",
				Content: "LLM compact summary: key user intent and decisions.",
			},
		},
	}

	handler := newCompactAPIHandler(root, store, mockClient, logger)
	reqBody, err := json.Marshal(map[string]any{
		"session_id":  sess.ID,
		"keep_recent": 5,
	})
	if err != nil {
		t.Fatalf("marshal compact request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/compact", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	messages, err := session.ReadMessages(transcriptPath)
	if err != nil {
		t.Fatalf("read compacted transcript: %v", err)
	}
	if len(messages) == 0 {
		t.Fatalf("expected compacted transcript entries")
	}
	if !strings.Contains(messages[0].Content, "LLM compact summary") {
		t.Fatalf("expected llm summary in compacted transcript, got %q", messages[0].Content)
	}
}

func TestCompactAPI_WithTokenBudget(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	logger := zerolog.New(io.Discard)
	store := session.NewStore(root)
	sess, err := store.Create("compact token budget")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	transcriptPath := store.TranscriptPath(sess.ID)
	for i := 0; i < 10; i++ {
		if err := session.AppendMessage(transcriptPath, session.Message{
			Role:      "user",
			Content:   fmt.Sprintf("token budget message %d %s", i, strings.Repeat("x", 80)),
			Timestamp: time.Date(2026, 2, 14, 12, 0, i, 0, time.UTC),
		}); err != nil {
			t.Fatalf("append message %d: %v", i, err)
		}
	}

	handler := newCompactAPIHandler(root, store, nil, logger)
	reqBody, err := json.Marshal(map[string]any{
		"session_id":         sess.ID,
		"keep_recent_tokens": 45,
	})
	if err != nil {
		t.Fatalf("marshal compact request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/compact", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	msgs, err := session.ReadMessages(transcriptPath)
	if err != nil {
		t.Fatalf("read compacted transcript: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected summary + 2 recent messages, got %d", len(msgs))
	}
}

func TestShouldForceMemoryToolCall(t *testing.T) {
	if !shouldForceMemoryToolCall("내 취향 기억나?") {
		t.Fatalf("expected korean memory query to be true")
	}
	if !shouldForceMemoryToolCall("what do you remember about me?") {
		t.Fatalf("expected english memory query to be true")
	}
	if shouldForceMemoryToolCall("hello there") {
		t.Fatalf("expected non-memory query to be false")
	}
}
