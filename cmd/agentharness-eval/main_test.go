package main

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/agentharness/evalpack"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestRunDeterministicWritesVersionedReports(t *testing.T) {
	root := filepath.Join("..", "..")
	outputDir := t.TempDir()
	jsonlPath := filepath.Join(outputDir, "baseline.jsonl")
	markdownPath := filepath.Join(outputDir, "baseline.md")
	var stdout, stderr bytes.Buffer
	err := run([]string{
		"--pack", filepath.Join(root, "testdata", "agent-harness", "scenarios.json"),
		"--version", "0.34.3",
		"--commit", "abc123",
		"--jsonl", jsonlPath,
		"--markdown", markdownPath,
	}, &stdout, &stderr, func(string) string { return "" })
	if err != nil {
		t.Fatalf("run: %v\nstderr: %s", err, stderr.String())
	}
	for _, path := range []string{jsonlPath, markdownPath} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(data)
		if !strings.Contains(text, "0.34.3") || !strings.Contains(text, "abc123") {
			t.Fatalf("report %s is missing version/commit metadata:\n%s", path, text)
		}
	}
	if !strings.Contains(stdout.String(), "12 scenarios") || !strings.Contains(stdout.String(), "baseline expectations met") {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
}

func TestRunLiveRequiresProviderConfiguration(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run([]string{
		"--mode", "live",
		"--pack", filepath.Join("..", "..", "testdata", "agent-harness", "scenarios.json"),
		"--version", "test",
		"--commit", "test",
	}, &stdout, &stderr, func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "TARS_AGENT_EVAL_PROVIDER") {
		t.Fatalf("expected provider guidance, got %v", err)
	}
}

func TestRunReturnsSummaryWriteFailure(t *testing.T) {
	outputDir := t.TempDir()
	err := run([]string{
		"--pack", filepath.Join("..", "..", "testdata", "agent-harness", "scenarios.json"),
		"--version", "test",
		"--commit", "test",
		"--jsonl", filepath.Join(outputDir, "baseline.jsonl"),
	}, failingWriter{}, &bytes.Buffer{}, func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "write evaluation summary") {
		t.Fatalf("expected summary write error, got %v", err)
	}
}

func TestRunLiveUsesOptInProviderAndSkipsDeterministicOnlyScenarios(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer live-key" {
			t.Errorf("authorization = %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"HARNESS_SINGLE_OK\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	outputDir := t.TempDir()
	jsonlPath := filepath.Join(outputDir, "live.jsonl")
	env := map[string]string{
		"TARS_AGENT_EVAL_PROVIDER": "openai",
		"TARS_AGENT_EVAL_BASE_URL": server.URL + "/v1",
		"TARS_AGENT_EVAL_MODEL":    "fake-live-model",
		"TARS_AGENT_EVAL_API_KEY":  "live-key",
	}
	var stdout, stderr bytes.Buffer
	err := run([]string{
		"--mode", "live",
		"--pack", filepath.Join("..", "..", "testdata", "agent-harness", "scenarios.json"),
		"--version", "test", "--commit", "test", "--jsonl", jsonlPath,
	}, &stdout, &stderr, func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("run live: %v\nstderr: %s", err, stderr.String())
	}
	data, err := os.ReadFile(jsonlPath)
	if err != nil {
		t.Fatalf("read live report: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `"task_success":true`) || strings.Count(text, `"status":"skipped"`) != 11 {
		t.Fatalf("unexpected live report:\n%s", text)
	}
}

func TestResolveVersionCommitAndAPIKeys(t *testing.T) {
	if got, err := resolveVersion(" 1.2.3 "); err != nil || got != "1.2.3" {
		t.Fatalf("explicit version = %q, %v", got, err)
	}
	root := t.TempDir()
	t.Chdir(root)
	if err := os.WriteFile("VERSION.txt", []byte("2.0.0\n"), 0o600); err != nil {
		t.Fatalf("write version: %v", err)
	}
	if got, err := resolveVersion(""); err != nil || got != "2.0.0" {
		t.Fatalf("file version = %q, %v", got, err)
	}
	if got := resolveCommit(" exact "); got != "exact" {
		t.Fatalf("explicit commit = %q", got)
	}

	values := map[string]string{
		"OPENAI_API_KEY":    "openai",
		"ANTHROPIC_API_KEY": "anthropic",
		"CLAUDE_API_KEY":    "claude",
		"GEMINI_API_KEY":    "gemini",
		"KIMI_API_KEY":      "kimi",
	}
	getenv := func(key string) string { return values[key] }
	for provider, want := range map[string]string{
		"openai": "openai", "openai-codex": "openai", "anthropic": "anthropic",
		"claude-code-cli": "anthropic", "gemini": "gemini", "gemini-native": "gemini", "kimi": "kimi",
	} {
		if got := resolveAPIKey(provider, getenv); got != want {
			t.Errorf("provider %s key = %q, want %q", provider, got, want)
		}
	}
	values["TARS_AGENT_EVAL_API_KEY"] = "override"
	if got := resolveAPIKey("unknown", getenv); got != "override" {
		t.Fatalf("override key = %q", got)
	}
}

func TestResolveVersionAndOutputErrors(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if _, err := resolveVersion(""); err == nil {
		t.Fatal("expected missing VERSION.txt error")
	}
	if err := os.WriteFile("VERSION.txt", nil, 0o600); err != nil {
		t.Fatalf("write empty version: %v", err)
	}
	if _, err := resolveVersion(""); err == nil {
		t.Fatal("expected empty VERSION.txt error")
	}
	if got := resolveCommit(""); got != "unknown" {
		t.Fatalf("non-repository commit = %q", got)
	}
	if err := writeOutput(failingWriter{}, "-", []byte("data")); err == nil {
		t.Fatal("expected stdout write failure")
	}
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("file"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	if err := writeOutput(&bytes.Buffer{}, filepath.Join(blocker, "report"), []byte("data")); err == nil {
		t.Fatal("expected file write failure")
	}
}

func TestEmitReportsRejectsTwoStdoutFormats(t *testing.T) {
	if err := emitReports(&bytes.Buffer{}, "-", "-", evalpack.Report{}); err == nil {
		t.Fatal("expected duplicate stdout error")
	}
}
