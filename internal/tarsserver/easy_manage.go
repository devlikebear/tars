package tarsserver

import (
	"errors"
	"net/http"
	"path/filepath"

	"github.com/devlikebear/tars/internal/easyproject"
	"github.com/devlikebear/tars/internal/serverauth"
)

// Managing easy-mode projects from the board (docs/plans/vibe-mode-redesign,
// unit 1):
//
//	PATCH  /v1/easy/projects/{id} {name?, archived?} → {project, board}
//	       400 name_required|name_too_long|nothing_to_change, 404, 409 busy
//	DELETE /v1/easy/projects/{id} → {deleted_to}
//	       404, 409 busy|folder_outside
//
// Who may call: the same callers as the other easy routes — a user token is
// enough, because nothing here takes a path from the request: the folder
// moved away is the one the server made and recorded. A paired phone (role
// focus) may rename, archive and bring back (vibe redesign unit 6) but not
// delete: deleting moves a folder on the desk, which the plan leaves to the
// desk.
//
// Busy: archiving and deleting need the team idle — not starting, not
// working by itself, and no turn holding the pipeline's session. Archiving
// a project the team works on would hide work in progress, so it is refused
// instead of stopping the team. Archiving stops the preview (nobody looks
// at an archived project); bringing one back and renaming are always
// allowed. Deleting moves the folder into the projects directory's .deleted
// folder (easyproject.DeleteFolder), which a person can move back by hand,
// and drops the record; the pipeline's session is left to session
// retention, which keeps it only while the project exists.

// easyIdle reports whether nothing works in the project now.
func (e *easyProjectsAPI) easyIdle(p easyproject.Project, board easyproject.Board) bool {
	switch board.State {
	case easyproject.StateStarting, easyproject.StateWorking:
		return false
	}
	return p.SessionID == "" || e.turnRunning == nil || !e.turnRunning(p.SessionID)
}

func (e *easyProjectsAPI) manageProject(w http.ResponseWriter, r *http.Request) (easyproject.Project, bool) {
	p, err := e.creator.Store.Get(r.PathValue("id"))
	if errors.Is(err, easyproject.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such project")
		return p, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", "the project could not be read")
		return p, false
	}
	return p, true
}

func (e *easyProjectsAPI) update(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     *string `json:"name"`
		Archived *bool   `json:"archived"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Name == nil && req.Archived == nil {
		writeError(w, http.StatusBadRequest, "nothing_to_change", "send a name or archived")
		return
	}
	name := ""
	if req.Name != nil {
		cleaned, err := easyproject.CleanName(*req.Name)
		switch {
		case errors.Is(err, easyproject.ErrNameRequired):
			writeError(w, http.StatusBadRequest, "name_required", err.Error())
			return
		case errors.Is(err, easyproject.ErrNameTooLong):
			writeError(w, http.StatusBadRequest, "name_too_long", err.Error())
			return
		}
		name = cleaned
	}

	// Under the change lock: a change or a going back does not start while
	// the project is being put away.
	e.changeMu.Lock()
	defer e.changeMu.Unlock()
	p, ok := e.manageProject(w, r)
	if !ok {
		return
	}
	archive := req.Archived != nil && *req.Archived && p.ArchivedAt == nil
	if archive && !e.easyIdle(p, e.boardOf(p)) {
		writeError(w, http.StatusConflict, "busy", "the team is still working on this project")
		return
	}
	now := e.now().UTC()
	p, err := e.creator.Store.Update(p.ID, func(p easyproject.Project) (easyproject.Project, error) {
		if name != "" {
			p.Name = name
		}
		if req.Archived != nil {
			switch {
			case *req.Archived && p.ArchivedAt == nil:
				p.ArchivedAt = &now
			case !*req.Archived:
				p.ArchivedAt = nil
			}
		}
		p.UpdatedAt = now
		return p, nil
	})
	if errors.Is(err, easyproject.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such project")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "write_failed", "the project could not be saved")
		return
	}
	if archive && e.previews != nil {
		e.previews.Stop(p.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": p, "board": e.boardOf(p)})
}

func (e *easyProjectsAPI) remove(w http.ResponseWriter, r *http.Request) {
	if serverauth.RoleFromRequest(r) == serverauth.RoleFocus {
		writeError(w, http.StatusForbidden, "forbidden", "a paired phone cannot delete a project")
		return
	}
	e.changeMu.Lock()
	defer e.changeMu.Unlock()
	p, ok := e.manageProject(w, r)
	if !ok {
		return
	}
	if !e.easyIdle(p, e.boardOf(p)) {
		writeError(w, http.StatusConflict, "busy", "the team is still working on this project")
		return
	}
	if e.previews != nil {
		e.previews.Stop(p.ID)
	}
	movedTo := ""
	if p.Folder != "" && !e.folderInUse(p) {
		base, err := e.creator.BaseDir()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "delete_failed", "the project could not be deleted")
			return
		}
		movedTo, err = easyproject.DeleteFolder(base, p.Folder, e.now())
		if errors.Is(err, easyproject.ErrOutsideProjects) {
			writeError(w, http.StatusConflict, "folder_outside", "the project's folder is not in the projects folder; it was left as it is")
			return
		}
		if err != nil {
			e.logger.Warn().Err(err).Str("project_id", p.ID).Msg("easy projects: move the deleted project's folder")
			writeError(w, http.StatusInternalServerError, "delete_failed", "the project could not be deleted")
			return
		}
	}
	if err := e.creator.Store.Remove(p.ID); err != nil && !errors.Is(err, easyproject.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "delete_failed", "the project could not be deleted")
		return
	}
	e.logger.Info().Str("project_id", p.ID).Str("moved_to", movedTo).Msg("easy projects: deleted")
	writeJSON(w, http.StatusOK, map[string]any{"deleted_to": movedTo})
}

// folderInUse reports whether another project records p's folder. A failed
// creation removes its half-made folder but keeps the path on its record;
// the next project made from the same idea takes the same name. Deleting
// the failed record must not move that project's folder away. An unreadable
// list counts as in use: the record goes, the folder stays.
func (e *easyProjectsAPI) folderInUse(p easyproject.Project) bool {
	list, err := e.creator.Store.List()
	if err != nil {
		return true
	}
	folder := filepath.Clean(p.Folder)
	for _, other := range list {
		if other.ID != p.ID && other.Folder != "" && filepath.Clean(other.Folder) == folder {
			return true
		}
	}
	return false
}
