package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/auth"
	"github.com/devlikebear/tars/internal/llmdefaults"
)

// ModelFetcher resolves provider model ids via provider-specific live APIs.
type ModelFetcher interface {
	FetchModels(ctx context.Context, opts ProviderOptions) ([]string, error)
}

type modelFetcher struct {
	httpClient           *http.Client
	resolveCredential    func(config auth.ProviderAuthConfig) (auth.ProviderCredential, error)
	refreshCredential    func(ctx context.Context, config auth.ProviderAuthConfig, cred auth.ProviderCredential, opts auth.ProviderRefreshOptions) (auth.ProviderCredential, error)
	openAICodexModelsURL string
}

type modelFetcherDeps struct {
	httpClient           *http.Client
	resolveCredential    func(config auth.ProviderAuthConfig) (auth.ProviderCredential, error)
	refreshCredential    func(ctx context.Context, config auth.ProviderAuthConfig, cred auth.ProviderCredential, opts auth.ProviderRefreshOptions) (auth.ProviderCredential, error)
	openAICodexModelsURL string
}

// NewModelFetcher returns a fetcher that lists a provider's available models.
func NewModelFetcher() ModelFetcher {
	return newModelFetcherWithDeps(modelFetcherDeps{})
}

func newModelFetcherWithDeps(deps modelFetcherDeps) *modelFetcher {
	httpClient := deps.httpClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	resolveCredential := deps.resolveCredential
	if resolveCredential == nil {
		resolveCredential = auth.ResolveProviderCredential
	}
	refreshCredential := deps.refreshCredential
	if refreshCredential == nil {
		refreshCredential = auth.RefreshProviderCredential
	}
	// Left empty in production: the Codex models URL is derived from the
	// provider's base URL per call, so it follows a custom endpoint the way the
	// chat URL does. Tests set it to point at their own server.
	openAICodexModelsURL := strings.TrimSpace(deps.openAICodexModelsURL)
	return &modelFetcher{
		httpClient:           httpClient,
		resolveCredential:    resolveCredential,
		refreshCredential:    refreshCredential,
		openAICodexModelsURL: openAICodexModelsURL,
	}
}

func (f *modelFetcher) FetchModels(ctx context.Context, opts ProviderOptions) ([]string, error) {
	provider := strings.TrimSpace(strings.ToLower(opts.Provider))
	// Default the base URL per provider, mirroring NewProvider, so callers that
	// only know the provider id (no custom endpoint) can list models.
	if defaults, ok := llmdefaults.ForKind(provider); ok {
		opts.BaseURL = firstNonEmptyTrimmed(opts.BaseURL, defaults.BaseURL)
	}
	switch provider {
	case "openai":
		return f.fetchOpenAICompatibleModels(ctx, opts)
	case "kimi":
		return f.fetchOpenAICompatibleModels(ctx, opts)
	case "anthropic":
		return f.fetchAnthropicModels(ctx, opts)
	case "gemini", "gemini-native":
		return f.fetchGeminiNativeModels(ctx, opts)
	case "openai-codex":
		return f.fetchOpenAICodexModels(ctx, opts)
	default:
		return nil, fmt.Errorf("unsupported llm provider: %s", provider)
	}
}

func (f *modelFetcher) fetchOpenAICompatibleModels(ctx context.Context, opts ProviderOptions) ([]string, error) {
	cred, err := f.resolveCredential(providerAuthConfig(opts))
	if err != nil {
		return nil, err
	}
	modelsURL, err := appendURLPath(opts.BaseURL, "/models")
	if err != nil {
		return nil, err
	}
	models, _, err := f.fetchOpenAIStyleModelIDs(ctx, strings.TrimSpace(strings.ToLower(opts.Provider)), modelsURL, map[string]string{
		"Authorization": "Bearer " + strings.TrimSpace(cred.AccessToken),
	})
	if err != nil {
		return nil, err
	}
	return models, nil
}

func (f *modelFetcher) fetchAnthropicModels(ctx context.Context, opts ProviderOptions) ([]string, error) {
	cred, err := f.resolveCredential(providerAuthConfig(opts))
	if err != nil {
		return nil, err
	}
	modelsURL, err := appendURLPath(opts.BaseURL, "/v1/models")
	if err != nil {
		return nil, err
	}
	models, _, err := f.fetchOpenAIStyleModelIDs(ctx, "anthropic", modelsURL, map[string]string{
		"x-api-key":         strings.TrimSpace(cred.AccessToken),
		"anthropic-version": anthropicAPIVersion,
		"content-type":      "application/json",
	})
	if err != nil {
		return nil, err
	}
	return models, nil
}

func (f *modelFetcher) fetchGeminiNativeModels(ctx context.Context, opts ProviderOptions) ([]string, error) {
	cred, err := f.resolveCredential(providerAuthConfig(opts))
	if err != nil {
		return nil, err
	}
	modelsURL, err := resolveGeminiNativeModelsURL(opts.BaseURL)
	if err != nil {
		return nil, err
	}
	body, _, err := f.fetchModelsBody(ctx, strings.TrimSpace(strings.ToLower(opts.Provider)), modelsURL, map[string]string{
		"x-goog-api-key": strings.TrimSpace(cred.AccessToken),
	})
	if err != nil {
		return nil, err
	}
	models, err := parseGeminiModelIDs(body)
	if err != nil {
		return nil, newProviderError(strings.TrimSpace(strings.ToLower(opts.Provider)), "parse", err)
	}
	return models, nil
}

// fetchOpenAICodexModels lists the models a ChatGPT-authenticated Codex
// session may use.
//
// This used to hit https://api.openai.com/v1/models -- the OpenAI *platform*
// API -- with the ChatGPT OAuth bearer token. A ChatGPT token is not a
// platform API key, so that endpoint answered 401 every time; the 401 was
// then read as an expired token, a perfectly good refresh token was spent
// on it, and the retry hit the same wrong host. The chat path never had this
// confusion: it talks to llmdefaults.OpenAICodexBaseURL, the ChatGPT backend.
//
// The Codex CLI lists models from that same backend, at <base>/codex/models
// with a client_version query (codex-rs/codex-api/src/endpoint/models.rs),
// and caches the answer in ~/.codex/models_cache.json. The response is not
// OpenAI-style {data:[{id}]} but {models:[{slug, visibility, priority, ...}]}
// (codex-rs/protocol/src/openai_models.rs, ModelInfo). Only visibility
// "list" entries are what the CLI shows; "hide" covers internal models such
// as codex-auto-review. They come back in the backend's priority order.
func (f *modelFetcher) fetchOpenAICodexModels(ctx context.Context, opts ProviderOptions) ([]string, error) {
	authConfig := providerAuthConfig(opts)
	cred, err := f.resolveCredential(authConfig)
	if err != nil {
		return nil, err
	}
	endpoint := f.openAICodexModelsURL
	if endpoint == "" {
		endpoint = resolveOpenAICodexModelsURL(opts.BaseURL)
	}
	endpoint = appendClientVersionQuery(endpoint, openAICodexClientVersion)

	models, status, err := f.fetchOpenAICodexModelSlugs(ctx, endpoint, cred)
	if err == nil {
		return models, nil
	}
	// A 401 from the right host is a stale token; refresh once and retry.
	// (A 401 from the wrong host was the old bug, and is why this refresh
	// is only reached after the URL above.)
	if status != http.StatusUnauthorized && status != http.StatusForbidden {
		return nil, err
	}
	if strings.TrimSpace(cred.RefreshToken) == "" {
		return nil, err
	}
	refreshed, refreshErr := f.refreshCredential(ctx, authConfig, cred, auth.ProviderRefreshOptions{PersistSource: true})
	if refreshErr != nil {
		return nil, refreshErr
	}
	models, _, err = f.fetchOpenAICodexModelSlugs(ctx, endpoint, refreshed)
	if err != nil {
		return nil, err
	}
	return models, nil
}

func (f *modelFetcher) fetchOpenAICodexModelSlugs(ctx context.Context, endpoint string, cred auth.ProviderCredential) ([]string, int, error) {
	accountID := strings.TrimSpace(cred.AccountID)
	if accountID == "" {
		accountID = auth.ParseCodexAccountIDFromJWT(cred.AccessToken)
	}
	// The same three headers the chat request sends; the backend keys the
	// account on chatgpt-account-id, not only on the bearer.
	body, status, err := f.fetchModelsBody(ctx, "openai-codex", endpoint, map[string]string{
		"Authorization":      "Bearer " + strings.TrimSpace(cred.AccessToken),
		"chatgpt-account-id": accountID,
		"originator":         codexOriginatorHeader,
	})
	if err != nil {
		return nil, status, err
	}
	models, err := parseOpenAICodexModelSlugs(body)
	if err != nil {
		return nil, status, newProviderError("openai-codex", "parse", err)
	}
	return models, status, nil
}

// resolveOpenAICodexModelsURL mirrors resolveOpenAICodexResponsesURL: the
// two endpoints are siblings under <base>/codex/.
func resolveOpenAICodexModelsURL(baseURL string) string {
	normalized := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if normalized == "" {
		normalized = llmdefaults.OpenAICodexBaseURL
	}
	if strings.HasSuffix(normalized, "/codex/models") {
		return normalized
	}
	if strings.HasSuffix(normalized, "/codex") {
		return normalized + "/models"
	}
	return normalized + "/codex/models"
}

// openAICodexClientVersion is what tars sends as client_version to the
// ChatGPT backend's /codex/models.
//
// v0.37.0 sent tars' own build version, on the theory that the query names
// the caller. It does not: the backend treats it as a compatibility floor
// and shapes the list by it. Measured against a live login on 2026-09-09
// with the same token:
//
//	dev      -> 400 {"detail":"Invalid client_version format"}
//	0.37.0   -> 200, models: []            (tars' version is "too old")
//	0.147.0  -> 200, 5 models, no gpt-6-astra
//	0.153.4  -> 200, 6 models, gpt-6-astra first
//	1.0.0    -> 200, same 6
//
// And when tars is a library -- linetta's case -- buildinfo.Version is never
// set at all, so every caller sent "dev" and got the 400. So the value must
// be a real Codex CLI version, and a recent one: whichever CLI release was
// last verified to list everything. Bump it when a new model the backend
// serves to a newer CLI fails to appear here.
const openAICodexClientVersion = "0.153.4"

// appendClientVersionQuery adds the client_version query the backend
// requires (ModelsClient::append_client_version_query in codex-rs). A blank
// version is sent as "dev", which the backend rejects -- that is the caller's
// bug to see, not one to paper over here.
func appendClientVersionQuery(endpoint, version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		version = "dev"
	}
	separator := "?"
	if strings.Contains(endpoint, "?") {
		separator = "&"
	}
	return endpoint + separator + "client_version=" + url.QueryEscape(version)
}

// parseOpenAICodexModelSlugs reads the Codex backend's models response and
// returns the listable slugs in the backend's priority order.
func parseOpenAICodexModelSlugs(body []byte) ([]string, error) {
	var payload struct {
		Models []struct {
			Slug       string `json:"slug"`
			Visibility string `json:"visibility"`
			Priority   int    `json:"priority"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode codex models response: %w", err)
	}
	type entry struct {
		slug     string
		priority int
	}
	var listed []entry
	for _, m := range payload.Models {
		slug := strings.TrimSpace(m.Slug)
		if slug == "" || strings.ToLower(strings.TrimSpace(m.Visibility)) != "list" {
			continue
		}
		listed = append(listed, entry{slug: slug, priority: m.Priority})
	}
	if len(listed) == 0 {
		return nil, fmt.Errorf("codex models response listed no models")
	}
	sort.SliceStable(listed, func(i, j int) bool {
		if listed[i].priority != listed[j].priority {
			return listed[i].priority < listed[j].priority
		}
		return listed[i].slug < listed[j].slug
	})
	out := make([]string, 0, len(listed))
	seen := map[string]bool{}
	for _, e := range listed {
		if seen[e.slug] {
			continue
		}
		seen[e.slug] = true
		out = append(out, e.slug)
	}
	return out, nil
}

func (f *modelFetcher) fetchOpenAIStyleModelIDs(ctx context.Context, provider, endpoint string, headers map[string]string) ([]string, int, error) {
	body, status, err := f.fetchModelsBody(ctx, provider, endpoint, headers)
	if err != nil {
		return nil, status, err
	}
	models, err := parseOpenAIStyleModelIDs(body)
	if err != nil {
		return nil, status, newProviderError(provider, "parse", err)
	}
	return models, status, nil
}

func (f *modelFetcher) fetchModelsBody(ctx context.Context, provider, endpoint string, headers map[string]string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(endpoint), nil)
	if err != nil {
		return nil, 0, newProviderError(provider, "request", fmt.Errorf("create models request: %w", err))
	}
	for key, value := range headers {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		trimmedValue := strings.TrimSpace(value)
		if trimmedValue == "" {
			continue
		}
		req.Header.Set(key, trimmedValue)
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, 0, newProviderError(provider, "request", fmt.Errorf("request %s models: %w", provider, err))
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, newProviderError(provider, "request", fmt.Errorf("read models response: %w", err))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode, newHTTPError(provider, resp.StatusCode, string(body))
	}
	return body, resp.StatusCode, nil
}

func appendURLPath(baseURL, pathSuffix string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", fmt.Errorf("invalid llm base url: %w", err)
	}
	if strings.TrimSpace(parsed.Scheme) == "" || strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("invalid llm base url")
	}
	basePath := strings.TrimRight(parsed.Path, "/")
	parsed.Path = basePath + pathSuffix
	parsed.RawQuery = ""
	return parsed.String(), nil
}

func resolveGeminiNativeModelsURL(baseURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", fmt.Errorf("invalid llm base url: %w", err)
	}
	if strings.TrimSpace(parsed.Scheme) == "" || strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("invalid llm base url")
	}
	basePath := strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(basePath, "/openai") {
		basePath = strings.TrimSuffix(basePath, "/openai")
	}
	basePath = strings.TrimRight(basePath, "/")
	parsed.Path = basePath + "/models"
	parsed.RawQuery = ""
	return parsed.String(), nil
}

func parseOpenAIStyleModelIDs(body []byte) ([]string, error) {
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode models response: %w", err)
	}
	models := make([]string, 0, len(payload.Data))
	for _, item := range payload.Data {
		models = append(models, strings.TrimSpace(item.ID))
	}
	return normalizeModelIDs(models), nil
}

func parseGeminiModelIDs(body []byte) ([]string, error) {
	var payload struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode gemini models response: %w", err)
	}
	models := make([]string, 0, len(payload.Models))
	for _, model := range payload.Models {
		name := strings.TrimSpace(model.Name)
		name = strings.TrimPrefix(name, "models/")
		models = append(models, name)
	}
	return normalizeModelIDs(models), nil
}

func normalizeModelIDs(raw []string) []string {
	set := make(map[string]struct{}, len(raw))
	models := make([]string, 0, len(raw))
	for _, item := range raw {
		model := strings.TrimSpace(item)
		model = strings.TrimPrefix(model, "models/")
		if model == "" {
			continue
		}
		if _, exists := set[model]; exists {
			continue
		}
		set[model] = struct{}{}
		models = append(models, model)
	}
	sort.Strings(models)
	return models
}
