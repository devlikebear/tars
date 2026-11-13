package tarsserver

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/devlikebear/tars/internal/consoleauth"
	"github.com/devlikebear/tars/internal/session"
)

// A paired phone (role focus, docs/decisions/mobile-remote.md §4) may start
// a focus pipeline, but only in a folder the desktop's admin registered for
// phones, and only with the options that do not reach past that pipeline:
// no goal mode (turning it on is admin-only everywhere), no release, no
// computer use on the desktop's screen, and no follow-up pipelines or
// sandbox runs opened by the server on its own.

var errDeviceCreateOption = errors.New("this option needs the admin token")

// focusDeviceStore is the auth store that holds the registered folders.
func focusDeviceStore(sessions *session.Store) *consoleauth.Store {
	if sessions == nil {
		return nil
	}
	return consoleauth.NewStore(sessions.WorkspaceDir())
}

// checkDeviceCreate reports why a focus device may not create req, with the
// HTTP status to answer; 0 means it may. It does not change req.
func checkDeviceCreate(store *consoleauth.Store, req focusCreateRequest) (int, string) {
	switch {
	case strings.TrimSpace(req.Kind) != "":
		return http.StatusForbidden, "a phone cannot start a release"
	case req.GoalMode:
		return http.StatusForbidden, "goal mode needs the admin token"
	case req.E2E, req.HarnessProposal, req.VerifyPromotion:
		return http.StatusForbidden, errDeviceCreateOption.Error()
	}
	if store == nil {
		return http.StatusServiceUnavailable, "registered folders are unavailable"
	}
	for _, dir := range append([]string{req.Cwd}, req.ExtraDirs...) {
		allowed, err := store.DeviceFolderAllowed(dir)
		if err != nil {
			return http.StatusInternalServerError, "failed to read registered folders"
		}
		if !allowed {
			return http.StatusForbidden, "folder is not registered for phones"
		}
	}
	return 0, ""
}

type focusDeviceFolder struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	Exists bool   `json:"exists"`
}

// folders lists the folders a phone may start a pipeline in, for the phone
// to pick from (GET /v1/focus/folders). Admin and user callers see the same
// list.
func (a *focusAPI) folders(w http.ResponseWriter, _ *http.Request) {
	store := focusDeviceStore(a.sessions)
	if store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "registered folders are unavailable"})
		return
	}
	paths, err := store.DeviceFolders()
	if err != nil {
		a.logger.Error().Err(err).Msg("focus: read registered folders")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read registered folders"})
		return
	}
	out := make([]focusDeviceFolder, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		out = append(out, focusDeviceFolder{Path: path, Name: filepath.Base(path), Exists: err == nil && info.IsDir()})
	}
	writeJSON(w, http.StatusOK, map[string]any{"folders": out})
}
