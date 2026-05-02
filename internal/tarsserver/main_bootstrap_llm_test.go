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

func TestBuildLLMDeps_FailsLoudOnEmptyTiers(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{
		RuntimeConfig: config.RuntimeConfig{WorkspaceDir: filepath.Join(dir, "workspace")},
	}
	logger := zerolog.New(io.Discard)
	base, err := buildBaseDeps(&options{}, cfg, time.Now, logger)
	if err != nil {
		t.Fatalf("buildBaseDeps: %v", err)
	}

	_, err = buildLLMDeps(base, cfg, logger)
	if err == nil {
		t.Fatalf("expected init_llm error on empty cfg")
	}
	var depErr *runtimeDepsError
	if !errors.As(err, &depErr) {
		t.Fatalf("expected *runtimeDepsError, got %T: %v", err, err)
	}
	if depErr.stage != "init_llm" {
		t.Fatalf("expected stage=init_llm, got %q (err=%v)", depErr.stage, err)
	}
}

func TestBuildLLMDeps_RejectsBaseWithoutUsageTracker(t *testing.T) {
	cfg := config.Config{}
	if _, err := buildLLMDeps(runtimeDeps{}, cfg, zerolog.New(io.Discard)); err == nil {
		t.Fatalf("expected error when base.usageTracker is nil")
	}
}

func TestBuildLLMDeps_PopulatesRouterOnValidConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := newAnthropicPoolCfg(
		map[string]string{"default": "sk-ant-shared"},
		map[string]config.LLMTierBinding{
			"heavy":    {Provider: "default", Model: "claude-opus-4-6", ReasoningEffort: "high"},
			"standard": {Provider: "default", Model: "claude-sonnet-4-6", ReasoningEffort: "medium"},
			"light":    {Provider: "default", Model: "claude-haiku-4-5", ReasoningEffort: "minimal"},
		},
		"standard",
		nil,
	)
	cfg.WorkspaceDir = filepath.Join(dir, "workspace")

	logger := zerolog.New(io.Discard)
	base, err := buildBaseDeps(&options{}, cfg, time.Now, logger)
	if err != nil {
		t.Fatalf("buildBaseDeps: %v", err)
	}

	deps, err := buildLLMDeps(base, cfg, logger)
	if err != nil {
		t.Fatalf("buildLLMDeps: %v", err)
	}
	if deps.llmRouter == nil {
		t.Fatalf("expected llmRouter populated")
	}
	if deps.usageTracker != base.usageTracker {
		t.Fatalf("expected base usageTracker preserved")
	}
}
