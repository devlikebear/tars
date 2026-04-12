package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectLegacyKeys_FlatLLMFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tars.config.yaml")
	content := `
llm_api_key: sk-test
llm_provider: openai
llm_model: gpt-4o
api_addr: 127.0.0.1:43180
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	warnings := DetectLegacyKeys(path)
	if len(warnings) != 3 {
		t.Fatalf("expected 3 warnings, got %d: %+v", len(warnings), warnings)
	}

	keys := map[string]bool{}
	for _, w := range warnings {
		keys[w.Key] = true
		if w.Migration == "" {
			t.Errorf("empty migration for key %q", w.Key)
		}
	}
	for _, expected := range []string{"llm_api_key", "llm_provider", "llm_model"} {
		if !keys[expected] {
			t.Errorf("expected warning for %q, got keys %v", expected, keys)
		}
	}
}

func TestDetectLegacyKeys_TierPrefixes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tars.config.yaml")
	content := `
llm_tier_heavy_model: gpt-5
llm_role_chat_main: heavy
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	warnings := DetectLegacyKeys(path)
	if len(warnings) != 2 {
		t.Fatalf("expected 2 warnings, got %d: %+v", len(warnings), warnings)
	}
}

func TestDetectLegacyKeys_HeartbeatKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tars.config.yaml")
	content := `
heartbeat_enabled: true
heartbeat_interval: 60
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	warnings := DetectLegacyKeys(path)
	if len(warnings) != 2 {
		t.Fatalf("expected 2 warnings, got %d: %+v", len(warnings), warnings)
	}
	for _, w := range warnings {
		if !strings.Contains(w.Migration, "pulse") {
			t.Errorf("expected pulse migration hint for %q, got %q", w.Key, w.Migration)
		}
	}
}

func TestDetectLegacyKeys_CleanConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tars.config.yaml")
	content := `
api_addr: 127.0.0.1:43180
pulse_enabled: true
llm_providers:
  default:
    kind: anthropic
    api_key: test
llm_tiers:
  heavy:
    provider: default
    model: claude-opus-4-6
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	warnings := DetectLegacyKeys(path)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings for clean config, got %+v", warnings)
	}
}

func TestDetectLegacyKeys_MissingFile(t *testing.T) {
	warnings := DetectLegacyKeys("/nonexistent/path.yaml")
	if warnings != nil {
		t.Fatalf("expected nil for missing file, got %+v", warnings)
	}
}

func TestDetectLegacyKeys_EmptyPath(t *testing.T) {
	warnings := DetectLegacyKeys("")
	if warnings != nil {
		t.Fatalf("expected nil for empty path, got %+v", warnings)
	}
}

func TestFormatLegacyKeyWarnings(t *testing.T) {
	warnings := []LegacyKeyWarning{
		{Key: "llm_api_key", Migration: "set api_key inside the provider entry under llm_providers"},
	}
	formatted := FormatLegacyKeyWarnings(warnings)
	if !strings.Contains(formatted, "llm_api_key") || !strings.Contains(formatted, "llm_providers") {
		t.Fatalf("unexpected format: %q", formatted)
	}

	empty := FormatLegacyKeyWarnings(nil)
	if empty != "" {
		t.Fatalf("expected empty string for nil warnings, got %q", empty)
	}
}
