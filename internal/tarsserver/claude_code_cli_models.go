package tarsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/llmdefaults"
)

// maxCLIConfigBytes bounds how much of ~/.claude.json is read; the file only
// grows with usage history, and the model cache sits in it as plain JSON.
const maxCLIConfigBytes = 32 << 20

// claudeCodeCLIModels answers /v1/models for claude-code-cli. The CLI has no
// command that lists models, so the list is assembled from, in priority order:
//
//  1. the live Anthropic Models API, when the provider pool has an
//     `anthropic` entry with an api_key (credentials stay at provider level;
//     the CLI's own OAuth login is never read),
//  2. the model options the CLI itself cached in ~/.claude.json
//     (read-only, only the `value` strings),
//  3. the built-in catalog in llmdefaults,
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
		reader = readClaudeCLIModelCache
	}
	info.Models = mergeClaudeCLIModels(live, reader(), llmdefaults.ClaudeCodeCLIModels,
		s.tierModelsForProviderAlias(resolved.ProviderAlias), []string{resolved.Model})
	return info, nil
}

// claudeCodeSetupModels is the list the setup wizard offers: the CLI's own
// cache plus the built-in catalog. No provider pool exists yet at that point,
// so there is no live lookup.
func (s *providerModelsService) claudeCodeSetupModels() []string {
	reader := s.readCLIModelCache
	if reader == nil {
		reader = readClaudeCLIModelCache
	}
	return mergeClaudeCLIModels(reader(), llmdefaults.ClaudeCodeCLIModels)
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
	for _, m := range fetched {
		if m = strings.TrimSpace(m); strings.HasPrefix(m, "claude-") {
			models = append(models, m)
		}
	}
	if len(models) == 0 {
		return nil, "", "", fmt.Sprintf("live model list from provider %q had no claude models", alias)
	}
	at := now.Format(time.RFC3339)
	if err := s.cache.put(cacheKind, baseURL, authMode, models, now); err != nil {
		return models, at, "", ""
	}
	return models, at, now.Add(s.cache.ttl).Format(time.RFC3339), ""
}

// readClaudeCLIModelCache reads the model ids the Claude Code CLI cached in its
// own config file. Only `additionalModelOptionsCache[].value` is read, never
// anything else in the file (it also holds OAuth account data). A missing,
// oversized or malformed file yields nothing.
func readClaudeCLIModelCache() []string {
	dir := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR"))
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		dir = home
	}
	f, err := os.Open(filepath.Join(dir, ".claude.json"))
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxCLIConfigBytes+1))
	if err != nil || len(data) > maxCLIConfigBytes {
		return nil
	}
	return parseClaudeCLIModelCache(data)
}

func parseClaudeCLIModelCache(data []byte) []string {
	var doc struct {
		Options []struct {
			Value string `json:"value"`
		} `json:"additionalModelOptionsCache"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return nil
	}
	var out []string
	for _, o := range doc.Options {
		v := strings.TrimSpace(o.Value)
		// "claude-fable-5-1[1m]": the suffix selects a context window; the
		// bare id is what the picker should hold.
		if i := strings.Index(v, "["); i >= 0 {
			v = strings.TrimSpace(v[:i])
		}
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// mergeClaudeCLIModels joins the sources without duplicates. Aliases (no
// "claude-" prefix) come first because the CLI maps them to its newest model,
// then the rest in source order.
func mergeClaudeCLIModels(sources ...[]string) []string {
	seen := map[string]bool{}
	var aliases, full []string
	for _, src := range sources {
		for _, m := range src {
			m = strings.TrimSpace(m)
			if m == "" || seen[m] {
				continue
			}
			seen[m] = true
			if strings.HasPrefix(m, "claude-") {
				full = append(full, m)
			} else {
				aliases = append(aliases, m)
			}
		}
	}
	return append(aliases, full...)
}
