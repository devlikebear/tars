package tarsserver

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/devlikebear/tars/internal/config"
	"github.com/rs/zerolog"
)

// setupStatusResponse is the body shape for GET /v1/setup/status.
// It tells the onboarding wizard where the config lives, which
// providers/tiers are present, and a list of checks suitable for
// rendering in a UI ("why is setup needed?").
type setupStatusResponse struct {
	NeedsSetup   bool                       `json:"needs_setup"`
	ConfigPath   string                     `json:"config_path,omitempty"`
	ConfigExists bool                       `json:"config_exists"`
	Providers    setupProviderStatus        `json:"providers"`
	Tiers        map[string]setupTierStatus `json:"tiers"`
	Capabilities setupCapabilityStatus      `json:"capabilities"`
	Checks       []setupCheck               `json:"checks"`
}

type setupProviderStatus struct {
	Configured []string `json:"configured"`
	Missing    bool     `json:"missing"`
}

type setupTierStatus struct {
	Configured bool   `json:"configured"`
	Provider   string `json:"provider,omitempty"`
	Model      string `json:"model,omitempty"`
}

// setupCapabilityStatus surfaces non-secret capability booleans so the
// onboarding completion matrix can render ✓/✗ markers without re-fetching
// the admin config schema. Per-capability flags drive individual rows;
// the *_configured aggregates group rows by wizard section.
type setupCapabilityStatus struct {
	WebSearchEnabled     bool `json:"web_search_enabled"`
	WebSearchAPIKeySet   bool `json:"web_search_api_key_set"`
	WebFetchEnabled      bool `json:"web_fetch_enabled"`
	MemoryEmbedAPIKeySet bool `json:"memory_embed_api_key_set"`
	TelegramEnabled      bool `json:"telegram_enabled"`
	TelegramBotTokenSet  bool `json:"telegram_bot_token_set"`
	WebhookEnabled       bool `json:"webhook_enabled"`

	ToolsConfigured        bool `json:"tools_configured"`
	IntegrationsConfigured bool `json:"integrations_configured"`
	ChannelsConfigured     bool `json:"channels_configured"`
}

type setupCheck struct {
	ID      string `json:"id"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// newSetupAPIHandler returns the /v1/setup/status handler. It reloads
// config from configPath when set so that the wizard sees changes
// patched via /v1/admin/config/values immediately. When configPath is
// empty (or read fails) it falls back to the cfg captured at boot.
//
// API key fields are never returned — only provider aliases and tier
// bindings (which carry no secret material).
func newSetupAPIHandler(configPath string, cfg config.Config, logger zerolog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/setup/status", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		handleGetSetupStatus(w, configPath, cfg, logger)
	})
	return mux
}

func handleGetSetupStatus(w http.ResponseWriter, configPath string, fallback config.Config, logger zerolog.Logger) {
	active := fallback
	exists := false
	trimmedPath := strings.TrimSpace(configPath)
	if trimmedPath != "" {
		if loaded, err := config.Load(trimmedPath); err == nil {
			active = loaded
		} else if !os.IsNotExist(err) {
			logger.Warn().Err(err).Str("path", trimmedPath).Msg("setup status reload failed; using cached config")
		}
		if info, err := os.Stat(trimmedPath); err == nil && !info.IsDir() {
			exists = true
		}
	}

	resp := setupStatusResponse{
		NeedsSetup:   config.NeedsSetup(active),
		ConfigPath:   trimmedPath,
		ConfigExists: exists,
		Providers:    buildProviderStatus(active),
		Tiers:        buildTierStatus(active),
		Capabilities: buildCapabilityStatus(active),
	}
	resp.Checks = buildSetupChecks(resp)
	writeJSON(w, http.StatusOK, resp)
}

func buildCapabilityStatus(cfg config.Config) setupCapabilityStatus {
	webSearchAPIKey := strings.TrimSpace(cfg.ToolsWebSearchAPIKey) != ""
	memoryEmbedAPIKey := strings.TrimSpace(cfg.MemoryEmbedAPIKey) != ""
	telegramToken := strings.TrimSpace(cfg.TelegramBotToken) != ""

	caps := setupCapabilityStatus{
		WebSearchEnabled:     cfg.ToolsWebSearchEnabled,
		WebSearchAPIKeySet:   webSearchAPIKey,
		WebFetchEnabled:      cfg.ToolsWebFetchEnabled,
		MemoryEmbedAPIKeySet: memoryEmbedAPIKey,
		TelegramEnabled:      cfg.ChannelsTelegramEnabled,
		TelegramBotTokenSet:  telegramToken,
		WebhookEnabled:       cfg.ChannelsWebhookEnabled,
	}
	caps.ToolsConfigured = caps.WebSearchEnabled || caps.WebFetchEnabled
	caps.IntegrationsConfigured = caps.WebSearchAPIKeySet || caps.MemoryEmbedAPIKeySet
	caps.ChannelsConfigured = (caps.TelegramEnabled && caps.TelegramBotTokenSet) || caps.WebhookEnabled
	return caps
}

func buildProviderStatus(cfg config.Config) setupProviderStatus {
	aliases := make([]string, 0, len(cfg.LLMProviders))
	for alias := range cfg.LLMProviders {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return setupProviderStatus{
		Configured: aliases,
		Missing:    len(aliases) == 0,
	}
}

func buildTierStatus(cfg config.Config) map[string]setupTierStatus {
	out := make(map[string]setupTierStatus, 3)
	for _, tier := range []string{"heavy", "standard", "light"} {
		binding, ok := cfg.LLMTiers[tier]
		provider := strings.TrimSpace(binding.Provider)
		model := strings.TrimSpace(binding.Model)
		out[tier] = setupTierStatus{
			Configured: ok && provider != "" && model != "",
			Provider:   provider,
			Model:      model,
		}
	}
	return out
}

func buildSetupChecks(resp setupStatusResponse) []setupCheck {
	checks := make([]setupCheck, 0, 2)

	providersOK := !resp.Providers.Missing
	providersMsg := "no providers configured"
	if providersOK {
		providersMsg = fmt.Sprintf("%d provider(s) configured", len(resp.Providers.Configured))
	}
	checks = append(checks, setupCheck{ID: "providers_present", OK: providersOK, Message: providersMsg})

	missingTiers := make([]string, 0, 3)
	for _, tier := range []string{"heavy", "standard", "light"} {
		if !resp.Tiers[tier].Configured {
			missingTiers = append(missingTiers, tier)
		}
	}
	tiersOK := len(missingTiers) == 0
	tiersMsg := "all 3 tiers bound"
	if !tiersOK {
		tiersMsg = "tier(s) missing: " + strings.Join(missingTiers, ", ")
	}
	checks = append(checks, setupCheck{ID: "tiers_complete", OK: tiersOK, Message: tiersMsg})

	return checks
}
