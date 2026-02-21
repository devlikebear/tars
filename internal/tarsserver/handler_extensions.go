package tarsserver

import (
	"context"
	"net/http"
	"strings"

	"github.com/devlikebear/tarsncase/internal/extensions"
	"github.com/devlikebear/tarsncase/internal/mcp"
	"github.com/devlikebear/tarsncase/internal/plugin"
	"github.com/devlikebear/tarsncase/internal/skill"
	"github.com/rs/zerolog"
)

type mcpProvider interface {
	ListServers(ctx context.Context) ([]mcp.ServerStatus, error)
	ListTools(ctx context.Context) ([]mcp.ToolInfo, error)
}

func newMCPAPIHandler(provider mcpProvider, logger zerolog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/mcp/servers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if provider == nil {
			writeJSON(w, http.StatusOK, []mcp.ServerStatus{})
			return
		}
		servers, err := provider.ListServers(r.Context())
		if err != nil {
			logger.Error().Err(err).Msg("list mcp servers failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list mcp servers failed"})
			return
		}
		writeJSON(w, http.StatusOK, servers)
	})
	mux.HandleFunc("/v1/mcp/tools", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if provider == nil {
			writeJSON(w, http.StatusOK, []mcp.ToolInfo{})
			return
		}
		tools, err := provider.ListTools(r.Context())
		if err != nil {
			logger.Error().Err(err).Msg("list mcp tools failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list mcp tools failed"})
			return
		}
		writeJSON(w, http.StatusOK, tools)
	})
	return mux
}

type extensionsProvider interface {
	Snapshot() extensions.Snapshot
	Reload(ctx context.Context) error
}

func newExtensionsAPIHandler(provider extensionsProvider, logger zerolog.Logger, afterReload func() (bool, int)) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/skills", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if provider == nil {
			writeJSON(w, http.StatusOK, []skill.Definition{})
			return
		}
		snapshot := provider.Snapshot()
		skills := snapshot.Skills
		if skills == nil {
			skills = []skill.Definition{}
		}
		writeJSON(w, http.StatusOK, skills)
	})
	mux.HandleFunc("/v1/skills/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if provider == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
			return
		}
		name := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/v1/skills/"))
		if name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "skill name is required"})
			return
		}
		snapshot := provider.Snapshot()
		for _, s := range snapshot.Skills {
			if strings.EqualFold(strings.TrimSpace(s.Name), name) {
				writeJSON(w, http.StatusOK, s)
				return
			}
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
	})
	mux.HandleFunc("/v1/plugins", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if provider == nil {
			writeJSON(w, http.StatusOK, []plugin.Definition{})
			return
		}
		snapshot := provider.Snapshot()
		plugins := snapshot.Plugins
		if plugins == nil {
			plugins = []plugin.Definition{}
		}
		writeJSON(w, http.StatusOK, plugins)
	})
	mux.HandleFunc("/v1/runtime/extensions/reload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if provider == nil {
			writeJSON(w, http.StatusOK, map[string]any{"reloaded": false})
			return
		}
		if err := provider.Reload(r.Context()); err != nil {
			logger.Error().Err(err).Msg("reload extensions failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "reload extensions failed"})
			return
		}
		gatewayRefreshed := false
		gatewayAgents := 0
		if afterReload != nil {
			gatewayRefreshed, gatewayAgents = afterReload()
		}
		snapshot := provider.Snapshot()
		writeJSON(w, http.StatusOK, map[string]any{
			"reloaded":          true,
			"version":           snapshot.Version,
			"skills":            len(snapshot.Skills),
			"plugins":           len(snapshot.Plugins),
			"mcp_count":         len(snapshot.MCPServers),
			"gateway_refreshed": gatewayRefreshed,
			"gateway_agents":    gatewayAgents,
		})
	})
	return mux
}
