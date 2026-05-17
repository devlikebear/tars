package config

import (
	"encoding/json"
	"os"
	"strings"
)

func parseEmbodimentProvidersJSON(raw string, fallback []EmbodimentProviderConfig) []EmbodimentProviderConfig {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback
	}

	var parsed []EmbodimentProviderConfig
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		var keyed map[string]EmbodimentProviderConfig
		if err := json.Unmarshal([]byte(trimmed), &keyed); err != nil {
			return fallback
		}
		parsed = make([]EmbodimentProviderConfig, 0, len(keyed))
		for name, provider := range keyed {
			if strings.TrimSpace(provider.Name) == "" {
				provider.Name = name
			}
			parsed = append(parsed, provider)
		}
	}

	out := make([]EmbodimentProviderConfig, 0, len(parsed))
	for _, provider := range parsed {
		normalized := normalizeEmbodimentProviderConfig(provider)
		if normalized.Name == "" {
			continue
		}
		out = append(out, normalized)
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}

func embodimentProvidersField(yamlKey string, envKeys []string) configInputField {
	return configInputField{
		yamlKey: yamlKey,
		envKeys: envKeys,
		apply: func(cfg *Config, raw string) {
			cfg.Embodiment.Providers = parseEmbodimentProvidersJSON(raw, cfg.Embodiment.Providers)
		},
		merge: func(dst *Config, src Config) {
			if len(src.Embodiment.Providers) == 0 {
				return
			}
			dst.Embodiment.Providers = cloneEmbodimentProviders(src.Embodiment.Providers)
		},
	}
}

func normalizeEmbodimentProviderConfig(provider EmbodimentProviderConfig) EmbodimentProviderConfig {
	provider.Name = strings.TrimSpace(os.ExpandEnv(provider.Name))
	provider.Transport = strings.TrimSpace(strings.ToLower(os.ExpandEnv(provider.Transport)))
	provider.Endpoint = strings.TrimSpace(os.ExpandEnv(provider.Endpoint))
	provider.SessionID = strings.TrimSpace(os.ExpandEnv(provider.SessionID))
	provider.Agent = strings.TrimSpace(os.ExpandEnv(provider.Agent))
	provider.MinTriggerInterval = strings.TrimSpace(os.ExpandEnv(provider.MinTriggerInterval))
	provider.Capabilities = normalizeEmbodimentCapabilities(provider.Capabilities)
	return provider
}

func NormalizeEmbodimentProviderForRuntime(provider EmbodimentProviderConfig) EmbodimentProviderConfig {
	return normalizeEmbodimentProviderConfig(provider)
}

func normalizeEmbodimentCapabilities(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		normalized := strings.TrimSpace(strings.ToLower(os.ExpandEnv(value)))
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func cloneEmbodimentProviders(src []EmbodimentProviderConfig) []EmbodimentProviderConfig {
	cloned := make([]EmbodimentProviderConfig, 0, len(src))
	for _, provider := range src {
		provider.Capabilities = append([]string(nil), provider.Capabilities...)
		cloned = append(cloned, provider)
	}
	return cloned
}
