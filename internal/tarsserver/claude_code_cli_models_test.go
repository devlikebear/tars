package tarsserver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/llm"
)

func newClaudeModelsTestService(t *testing.T, cfg config.Config, fetcher *fakeModelFetcher, cliCache []string) *providerModelsService {
	t.Helper()
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	cache, err := newProviderModelsCache(filepath.Join(t.TempDir(), "c.json"), providerModelsCacheTTL, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	s := newProviderModelsService(cfg, cache, fetcher, func() time.Time { return now })
	s.findCLI = func(string) (string, error) { return "/bin/claude", nil }
	s.readCLIModelCache = func() []string { return cliCache }
	return s
}

func claudePoolCfg(withAnthropic bool) config.Config {
	cfg := makePoolTestCfg("claude-code-cli", "sonnet", "cli", "")
	if withAnthropic {
		cfg.LLMProviders["api"] = config.LLMProviderSettings{Kind: "anthropic", AuthMode: "api-key", APIKey: "k"}
	}
	return cfg
}

func TestClaudeCodeCLIModels_CatalogOnly(t *testing.T) {
	f := &fakeModelFetcher{}
	s := newClaudeModelsTestService(t, claudePoolCfg(false), f, nil)
	info, err := s.models(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if info.Source != "catalog" || f.calls != 0 || info.CLIPath != "/bin/claude" {
		t.Fatalf("unexpected: %+v calls=%d", info, f.calls)
	}
	for _, want := range []string{"opus", "sonnet", "haiku", "claude-opus-5-5"} {
		if !slices.Contains(info.Models, want) {
			t.Fatalf("missing %q in %v", want, info.Models)
		}
	}
	if info.Models[0] != "opus" && info.Models[0] != "sonnet" && info.Models[0] != "haiku" {
		t.Fatalf("aliases should lead: %v", info.Models)
	}
}

func TestClaudeCodeCLIModels_LiveFiltersAndCaches(t *testing.T) {
	f := &fakeModelFetcher{models: []string{"claude-opus-9", "not-claude", "claude-opus-5-5"}}
	s := newClaudeModelsTestService(t, claudePoolCfg(true), f, []string{"claude-fable-5-1"})
	info, err := s.models(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if info.Source != "live" || info.Warning != "" {
		t.Fatalf("unexpected: %+v", info)
	}
	if !slices.Contains(info.Models, "claude-opus-9") || slices.Contains(info.Models, "not-claude") || !slices.Contains(info.Models, "claude-fable-5-1") {
		t.Fatalf("models: %v", info.Models)
	}
	if f.lastOps.Provider != "anthropic" || f.lastOps.APIKey != "k" {
		t.Fatalf("fetch opts: %+v", f.lastOps)
	}
	if _, err := s.models(context.Background(), ""); err != nil || f.calls != 1 {
		t.Fatalf("second call should hit cache: err=%v calls=%d", err, f.calls)
	}
}

func TestClaudeCodeCLIModels_LiveFailureFallsBackWithWarning(t *testing.T) {
	f := &fakeModelFetcher{err: &llm.ProviderError{StatusCode: 401, Message: "bad key"}}
	s := newClaudeModelsTestService(t, claudePoolCfg(true), f, nil)
	info, err := s.models(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if info.Source != "catalog" || info.Warning == "" || !slices.Contains(info.Models, "claude-opus-5-5") {
		t.Fatalf("unexpected: %+v", info)
	}
	f2 := &fakeModelFetcher{err: errors.New("boom")}
	s2 := newClaudeModelsTestService(t, claudePoolCfg(true), f2, nil)
	if info, err := s2.models(context.Background(), ""); err != nil || info.Source != "catalog" {
		t.Fatalf("unexpected: %+v %v", info, err)
	}
}

func TestClaudeCodeCLIModels_LiveEmptyIsIgnored(t *testing.T) {
	f := &fakeModelFetcher{models: []string{"other"}}
	s := newClaudeModelsTestService(t, claudePoolCfg(true), f, nil)
	info, _ := s.models(context.Background(), "")
	if info.Source != "catalog" || info.Warning == "" {
		t.Fatalf("unexpected: %+v", info)
	}
}

func TestParseClaudeCLIModelCache(t *testing.T) {
	got := parseClaudeCLIModelCache([]byte(`{"oauthAccount":{"x":"secret"},"additionalModelOptionsCache":[{"value":"claude-fable-5-1[1m]"},{"value":" "},{"value":"opus"}]}`))
	if !slices.Equal(got, []string{"claude-fable-5-1", "opus"}) {
		t.Fatalf("got %v", got)
	}
	for _, bad := range []string{"", "{", `{"additionalModelOptionsCache":"x"}`, `{}`} {
		if got := parseClaudeCLIModelCache([]byte(bad)); len(got) != 0 {
			t.Fatalf("%q => %v", bad, got)
		}
	}
}

func TestReadClaudeCLIModelCache(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	if got := readClaudeCLIModelCache(); got != nil {
		t.Fatalf("missing file: %v", got)
	}
	if err := os.WriteFile(filepath.Join(dir, ".claude.json"), []byte(`{"additionalModelOptionsCache":[{"value":"claude-x-1"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readClaudeCLIModelCache(); !slices.Equal(got, []string{"claude-x-1"}) {
		t.Fatalf("got %v", got)
	}
	if err := os.WriteFile(filepath.Join(dir, ".claude.json"), []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readClaudeCLIModelCache(); got != nil {
		t.Fatalf("corrupt: %v", got)
	}
}

func TestClaudeCodeSetupModels(t *testing.T) {
	s := newClaudeModelsTestService(t, claudePoolCfg(false), &fakeModelFetcher{}, []string{"claude-fable-5-1"})
	if got := s.claudeCodeSetupModels(); !slices.Contains(got, "claude-fable-5-1") || !slices.Contains(got, "haiku") {
		t.Fatalf("got %v", got)
	}
}
