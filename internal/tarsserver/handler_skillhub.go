package tarsserver

import (
	"errors"
	"net/http"
	"strings"

	"github.com/devlikebear/tars/internal/skillhub"
	"github.com/rs/zerolog"
)

type hubUpdateDiagnosticPayload struct {
	Name   string `json:"name"`
	Reason string `json:"reason,omitempty"`
	Error  string `json:"error,omitempty"`
}

type hubUpdateResultPayloadBody struct {
	Updated []string                     `json:"updated"`
	Skipped []hubUpdateDiagnosticPayload `json:"skipped"`
	Failed  []hubUpdateDiagnosticPayload `json:"failed"`
}

func hubUpdateResultPayload(result skillhub.UpdateResult) hubUpdateResultPayloadBody {
	return hubUpdateResultPayloadBody{
		Updated: result.Updated,
		Skipped: hubUpdateDiagnosticsPayload(result.Skipped),
		Failed:  hubUpdateDiagnosticsPayload(result.Failed),
	}
}

func hubUpdateDiagnosticsPayload(items []skillhub.UpdateDiagnostic) []hubUpdateDiagnosticPayload {
	out := make([]hubUpdateDiagnosticPayload, 0, len(items))
	for _, item := range items {
		payload := hubUpdateDiagnosticPayload{Name: item.Name, Reason: strings.TrimSpace(item.Reason)}
		if item.Err != nil {
			payload.Error = item.Err.Error()
		}
		out = append(out, payload)
	}
	return out
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// hubSourceLabel returns the human-readable label for a HubSource ID. The
// console uses this when rendering the source selector. Unknown IDs are
// shown verbatim so future hubs work without server changes.
func hubSourceLabel(id string) string {
	switch id {
	case "tars-hub":
		return "TARS Hub"
	case "openclaw":
		return "openclaw"
	case "hermes":
		return "hermes-agent"
	case "anthropic":
		return "Anthropic skills"
	default:
		return id
	}
}

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

	// GET /v1/hub/sources — list every registered HubSource
	mux.HandleFunc("/v1/hub/sources", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if installer == nil || installer.Sources == nil {
			writeJSON(w, http.StatusOK, map[string]any{"sources": []any{}})
			return
		}
		out := make([]map[string]any, 0, installer.Sources.Len())
		for _, src := range installer.Sources.List() {
			id := src.ID()
			out = append(out, map[string]any{
				"id":       id,
				"label":    hubSourceLabel(id),
				"default":  id == skillhub.DefaultSourceID,
				"external": id != skillhub.DefaultSourceID,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"sources": out})
	})

	// GET /v1/hub/skills — federated skill search across registered sources
	// Query params:
	//   q       — substring filter (case-insensitive)
	//   source  — restrict to a single HubSource ID
	mux.HandleFunc("/v1/hub/skills", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if installer == nil {
			writeJSON(w, http.StatusOK, map[string]any{"skills": []any{}})
			return
		}
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		source := strings.TrimSpace(r.URL.Query().Get("source"))
		results, err := installer.SearchAllSkills(r.Context(), query, source)
		if err != nil {
			logger.Error().Err(err).Msg("federated skill search failed")
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if results == nil {
			results = []skillhub.SkillSearchResult{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"skills": results})
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
			Type   string `json:"type"` // "skill", "plugin", "mcp"
			Name   string `json:"name"`
			Source string `json:"source,omitempty"` // optional "openclaw"/"hermes"/"anthropic"
			Yes    bool   `json:"yes,omitempty"`    // auto-approve external-hub installs
			DryRun bool   `json:"dry_run,omitempty"`
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
		var installResult *skillhub.InstallResult
		switch strings.TrimSpace(req.Type) {
		case "skill":
			ref := name
			if s := strings.TrimSpace(req.Source); s != "" {
				ref = s + ":" + name
			}
			installResult, installErr = installer.InstallWithOptions(r.Context(), ref, skillhub.InstallOptions{
				Yes:    req.Yes || req.DryRun, // dry-run doesn't materialize so no confirm needed
				DryRun: req.DryRun,
			})
		case "plugin":
			installResult, installErr = installer.InstallPlugin(r.Context(), name)
		case "mcp":
			installResult, installErr = installer.InstallMCP(r.Context(), name)
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "type must be skill, plugin, or mcp"})
			return
		}
		if installErr != nil {
			logger.Error().Err(installErr).Str("type", req.Type).Str("name", name).Msg("hub install failed")
			payload := map[string]any{"error": installErr.Error()}
			var sandboxErr *skillhub.SandboxError
			if errors.As(installErr, &sandboxErr) {
				payload["sandbox_report"] = sandboxErr.Report
			}
			writeJSON(w, http.StatusBadRequest, payload)
			return
		}

		// Auto-reload extensions after a real install (skip on dry-run).
		if extensions != nil && !req.DryRun {
			_ = extensions.Reload(r.Context())
		}

		logger.Info().Str("type", req.Type).Str("name", name).Bool("dry_run", req.DryRun).Msg("hub install handled")
		payload := map[string]any{"ok": "true", "type": req.Type, "name": name, "dry_run": req.DryRun}
		if installResult != nil {
			payload["sandbox_report"] = installResult.Sandbox
			if installResult.RequiresPlugin != "" {
				payload["requires_plugin"] = installResult.RequiresPlugin
			}
			if installResult.DryRunPreview != nil {
				payload["preview"] = installResult.DryRunPreview
			}
		}
		writeJSON(w, http.StatusOK, payload)
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
		updatedSkills, skillErr := installer.Update(r.Context())
		updatedPlugins, pluginErr := installer.UpdatePlugins(r.Context())
		updateErr := errors.Join(skillErr, pluginErr)

		if extensions != nil && (len(updatedSkills.Updated) > 0 || len(updatedPlugins.Updated) > 0) {
			_ = extensions.Reload(r.Context())
		}

		logger.Info().Int("skills", len(updatedSkills.Updated)).Int("plugins", len(updatedPlugins.Updated)).Err(updateErr).Msg("hub packages updated")
		status := http.StatusOK
		if updateErr != nil {
			status = http.StatusInternalServerError
		}
		writeJSON(w, status, map[string]any{
			"updated_skills":       updatedSkills.Updated,
			"updated_plugins":      updatedPlugins.Updated,
			"skill_update_result":  hubUpdateResultPayload(updatedSkills),
			"plugin_update_result": hubUpdateResultPayload(updatedPlugins),
			"error":                errorString(updateErr),
		})
	})

	// GET /v1/hub/skill-content?name=X — fetch SKILL.md content from registry
	mux.HandleFunc("/v1/hub/skill-content", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if installer == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "hub is not configured"})
			return
		}
		name := strings.TrimSpace(r.URL.Query().Get("name"))
		if name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}
		entry, err := installer.Registry.FindByName(r.Context(), name)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		content, err := installer.Registry.FetchSkillContent(r.Context(), entry)
		if err != nil {
			logger.Error().Err(err).Str("name", name).Msg("fetch skill content failed")
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to fetch skill content"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"name":    entry.Name,
			"version": entry.Version,
			"content": string(content),
		})
	})

	return mux
}
