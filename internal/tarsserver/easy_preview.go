package tarsserver

import (
	"errors"
	"net/http"
	"path/filepath"
	"time"

	"github.com/devlikebear/tars/internal/easyproject"
	"github.com/devlikebear/tars/internal/serverauth"
)

// An easy-mode project's preview (vibe-coder-identity.md §4, phase 3):
//
//	GET    /v1/easy/projects/{id}/preview → {preview}; reading it keeps it running
//	POST   /v1/easy/projects/{id}/preview → {preview}; starts it unless it runs
//	DELETE /v1/easy/projects/{id}/preview → {preview} (stopped)
//
// The app runs on this computer's loopback address. A paired phone (role
// focus) may start and read the preview (vibe redesign unit 7) but gets no
// address — it opens the app through the preview relay's ticket
// (easy_preview_relay.go) — and cannot stop it: the 10-minute idle sweep
// does. 404 for an unknown project, 409 not_ready while the project is
// still being made or could not be.

// easyPreviewSweep is how often idle previews are looked for.
const easyPreviewSweep = time.Minute

// newEasyPreviews makes the previews of the projects in workspace and stops
// the idle ones every minute until the returned stop is called.
func newEasyPreviews(workspace string) (*easyproject.Previews, func()) {
	previews := &easyproject.Previews{
		Runner: easyproject.PreviewRunnerFromEnv(),
		LogDir: filepath.Join(workspace, "_shared", "easy", "previews"),
	}
	done := make(chan struct{})
	go func() {
		tick := time.NewTicker(easyPreviewSweep)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				previews.StopIdle()
			}
		}
	}()
	return previews, func() {
		close(done)
		previews.Close()
	}
}

// previewProject answers the ready project the request names, or writes the
// error and answers false.
func (e *easyProjectsAPI) previewProject(w http.ResponseWriter, r *http.Request) (easyproject.Project, bool) {
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

func (e *easyProjectsAPI) previewStatus(w http.ResponseWriter, r *http.Request) {
	p, ok := e.previewProject(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"preview": previewFor(r, e.previews.Status(p.ID))})
}

func (e *easyProjectsAPI) previewStart(w http.ResponseWriter, r *http.Request) {
	p, ok := e.previewProject(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"preview": previewFor(r, e.previews.Start(p.ID, p.Folder))})
}

func (e *easyProjectsAPI) previewStop(w http.ResponseWriter, r *http.Request) {
	if serverauth.RoleFromRequest(r) == serverauth.RoleFocus {
		writeError(w, http.StatusForbidden, "forbidden", "a paired phone cannot stop a preview")
		return
	}
	p, ok := e.previewProject(w, r)
	if !ok {
		return
	}
	e.previews.Stop(p.ID)
	writeJSON(w, http.StatusOK, map[string]any{"preview": e.previews.Status(p.ID)})
}
