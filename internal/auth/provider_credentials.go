package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

type CredentialSource = CodexCredentialSource

const (
	CredentialSourceEnv  CredentialSource = CodexCredentialSourceEnv
	CredentialSourceFile CredentialSource = CodexCredentialSourceFile
)

type ProviderCredential = CodexCredential

type ProviderAuthConfig struct {
	Provider      string
	AuthMode      string
	OAuthProvider string
	APIKey        string
	CodexHome     string
}

type ProviderRefreshOptions struct {
	TokenURL      string
	HTTPClient    *http.Client
	PersistSource bool
}

type providerCredentialStrategy struct {
	resolve func(ProviderAuthConfig) (ProviderCredential, error)
	refresh func(context.Context, ProviderAuthConfig, ProviderCredential, ProviderRefreshOptions) (ProviderCredential, error)
}

var providerCredentialStrategies = map[string]providerCredentialStrategy{
	"openai-codex": {
		resolve: resolveOpenAICodexProviderCredential,
		refresh: refreshOpenAICodexProviderCredential,
	},
}

var defaultProviderCredentialStrategy = providerCredentialStrategy{
	resolve: resolveGenericProviderCredential,
}

func ResolveProviderCredential(config ProviderAuthConfig) (ProviderCredential, error) {
	config = normalizeProviderAuthConfig(config)
	return providerCredentialStrategyFor(config.Provider).resolve(config)
}

func RefreshProviderCredential(
	ctx context.Context,
	config ProviderAuthConfig,
	cred ProviderCredential,
	opts ProviderRefreshOptions,
) (ProviderCredential, error) {
	config = normalizeProviderAuthConfig(config)
	strategy := providerCredentialStrategyFor(config.Provider)
	if strategy.refresh == nil {
		label := strings.TrimSpace(config.Provider)
		if label == "" {
			label = "provider"
		}
		return ProviderCredential{}, fmt.Errorf("%s does not support credential refresh", label)
	}
	return strategy.refresh(ctx, config, cred, opts)
}

func providerCredentialStrategyFor(provider string) providerCredentialStrategy {
	key := strings.TrimSpace(strings.ToLower(provider))
	if strategy, ok := providerCredentialStrategies[key]; ok {
		return strategy
	}
	return defaultProviderCredentialStrategy
}

func normalizeProviderAuthConfig(config ProviderAuthConfig) ProviderAuthConfig {
	config.Provider = strings.TrimSpace(strings.ToLower(config.Provider))
	config.AuthMode = strings.TrimSpace(strings.ToLower(config.AuthMode))
	config.OAuthProvider = strings.TrimSpace(strings.ToLower(config.OAuthProvider))
	config.APIKey = strings.TrimSpace(config.APIKey)
	config.CodexHome = strings.TrimSpace(config.CodexHome)
	return config
}

func resolveGenericProviderCredential(config ProviderAuthConfig) (ProviderCredential, error) {
	mode := config.AuthMode
	if mode == "" {
		mode = "api-key"
	}

	switch mode {
	case "api-key":
		if config.APIKey == "" {
			return ProviderCredential{}, fmt.Errorf("api key is required for auth mode api-key")
		}
		return ProviderCredential{
			AccessToken: config.APIKey,
			Source:      CredentialSourceEnv,
		}, nil
	case "oauth":
		token, err := resolveOAuthToken(config.Provider, config.OAuthProvider)
		if err != nil {
			return ProviderCredential{}, err
		}
		return ProviderCredential{
			AccessToken: strings.TrimSpace(token),
			Source:      CredentialSourceEnv,
		}, nil
	default:
		return ProviderCredential{}, fmt.Errorf("unsupported auth mode: %s", config.AuthMode)
	}
}

func resolveOpenAICodexProviderCredential(config ProviderAuthConfig) (ProviderCredential, error) {
	mode := config.AuthMode
	if mode == "" {
		mode = "oauth"
	}

	switch mode {
	case "api-key":
		if config.APIKey == "" {
			return ProviderCredential{}, fmt.Errorf("openai-codex api key is required for auth mode api-key")
		}
		return ProviderCredential{
			AccessToken: config.APIKey,
			AccountID:   ParseCodexAccountIDFromJWT(config.APIKey),
			Source:      CredentialSourceEnv,
		}, nil
	case "oauth":
		if cred, ok := resolveCodexCredentialFromEnv(); ok {
			return cred, nil
		}
		path, err := resolveCodexAuthPath(config.CodexHome)
		if err != nil {
			return ProviderCredential{}, err
		}
		return resolveCodexCredentialFromFile(path)
	default:
		return ProviderCredential{}, fmt.Errorf("openai-codex unsupported auth mode: %s", config.AuthMode)
	}
}

func refreshOpenAICodexProviderCredential(
	ctx context.Context,
	config ProviderAuthConfig,
	cred ProviderCredential,
	opts ProviderRefreshOptions,
) (ProviderCredential, error) {
	mode := config.AuthMode
	if mode == "" {
		mode = "oauth"
	}
	if mode != "oauth" {
		return ProviderCredential{}, fmt.Errorf("openai-codex auth mode %s does not support credential refresh", mode)
	}
	return refreshOpenAICodexCredential(ctx, cred, opts)
}
