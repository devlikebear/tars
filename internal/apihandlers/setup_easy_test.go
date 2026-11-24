package apihandlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/onboarding"
	"github.com/rs/zerolog"
)

func TestEasySetupWritesClaudeCodeAndRestarts(t *testing.T) {
	prevGOOS := restartRuntimeGOOS
	restartRuntimeGOOS = "linux"
	t.Cleanup(func() { restartRuntimeGOOS = prevGOOS })

	dir := t.TempDir()
	path := filepath.Join(dir, "tars.config.yaml")
	// The file a first `tars serve` writes: no provider yet.
	if err := onboarding.WriteSkeletonConfig(path, filepath.Join(dir, "workspace"), "127.0.0.1:43180"); err != nil {
		t.Fatal(err)
	}
	restarted := make(chan struct{}, 2)
	handler := NewConfigHandler(path, config.Config{}, dir, func(string, []string, []string) error {
		restarted <- struct{}{}
		return nil
	}, zerolog.Nop())
	post := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/admin/setup/easy", nil))
		return rec
	}

	if rec := post(); rec.Code != http.StatusOK {
		t.Fatalf("easy setup: %d %s", rec.Code, rec.Body.String())
	}
	select {
	case <-restarted:
	case <-time.After(10 * time.Second):
		t.Fatal("the server was not restarted")
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.NeedsSetup(cfg) {
		t.Fatalf("still needs setup: %+v %+v", cfg.LLMProviders, cfg.LLMTiers)
	}
	if got := cfg.LLMProviders[EasySetupAlias].Kind; got != "claude-code-cli" {
		t.Fatalf("provider kind = %q", got)
	}
	for tier, model := range map[string]string{"heavy": "opus", "standard": "sonnet", "light": "haiku"} {
		if b := cfg.LLMTiers[tier]; b.Provider != EasySetupAlias || b.Model != model {
			t.Fatalf("%s = %+v", tier, b)
		}
	}
	// The workspace the skeleton named is kept.
	if raw, _ := os.ReadFile(path); !strings.Contains(string(raw), "workspace_dir") {
		t.Fatalf("the rest of the file was lost:\n%s", raw)
	}

	// Once set up, it never replaces what is there.
	if rec := post(); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "already_set_up") {
		t.Fatalf("second easy setup: %d %s", rec.Code, rec.Body.String())
	}
}

// A brand-new install may have no config file, or even its folder, yet.
func TestEasySetupWithoutAConfigFile(t *testing.T) {
	prevGOOS := restartRuntimeGOOS
	restartRuntimeGOOS = "linux"
	t.Cleanup(func() { restartRuntimeGOOS = prevGOOS })

	path := filepath.Join(t.TempDir(), "fresh", "tars.config.yaml")
	handler := NewConfigHandler(path, config.Config{}, "", nil, zerolog.Nop())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/admin/setup/easy", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	cfg, err := config.Load(path)
	if err != nil || config.NeedsSetup(cfg) {
		t.Fatalf("not set up: %v", err)
	}
}

func TestEasySetupRefusals(t *testing.T) {
	broken := filepath.Join(t.TempDir(), "tars.config.yaml")
	if err := os.WriteFile(broken, []byte("llm_providers: [unclosed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, path, method string
		code               int
	}{
		{"no config path", "", http.MethodPost, http.StatusBadRequest},
		{"a config that does not read", broken, http.MethodPost, http.StatusInternalServerError},
		{"the wrong method", broken, http.MethodGet, http.StatusMethodNotAllowed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewConfigHandler(tc.path, config.Config{}, "", nil, zerolog.Nop())
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(tc.method, "/v1/admin/setup/easy", nil))
			if rec.Code != tc.code {
				t.Fatalf("%d %s", rec.Code, rec.Body.String())
			}
		})
	}
}
