package tarsserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

var errTerminalUnsupported = errors.New("external terminal launch is only supported on macOS")

type terminalOpenResult struct {
	OK      bool   `json:"ok"`
	CWD     string `json:"cwd"`
	App     string `json:"app"`
	Message string `json:"message,omitempty"`
}

type terminalOpenFunc func(context.Context, string) (terminalOpenResult, error)

func newTerminalAPIHandler(workspaceDir string, store *session.Store, logger zerolog.Logger) http.Handler {
	return newTerminalAPIHandlerWithOpener(workspaceDir, store, openExternalTerminal, logger)
}

func newTerminalAPIHandlerWithOpener(workspaceDir string, store *session.Store, opener terminalOpenFunc, logger zerolog.Logger) http.Handler {
	if opener == nil {
		opener = openExternalTerminal
	}
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/terminal/open", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if strings.TrimSpace(serverauth.RoleFromRequest(r)) != serverauth.RoleAdmin {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}

		var req struct {
			SessionID string `json:"session_id"`
			CWD       string `json:"cwd"`
		}
		if !decodeJSONBody(w, r, &req) {
			return
		}
		sessionID := strings.TrimSpace(req.SessionID)
		if sessionID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session_id is required"})
			return
		}

		reqStore, _, _, err := resolveSessionStoreForRequest(workspaceDir, store, r)
		if err != nil {
			logger.Error().Err(err).Msg("resolve workspace session store failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "resolve workspace failed"})
			return
		}
		if reqStore == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "session store is not configured"})
			return
		}
		actualSessionID := sessionID
		if strings.EqualFold(sessionID, "main") {
			mainSession, err := reqStore.EnsureMain()
			if err != nil {
				logger.Error().Err(err).Msg("resolve main session failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "resolve main session failed"})
				return
			}
			actualSessionID = strings.TrimSpace(mainSession.ID)
		}
		sess, err := reqStore.Get(actualSessionID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			return
		}

		cwd, err := resolveTerminalCWD(sess, req.CWD)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		result, err := opener(r.Context(), cwd)
		if err != nil {
			if errors.Is(err, errTerminalUnsupported) {
				writeJSON(w, http.StatusNotImplemented, map[string]string{"error": err.Error()})
				return
			}
			logger.Error().Err(err).Str("cwd", cwd).Msg("open external terminal failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		result.OK = true
		if strings.TrimSpace(result.CWD) == "" {
			result.CWD = cwd
		}
		if strings.TrimSpace(result.App) == "" {
			result.App = "Terminal"
		}
		writeJSON(w, http.StatusOK, result)
	})

	return mux
}

func resolveTerminalCWD(sess session.Session, rawCWD string) (string, error) {
	cwd := strings.TrimSpace(rawCWD)
	if cwd == "" {
		cwd = strings.TrimSpace(sess.CurrentDir)
	}
	if cwd == "" && len(sess.WorkDirs) > 0 {
		cwd = strings.TrimSpace(sess.WorkDirs[0])
	}
	if cwd == "" {
		return "", fmt.Errorf("cwd is required")
	}
	if !filepath.IsAbs(cwd) {
		base := strings.TrimSpace(sess.CurrentDir)
		if base == "" && len(sess.WorkDirs) > 0 {
			base = strings.TrimSpace(sess.WorkDirs[0])
		}
		if base == "" {
			return "", fmt.Errorf("current_dir is required for relative cwd")
		}
		cwd = filepath.Join(base, cwd)
	}

	canonicalCWD := canonicalWorkspacePath(cwd)
	allowed := false
	for _, dir := range sess.WorkDirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		if workspacePathWithinRoot(dir, canonicalCWD) {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", fmt.Errorf("cwd must be inside a session working directory")
	}

	info, err := os.Stat(canonicalCWD)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("directory not found")
		}
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("cwd is not a directory")
	}
	return canonicalCWD, nil
}

func openExternalTerminal(ctx context.Context, cwd string) (terminalOpenResult, error) {
	if runtime.GOOS != "darwin" {
		return terminalOpenResult{}, errTerminalUnsupported
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/open", "-a", "Terminal", cwd)
	if err := cmd.Run(); err != nil {
		return terminalOpenResult{}, err
	}
	return terminalOpenResult{
		OK:      true,
		CWD:     cwd,
		App:     "Terminal",
		Message: "opened external terminal",
	}, nil
}
