package tarsserver

import (
	"errors"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/rs/zerolog"
)

func TestBuildBaseDeps_SucceedsWithoutLLMConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{
		RuntimeConfig: config.RuntimeConfig{WorkspaceDir: filepath.Join(dir, "workspace")},
		// No LLMProviders / LLMTiers — buildBaseDeps must not care.
	}
	opts := &options{}
	logger := zerolog.New(io.Discard)

	deps, err := buildBaseDeps(opts, cfg, time.Now, logger)
	if err != nil {
		t.Fatalf("buildBaseDeps with empty LLM config: %v", err)
	}
	if deps.usageTracker == nil {
		t.Fatalf("expected usage tracker populated")
	}
	if deps.sessionStore == nil {
		t.Fatalf("expected session store populated")
	}
	if deps.sessionStoreResolver == nil {
		t.Fatalf("expected session store resolver populated")
	}
	if deps.llmRouter != nil {
		t.Fatalf("expected llmRouter to remain nil — buildLLMDeps is the populator")
	}
}

func TestBuildBaseDeps_NilOptsErrors(t *testing.T) {
	cfg := config.Config{}
	if _, err := buildBaseDeps(nil, cfg, time.Now, zerolog.New(io.Discard)); err == nil {
		t.Fatalf("expected error on nil opts")
	}
}

func TestBuildBaseDeps_RejectsInsecureAuthWithoutOptIn(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{
		RuntimeConfig: config.RuntimeConfig{WorkspaceDir: filepath.Join(dir, "workspace")},
		APIConfig: config.APIConfig{
			APIAuthMode:               "off",
			APIAllowInsecureLocalAuth: false,
		},
	}
	opts := &options{}
	_, err := buildBaseDeps(opts, cfg, time.Now, zerolog.New(io.Discard))
	if err == nil {
		t.Fatalf("expected validate_config error")
	}
	var depErr *runtimeDepsError
	if !errors.As(err, &depErr) {
		t.Fatalf("expected *runtimeDepsError, got %T: %v", err, err)
	}
	if depErr.stage != "validate_config" {
		t.Fatalf("expected stage=validate_config, got %q", depErr.stage)
	}
}
