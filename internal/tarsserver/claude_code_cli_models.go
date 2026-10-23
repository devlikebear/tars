package tarsserver

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/llmdefaults"
	"github.com/devlikebear/tars/pkg/llm"
)

// claudeCodeCLIModels answers /v1/models for claude-code-cli. The CLI has no
// command that lists models, so the list is assembled from, in priority order:
//
//  1. the live Anthropic Models API, when the provider pool has an
//     `anthropic` entry with an api_key (credentials stay at provider level;
//     the CLI's own OAuth login is never read),
//  2. the model options the CLI itself cached in ~/.claude.json
//     (read-only, only the `value` strings),
//  3. the built-in catalog (llm.ClaudeCodeCLIModelCatalog),
//
// plus the models the tiers are bound to and the current one, so a pinned
// model never disappears from the picker.
func (s *providerModelsService) claudeCodeCLIModels(ctx context.Context, resolved config.ResolvedLLMTier, provider string) (modelsAPIInfo, error) {
	cliPath, err := s.findCLI(provider)
	if err != nil {
		return modelsAPIInfo{}, fmt.Errorf("%w: %v", errCLIProviderUnavailable, err)
	}
	info := modelsAPIInfo{
		Provider:     provider,
		CurrentModel: resolved.Model,
		Source:       "catalog",
		CLIPath:      cliPath,
	}

	var live []string
	if alias, p, ok := s.anthropicPoolProvider(); ok {
		var warn string
		live, info.FetchedAt, info.ExpiresAt, warn = s.liveClaudeModels(ctx, alias, p)
		if live != nil {
			info.Source = "live"
		}
		info.Warning = warn
	}

	reader := s.readCLIModelCache
	if reader == nil {
		reader = llm.ReadClaudeCodeCLIModelCache
	}
	info.Models = llm.MergeClaudeCodeCLIModels(live, reader(), llm.ClaudeCodeCLIModelCatalog(),
		s.tierModelsForProviderAlias(resolved.ProviderAlias), []string{resolved.Model})
	return info, nil
}

// claudeCodeSetupModels is the list the setup wizard offers: the CLI's own
// cache plus the built-in catalog. No provider pool exists yet at that point,
// so there is no live lookup.
func (s *providerModelsService) claudeCodeSetupModels() []string {
	reader := s.readCLIModelCache
	if reader == nil {
		reader = llm.ReadClaudeCodeCLIModelCache
	}
	return llm.MergeClaudeCodeCLIModels(reader(), llm.ClaudeCodeCLIModelCatalog())
}

// anthropicPoolProvider returns the first (by alias) anthropic provider that
// has an api_key.
func (s *providerModelsService) anthropicPoolProvider() (string, config.LLMProviderSettings, bool) {
	aliases := make([]string, 0, len(s.cfg.LLMProviders))
	for alias := range s.cfg.LLMProviders {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	for _, alias := range aliases {
		p := s.cfg.LLMProviders[alias]
		if normalizeProviderValue(p.Kind) == llmdefaults.ProviderAnthropic && strings.TrimSpace(p.APIKey) != "" {
			return alias, p, true
		}
	}
	return "", config.LLMProviderSettings{}, false
}

// liveClaudeModels returns the claude- model ids from the Anthropic Models API,
// via the shared cache. A nil slice means "not available" and warn says why
// when that is worth showing.
func (s *providerModelsService) liveClaudeModels(ctx context.Context, alias string, p config.LLMProviderSettings) (models []string, fetchedAt, expiresAt, warn string) {
	baseURL := normalizeBaseURL(p.BaseURL)
	if baseURL == "" {
		baseURL = llmdefaults.AnthropicBaseURL
	}
	authMode := normalizeAuthMode(p.AuthMode)
	now := s.nowFn().UTC()
	// Keyed by the claude-code-cli kind: the entry holds the filtered list,
	// not the anthropic provider's own full list.
	const cacheKind = llmdefaults.ProviderClaudeCodeCLI

	cached, hasCached := s.cache.get(cacheKind, baseURL, authMode)
	if hasCached && s.cache.isFresh(cached, now) {
		t, _ := parseRFC3339(cached.FetchedAt)
		return cached.Models, cached.FetchedAt, t.Add(s.cache.ttl).Format(time.RFC3339), ""
	}

	fetched, err := s.fetcher.FetchModels(ctx, llm.ProviderOptions{
		Provider: llmdefaults.ProviderAnthropic,
		AuthMode: authMode,
		BaseURL:  baseURL,
		APIKey:   strings.TrimSpace(p.APIKey),
	})
	if err != nil {
		warn = fmt.Sprintf("live model list from provider %q unavailable: %s", alias, providerModelsWarnMessage(err))
		if hasCached {
			return cached.Models, cached.FetchedAt, "", warn
		}
		return nil, "", "", warn
	}
	models = llm.ClaudeModelIDs(fetched)
	if len(models) == 0 {
		return nil, "", "", fmt.Sprintf("live model list from provider %q had no claude models", alias)
	}
	at := now.Format(time.RFC3339)
	if err := s.cache.put(cacheKind, baseURL, authMode, models, now); err != nil {
		return models, at, "", ""
	}
	return models, at, now.Add(s.cache.ttl).Format(time.RFC3339), ""
}
