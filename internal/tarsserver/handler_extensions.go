package tarsserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/extensions"
	"github.com/devlikebear/tars/internal/mcp"
	"github.com/devlikebear/tars/internal/plugin"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/skill"
	"github.com/rs/zerolog"
)

type mcpProvider interface {
	ListServers(ctx context.Context) ([]mcp.ServerStatus, error)
	ListTools(ctx context.Context) ([]mcp.ToolInfo, error)
}

func newMCPAPIHandler(provider mcpProvider, logger zerolog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/mcp/servers", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
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
		if !requireMethod(w, r, http.MethodGet) {
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
	DisabledSet() extensions.DisabledSet
	SetDisabled(ctx context.Context, kind, name string, disabled bool) error
}

type extensionHealthStatus string

const (
	extensionHealthPass    extensionHealthStatus = "pass"
	extensionHealthWarn    extensionHealthStatus = "warn"
	extensionHealthFail    extensionHealthStatus = "fail"
	extensionHealthUnknown extensionHealthStatus = "unknown"
)

type extensionHealthResponse struct {
	Skills     []extensionHealthItem `json:"skills"`
	MCPServers []extensionHealthItem `json:"mcp_servers"`
}

type extensionHealthItem struct {
	Kind       string                 `json:"kind"`
	Name       string                 `json:"name"`
	Status     extensionHealthStatus  `json:"status"`
	Summary    string                 `json:"summary,omitempty"`
	Repairable bool                   `json:"repairable,omitempty"`
	Checks     []extensionHealthCheck `json:"checks,omitempty"`
}

type extensionHealthCheck struct {
	Name    string                `json:"name"`
	Status  extensionHealthStatus `json:"status"`
	Message string                `json:"message,omitempty"`
	Detail  string                `json:"detail,omitempty"`
}

type extensionHealthOptions struct {
	MCPProvider  mcpProvider
	WorkspaceDir string
}

type extensionRepairRequest struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type extensionRepairResponse struct {
	Repaired bool                   `json:"repaired"`
	Kind     string                 `json:"kind"`
	Name     string                 `json:"name"`
	Actions  []extensionHealthCheck `json:"actions,omitempty"`
	Reloaded bool                   `json:"reloaded,omitempty"`
}

type extensionRepairCommandResult struct {
	Stdout     string
	Stderr     string
	ExitCode   int
	DurationMS int64
}

var (
	errExtensionNoAutomaticRepair = errors.New("no automated repair available")
	errExtensionNotFound          = errors.New("extension not found")
	runExtensionRepairCommand     = runExtensionRepairCommandDefault
)

func newExtensionsAPIHandler(provider extensionsProvider, logger zerolog.Logger, afterReload func() (bool, int)) http.Handler {
	return newExtensionsAPIHandlerWithSessionStore(provider, logger, afterReload, nil)
}

func newExtensionsAPIHandlerWithSessionStore(provider extensionsProvider, logger zerolog.Logger, afterReload func() (bool, int), store *session.Store) http.Handler {
	return newExtensionsAPIHandlerWithHealth(provider, logger, afterReload, store, extensionHealthOptions{})
}

func newExtensionsAPIHandlerWithHealth(provider extensionsProvider, logger zerolog.Logger, afterReload func() (bool, int), store *session.Store, healthOpts extensionHealthOptions) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/skills", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		snapshot := extensions.Snapshot{}
		if provider != nil {
			snapshot = provider.Snapshot()
		}
		if store != nil {
			if sessionID := strings.TrimSpace(r.URL.Query().Get("session_id")); sessionID != "" {
				sess, err := store.Get(sessionID)
				if err != nil {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
					return
				}
				snapshot = augmentSnapshotWithCwdSkills(snapshot, sess.CurrentDir)
			}
		}
		skills := snapshot.Skills
		if skills == nil {
			skills = []skill.Definition{}
		}
		writeJSON(w, http.StatusOK, skills)
	})
	mux.HandleFunc("/v1/skills/", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
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
		if !requireMethod(w, r, http.MethodGet) {
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
	mux.HandleFunc("/v1/runtime/extensions/disabled", func(w http.ResponseWriter, r *http.Request) {
		if provider == nil {
			writeJSON(w, http.StatusOK, extensions.DisabledSet{})
			return
		}
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, provider.DisabledSet())
		case http.MethodPost:
			var req struct {
				Kind     string `json:"kind"` // skill, plugin, mcp
				Name     string `json:"name"`
				Disabled bool   `json:"disabled"`
			}
			if !decodeJSONBody(w, r, &req) {
				return
			}
			if strings.TrimSpace(req.Name) == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
				return
			}
			if err := provider.SetDisabled(r.Context(), req.Kind, req.Name, req.Disabled); err != nil {
				logger.Error().Err(err).Str("kind", req.Kind).Str("name", req.Name).Bool("disabled", req.Disabled).Msg("set disabled failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			agentRuntimeRefreshed := false
			agentRuntimeAgents := 0
			if afterReload != nil {
				agentRuntimeRefreshed, agentRuntimeAgents = afterReload()
			}
			_ = agentRuntimeRefreshed
			_ = agentRuntimeAgents
			logger.Info().Str("kind", req.Kind).Str("name", req.Name).Bool("disabled", req.Disabled).Msg("extension disabled state changed")
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "kind": req.Kind, "name": req.Name, "disabled": req.Disabled})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/v1/runtime/extensions/health", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		payload, err := buildExtensionHealth(r.Context(), provider, healthOpts.MCPProvider, healthOpts.WorkspaceDir)
		if err != nil {
			logger.Error().Err(err).Msg("extension health check failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, payload)
	})

	mux.HandleFunc("/v1/runtime/extensions/repair", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var req extensionRepairRequest
		if !decodeJSONBody(w, r, &req) {
			return
		}
		req.Kind = normalizeExtensionKind(req.Kind)
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}
		if req.Kind == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "kind is required"})
			return
		}
		result, err := repairExtension(r.Context(), provider, healthOpts.WorkspaceDir, req)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, errExtensionNoAutomaticRepair) {
				status = http.StatusBadRequest
			}
			if errors.Is(err, errExtensionNotFound) {
				status = http.StatusNotFound
			}
			logger.Error().Err(err).Str("kind", req.Kind).Str("name", req.Name).Msg("extension repair failed")
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		if provider != nil {
			if err := provider.Reload(r.Context()); err != nil {
				logger.Error().Err(err).Msg("reload extensions after repair failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "reload extensions after repair failed"})
				return
			}
			result.Reloaded = true
			if afterReload != nil {
				afterReload()
			}
		}
		writeJSON(w, http.StatusOK, result)
	})

	mux.HandleFunc("/v1/runtime/extensions/reload", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
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
		agentRuntimeRefreshed := false
		agentRuntimeAgents := 0
		if afterReload != nil {
			agentRuntimeRefreshed, agentRuntimeAgents = afterReload()
		}
		snapshot := provider.Snapshot()
		writeJSON(w, http.StatusOK, map[string]any{
			"reloaded":               true,
			"version":                snapshot.Version,
			"skills":                 len(snapshot.Skills),
			"plugins":                len(snapshot.Plugins),
			"mcp_count":              len(snapshot.MCPServers),
			"agentruntime_refreshed": agentRuntimeRefreshed,
			"agentruntime_agents":    agentRuntimeAgents,
		})
	})
	return mux
}

func buildExtensionHealth(ctx context.Context, provider extensionsProvider, mcpProvider mcpProvider, workspaceDir string) (extensionHealthResponse, error) {
	snapshot := extensions.Snapshot{}
	if provider != nil {
		snapshot = provider.Snapshot()
	}
	payload := extensionHealthResponse{
		Skills:     make([]extensionHealthItem, 0, len(snapshot.Skills)),
		MCPServers: []extensionHealthItem{},
	}
	for _, s := range snapshot.Skills {
		payload.Skills = append(payload.Skills, buildSkillHealth(s))
	}

	serverConfigs := make(map[string]config.MCPServer, len(snapshot.MCPServers))
	for _, server := range snapshot.MCPServers {
		serverConfigs[strings.ToLower(strings.TrimSpace(server.Name))] = server
	}

	seenMCP := map[string]struct{}{}
	if mcpProvider != nil {
		statuses, err := mcpProvider.ListServers(ctx)
		if err != nil {
			return payload, fmt.Errorf("list mcp servers: %w", err)
		}
		payload.MCPServers = make([]extensionHealthItem, 0, len(statuses))
		for _, status := range statuses {
			key := strings.ToLower(strings.TrimSpace(status.Name))
			seenMCP[key] = struct{}{}
			payload.MCPServers = append(payload.MCPServers, buildMCPHealth(status, serverConfigs[key], true, workspaceDir))
		}
	}
	for _, server := range snapshot.MCPServers {
		key := strings.ToLower(strings.TrimSpace(server.Name))
		if _, ok := seenMCP[key]; ok {
			continue
		}
		payload.MCPServers = append(payload.MCPServers, buildMCPHealth(mcp.ServerStatus{Name: server.Name}, server, false, workspaceDir))
	}
	return payload, nil
}

func buildSkillHealth(s skill.Definition) extensionHealthItem {
	name := strings.TrimSpace(s.Name)
	if name == "" {
		name = "(unnamed skill)"
	}
	checks := []extensionHealthCheck{}
	if strings.TrimSpace(s.Name) == "" {
		checks = append(checks, extensionHealthCheck{Name: "metadata", Status: extensionHealthFail, Message: "skill name is missing"})
	} else if strings.TrimSpace(s.Description) == "" {
		checks = append(checks, extensionHealthCheck{Name: "metadata", Status: extensionHealthWarn, Message: "description is empty"})
	} else {
		checks = append(checks, extensionHealthCheck{Name: "metadata", Status: extensionHealthPass, Message: "metadata is present"})
	}

	if path := strings.TrimSpace(s.FilePath); path != "" {
		if _, err := os.Stat(path); err != nil {
			checks = append(checks, extensionHealthCheck{Name: "skill_file", Status: extensionHealthFail, Message: "skill file is not readable", Detail: err.Error()})
		} else {
			checks = append(checks, extensionHealthCheck{Name: "skill_file", Status: extensionHealthPass, Message: path})
		}
	} else {
		checks = append(checks, extensionHealthCheck{Name: "skill_file", Status: extensionHealthWarn, Message: "skill did not report a local file path"})
	}

	if len(s.RequiresBins) > 0 {
		missing := []string{}
		for _, bin := range s.RequiresBins {
			bin = strings.TrimSpace(bin)
			if bin == "" {
				continue
			}
			if _, err := osexec.LookPath(bin); err != nil {
				missing = append(missing, bin)
			}
		}
		if len(missing) > 0 {
			checks = append(checks, extensionHealthCheck{Name: "required_bins", Status: extensionHealthFail, Message: "required commands are missing", Detail: strings.Join(missing, ", ")})
		} else {
			checks = append(checks, extensionHealthCheck{Name: "required_bins", Status: extensionHealthPass, Message: "required commands are available"})
		}
	}

	if len(s.RequiresEnv) > 0 {
		missing := []string{}
		for _, key := range s.RequiresEnv {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			if os.Getenv(key) == "" {
				missing = append(missing, key)
			}
		}
		if len(missing) > 0 {
			checks = append(checks, extensionHealthCheck{Name: "required_env", Status: extensionHealthFail, Message: "required environment variables are missing", Detail: strings.Join(missing, ", ")})
		} else {
			checks = append(checks, extensionHealthCheck{Name: "required_env", Status: extensionHealthPass, Message: "required environment variables are present"})
		}
	}

	if len(s.SmokeTests) > 0 {
		checks = append(checks, extensionHealthCheck{Name: "smoke_tests", Status: extensionHealthWarn, Message: "smoke tests are declared but are not run automatically from the console"})
	}

	status := aggregateExtensionHealthStatus(checks)
	return extensionHealthItem{
		Kind:    "skill",
		Name:    name,
		Status:  status,
		Summary: extensionHealthSummary(status, checks),
		Checks:  checks,
	}
}

func buildMCPHealth(status mcp.ServerStatus, server config.MCPServer, hasRuntimeStatus bool, workspaceDir string) extensionHealthItem {
	name := strings.TrimSpace(status.Name)
	if name == "" {
		name = strings.TrimSpace(server.Name)
	}
	if name == "" {
		name = "(unnamed mcp)"
	}
	checks := []extensionHealthCheck{}
	if strings.TrimSpace(server.Command) == "" && strings.TrimSpace(server.URL) == "" {
		checks = append(checks, extensionHealthCheck{Name: "configuration", Status: extensionHealthFail, Message: "server command or URL is missing"})
	} else {
		checks = append(checks, extensionHealthCheck{Name: "configuration", Status: extensionHealthPass, Message: "server configuration is present"})
	}
	if dir, ok := inferMCPServerDir(server, workspaceDir); ok {
		manifestPath := filepath.Join(dir, "tars.mcp.json")
		if _, err := os.Stat(manifestPath); err != nil {
			checks = append(checks, extensionHealthCheck{Name: "manifest", Status: extensionHealthWarn, Message: "tars.mcp.json was not found", Detail: err.Error()})
		} else {
			checks = append(checks, extensionHealthCheck{Name: "manifest", Status: extensionHealthPass, Message: manifestPath})
		}
	}
	if !hasRuntimeStatus {
		checks = append(checks, extensionHealthCheck{Name: "connection", Status: extensionHealthUnknown, Message: "runtime connection status is not available"})
	} else if status.Connected {
		checks = append(checks, extensionHealthCheck{Name: "connection", Status: extensionHealthPass, Message: fmt.Sprintf("connected with %d tools", status.ToolCount)})
	} else {
		message := strings.TrimSpace(status.Error)
		if message == "" {
			message = "server is not connected"
		}
		checks = append(checks, extensionHealthCheck{Name: "connection", Status: extensionHealthFail, Message: message})
	}
	healthStatus := aggregateExtensionHealthStatus(checks)
	return extensionHealthItem{
		Kind:       "mcp",
		Name:       name,
		Status:     healthStatus,
		Summary:    extensionHealthSummary(healthStatus, checks),
		Repairable: healthStatus != extensionHealthPass && mcpRepairAvailable(server, workspaceDir),
		Checks:     checks,
	}
}

func aggregateExtensionHealthStatus(checks []extensionHealthCheck) extensionHealthStatus {
	status := extensionHealthPass
	for _, check := range checks {
		switch check.Status {
		case extensionHealthFail:
			return extensionHealthFail
		case extensionHealthWarn:
			if status == extensionHealthPass {
				status = extensionHealthWarn
			}
		case extensionHealthUnknown:
			if status == extensionHealthPass {
				status = extensionHealthUnknown
			}
		}
	}
	return status
}

func extensionHealthSummary(status extensionHealthStatus, checks []extensionHealthCheck) string {
	failed := 0
	warned := 0
	for _, check := range checks {
		switch check.Status {
		case extensionHealthFail:
			failed++
		case extensionHealthWarn, extensionHealthUnknown:
			warned++
		}
	}
	switch status {
	case extensionHealthFail:
		return fmt.Sprintf("%d check(s) failed", failed)
	case extensionHealthWarn, extensionHealthUnknown:
		return fmt.Sprintf("%d check(s) need attention", warned)
	default:
		return "all checks passed"
	}
}

func repairExtension(ctx context.Context, provider extensionsProvider, workspaceDir string, req extensionRepairRequest) (extensionRepairResponse, error) {
	if req.Kind != "mcp" {
		return extensionRepairResponse{}, fmt.Errorf("%w for %s extensions", errExtensionNoAutomaticRepair, req.Kind)
	}
	if provider == nil {
		return extensionRepairResponse{}, errExtensionNotFound
	}
	snapshot := provider.Snapshot()
	for _, server := range snapshot.MCPServers {
		if strings.EqualFold(strings.TrimSpace(server.Name), req.Name) {
			return repairMCPServer(ctx, server, workspaceDir)
		}
	}
	return extensionRepairResponse{}, fmt.Errorf("%w: %s", errExtensionNotFound, req.Name)
}

func repairMCPServer(ctx context.Context, server config.MCPServer, workspaceDir string) (extensionRepairResponse, error) {
	dir, ok := inferMCPServerDir(server, workspaceDir)
	if !ok {
		return extensionRepairResponse{}, fmt.Errorf("%w: local MCP server directory was not found", errExtensionNoAutomaticRepair)
	}
	if !isPathInside(filepath.Join(workspaceDir, "mcp-servers"), dir) {
		return extensionRepairResponse{}, fmt.Errorf("%w: MCP directory is outside workspace/mcp-servers", errExtensionNoAutomaticRepair)
	}

	actions := []extensionHealthCheck{}
	requirementsPath := filepath.Join(dir, "requirements.txt")
	if _, err := os.Stat(requirementsPath); err == nil {
		pythonCommand := strings.TrimSpace(server.Command)
		if pythonCommand == "" || !strings.Contains(strings.ToLower(filepath.Base(pythonCommand)), "python") {
			pythonCommand = "python3"
		}
		result, runErr := runExtensionRepairCommand(ctx, dir, pythonCommand, "-m", "pip", "install", "-r", "requirements.txt", "--target", ".python", "--disable-pip-version-check")
		check := commandResultHealthCheck("python_requirements", pythonCommand, []string{"-m", "pip", "install", "-r", "requirements.txt", "--target", ".python", "--disable-pip-version-check"}, result, runErr)
		actions = append(actions, check)
		if runErr != nil {
			return extensionRepairResponse{}, fmt.Errorf("install python requirements: %w", runErr)
		}
		patched, err := patchMCPManifestEnv(dir, "PYTHONPATH", "${MCP_DIR}/.python")
		if err != nil {
			return extensionRepairResponse{}, fmt.Errorf("patch mcp manifest env: %w", err)
		}
		if patched {
			actions = append(actions, extensionHealthCheck{Name: "manifest_env", Status: extensionHealthPass, Message: "set PYTHONPATH to ${MCP_DIR}/.python"})
		} else {
			actions = append(actions, extensionHealthCheck{Name: "manifest_env", Status: extensionHealthPass, Message: "PYTHONPATH already includes ${MCP_DIR}/.python"})
		}
	}

	packagePath := filepath.Join(dir, "package.json")
	if _, err := os.Stat(packagePath); err == nil {
		args := []string{"install", "--no-audit", "--no-fund", "--package-lock=false", "--loglevel=error"}
		result, runErr := runExtensionRepairCommand(ctx, dir, "npm", args...)
		actions = append(actions, commandResultHealthCheck("node_dependencies", "npm", args, result, runErr))
		if runErr != nil {
			return extensionRepairResponse{}, fmt.Errorf("install node dependencies: %w", runErr)
		}
	}

	if len(actions) == 0 {
		return extensionRepairResponse{}, fmt.Errorf("%w: no requirements.txt or package.json found", errExtensionNoAutomaticRepair)
	}
	return extensionRepairResponse{
		Repaired: true,
		Kind:     "mcp",
		Name:     server.Name,
		Actions:  actions,
	}, nil
}

func mcpRepairAvailable(server config.MCPServer, workspaceDir string) bool {
	dir, ok := inferMCPServerDir(server, workspaceDir)
	if !ok || !isPathInside(filepath.Join(workspaceDir, "mcp-servers"), dir) {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, "requirements.txt")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
		return true
	}
	return false
}

func inferMCPServerDir(server config.MCPServer, workspaceDir string) (string, bool) {
	workspaceDir = strings.TrimSpace(workspaceDir)
	name := strings.TrimSpace(server.Name)
	if workspaceDir != "" && name != "" {
		if dir := filepath.Join(workspaceDir, "mcp-servers", name); fileExists(filepath.Join(dir, "tars.mcp.json")) {
			return dir, true
		}
	}
	for _, value := range append([]string{server.Command}, server.Args...) {
		dir, ok := inferMCPServerDirFromPath(value, workspaceDir, name)
		if ok {
			return dir, true
		}
	}
	return "", false
}

func inferMCPServerDirFromPath(value string, workspaceDir string, serverName string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "${MCP_DIR}") || !filepath.IsAbs(value) {
		return "", false
	}
	path := filepath.Clean(value)
	if !isPathInside(workspaceDir, path) {
		return "", false
	}
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		path = filepath.Dir(path)
	} else if filepath.Ext(path) != "" {
		path = filepath.Dir(path)
	}
	for {
		if filepath.Base(filepath.Dir(path)) == "mcp-servers" && (serverName == "" || filepath.Base(path) == serverName) {
			return path, true
		}
		next := filepath.Dir(path)
		if next == path || !isPathInside(workspaceDir, next) {
			break
		}
		path = next
	}
	return "", false
}

func isPathInside(root string, path string) bool {
	root = strings.TrimSpace(root)
	path = strings.TrimSpace(path)
	if root == "" || path == "" {
		return false
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func patchMCPManifestEnv(mcpDir string, key string, value string) (bool, error) {
	manifestPath := filepath.Join(mcpDir, "tars.mcp.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(manifestPath)
	if err != nil {
		return false, err
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return false, err
	}
	serverRaw, ok := doc["server"].(map[string]any)
	if !ok {
		return false, fmt.Errorf("manifest server object is missing")
	}
	envRaw, ok := serverRaw["env"].(map[string]any)
	if !ok {
		envRaw = map[string]any{}
		serverRaw["env"] = envRaw
	}
	existing, _ := envRaw[key].(string)
	if strings.Contains(existing, value) {
		return false, nil
	}
	if strings.TrimSpace(existing) == "" {
		envRaw[key] = value
	} else {
		envRaw[key] = value + string(os.PathListSeparator) + existing
	}
	next, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return false, err
	}
	next = append(next, '\n')
	if err := os.WriteFile(manifestPath, next, info.Mode().Perm()); err != nil {
		return false, err
	}
	return true, nil
}

func commandResultHealthCheck(name string, command string, args []string, result extensionRepairCommandResult, err error) extensionHealthCheck {
	status := extensionHealthPass
	message := strings.TrimSpace(extensionCommandLine(command, args))
	if err != nil {
		status = extensionHealthFail
		message = err.Error()
	}
	detail := strings.TrimSpace(result.Stdout + "\n" + result.Stderr)
	if detail == "" && result.ExitCode != 0 {
		detail = fmt.Sprintf("exit code %d", result.ExitCode)
	}
	return extensionHealthCheck{
		Name:    name,
		Status:  status,
		Message: message,
		Detail:  truncateExtensionDetail(detail),
	}
}

func extensionCommandLine(command string, args []string) string {
	parts := append([]string{command}, args...)
	return strings.Join(parts, " ")
}

func truncateExtensionDetail(value string) string {
	const maxLen = 4000
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen] + "\n..."
}

func runExtensionRepairCommandDefault(ctx context.Context, dir, name string, args ...string) (extensionRepairCommandResult, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	started := time.Now()
	cmd := osexec.CommandContext(timeoutCtx, name, args...)
	cmd.Dir = dir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := extensionRepairCommandResult{
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		ExitCode:   0,
		DurationMS: time.Since(started).Milliseconds(),
	}
	if err != nil {
		result.ExitCode = -1
		var exitErr *osexec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		}
	}
	if timeoutCtx.Err() != nil {
		return result, timeoutCtx.Err()
	}
	return result, err
}

func normalizeExtensionKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "mcp", "mcp_server", "mcp-server", "mcp_servers", "mcp-servers":
		return "mcp"
	case "skill", "skills":
		return "skill"
	case "plugin", "plugins":
		return "plugin"
	default:
		return strings.ToLower(strings.TrimSpace(kind))
	}
}
