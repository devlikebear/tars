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
		results, sourceErrors, err := installer.SearchAllSkillsWithErrors(r.Context(), query, source)
		if err != nil {
			logger.Error().Err(err).Msg("federated skill search failed")
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if results == nil {
			results = []skillhub.SkillSearchResult{}
		}
		// source_errors names each hub that could not be searched, so the
		// console says so instead of showing a silently shorter list.
		writeJSON(w, http.StatusOK, map[string]any{"skills": results, "source_errors": sourceErrors})
	})

	// GET /v1/hub/installed — list locally installed hub packages
	mux.HandleFunc("/v1/hub/installed", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if installer == nil {
			writeJSON(w, http.StatusOK, map[string]any{"skills": []any{}, "plugins": []any{}, "mcps": []any{}, "focus_templates": []any{}})
			return
		}
		skills, skillErr := installer.List()
		plugins, pluginErr := installer.ListPlugins()
		mcps, mcpErr := installer.ListMCPs()
		templates, templateErr := installer.ListFocusTemplates()
		// A broken skillhub.json must not read as "nothing installed": the
		// console would offer installs that then fail on the same file.
		if err := errors.Join(skillErr, pluginErr, mcpErr, templateErr); err != nil {
			logger.Error().Err(err).Msg("list installed hub packages failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if skills == nil {
			skills = []skillhub.InstalledSkill{}
		}
		if plugins == nil {
			plugins = []skillhub.InstalledPlugin{}
		}
		if mcps == nil {
			mcps = []skillhub.InstalledMCP{}
		}
		if templates == nil {
			templates = []skillhub.InstalledFocusTemplate{}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"skills":          skills,
			"plugins":         plugins,
			"mcps":            mcps,
			"focus_templates": templates,
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
			Type   string `json:"type"` // "skill", "plugin", "mcp", "focus_template", "pack"
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

		if strings.TrimSpace(req.Type) == "pack" {
			handlePackInstall(w, r, installer, extensions, logger, name, req.DryRun)
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
		case "focus_template":
			installResult, installErr = installer.InstallFocusTemplate(r.Context(), name)
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "type must be skill, plugin, mcp, focus_template, or pack"})
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

		// Auto-reload extensions after a real install (skip on dry-run; a
		// focus template is read from disk by the focus pages, not loaded).
		var reloadErr error
		if !req.DryRun && req.Type != "focus_template" {
			reloadErr = reloadHubExtensions(r, extensions, logger)
		}

		logger.Info().Str("type", req.Type).Str("name", name).Bool("dry_run", req.DryRun).Msg("hub install handled")
		payload := map[string]any{"ok": "true", "type": req.Type, "name": name, "dry_run": req.DryRun}
		if reloadErr != nil {
			payload["reload_error"] = reloadErr.Error()
		}
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
		case "focus_template":
			uninstallErr = installer.UninstallFocusTemplate(name)
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "type must be skill, plugin, mcp, or focus_template"})
			return
		}
		if uninstallErr != nil {
			logger.Error().Err(uninstallErr).Str("type", req.Type).Str("name", name).Msg("hub uninstall failed")
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": uninstallErr.Error()})
			return
		}

		payload := map[string]string{"ok": "true", "type": req.Type, "name": name}
		if req.Type != "focus_template" && extensions != nil {
			// Forget the package's on/off state: a later reinstall must
			// come back enabled, not silently off. SetDisabled reloads.
			if err := extensions.SetDisabled(r.Context(), req.Type, name, false); err != nil {
				logger.Warn().Err(err).Str("type", req.Type).Str("name", name).Msg("hub uninstall: reload failed")
				payload["reload_error"] = err.Error()
			}
		}

		logger.Info().Str("type", req.Type).Str("name", name).Msg("hub package uninstalled")
		writeJSON(w, http.StatusOK, payload)
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
		ctx := r.Context()
		skills, skillErr := installer.Update(ctx)
		plugins, pluginErr := installer.UpdatePlugins(ctx)
		mcps, mcpErr := installer.UpdateMCPs(ctx)
		templates, templateErr := installer.UpdateFocusTemplates(ctx)
		updateErr := errors.Join(skillErr, pluginErr, mcpErr, templateErr)

		var reloadErr error
		if len(skills.Updated)+len(plugins.Updated)+len(mcps.Updated) > 0 {
			reloadErr = reloadHubExtensions(r, extensions, logger)
		}

		logger.Info().
			Int("skills", len(skills.Updated)).
			Int("plugins", len(plugins.Updated)).
			Int("mcps", len(mcps.Updated)).
			Int("focus_templates", len(templates.Updated)).
			Err(updateErr).Msg("hub packages updated")
		// The update ran even when some packages failed, so the answer is
		// 200 with the per-package results; "error" says what failed. A
		// 500 made the console drop the lists of what did update.
		writeJSON(w, http.StatusOK, map[string]any{
			"updated_skills":               skills.Updated,
			"updated_plugins":              plugins.Updated,
			"updated_mcps":                 mcps.Updated,
			"updated_focus_templates":      templates.Updated,
			"skill_update_result":          hubUpdateResultPayload(skills),
			"plugin_update_result":         hubUpdateResultPayload(plugins),
			"mcp_update_result":            hubUpdateResultPayload(mcps),
			"focus_template_update_result": hubUpdateResultPayload(templates),
			"error":                        errorString(updateErr),
			"reload_error":                 errorString(reloadErr),
		})
	})

	// GET /v1/hub/skill-content?name=X&source=Y — fetch SKILL.md content
	// from the named HubSource (tars-hub when source is empty), so an
	// external skill's detail is not the tars-hub skill of the same name.
	mux.HandleFunc("/v1/hub/skill-content", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if installer == nil || installer.Sources == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "hub is not configured"})
			return
		}
		name := strings.TrimSpace(r.URL.Query().Get("name"))
		if name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}
		sourceID := strings.TrimSpace(r.URL.Query().Get("source"))
		if sourceID == "" {
			sourceID = skillhub.DefaultSourceID
		}
		src, ok := installer.Sources.Get(sourceID)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown hub source: " + sourceID})
			return
		}
		entry, err := src.FindSkillByName(r.Context(), name)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		content, err := src.FetchSkillContent(r.Context(), entry)
		if err != nil {
			logger.Error().Err(err).Str("name", name).Str("source", sourceID).Msg("fetch skill content failed")
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

// reloadHubExtensions reloads the runtime after a hub change and returns
// the reload error so the response can say the package is on disk but not
// loaded, instead of answering ok regardless.
func reloadHubExtensions(r *http.Request, extensions extensionsProvider, logger zerolog.Logger) error {
	if extensions == nil {
		return nil
	}
	if err := extensions.Reload(r.Context()); err != nil {
		logger.Warn().Err(err).Msg("reload extensions after hub change failed")
		return err
	}
	return nil
}

// handlePackInstall previews (dry_run) or installs a domain pack. The plan
// is what the console shows before the user confirms.
func handlePackInstall(w http.ResponseWriter, r *http.Request, installer *skillhub.Installer, extensions extensionsProvider, logger zerolog.Logger, name string, dryRun bool) {
	if dryRun {
		plan, err := installer.PlanPackInstall(r.Context(), name)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": "true", "type": "pack", "name": name, "dry_run": true, "plan": plan})
		return
	}
	result, err := installer.InstallPack(r.Context(), name)
	payload := map[string]any{"type": "pack", "name": name}
	if result != nil {
		payload["result"] = result
	}
	// Members installed before a failure are on disk either way.
	var reloadErr error
	if result != nil && len(result.Installed) > 0 {
		reloadErr = reloadHubExtensions(r, extensions, logger)
	}
	if reloadErr != nil {
		payload["reload_error"] = reloadErr.Error()
	}
	if err != nil {
		logger.Error().Err(err).Str("pack", name).Msg("hub pack install failed")
		payload["error"] = err.Error()
		var sandboxErr *skillhub.SandboxError
		if errors.As(err, &sandboxErr) {
			payload["sandbox_report"] = sandboxErr.Report
		}
		writeJSON(w, http.StatusBadRequest, payload)
		return
	}
	logger.Info().Str("pack", name).Int("installed", len(result.Installed)).Msg("hub pack installed")
	payload["ok"] = "true"
	writeJSON(w, http.StatusOK, payload)
}
