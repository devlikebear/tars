package tarsserver

import (
	"net/http"
	"strings"

	"github.com/devlikebear/tars/internal/skillhub"
	"github.com/rs/zerolog"
)

func newSkillhubAPIHandler(
	installer *skillhub.Installer,
	extensions extensionsProvider,
	logger zerolog.Logger,
) http.Handler {
	mux := http.NewServeMux()

	// GET /v1/hub/registry — fetch remote registry index
	mux.HandleFunc("/v1/hub/registry", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if installer == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "hub is not configured"})
			return
		}
		index, err := installer.Registry.FetchIndex(r.Context())
		if err != nil {
			logger.Error().Err(err).Msg("fetch hub registry failed")
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to fetch registry: " + err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, index)
	})

	// GET /v1/hub/installed — list locally installed hub packages
	mux.HandleFunc("/v1/hub/installed", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if installer == nil {
			writeJSON(w, http.StatusOK, map[string]any{"skills": []any{}, "plugins": []any{}, "mcps": []any{}})
			return
		}
		skills, _ := installer.List()
		plugins, _ := installer.ListPlugins()
		mcps, _ := installer.ListMCPs()
		if skills == nil {
			skills = []skillhub.InstalledSkill{}
		}
		if plugins == nil {
			plugins = []skillhub.InstalledPlugin{}
		}
		if mcps == nil {
			mcps = []skillhub.InstalledMCP{}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"skills":  skills,
			"plugins": plugins,
			"mcps":    mcps,
		})
	})

	// POST /v1/hub/install — install a package from registry
	mux.HandleFunc("/v1/hub/install", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if installer == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "hub is not configured"})
			return
		}
		var req struct {
			Type string `json:"type"` // "skill", "plugin", "mcp"
			Name string `json:"name"`
		}
		if !decodeJSONBody(w, r, &req) {
			return
		}
		name := strings.TrimSpace(req.Name)
		if name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}

		var installErr error
		switch strings.TrimSpace(req.Type) {
		case "skill":
			_, installErr = installer.Install(r.Context(), name)
		case "plugin":
			installErr = installer.InstallPlugin(r.Context(), name)
		case "mcp":
			installErr = installer.InstallMCP(r.Context(), name)
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "type must be skill, plugin, or mcp"})
			return
		}
		if installErr != nil {
			logger.Error().Err(installErr).Str("type", req.Type).Str("name", name).Msg("hub install failed")
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": installErr.Error()})
			return
		}

		// Auto-reload extensions after install
		if extensions != nil {
			_ = extensions.Reload(r.Context())
		}

		logger.Info().Str("type", req.Type).Str("name", name).Msg("hub package installed")
		writeJSON(w, http.StatusOK, map[string]string{"ok": "true", "type": req.Type, "name": name})
	})

	// POST /v1/hub/uninstall — remove an installed package
	mux.HandleFunc("/v1/hub/uninstall", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if installer == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "hub is not configured"})
			return
		}
		var req struct {
			Type string `json:"type"`
			Name string `json:"name"`
		}
		if !decodeJSONBody(w, r, &req) {
			return
		}
		name := strings.TrimSpace(req.Name)
		if name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}

		var uninstallErr error
		switch strings.TrimSpace(req.Type) {
		case "skill":
			uninstallErr = installer.Uninstall(name)
		case "plugin":
			uninstallErr = installer.UninstallPlugin(name)
		case "mcp":
			uninstallErr = installer.UninstallMCP(name)
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "type must be skill, plugin, or mcp"})
			return
		}
		if uninstallErr != nil {
			logger.Error().Err(uninstallErr).Str("type", req.Type).Str("name", name).Msg("hub uninstall failed")
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": uninstallErr.Error()})
			return
		}

		if extensions != nil {
			_ = extensions.Reload(r.Context())
		}

		logger.Info().Str("type", req.Type).Str("name", name).Msg("hub package uninstalled")
		writeJSON(w, http.StatusOK, map[string]string{"ok": "true", "type": req.Type, "name": name})
	})

	// POST /v1/hub/update — update all installed packages to latest
	mux.HandleFunc("/v1/hub/update", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if installer == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "hub is not configured"})
			return
		}
		updatedSkills, _ := installer.Update(r.Context())
		updatedPlugins, _ := installer.UpdatePlugins(r.Context())

		if extensions != nil && (len(updatedSkills) > 0 || len(updatedPlugins) > 0) {
			_ = extensions.Reload(r.Context())
		}

		logger.Info().Int("skills", len(updatedSkills)).Int("plugins", len(updatedPlugins)).Msg("hub packages updated")
		writeJSON(w, http.StatusOK, map[string]any{
			"updated_skills":  updatedSkills,
			"updated_plugins": updatedPlugins,
		})
	})

	return mux
}
