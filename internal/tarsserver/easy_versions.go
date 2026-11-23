package tarsserver

import (
	"errors"
	"net/http"

	"github.com/devlikebear/tars/internal/easyproject"
	"github.com/devlikebear/tars/internal/serverauth"
)

// An easy-mode project's versions (vibe-coder-identity.md §4, phase 3):
//
//	GET  /v1/easy/projects/{id}/versions                  → {versions} newest first
//	POST /v1/easy/projects/{id}/versions/{commit}/restore → {version, versions}
//	     404 not_found (project or version), 409 not_ready|busy
//
// Going back saves the old version's files as a new version (nothing is
// rewritten), and only while no run works in the folder — the same rule
// as a change, under the same lock. The preview is stopped afterwards, so
// the result screen starts it again on the files as they are now.

// versionsProject answers the ready project the request names, or writes
// the error and answers false.
func (e *easyProjectsAPI) versionsProject(w http.ResponseWriter, r *http.Request) (easyproject.Project, bool) {
	if serverauth.RoleFromRequest(r) == serverauth.RoleFocus {
		writeError(w, http.StatusForbidden, "forbidden", "a paired phone cannot change a project's versions")
		return easyproject.Project{}, false
	}
	p, err := e.creator.Store.Get(r.PathValue("id"))
	if errors.Is(err, easyproject.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such project")
		return p, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", "the project could not be read")
		return p, false
	}
	if p.Status != easyproject.StatusReady {
		writeError(w, http.StatusConflict, "not_ready", "the project is not ready")
		return p, false
	}
	return p, true
}

func (e *easyProjectsAPI) versions(w http.ResponseWriter, r *http.Request) {
	p, ok := e.versionsProject(w, r)
	if !ok {
		return
	}
	list, err := easyproject.Versions(r.Context(), p.Folder)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", "the versions could not be read")
		return
	}
	if list == nil {
		list = []easyproject.Version{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": list})
}

func (e *easyProjectsAPI) restore(w http.ResponseWriter, r *http.Request) {
	e.changeMu.Lock()
	defer e.changeMu.Unlock()
	p, ok := e.versionsProject(w, r)
	if !ok {
		return
	}
	if p.SessionID != "" {
		current, found, err := e.pipelines().Get(p.SessionID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "read_failed", "the project could not be read")
			return
		}
		if found && !easyChangeReady(current) {
			writeError(w, http.StatusConflict, "busy", "the team is still working on this project")
			return
		}
	}
	version, err := easyproject.RestoreVersion(r.Context(), p.Folder, r.PathValue("commit"))
	if errors.Is(err, easyproject.ErrVersionNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such version")
		return
	}
	if err != nil {
		e.logger.Warn().Err(err).Str("project_id", p.ID).Msg("easy projects: going back to a version failed")
		writeError(w, http.StatusInternalServerError, "restore_failed", "the version could not be brought back")
		return
	}
	e.previews.Stop(p.ID)
	list, err := easyproject.Versions(r.Context(), p.Folder)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", "the versions could not be read")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": version, "versions": list})
}
