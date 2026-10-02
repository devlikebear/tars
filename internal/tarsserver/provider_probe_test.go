package tarsserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/rs/zerolog"
)

type fakeCLIRun struct {
	out []byte
	err error
}

// fakeCLIRunner answers by the joined arguments and records every call.
type fakeCLIRunner struct {
	mu    sync.Mutex
	runs  map[string]fakeCLIRun
	calls []string
}

func (f *fakeCLIRunner) run(_ context.Context, path string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := strings.Join(args, " ")
	f.calls = append(f.calls, path+" "+key)
	r, ok := f.runs[key]
	if !ok {
		return nil, errors.New("unexpected command: " + key)
	}
	return r.out, r.err
}

func newProbeTestService(t *testing.T, cfg config.Config, fetcher *fakeModelFetcher, runner *fakeCLIRunner) *providerModelsService {
	t.Helper()
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	cache, err := newProviderModelsCache(filepath.Join(t.TempDir(), "provider_models_cache.json"), providerModelsCacheTTL, func() time.Time { return now })
	if err != nil {
		t.Fatalf("newProviderModelsCache: %v", err)
	}
	service := newProviderModelsService(cfg, cache, fetcher, func() time.Time { return now })
	service.findCLI = func(kind string) (string, error) { return "/bin/" + kind, nil }
	service.runCLI = runner.run
	return service
}

func probeOne(t *testing.T, service *providerModelsService) providerProbeResult {
	t.Helper()
	out := service.probeAll(context.Background())
	if len(out.Results) != 1 {
		t.Fatalf("expected one result, got %+v", out.Results)
	}
	return out.Results[0]
}

func TestProviderProbe_ClaudeCodeCLILoggedIn(t *testing.T) {
	runner := &fakeCLIRunner{runs: map[string]fakeCLIRun{
		"--version":          {out: []byte("2.1.283 (Claude Code)\n")},
		"auth status --json": {out: []byte(`{"loggedIn":true,"authMethod":"claude.ai","email":"someone@example.com"}`)},
	}}
	service := newProbeTestService(t, makePoolTestCfg("claude-code-cli", "sonnet", "cli", ""), &fakeModelFetcher{}, runner)

	got := probeOne(t, service)
	if got.Status != providerProbeOK || got.Problem != "" || !got.Default {
		t.Fatalf("unexpected result: %+v", got)
	}
	if got.Version != "2.1.283" || got.AuthMethod != "claude.ai" || got.CLIPath != "/bin/claude-code-cli" {
		t.Fatalf("unexpected details: %+v", got)
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "example.com") {
		t.Fatalf("probe must not echo the account email: %s", raw)
	}
}

func TestProviderProbe_ClaudeCodeCLILoggedOut(t *testing.T) {
	runner := &fakeCLIRunner{runs: map[string]fakeCLIRun{
		"--version":          {out: []byte("2.1.283 (Claude Code)\n")},
		"auth status --json": {out: []byte(`{"loggedIn":false}`), err: errors.New("exit status 1")},
	}}
	service := newProbeTestService(t, makePoolTestCfg("claude-code-cli", "sonnet", "cli", ""), &fakeModelFetcher{}, runner)

	got := probeOne(t, service)
	if got.Status != providerProbeError || got.Problem != providerProbeNotLoggedIn {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestProviderProbe_ClaudeCodeCLIWithoutAuthSubcommandIsUnverified(t *testing.T) {
	runner := &fakeCLIRunner{runs: map[string]fakeCLIRun{
		"--version":          {out: []byte("1.0.0 (Claude Code)\n")},
		"auth status --json": {err: errors.New("exit status 1: error: unknown command 'auth'")},
	}}
	service := newProbeTestService(t, makePoolTestCfg("claude-code-cli", "sonnet", "cli", ""), &fakeModelFetcher{}, runner)

	got := probeOne(t, service)
	if got.Status != providerProbeInfo || got.Problem != providerProbeAuthUnknown || !strings.Contains(got.Detail, "unknown command") {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestProviderProbe_CLIMissing(t *testing.T) {
	runner := &fakeCLIRunner{}
	service := newProbeTestService(t, makePoolTestCfg("antigravity-cli", "gemini-3.8-flash-high", "cli", ""), &fakeModelFetcher{}, runner)
	service.findCLI = func(string) (string, error) { return "", errors.New("agy executable not found in PATH") }

	got := probeOne(t, service)
	if got.Status != providerProbeError || got.Problem != providerProbeCLIMissing || !strings.Contains(got.Detail, "not found") {
		t.Fatalf("unexpected result: %+v", got)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("expected no CLI runs, got %v", runner.calls)
	}
}

func TestProviderProbe_AntigravityCLI(t *testing.T) {
	models := "Fetching available models...\ngemini-3.8-flash-high\tGemini 3.8 Flash (High)\nclaude-sonnet-4-6\tClaude Sonnet 4.6\n"
	cases := []struct {
		name        string
		version     string
		modelsRun   fakeCLIRun
		wantStatus  string
		wantProblem string
		wantCount   int
	}{
		{name: "current", version: "1.1.24", modelsRun: fakeCLIRun{out: []byte(models)}, wantStatus: providerProbeOK, wantCount: 2},
		{name: "too old", version: "1.1.9", modelsRun: fakeCLIRun{out: []byte(models)}, wantStatus: providerProbeWarn, wantProblem: providerProbeVersionOld, wantCount: 2},
		{name: "not signed in", version: "1.1.24", modelsRun: fakeCLIRun{err: errors.New("exit status 1: not authenticated")}, wantStatus: providerProbeError, wantProblem: providerProbeAuthFailed},
		{name: "no models", version: "1.1.24", modelsRun: fakeCLIRun{out: []byte("Fetching available models...\n")}, wantStatus: providerProbeWarn, wantProblem: providerProbeModelsNoneSeen},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &fakeCLIRunner{runs: map[string]fakeCLIRun{
				"--version": {out: []byte(tc.version + "\n")},
				"models":    tc.modelsRun,
			}}
			service := newProbeTestService(t, makePoolTestCfg("antigravity-cli", "gemini-3.8-flash-high", "cli", ""), &fakeModelFetcher{}, runner)
			got := probeOne(t, service)
			if got.Status != tc.wantStatus || got.Problem != tc.wantProblem || got.ModelCount != tc.wantCount || got.Version != tc.version {
				t.Fatalf("unexpected result: %+v", got)
			}
			if tc.wantProblem == providerProbeVersionOld && got.MinVersion != antigravityCLIMinVersion {
				t.Fatalf("expected min version, got %+v", got)
			}
		})
	}
}

func TestProviderProbe_HTTPProviders(t *testing.T) {
	cases := []struct {
		name        string
		fetcher     *fakeModelFetcher
		wantStatus  string
		wantProblem string
	}{
		{name: "listed", fetcher: &fakeModelFetcher{models: []string{"gpt-5.3-codex", "gpt-5.4"}}, wantStatus: providerProbeOK},
		{name: "failed", fetcher: &fakeModelFetcher{err: errors.New("dial tcp: connection refused")}, wantStatus: providerProbeError, wantProblem: providerProbeModelsFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &fakeCLIRunner{}
			service := newProbeTestService(t, makePoolTestCfg("openai-codex", "gpt-5.3-codex", "oauth", ""), tc.fetcher, runner)
			got := probeOne(t, service)
			if got.Status != tc.wantStatus || got.Problem != tc.wantProblem {
				t.Fatalf("unexpected result: %+v", got)
			}
			if len(runner.calls) != 0 {
				t.Fatalf("HTTP providers must not run a CLI, got %v", runner.calls)
			}
		})
	}
}

func TestProviderProbeAPI_TestsEveryAliasDefaultFirst(t *testing.T) {
	cfg := config.Config{LLMConfig: config.LLMConfig{
		LLMProviders: map[string]config.LLMProviderSettings{
			"anthropic-direct": {Kind: "anthropic", APIKey: "test-key"},
			"codex":            {Kind: "openai-codex", AuthMode: "oauth"},
			"claude":           {Kind: "claude-code-cli"},
		},
		LLMTiers: map[string]config.LLMTierBinding{
			"standard": {Provider: "claude", Model: "sonnet"},
			"light":    {Provider: "codex", Model: "gpt-5.3-codex"},
		},
		LLMDefaultTier: "standard",
	}}
	runner := &fakeCLIRunner{runs: map[string]fakeCLIRun{
		"--version":          {out: []byte("2.1.283 (Claude Code)\n")},
		"auth status --json": {out: []byte(`{"loggedIn":true,"authMethod":"claude.ai"}`)},
	}}
	service := newProbeTestService(t, cfg, &fakeModelFetcher{models: []string{"m1"}}, runner)
	handler := newProvidersModelsAPIHandler(service, zerolog.New(io.Discard))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/admin/providers/test", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	var out providerProbeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var order []string
	for _, r := range out.Results {
		order = append(order, r.Alias+":"+r.Status)
	}
	if got := strings.Join(order, ","); got != "claude:ok,anthropic-direct:ok,codex:ok" {
		t.Fatalf("unexpected results order/status: %s (%+v)", got, out.Results)
	}
	if !out.Results[0].Default || out.Results[1].Default {
		t.Fatalf("only the default tier's alias is default: %+v", out.Results)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/admin/providers/test", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET, got %d", rec.Code)
	}
}

func TestCompareCLIVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.1.12", "1.1.12", 0},
		{"1.1.9", "1.1.12", -1},
		{"1.2.0", "1.1.12", 1},
		{"2.0", "2.0.0", 0},
	}
	for _, tc := range cases {
		if got := compareCLIVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("compareCLIVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
	if got := parseCLIVersion("agy version 1.1.24 (build abc)"); got != "1.1.24" {
		t.Errorf("parseCLIVersion = %q", got)
	}
}

func TestRunProviderCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	out, err := runProviderCLI(context.Background(), "/bin/sh", "-c", "echo 1.2.3")
	if err != nil || strings.TrimSpace(string(out)) != "1.2.3" {
		t.Fatalf("expected stdout, got %q err=%v", out, err)
	}
	out, err = runProviderCLI(context.Background(), "/bin/sh", "-c", `echo '{"loggedIn":false}'; echo 'not signed in' >&2; exit 1`)
	if err == nil || !strings.Contains(err.Error(), "not signed in") {
		t.Fatalf("expected stderr in the error, got %v", err)
	}
	if !strings.Contains(string(out), "loggedIn") {
		t.Fatalf("a failed run still returns its stdout, got %q", out)
	}
	if _, err := runProviderCLI(context.Background(), "/bin/sh", "-c", "exit 2"); err == nil || !strings.Contains(err.Error(), "exit status 2") {
		t.Fatalf("expected the bare exit error, got %v", err)
	}
	if got := clipProbeOutput(strings.Repeat("가", providerProbeOutputCap+5)); len([]rune(got)) != providerProbeOutputCap+1 {
		t.Fatalf("clip keeps whole runes plus an ellipsis, got %d runes", len([]rune(got)))
	}
}
