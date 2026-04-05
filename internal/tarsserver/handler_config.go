package tarsserver

import (
	"encoding/json"
	"fmt"
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
	"github.com/devlikebear/tars/internal/memory"
	"github.com/rs/zerolog"
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
		handleGetConfigSchema(w, configPath, cfg)
	})

	return mux
}

type configSchemaResponse struct {
	Path   string             `json:"path"`
	Fields []config.FieldMeta `json:"fields"`
	Values map[string]any     `json:"values"`
}

func handleGetConfigSchema(w http.ResponseWriter, configPath string, cfg config.Config) {
	values := config.ConfigToMap(cfg)

	// Mask sensitive values
	schema := config.Schema()
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

	writeJSON(w, http.StatusOK, configSchemaResponse{
		Path:   configPath,
		Fields: schema,
		Values: values,
	})
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
		label := "io.tars.server"
		domain := fmt.Sprintf("gui/%d", os.Getuid())
		writeJSON(w, http.StatusOK, map[string]string{
			"ok":   "true",
			"mode": "launchd",
			"info": "restarting via launchctl",
		})

		go func() {
			time.Sleep(500 * time.Millisecond)
			out, err := exec.Command("launchctl", "kickstart", "-k", domain+"/"+label).CombinedOutput()
			if err != nil {
				logger.Error().Err(err).Str("output", string(out)).Msg("launchctl kickstart failed")
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
	if runtime.GOOS != "darwin" {
		return "direct"
	}
	label := "io.tars.server"
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	out, err := exec.Command("launchctl", "print", domain+"/"+label).CombinedOutput()
	if err == nil && strings.Contains(string(out), "state =") {
		// Check if our PID matches the launchd-managed PID
		pidStr := ""
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "pid = ") {
				pidStr = strings.TrimSpace(strings.TrimPrefix(line, "pid = "))
				break
			}
		}
		if pidStr != "" {
			if managedPID, convErr := strconv.Atoi(pidStr); convErr == nil && managedPID == os.Getpid() {
				return "launchd"
			}
		}
	}
	return "direct"
}

func handleResetWorkspace(w http.ResponseWriter, workspaceDir string, logger zerolog.Logger) {
	if workspaceDir == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "workspace directory not configured"})
		return
	}

	// Preserve only: config/ directory and top-level .md template files
	// Remove everything else (sessions, projects, cron, gateway, skills, plugins, etc.)
	preserve := map[string]bool{"config": true}
	entries, err := os.ReadDir(workspaceDir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read workspace directory failed"})
		return
	}

	removed := 0
	var removedItems []string
	for _, entry := range entries {
		name := entry.Name()
		if preserve[name] {
			continue
		}
		// Preserve top-level .md template files (HEARTBEAT.md, MEMORY.md, etc.)
		if !entry.IsDir() && filepath.Ext(name) == ".md" {
			continue
		}
		target := filepath.Join(workspaceDir, name)
		if err := os.RemoveAll(target); err != nil {
			logger.Error().Err(err).Str("path", target).Msg("failed to remove workspace item")
			continue
		}
		removed++
		removedItems = append(removedItems, name)
	}

	// Re-initialize workspace to pristine state (recreate dirs + template files)
	if err := memory.EnsureWorkspace(workspaceDir); err != nil {
		logger.Error().Err(err).Msg("re-initialize workspace failed")
	}

	logger.Info().Int("removed", removed).Strs("items", removedItems).Str("workspace", workspaceDir).Msg("workspace reset to initial state")
	writeJSON(w, http.StatusOK, map[string]any{
		"removed":       removed,
		"removed_items": removedItems,
	})
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
