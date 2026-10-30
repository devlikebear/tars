package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyMemoryDefaults_OpenAIProviderDoesNotInheritGeminiEndpoint(t *testing.T) {
	cfg := Config{MemoryConfig: MemoryConfig{MemoryEmbedProvider: " OpenAI "}}
	applyMemoryDefaults(&cfg, Default())
	if cfg.MemoryEmbedProvider != "openai" {
		t.Fatalf("expected normalized provider openai, got %q", cfg.MemoryEmbedProvider)
	}
	if cfg.MemoryEmbedBaseURL != "https://api.openai.com/v1" {
		t.Fatalf("expected OpenAI base URL default, got %q", cfg.MemoryEmbedBaseURL)
	}
	if cfg.MemoryEmbedModel != "" {
		t.Fatalf("expected no Gemini model default for openai, got %q", cfg.MemoryEmbedModel)
	}
	if cfg.MemoryEmbedDimensions != 768 {
		t.Fatalf("expected dimensions default 768, got %d", cfg.MemoryEmbedDimensions)
	}
}

func TestApplyMemoryDefaults_OpenAIProviderKeepsExplicitValues(t *testing.T) {
	cfg := Config{MemoryConfig: MemoryConfig{
		MemoryEmbedProvider:   "openai",
		MemoryEmbedBaseURL:    "http://127.0.0.1:8000/v1",
		MemoryEmbedModel:      "kakaocorp/kanana-2-1.3b-embedding",
		MemoryEmbedDimensions: 1024,
	}}
	applyMemoryDefaults(&cfg, Default())
	if cfg.MemoryEmbedBaseURL != "http://127.0.0.1:8000/v1" || cfg.MemoryEmbedModel != "kakaocorp/kanana-2-1.3b-embedding" || cfg.MemoryEmbedDimensions != 1024 {
		t.Fatalf("expected explicit values kept, got %#v", cfg.MemoryConfig)
	}
}

func TestApplyMemoryDefaults_LocalProviderUsesManagedSidecarDefaults(t *testing.T) {
	cfg := Config{MemoryConfig: MemoryConfig{MemoryEmbedProvider: "local"}}
	applyMemoryDefaults(&cfg, Default())
	if cfg.MemoryEmbedBaseURL != DefaultLocalEmbedBaseURL {
		t.Fatalf("expected local sidecar base URL, got %q", cfg.MemoryEmbedBaseURL)
	}
	if cfg.MemoryEmbedModel != DefaultLocalEmbedModel {
		t.Fatalf("expected kanana default model, got %q", cfg.MemoryEmbedModel)
	}
	if cfg.MemoryEmbedDimensions != 1024 {
		t.Fatalf("expected local default dimensions 1024, got %d", cfg.MemoryEmbedDimensions)
	}
}

// Load seeds Default() (Gemini's endpoint, model and 768 dims) before the
// file is merged; a provider switch in the file alone must not inherit them.
func TestLoad_ProviderSwitchDoesNotInheritGeminiValues(t *testing.T) {
	for provider, want := range map[string]MemoryConfig{
		"openai": {MemoryEmbedBaseURL: "https://api.openai.com/v1", MemoryEmbedModel: "", MemoryEmbedDimensions: 768},
		"local":  {MemoryEmbedBaseURL: DefaultLocalEmbedBaseURL, MemoryEmbedModel: DefaultLocalEmbedModel, MemoryEmbedDimensions: DefaultLocalEmbedDimensions},
	} {
		t.Run(provider, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte("memory_embed_provider: "+provider+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if cfg.MemoryEmbedBaseURL != want.MemoryEmbedBaseURL || cfg.MemoryEmbedModel != want.MemoryEmbedModel || cfg.MemoryEmbedDimensions != want.MemoryEmbedDimensions {
				t.Fatalf("got %+v, want %+v", cfg.MemoryConfig, want)
			}
		})
	}
}
