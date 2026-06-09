package tarsserver

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/launchagent"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/rs/zerolog"
)

var (
	restartRuntimeGOOS  = runtime.GOOS
	restartGetuid       = os.Getuid
	restartGetpid       = os.Getpid
	restartLaunchctlRun = runRestartLaunchctl
)

func newConfigAPIHandler(configPath string, cfg config.Config, workspaceDir string, logger zerolog.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/admin/reset/workspace", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		handleResetWorkspace(w, workspaceDir, logger)
	})

	mux.HandleFunc("/v1/admin/restart", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		handleRestart(w, logger)
	})

	mux.HandleFunc("/v1/admin/config", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleGetConfig(w, configPath, logger)
		case http.MethodPut:
			handlePutConfig(w, r, configPath, logger)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/v1/admin/config/values", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPatch) {
			return
		}
		handlePatchConfigValues(w, r, configPath, logger)
	})

	mux.HandleFunc("/v1/admin/config/schema", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		handleGetConfigSchema(w, configPath, cfg, workspaceDir)
	})

	return mux
}

type configSchemaResponse struct {
	Path            string                            `json:"path"`
	UpdatedAt       string                            `json:"updated_at,omitempty"`
	Fields          []config.FieldMeta                `json:"fields"`
	Values          map[string]any                    `json:"values"`
	EffectiveValues map[string]any                    `json:"effective_values,omitempty"`
	EnvOverrides    map[string]config.EnvOverrideMeta `json:"env_overrides,omitempty"`
}

func handleGetConfigSchema(w http.ResponseWriter, configPath string, cfg config.Config, workspaceDir string) {
	activeCfg := cfg
	effectiveCfg := cfg
	if strings.TrimSpace(configPath) != "" {
		loaded, err := config.LoadFile(configPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		activeCfg = loaded
		effectiveLoaded, err := config.Load(configPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		effectiveCfg = effectiveLoaded
		if strings.TrimSpace(workspaceDir) != "" {
			activeCfg.WorkspaceDir = workspaceDir
			effectiveCfg.WorkspaceDir = workspaceDir
		}
	}
	values := config.ConfigToMap(activeCfg)
	effectiveValues := config.ConfigToMap(effectiveCfg)

	// Mask sensitive values
	schema := config.Schema()
	maskSensitiveConfigValues(schema, values)
	maskSensitiveConfigValues(schema, effectiveValues)

	updatedAt := ""
	if info, err := os.Stat(configPath); err == nil {
		updatedAt = info.ModTime().UTC().Format(time.RFC3339)
	}

	writeJSON(w, http.StatusOK, configSchemaResponse{
		Path:            configPath,
		UpdatedAt:       updatedAt,
		Fields:          schema,
		Values:          values,
		EffectiveValues: effectiveValues,
		EnvOverrides:    config.ActiveEnvOverrides(),
	})
}

func maskSensitiveConfigValues(schema []config.FieldMeta, values map[string]any) {
	sensitiveKeys := map[string]bool{}
	for _, f := range schema {
		if f.Sensitive {
			sensitiveKeys[f.Key] = true
		}
	}
	for k, v := range values {
		if sensitiveKeys[k] {
			if s, ok := v.(string); ok && len(s) > 0 {
				values[k] = maskString(s)
			}
		}
	}
}

func maskString(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 8 {
		return strings.Repeat("*", len(s))
	}
	return s[:4] + strings.Repeat("*", len(s)-8) + s[len(s)-4:]
}

func handlePatchConfigValues(w http.ResponseWriter, r *http.Request, configPath string, logger zerolog.Logger) {
	if configPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no config file path configured"})
		return
	}

	var req struct {
		Updates map[string]any `json:"updates"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if len(req.Updates) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no updates provided"})
		return
	}

	if err := config.PatchYAML(configPath, req.Updates); err != nil {
		logger.Error().Err(err).Str("path", configPath).Msg("failed to patch config")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	logger.Info().Int("fields", len(req.Updates)).Str("path", configPath).Msg("config values patched")
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// handleRestart attempts to restart the TARS server process.
// Service mode (macOS launchd): uses launchctl kickstart -k.
// Direct mode: re-execs the same binary with the same arguments.
func handleRestart(w http.ResponseWriter, logger zerolog.Logger) {
	mode := detectRunMode()
	logger.Info().Str("mode", mode).Msg("server restart requested")

	switch mode {
	case "launchd":
		label, domain := launchdServiceIdentity()
		writeJSON(w, http.StatusOK, map[string]string{
			"ok":   "true",
			"mode": "launchd",
			"info": "restarting via launchctl",
		})

		go func() {
			time.Sleep(500 * time.Millisecond)
			out, err := restartLaunchctlRun("kickstart", "-k", domain+"/"+label)
			if err != nil {
				logger.Error().Err(err).Str("output", out).Msg("launchctl kickstart failed")
			}
		}()

	default:
		writeJSON(w, http.StatusOK, map[string]string{
			"ok":   "true",
			"mode": "exec",
			"info": "re-executing process",
		})

		go func() {
			time.Sleep(500 * time.Millisecond)
			exe, err := os.Executable()
			if err != nil {
				logger.Error().Err(err).Msg("resolve executable for restart failed")
				return
			}
			logger.Info().Str("exe", exe).Strs("args", os.Args).Msg("re-executing")
			if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil {
				logger.Error().Err(err).Msg("exec restart failed")
			}
		}()
	}
}

func detectRunMode() string {
	if restartRuntimeGOOS != "darwin" {
		return "direct"
	}
	label, domain := launchdServiceIdentity()
	out, err := restartLaunchctlRun("print", domain+"/"+label)
	if err == nil && strings.Contains(out, "state =") {
		// Check if our PID matches the launchd-managed PID
		pidStr := ""
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "pid = ") {
				pidStr = strings.TrimSpace(strings.TrimPrefix(line, "pid = "))
				break
			}
		}
		if pidStr != "" {
			if managedPID, convErr := strconv.Atoi(pidStr); convErr == nil && managedPID == restartGetpid() {
				return "launchd"
			}
		}
	}
	return "direct"
}

func launchdServiceIdentity() (string, string) {
	return launchagent.ResolveServiceIdentity(
		launchagent.DefaultServerLabel,
		launchagent.DefaultDomainForUID(restartGetuid()),
	)
}

func runRestartLaunchctl(args ...string) (string, error) {
	out, err := exec.Command("/bin/launchctl", args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

type workspaceResetError struct {
	Name  string `json:"name,omitempty"`
	Path  string `json:"path,omitempty"`
	Stage string `json:"stage,omitempty"`
	Error string `json:"error"`
}

type workspaceResetResponse struct {
	Removed       int                   `json:"removed"`
	RemovedDirs   int                   `json:"removed_dirs,omitempty"`
	RemovedItems  []string              `json:"removed_items"`
	FailedItems   []workspaceResetError `json:"failed_items,omitempty"`
	Reinitialized bool                  `json:"reinitialized"`
	Error         string                `json:"error,omitempty"`
}

func handleResetWorkspace(w http.ResponseWriter, workspaceDir string, logger zerolog.Logger) {
	if workspaceDir == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "workspace directory not configured"})
		return
	}

	// Preserve only: config/ directory and top-level .md template files
	// Remove everything else (sessions, projects, cron, agent runtime, skills, plugins, etc.)
	preserve := map[string]bool{"config": true}
	entries, err := os.ReadDir(workspaceDir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read workspace directory failed"})
		return
	}

	removed := 0
	var removedItems []string
	var failedItems []workspaceResetError
	for _, entry := range entries {
		name := entry.Name()
		if preserve[name] {
			continue
		}
		// Preserve top-level .md template files (MEMORY.md, IDENTITY.md, etc.)
		if !entry.IsDir() && filepath.Ext(name) == ".md" {
			continue
		}
		target := filepath.Join(workspaceDir, name)
		if err := os.RemoveAll(target); err != nil {
			logger.Error().Err(err).Str("path", target).Msg("failed to remove workspace item")
			failedItems = append(failedItems, workspaceResetError{
				Name:  name,
				Path:  target,
				Stage: "remove",
				Error: err.Error(),
			})
			continue
		}
		removed++
		removedItems = append(removedItems, name)
	}

	// Re-initialize workspace to pristine state (recreate dirs + template files)
	reinitialized := true
	if err := memory.EnsureWorkspace(workspaceDir); err != nil {
		logger.Error().Err(err).Msg("re-initialize workspace failed")
		reinitialized = false
		failedItems = append(failedItems, workspaceResetError{
			Path:  workspaceDir,
			Stage: "reinitialize",
			Error: err.Error(),
		})
	}

	response := workspaceResetResponse{
		Removed:       removed,
		RemovedDirs:   removed,
		RemovedItems:  removedItems,
		FailedItems:   failedItems,
		Reinitialized: reinitialized,
	}
	if len(failedItems) > 0 {
		response.Error = "workspace reset incomplete"
		logger.Error().
			Int("removed", removed).
			Int("failed", len(failedItems)).
			Strs("items", removedItems).
			Str("workspace", workspaceDir).
			Msg("workspace reset incomplete")
		writeJSON(w, http.StatusInternalServerError, response)
		return
	}

	logger.Info().Int("removed", removed).Strs("items", removedItems).Str("workspace", workspaceDir).Msg("workspace reset to initial state")
	writeJSON(w, http.StatusOK, response)
}

func handleGetConfig(w http.ResponseWriter, configPath string, logger zerolog.Logger) {
	if configPath == "" {
		writeJSON(w, http.StatusOK, map[string]string{
			"path":    "",
			"content": "",
		})
		return
	}

	raw, err := config.LoadRaw(configPath)
	if err != nil {
		logger.Error().Err(err).Str("path", configPath).Msg("failed to read config file")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read config file"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"path":    configPath,
		"content": string(raw),
	})
}

func handlePutConfig(w http.ResponseWriter, r *http.Request, configPath string, logger zerolog.Logger) {
	if configPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no config file path configured"})
		return
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if err := config.SaveRaw(configPath, []byte(req.Content)); err != nil {
		logger.Error().Err(err).Str("path", configPath).Msg("failed to save config file")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	logger.Info().Str("path", configPath).Msg("config file saved")
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}
