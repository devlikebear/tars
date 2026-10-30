package config

import "testing"

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
