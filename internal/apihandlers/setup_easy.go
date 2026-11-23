package apihandlers

import (
	"errors"
	"io/fs"
	"net/http"
	"strings"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/httpapi"
	"github.com/devlikebear/tars/internal/llmdefaults"
	"github.com/rs/zerolog"
)

// POST /v1/admin/setup/easy (vibe-coder-identity.md phase 4): easy mode's
// first run sets the engine up without asking. A beginner cannot choose a
// provider or a tier, and the first version runs on Claude Code (§7), so
// the server writes the one setup that version knows — Claude Code on every
// tier — and restarts. Whether Claude Code is installed and signed in is
// not checked here: the Start screen (GET /v1/easy/readiness) explains that
// step once the server is back.
//
// Only a server that still needs setup takes it (409 otherwise), so it never
// replaces a provider someone chose. It is mounted in setup-only mode, next
// to the wizard's routes, and is an admin path like theirs.

// EasySetupAlias is the provider alias easy setup writes.
const EasySetupAlias = "claude"

// EasySetupUpdates is the config patch easy setup applies: Claude Code as
// the only provider, with the CLI's own aliases per tier (the CLI resolves
// each to its newest model), the same picks the wizard recommends.
func EasySetupUpdates() map[string]any {
	tier := func(model string) map[string]any {
		return map[string]any{"provider": EasySetupAlias, "model": model}
	}
	return map[string]any{
		"llm_providers": map[string]any{
			EasySetupAlias: map[string]any{"kind": llmdefaults.ProviderClaudeCodeCLI, "auth_mode": "cli"},
		},
		"llm_tiers": map[string]any{
			"heavy":    tier("opus"),
			"standard": tier("sonnet"),
			"light":    tier("haiku"),
		},
	}
}

func handleEasySetup(w http.ResponseWriter, configPath string, restart RestartFunc, logger zerolog.Logger) {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		httpapi.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "no config file path configured"})
		return
	}
	// The file as it is now: the wizard may have saved a provider since
	// this server started.
	current, err := config.Load(configPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		logger.Error().Err(err).Str("path", configPath).Msg("easy setup: read config")
		httpapi.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "config could not be read"})
		return
	}
	if err == nil && !config.NeedsSetup(current) {
		httpapi.WriteJSON(w, http.StatusConflict, map[string]string{"error": "already set up", "code": "already_set_up"})
		return
	}
	if err := config.PatchYAML(configPath, EasySetupUpdates()); err != nil {
		logger.Error().Err(err).Str("path", configPath).Msg("easy setup: write config")
		httpapi.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "config could not be written"})
		return
	}
	logger.Info().Str("path", configPath).Msg("easy setup: Claude Code set up on every tier")
	handleRestart(w, restart, logger)
}
