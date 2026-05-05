package tarsserver

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/llmdefaults"
	"github.com/rs/zerolog"
)

func providerModelsWarnMessage(err error) string {
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		return ""
	}
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "scope") || strings.Contains(lower, "permission") || strings.Contains(lower, "insufficient") {
		return "Model listing is unavailable: API key lacks model list permissions. Enter the model ID manually."
	}
	return msg
}

var supportedLiveModelProviders = []string{
	"openai",
	"openai-codex",
	"kimi",
	"claude-code-cli",
	"gemini",
	"gemini-native",
	"anthropic",
}

type providersAPIInfo struct {
	// CurrentProvider is the Kind (e.g. "anthropic", "openai-codex") of
	// the default tier's resolved provider. Kept as "current_provider"
	// in the wire format to preserve existing console behavior.
	CurrentProvider string `json:"current_provider"`
	// CurrentModel is the model name bound to the default tier.
	CurrentModel string              `json:"current_model"`
	AuthMode     string              `json:"auth_mode"`
	Providers    []providerAPIStatus `json:"providers"`
	// Pool lists every entry in cfg.LLMProviders with its alias and
	// kind. Added for future multi-provider UI; existing console code
	// can ignore this field.
	Pool []providerPoolEntry `json:"pool"`
}

type providerAPIStatus struct {
	ID                 string `json:"id"`
	SupportsLiveModels bool   `json:"supports_live_models"`
}

// providerPoolEntry is one row in the providers API's `pool` array —
// an alias → kind mapping derived from cfg.LLMProviders.
type providerPoolEntry struct {
	Alias string `json:"alias"`
	Kind  string `json:"kind"`
}

type modelsAPIInfo struct {
	Provider     string   `json:"provider"`
	CurrentModel string   `json:"current_model"`
	Source       string   `json:"source"`
	Stale        bool     `json:"stale"`
	FetchedAt    string   `json:"fetched_at,omitempty"`
	ExpiresAt    string   `json:"expires_at,omitempty"`
	Models       []string `json:"models"`
	Warning      string   `json:"warning,omitempty"`
}

type providerModelsService struct {
	cfg     config.Config
	cache   *providerModelsCache
	fetcher llm.ModelFetcher
	nowFn   func() time.Time
}

func newProviderModelsService(cfg config.Config, cache *providerModelsCache, fetcher llm.ModelFetcher, nowFn func() time.Time) *providerModelsService {
	if fetcher == nil {
		fetcher = llm.NewModelFetcher()
	}
	if nowFn == nil {
		nowFn = time.Now
	}
	return &providerModelsService{
		cfg:     cfg,
		cache:   cache,
		fetcher: fetcher,
		nowFn:   nowFn,
	}
}

// defaultResolved returns the ResolvedLLMTier for cfg.LLMDefaultTier,
// or the zero value + false when it cannot be resolved (missing tier,
// unknown alias, empty pool, etc). The providers/models handlers fall
// back to empty-string responses rather than erroring so the console
// can still render.
func (s *providerModelsService) defaultResolved() (config.ResolvedLLMTier, bool) {
	tierName := strings.ToLower(strings.TrimSpace(s.cfg.LLMDefaultTier))
	if tierName == "" {
		tierName = "standard"
	}
	resolved, err := config.ResolveLLMTier(&s.cfg, tierName)
	if err != nil {
		return config.ResolvedLLMTier{}, false
	}
	return resolved, true
}

func (s *providerModelsService) providers() providersAPIInfo {
	var currentProvider, currentModel, authMode string
	if resolved, ok := s.defaultResolved(); ok {
		currentProvider = normalizeProviderValue(resolved.Kind)
		currentModel = resolved.Model
		authMode = normalizeAuthMode(resolved.AuthMode)
	}

	items := make([]providerAPIStatus, 0, len(supportedLiveModelProviders))
	for _, provider := range supportedLiveModelProviders {
		items = append(items, providerAPIStatus{
			ID:                 provider,
			SupportsLiveModels: providerSupportsLiveModels(provider),
		})
	}

	pool := make([]providerPoolEntry, 0, len(s.cfg.LLMProviders))
	for alias, p := range s.cfg.LLMProviders {
		pool = append(pool, providerPoolEntry{
			Alias: alias,
			Kind:  normalizeProviderValue(p.Kind),
		})
	}
	sort.Slice(pool, func(i, j int) bool { return pool[i].Alias < pool[j].Alias })

	return providersAPIInfo{
		CurrentProvider: currentProvider,
		CurrentModel:    currentModel,
		AuthMode:        authMode,
		Providers:       items,
		Pool:            pool,
	}
}

func (s *providerModelsService) models(ctx context.Context, providerAlias string) (modelsAPIInfo, error) {
	if s == nil {
		return modelsAPIInfo{}, fmt.Errorf("provider models service is not configured")
	}
	if s.cache == nil {
		return modelsAPIInfo{}, fmt.Errorf("provider models cache is not configured")
	}
	resolved, err := s.resolveProvider(providerAlias)
	if err != nil {
		return modelsAPIInfo{}, err
	}
	provider := normalizeProviderValue(resolved.Kind)
	if !s.supportsProvider(provider) {
		return modelsAPIInfo{}, fmt.Errorf("unsupported llm provider: %s", provider)
	}
	if !providerSupportsLiveModels(provider) {
		return modelsAPIInfo{}, fmt.Errorf("live model listing is unsupported for llm provider: %s", provider)
	}
	baseURL := normalizeBaseURL(resolved.BaseURL)
	authMode := normalizeAuthMode(resolved.AuthMode)
	currentModel := resolved.Model
	now := s.nowFn().UTC()

	cached, hasCached := s.cache.get(provider, baseURL, authMode)
	if hasCached && s.cache.isFresh(cached, now) {
		return s.responseFromCacheEntry(cached, currentModel, false, "", now), nil
	}

	models, err := s.fetcher.FetchModels(ctx, llm.ProviderOptions{
		Provider:      provider,
		AuthMode:      authMode,
		OAuthProvider: resolved.OAuthProvider,
		BaseURL:       baseURL,
		Model:         currentModel,
		APIKey:        resolved.APIKey,
	})
	if err == nil {
		models = appendCurrentModel(models, currentModel)
		fetchedAt := now.UTC()
		if cacheErr := s.cache.put(provider, baseURL, authMode, models, fetchedAt); cacheErr != nil {
			return modelsAPIInfo{}, cacheErr
		}
		return modelsAPIInfo{
			Provider:     provider,
			CurrentModel: currentModel,
			Source:       "live",
			Stale:        false,
			FetchedAt:    fetchedAt.Format(time.RFC3339),
			ExpiresAt:    fetchedAt.Add(s.cache.ttl).Format(time.RFC3339),
			Models:       append([]string(nil), models...),
		}, nil
	}

	if providerErr, ok := err.(*llm.ProviderError); ok && (providerErr.StatusCode == http.StatusUnauthorized || providerErr.StatusCode == http.StatusForbidden) {
		warn := providerModelsWarnMessage(err)
		if warn == "" {
			warn = fmt.Sprintf("provider models unavailable for %s", provider)
		}
		if hasCached {
			return s.responseFromCacheEntry(cached, currentModel, true, warn, now), nil
		}
		return modelsAPIInfo{
			Provider:     provider,
			CurrentModel: "",
			Source:       "provider_models_unavailable",
			Stale:        false,
			Models:       []string{},
			Warning:      warn,
		}, nil
	}

	if hasCached {
		return s.responseFromCacheEntry(cached, currentModel, true, err.Error(), now), nil
	}
	return modelsAPIInfo{}, err
}

func (s *providerModelsService) responseFromCacheEntry(entry providerModelsCacheEntry, currentModel string, stale bool, warning string, now time.Time) modelsAPIInfo {
	fetchedAt, ok := parseRFC3339(entry.FetchedAt)
	if !ok {
		fetchedAt = now.UTC()
	}
	expiresAt := fetchedAt.Add(s.cache.ttl)
	models := appendCurrentModel(entry.Models, currentModel)
	return modelsAPIInfo{
		Provider:     normalizeProviderValue(entry.Provider),
		CurrentModel: strings.TrimSpace(currentModel),
		Source:       "cache",
		Stale:        stale,
		FetchedAt:    fetchedAt.Format(time.RFC3339),
		ExpiresAt:    expiresAt.Format(time.RFC3339),
		Models:       models,
		Warning:      strings.TrimSpace(warning),
	}
}

func (s *providerModelsService) resolveProvider(providerAlias string) (config.ResolvedLLMTier, error) {
	alias := strings.TrimSpace(providerAlias)
	if alias == "" {
		resolved, ok := s.defaultResolved()
		if !ok {
			return config.ResolvedLLMTier{}, fmt.Errorf("default tier not resolvable — check llm_providers and llm_tiers config")
		}
		return resolved, nil
	}

	provider, ok := s.cfg.LLMProviders[alias]
	if !ok {
		return config.ResolvedLLMTier{}, fmt.Errorf("provider alias %q not found", alias)
	}
	kind := strings.ToLower(strings.TrimSpace(provider.Kind))
	if kind == "" {
		return config.ResolvedLLMTier{}, fmt.Errorf("provider %q has empty kind", alias)
	}

	authMode := normalizeAuthMode(provider.AuthMode)
	oauthProvider := ""
	if authMode == "oauth" {
		oauthProvider = strings.ToLower(strings.TrimSpace(llmdefaults.OAuthProvider(kind)))
	}

	return config.ResolvedLLMTier{
		Kind:          kind,
		AuthMode:      authMode,
		OAuthProvider: oauthProvider,
		BaseURL:       normalizeBaseURL(provider.BaseURL),
		APIKey:        strings.TrimSpace(provider.APIKey),
		Model:         strings.TrimSpace(s.currentModelForProviderAlias(alias)),
		ProviderAlias: alias,
	}, nil
}

func (s *providerModelsService) currentModelForProviderAlias(providerAlias string) string {
	alias := strings.TrimSpace(providerAlias)
	for _, tier := range sortedTierKeys(s.cfg.LLMTiers) {
		binding := s.cfg.LLMTiers[tier]
		if strings.TrimSpace(binding.Provider) != alias {
			continue
		}
		if model := strings.TrimSpace(binding.Model); model != "" {
			return model
		}
	}
	return ""
}

func sortedTierKeys(tiers map[string]config.LLMTierBinding) []string {
	keys := make([]string, 0, len(tiers))
	for name := range tiers {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	return keys
}

func (s *providerModelsService) supportsProvider(provider string) bool {
	return slices.Contains(supportedLiveModelProviders, normalizeProviderValue(provider))
}

func providerSupportsLiveModels(provider string) bool {
	switch normalizeProviderValue(provider) {
	case "claude-code-cli":
		return false
	default:
		return true
	}
}

func appendCurrentModel(models []string, currentModel string) []string {
	list := make([]string, 0, len(models)+1)
	set := map[string]struct{}{}
	for _, model := range models {
		trimmed := strings.TrimSpace(model)
		if trimmed == "" {
			continue
		}
		if _, exists := set[trimmed]; exists {
			continue
		}
		set[trimmed] = struct{}{}
		list = append(list, trimmed)
	}
	current := strings.TrimSpace(currentModel)
	if current != "" {
		if _, exists := set[current]; !exists {
			list = append(list, current)
		}
	}
	sort.Strings(list)
	return list
}

func newProvidersModelsAPIHandler(service *providerModelsService, logger zerolog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/providers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w)
			return
		}
		if service == nil {
			writeError(w, http.StatusInternalServerError, "providers_unavailable", "provider metadata service is not configured")
			return
		}
		writeJSON(w, http.StatusOK, service.providers())
	})

	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w)
			return
		}
		if service == nil {
			writeError(w, http.StatusInternalServerError, "models_unavailable", "provider models service is not configured")
			return
		}
		providerAlias := strings.TrimSpace(r.URL.Query().Get("provider_alias"))
		if providerAlias == "" {
			providerAlias = strings.TrimSpace(r.URL.Query().Get("provider"))
		}

		models, err := service.models(r.Context(), providerAlias)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unsupported for llm provider") || strings.Contains(strings.ToLower(err.Error()), "provider alias") {
				writeError(w, http.StatusBadRequest, "models_unsupported", err.Error())
				return
			}
			logger.Error().Err(err).Msg("fetch provider models failed")
			writeError(w, http.StatusInternalServerError, "models_unavailable", fmt.Sprintf("fetch provider models failed: %v", err))
			return
		}
		writeJSON(w, http.StatusOK, models)
	})

	return mux
}
