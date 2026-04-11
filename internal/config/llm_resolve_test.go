package config

import (
	"strings"
	"testing"
)

func TestResolveLLMTier_TableCases(t *testing.T) {
	// Shared pool fixtures reused across cases.
	codex := LLMProviderSettings{
		Kind:          "openai-codex",
		AuthMode:      "oauth",
		OAuthProvider: "openai-codex",
		BaseURL:       "https://chatgpt.com/backend-api",
		ServiceTier:   "priority",
	}
	anthropic := LLMProviderSettings{
		Kind:     "anthropic",
		AuthMode: "api-key",
		APIKey:   "sk-ant-test",
		BaseURL:  "https://api.anthropic.com",
	}
	minimax := LLMProviderSettings{
		Kind:     "anthropic",
		AuthMode: "api-key",
		APIKey:   "sk-mm-test",
		BaseURL:  "https://api.minimax.io/anthropic",
	}

	tests := []struct {
		name    string
		cfg     Config
		tier    string
		want    ResolvedLLMTier
		wantErr string
	}{
		// ------------------------------------------------------------------
		// 1. 단일 provider, 세 tier 전부 같은 alias — shared credential view
		// ------------------------------------------------------------------
		{
			name: "single provider shared across tiers (heavy binding)",
			cfg: Config{
				LLMConfig: LLMConfig{
					LLMProviders: map[string]LLMProviderSettings{
						"default": anthropic,
					},
					LLMTiers: map[string]LLMTierBinding{
						"heavy":    {Provider: "default", Model: "claude-opus-4-6", ReasoningEffort: "high"},
						"standard": {Provider: "default", Model: "claude-sonnet-4-6", ReasoningEffort: "medium"},
						"light":    {Provider: "default", Model: "claude-haiku-4-5", ReasoningEffort: "minimal"},
					},
				},
			},
			tier: "heavy",
			want: ResolvedLLMTier{
				Tier:            "heavy",
				Kind:            "anthropic",
				AuthMode:        "api-key",
				BaseURL:         "https://api.anthropic.com",
				APIKey:          "sk-ant-test",
				Model:           "claude-opus-4-6",
				ReasoningEffort: "high",
				ProviderAlias:   "default",
			},
		},

		// ------------------------------------------------------------------
		// 2. 멀티 provider mix — tiers가 각자 다른 alias 참조
		// ------------------------------------------------------------------
		{
			name: "multi-provider mix (light tier uses minimax)",
			cfg: Config{
				LLMConfig: LLMConfig{
					LLMProviders: map[string]LLMProviderSettings{
						"codex":   codex,
						"minimax": minimax,
					},
					LLMTiers: map[string]LLMTierBinding{
						"heavy":    {Provider: "codex", Model: "gpt-5.4", ReasoningEffort: "high"},
						"standard": {Provider: "codex", Model: "gpt-5.4", ReasoningEffort: "medium"},
						"light":    {Provider: "minimax", Model: "MiniMax-M2.7", ReasoningEffort: "minimal"},
					},
				},
			},
			tier: "light",
			want: ResolvedLLMTier{
				Tier:            "light",
				Kind:            "anthropic",
				AuthMode:        "api-key",
				BaseURL:         "https://api.minimax.io/anthropic",
				APIKey:          "sk-mm-test",
				Model:           "MiniMax-M2.7",
				ReasoningEffort: "minimal",
				ProviderAlias:   "minimax",
			},
		},

		// ------------------------------------------------------------------
		// 3. 같은 provider, tier마다 다른 model — credential은 자동 공유
		// ------------------------------------------------------------------
		{
			name: "same provider different model per tier (standard)",
			cfg: Config{
				LLMConfig: LLMConfig{
					LLMProviders: map[string]LLMProviderSettings{
						"codex": codex,
					},
					LLMTiers: map[string]LLMTierBinding{
						"heavy":    {Provider: "codex", Model: "gpt-5.4", ReasoningEffort: "high"},
						"standard": {Provider: "codex", Model: "gpt-5.4", ReasoningEffort: "medium"},
						"light":    {Provider: "codex", Model: "gpt-5.4-mini", ReasoningEffort: "minimal"},
					},
				},
			},
			tier: "standard",
			want: ResolvedLLMTier{
				Tier:            "standard",
				Kind:            "openai-codex",
				AuthMode:        "oauth",
				OAuthProvider:   "openai-codex",
				BaseURL:         "https://chatgpt.com/backend-api",
				Model:           "gpt-5.4",
				ReasoningEffort: "medium",
				ServiceTier:     "priority", // inherited from provider default
				ProviderAlias:   "codex",
			},
		},

		// ------------------------------------------------------------------
		// 4. ServiceTier binding override beats provider default
		// ------------------------------------------------------------------
		{
			name: "binding ServiceTier overrides provider default",
			cfg: Config{
				LLMConfig: LLMConfig{
					LLMProviders: map[string]LLMProviderSettings{
						"codex": codex, // provider default: priority
					},
					LLMTiers: map[string]LLMTierBinding{
						"heavy": {
							Provider:    "codex",
							Model:       "gpt-5.4",
							ServiceTier: "flex", // binding override
						},
					},
				},
			},
			tier: "heavy",
			want: ResolvedLLMTier{
				Tier:          "heavy",
				Kind:          "openai-codex",
				AuthMode:      "oauth",
				OAuthProvider: "openai-codex",
				BaseURL:       "https://chatgpt.com/backend-api",
				Model:         "gpt-5.4",
				ServiceTier:   "flex",
				ProviderAlias: "codex",
			},
		},

		// ------------------------------------------------------------------
		// 5. 잘못된 alias 참조 — loud error
		// ------------------------------------------------------------------
		{
			name: "binding references unknown provider alias",
			cfg: Config{
				LLMConfig: LLMConfig{
					LLMProviders: map[string]LLMProviderSettings{
						"codex": codex,
					},
					LLMTiers: map[string]LLMTierBinding{
						"light": {Provider: "cdex", Model: "gpt-5.4"}, // typo
					},
				},
			},
			tier:    "light",
			wantErr: `unknown provider alias "cdex"`,
		},

		// ------------------------------------------------------------------
		// 6. tier 누락 — resolve 요청이 없는 tier
		// ------------------------------------------------------------------
		{
			name: "requested tier missing from llm_tiers",
			cfg: Config{
				LLMConfig: LLMConfig{
					LLMProviders: map[string]LLMProviderSettings{
						"default": anthropic,
					},
					LLMTiers: map[string]LLMTierBinding{
						"heavy": {Provider: "default", Model: "claude-opus-4-6"},
						// "standard" and "light" intentionally missing
					},
				},
			},
			tier:    "standard",
			wantErr: `tier "standard" not configured in llm_tiers`,
		},

		// ------------------------------------------------------------------
		// 7. provider Kind가 빈 값
		// ------------------------------------------------------------------
		{
			name: "provider has empty Kind",
			cfg: Config{
				LLMConfig: LLMConfig{
					LLMProviders: map[string]LLMProviderSettings{
						"broken": {Kind: "", AuthMode: "api-key"},
					},
					LLMTiers: map[string]LLMTierBinding{
						"heavy": {Provider: "broken", Model: "some-model"},
					},
				},
			},
			tier:    "heavy",
			wantErr: `provider "broken" has empty kind`,
		},

		// ------------------------------------------------------------------
		// 8. binding.Model 빈 값
		// ------------------------------------------------------------------
		{
			name: "binding has empty model",
			cfg: Config{
				LLMConfig: LLMConfig{
					LLMProviders: map[string]LLMProviderSettings{
						"default": anthropic,
					},
					LLMTiers: map[string]LLMTierBinding{
						"heavy": {Provider: "default", Model: "   "}, // whitespace only
					},
				},
			},
			tier:    "heavy",
			wantErr: `tier "heavy" binding has empty model`,
		},

		// ------------------------------------------------------------------
		// 9. Tier 이름 정규화 — "HEAVY" 입력이 "heavy"로 해석되고 결과도 소문자
		// ------------------------------------------------------------------
		{
			name: "tier name is normalized to lowercase",
			cfg: Config{
				LLMConfig: LLMConfig{
					LLMProviders: map[string]LLMProviderSettings{
						"default": anthropic,
					},
					LLMTiers: map[string]LLMTierBinding{
						"heavy": {Provider: "default", Model: "claude-opus-4-6", ReasoningEffort: "high"},
					},
				},
			},
			tier: "  HEAVY  ",
			want: ResolvedLLMTier{
				Tier:            "heavy",
				Kind:            "anthropic",
				AuthMode:        "api-key",
				BaseURL:         "https://api.anthropic.com",
				APIKey:          "sk-ant-test",
				Model:           "claude-opus-4-6",
				ReasoningEffort: "high",
				ProviderAlias:   "default",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveLLMTier(&tc.cfg, tc.tier)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil (result=%+v)", tc.wantErr, got)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tc.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("mismatch\n got: %+v\nwant: %+v", got, tc.want)
			}
		})
	}
}

func TestResolveLLMTier_NilConfig(t *testing.T) {
	if _, err := ResolveLLMTier(nil, "heavy"); err == nil {
		t.Fatal("expected error for nil config, got nil")
	}
}

func TestResolveLLMTier_EmptyTier(t *testing.T) {
	cfg := &Config{}
	if _, err := ResolveLLMTier(cfg, "   "); err == nil {
		t.Fatal("expected error for empty tier name, got nil")
	}
}

func TestResolveAllLLMTiers_Success(t *testing.T) {
	cfg := &Config{
		LLMConfig: LLMConfig{
			LLMProviders: map[string]LLMProviderSettings{
				"codex":   {Kind: "openai-codex", AuthMode: "oauth", BaseURL: "https://chatgpt.com/backend-api"},
				"minimax": {Kind: "anthropic", AuthMode: "api-key", APIKey: "sk-mm", BaseURL: "https://api.minimax.io/anthropic"},
			},
			LLMTiers: map[string]LLMTierBinding{
				"heavy":    {Provider: "codex", Model: "gpt-5.4", ReasoningEffort: "high"},
				"standard": {Provider: "codex", Model: "gpt-5.4", ReasoningEffort: "medium"},
				"light":    {Provider: "minimax", Model: "MiniMax-M2.7", ReasoningEffort: "minimal"},
			},
		},
	}

	resolved, err := ResolveAllLLMTiers(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved) != 3 {
		t.Fatalf("expected 3 resolved tiers, got %d", len(resolved))
	}

	for _, wantTier := range []string{"heavy", "standard", "light"} {
		entry, ok := resolved[wantTier]
		if !ok {
			t.Errorf("tier %q missing from resolved map", wantTier)
			continue
		}
		if entry.Tier != wantTier {
			t.Errorf("tier %q: Tier field = %q, want %q", wantTier, entry.Tier, wantTier)
		}
	}

	// Verify light tier resolved to the minimax provider, not codex.
	if got := resolved["light"].Kind; got != "anthropic" {
		t.Errorf("light.Kind = %q, want anthropic", got)
	}
	if got := resolved["light"].BaseURL; got != "https://api.minimax.io/anthropic" {
		t.Errorf("light.BaseURL = %q, want https://api.minimax.io/anthropic", got)
	}
	if got := resolved["light"].ProviderAlias; got != "minimax" {
		t.Errorf("light.ProviderAlias = %q, want minimax", got)
	}
}

func TestResolveAllLLMTiers_PropagatesError(t *testing.T) {
	cfg := &Config{
		LLMConfig: LLMConfig{
			LLMProviders: map[string]LLMProviderSettings{
				"codex": {Kind: "openai-codex"},
			},
			LLMTiers: map[string]LLMTierBinding{
				"heavy": {Provider: "codex", Model: "gpt-5.4"},
				"light": {Provider: "wrong", Model: "claude-haiku-4-5"}, // unknown alias
			},
		},
	}
	_, err := ResolveAllLLMTiers(cfg)
	if err == nil {
		t.Fatal("expected error from unknown alias, got nil")
	}
	if !strings.Contains(err.Error(), "unknown provider alias") {
		t.Fatalf("expected unknown alias error, got %q", err.Error())
	}
}

func TestResolveAllLLMTiers_NilConfig(t *testing.T) {
	if _, err := ResolveAllLLMTiers(nil); err == nil {
		t.Fatal("expected error for nil config, got nil")
	}
}
