package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault_ConfiguresCompactionKnobs(t *testing.T) {
	cfg := Default()
	if cfg.CompactionTriggerTokens != 100000 {
		t.Fatalf("expected trigger tokens 100000, got %d", cfg.CompactionTriggerTokens)
	}
	if cfg.CompactionKeepRecentTokens != 12000 {
		t.Fatalf("expected keep recent tokens 12000, got %d", cfg.CompactionKeepRecentTokens)
	}
	if cfg.CompactionKeepRecentFraction != 0.30 {
		t.Fatalf("expected keep recent fraction 0.30, got %v", cfg.CompactionKeepRecentFraction)
	}
	if cfg.CompactionLLMMode != "auto" {
		t.Fatalf("expected llm mode auto, got %q", cfg.CompactionLLMMode)
	}
	if cfg.CompactionLLMTimeoutSeconds != 15 {
		t.Fatalf("expected llm timeout 15, got %d", cfg.CompactionLLMTimeoutSeconds)
	}
}

func TestLoad_CompactionKnobsFromYAMLAndEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	raw := []byte("compaction_trigger_tokens: 123456\ncompaction_keep_recent_tokens: 2345\ncompaction_keep_recent_fraction: 0.42\ncompaction_llm_mode: deterministic\ncompaction_llm_timeout_seconds: 9\n")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("TARS_COMPACTION_KEEP_RECENT_TOKENS", "3456")
	t.Setenv("TARS_COMPACTION_LLM_TIMEOUT_SECONDS", "11")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.CompactionTriggerTokens != 123456 {
		t.Fatalf("expected trigger tokens from yaml, got %d", cfg.CompactionTriggerTokens)
	}
	if cfg.CompactionKeepRecentTokens != 3456 {
		t.Fatalf("expected env override keep recent tokens 3456, got %d", cfg.CompactionKeepRecentTokens)
	}
	if cfg.CompactionKeepRecentFraction != 0.42 {
		t.Fatalf("expected keep recent fraction 0.42, got %v", cfg.CompactionKeepRecentFraction)
	}
	if cfg.CompactionLLMMode != "deterministic" {
		t.Fatalf("expected deterministic llm mode, got %q", cfg.CompactionLLMMode)
	}
	if cfg.CompactionLLMTimeoutSeconds != 11 {
		t.Fatalf("expected env override llm timeout 11, got %d", cfg.CompactionLLMTimeoutSeconds)
	}
}
