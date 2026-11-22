package tarsserver

import (
	"errors"
	"net/http"

	"github.com/devlikebear/tars/internal/easyproject"
	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/serverauth"
)

// "Change it like this" (vibe-coder-identity.md §4, phase 3):
//
//	POST /v1/easy/projects/{id}/changes {request} → 202 {project}
//	     400 change_required|change_too_long, 404, 409 not_ready|busy, 503 cannot_run
//
// A change is a new webapp pipeline in the project's own folder, opened the
// way the first one was (the plan waits for the person, goal mode after
// it, the finished work saved as a version), with the person's words as
// its goal. The project then points at that pipeline, so the progress
// screen follows it. Only a finished, saved project takes a change: one
// pipeline works in the folder at a time.

// easyChangeReady reports whether the project's current pipeline is done
// with the folder: finished, and its version saved (or saving not asked).
func easyChangeReady(p focuspipeline.Pipeline) bool {
	if !focuspipeline.Finished(p) {
		return false
	}
	return !p.SaveVersion || p.Saved != nil
}

func (e *easyProjectsAPI) change(w http.ResponseWriter, r *http.Request) {
	role := serverauth.RoleFromRequest(r)
	if role == serverauth.RoleFocus {
		writeError(w, http.StatusForbidden, "forbidden", "a paired phone cannot change a project")
		return
	}
	var req struct {
		Request string `json:"request"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	request, err := easyproject.CleanChange(req.Request)
	switch {
	case errors.Is(err, easyproject.ErrChangeRequired):
		writeError(w, http.StatusBadRequest, "change_required", err.Error())
		return
	case errors.Is(err, easyproject.ErrChangeTooLong):
		writeError(w, http.StatusBadRequest, "change_too_long", err.Error())
		return
	}
	if !e.canRun() {
		writeError(w, http.StatusServiceUnavailable, "cannot_run", "the server cannot start the first turn")
		return
	}

	// One change at a time across projects: reading the project, checking
	// its pipeline, opening the next one and recording it is one step.
	e.changeMu.Lock()
	defer e.changeMu.Unlock()
	p, err := e.creator.Store.Get(r.PathValue("id"))
	if errors.Is(err, easyproject.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such project")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", "the project could not be read")
		return
	}
	if p.Status != easyproject.StatusReady || p.SessionID == "" {
		writeError(w, http.StatusConflict, "not_ready", "the project is not ready")
		return
	}
	current, ok, err := e.pipelines().Get(p.SessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", "the project could not be read")
		return
	}
	if ok && !easyChangeReady(current) {
		writeError(w, http.StatusConflict, "busy", "the team is still working on this project")
		return
	}
	sessionID, err := e.creator.Open(serverauth.WithRoleContext(r.Context(), role), p.Folder, easyproject.ChangeGoal(request))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "pipeline_failed", "the change could not be started")
		return
	}
	p = p.WithChange(request, sessionID, e.now().UTC())
	if err := e.creator.Store.Put(p); err != nil {
		writeError(w, http.StatusInternalServerError, "write_failed", "the change could not be recorded")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"project": p})
}
