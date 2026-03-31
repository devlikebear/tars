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
)

const defaultOpenAICodexModelsURL = "https://api.openai.com/v1/models"

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
	openAICodexModelsURL := strings.TrimSpace(deps.openAICodexModelsURL)
	if openAICodexModelsURL == "" {
		openAICodexModelsURL = defaultOpenAICodexModelsURL
	}
	return &modelFetcher{
		httpClient:           httpClient,
		resolveCredential:    resolveCredential,
		refreshCredential:    refreshCredential,
		openAICodexModelsURL: openAICodexModelsURL,
	}
}

func (f *modelFetcher) FetchModels(ctx context.Context, opts ProviderOptions) ([]string, error) {
	provider := strings.TrimSpace(strings.ToLower(opts.Provider))
	switch provider {
	case "openai":
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

func (f *modelFetcher) fetchOpenAICodexModels(ctx context.Context, opts ProviderOptions) ([]string, error) {
	authConfig := providerAuthConfig(opts)
	cred, err := f.resolveCredential(authConfig)
	if err != nil {
		return nil, err
	}
	models, status, err := f.fetchOpenAIStyleModelIDs(ctx, "openai-codex", f.openAICodexModelsURL, map[string]string{
		"Authorization": "Bearer " + strings.TrimSpace(cred.AccessToken),
	})
	if err == nil {
		return models, nil
	}
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
	models, _, err = f.fetchOpenAIStyleModelIDs(ctx, "openai-codex", f.openAICodexModelsURL, map[string]string{
		"Authorization": "Bearer " + strings.TrimSpace(refreshed.AccessToken),
	})
	if err != nil {
		return nil, err
	}
	return models, nil
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
