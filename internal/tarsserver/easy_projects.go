package tarsserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/easyproject"
	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/rs/zerolog"
)

// Easy-mode projects (docs/decisions/vibe-coder-identity.md, phase 2):
//
//	POST /v1/easy/projects        {idea} → 202 {project}; 400 idea_required|idea_too_long, 503 node_missing|cannot_run
//	GET  /v1/easy/projects[?state=&limit=&cursor=&archived=1]
//	     → {projects: [project + board], next_cursor?, counts, archived}; 400 bad_state|bad_cursor|bad_limit
//	GET  /v1/easy/projects/{id}   → {project, board}; 404
//	PATCH  /v1/easy/projects/{id} {name?, archived?} → {project, board} (easy_manage.go)
//	DELETE /v1/easy/projects/{id} → {deleted_to?} (easy_manage.go)
//
// One sentence is all the caller sends. The server picks the folder
// (~/TARS Projects/<name>), sets the web app stack up in it, starts version
// control and opens a webapp-template focus pipeline there whose first turn
// it sends itself. The folder is the server's own choice, so — unlike a
// pipeline in a folder the caller names — a user token is enough, and a
// paired phone (role focus) may call them too (docs/plans/vibe-mode-redesign
// unit 6): the request carries only an idea, so a phone chooses no folder,
// tier or opt-in, and every id these routes take names a record in the easy
// project store, never a session. Deleting a project and stopping its
// preview stay refused to a phone (easy_manage.go, easy_preview.go); the
// phone opens the app through the preview relay (easy_preview_relay.go).

type easyProjectsAPI struct {
	creator *easyproject.Creator
	// canRun reports whether the server can send a pipeline's first turn.
	canRun func() bool
	// previews run the projects' apps for the result screen (easy_preview.go).
	previews *easyproject.Previews
	// phone is the preview relay a paired phone opens the app through
	// (easy_preview_relay.go); nil without one.
	phone *easyPhonePreview
	// pipelines reads the projects' focus pipelines; now stamps changes.
	pipelines func() *focuspipeline.Store
	// turnRunning reports a turn holding a session (a person's own turn
	// in the project's pipeline session, say), so it is not deleted under
	// it.
	turnRunning func(string) bool
	now         func() time.Time
	// changeMu makes a change, or a going back, one step (easy_change.go,
	// easy_versions.go): one thing works in a project's folder at a time.
	changeMu sync.Mutex
	logger   zerolog.Logger
}

// registerEasyProjects adds the routes to the focus API's mux: opening the
// pipeline is that API's create path.
func registerEasyProjects(mux *http.ServeMux, api *focusAPI, logger zerolog.Logger) {
	if api.sessions == nil {
		return
	}
	store := easyproject.NewStore(api.sessions.WorkspaceDir())
	// No creation from before a restart can still be running.
	if n, err := store.MarkInterrupted(api.now()); err != nil {
		logger.Warn().Err(err).Msg("easy projects: read the project list")
	} else if n > 0 {
		logger.Info().Int("count", n).Msg("easy projects: marked interrupted creations as failed")
	}
	easy := &easyProjectsAPI{
		creator: &easyproject.Creator{
			Store: store, BaseDir: easyproject.BaseDir, Scaffolder: easyproject.ScaffolderFromEnv(),
			Open: easyOpenPipeline(api), Now: api.now,
			OnFailure: func(p easyproject.Project, err error) {
				logger.Warn().Err(err).Str("project_id", p.ID).Str("code", p.ErrorCode).Msg("easy projects: creation failed")
			},
		},
		canRun:      func() bool { return api.driver != nil && api.driver.canRunTurns() },
		pipelines:   api.store,
		turnRunning: api.driver.turnRunning,
		now:         api.now,
		logger:      logger,
	}
	previews, stopPreviews := newEasyPreviews(api.sessions.WorkspaceDir())
	easy.previews = previews
	if api.driver != nil && api.driver.phonePreview != nil {
		easy.phone = api.driver.phonePreview
		easy.phone.previews.Store(previews)
	}
	if api.driver != nil {
		// A preview must not outlive the server: it holds a port.
		api.driver.onClose(stopPreviews)
	}
	mux.HandleFunc("POST /v1/easy/projects", easy.create)
	mux.HandleFunc("GET /v1/easy/projects", easy.list)
	mux.HandleFunc("GET /v1/easy/projects/{id}", easy.get)
	mux.HandleFunc("PATCH /v1/easy/projects/{id}", easy.update)
	mux.HandleFunc("DELETE /v1/easy/projects/{id}", easy.remove)
	mux.HandleFunc("GET /v1/easy/projects/{id}/preview", easy.previewStatus)
	mux.HandleFunc("POST /v1/easy/projects/{id}/preview", easy.previewStart)
	mux.HandleFunc("DELETE /v1/easy/projects/{id}/preview", easy.previewStop)
	mux.HandleFunc("POST /v1/easy/projects/{id}/preview/ticket", easy.previewTicket)
	mux.HandleFunc("POST /v1/easy/projects/{id}/changes", easy.change)
	mux.HandleFunc("GET /v1/easy/projects/{id}/versions", easy.versions)
	mux.HandleFunc("POST /v1/easy/projects/{id}/versions/{commit}/restore", easy.restore)
}

// easyOpenPipeline opens a project's pipeline through the focus API's
// create path: the webapp template, in the project folder and never moved
// out of it (the folder is the project's own, stayInFolder), the first
// turn sent by the server and run as the role that asked. A change opens
// its pipeline the same way, with the change as the goal.
func easyOpenPipeline(api *focusAPI) easyproject.Opener {
	return func(ctx context.Context, folder, goal string) (string, error) {
		role := serverauth.RoleFromContext(ctx)
		if role == "" {
			role = serverauth.RoleUser
		}
		status, resp := api.open(ctx, focusCreateRequest{
			Goal: goal, Cwd: folder, Template: focuspipeline.WebAppTemplateID, Start: true,
			saveVersion: true, goalAfterPlan: true, stayInFolder: true,
		}, role)
		if status != http.StatusCreated {
			msg, _ := resp["error"].(string)
			return "", fmt.Errorf("open pipeline: %d %s", status, msg)
		}
		id, _ := resp["session_id"].(string)
		return id, nil
	}
}

func (e *easyProjectsAPI) create(w http.ResponseWriter, r *http.Request) {
	role := serverauth.RoleFromRequest(r)
	var req struct {
		Idea string `json:"idea"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if !e.canRun() {
		writeError(w, http.StatusServiceUnavailable, "cannot_run", "the server cannot start the first turn")
		return
	}
	p, err := e.creator.Start(serverauth.WithRoleContext(r.Context(), role), req.Idea)
	switch {
	case errors.Is(err, easyproject.ErrIdeaRequired):
		writeError(w, http.StatusBadRequest, "idea_required", err.Error())
	case errors.Is(err, easyproject.ErrIdeaTooLong):
		writeError(w, http.StatusBadRequest, "idea_too_long", err.Error())
	case errors.Is(err, easyproject.ErrNodeMissing):
		writeError(w, http.StatusServiceUnavailable, easyproject.ErrorNodeMissing, err.Error())
	case err != nil:
		writeError(w, http.StatusInternalServerError, "create_failed", "the project could not be created")
	default:
		writeJSON(w, http.StatusAccepted, map[string]any{"project": p})
	}
}

// boardOf reads the project's current pipeline and gives its board. A
// pipeline that cannot be read counts as gone.
func (e *easyProjectsAPI) boardOf(p easyproject.Project) easyproject.Board {
	if p.SessionID == "" || p.Status != easyproject.StatusReady {
		return easyproject.BoardOf(p, nil)
	}
	pipe, ok, err := e.pipelines().Get(p.SessionID)
	if err != nil || !ok {
		return easyproject.BoardOf(p, nil)
	}
	return easyproject.BoardOf(p, &pipe)
}

// list answers the board: every project with its state, read in this one
// request. Without a limit it lists every project on the board (the
// console's first easy screens read the whole list).
func (e *easyProjectsAPI) list(w http.ResponseWriter, r *http.Request) {
	q := easyproject.ListQuery{
		State:    easyproject.State(r.URL.Query().Get("state")),
		Archived: r.URL.Query().Get("archived") == "1",
		Cursor:   r.URL.Query().Get("cursor"),
	}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "bad_limit", "limit is a positive number")
			return
		}
		q.Limit = n
	}
	list, err := e.creator.Store.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", "the project list could not be read")
		return
	}
	all := make([]easyproject.Listed, 0, len(list))
	for _, p := range list {
		all = append(all, easyproject.Listed{Project: p, Board: e.boardOf(p)})
	}
	page, err := easyproject.Page(all, q)
	switch {
	case errors.Is(err, easyproject.ErrBadState):
		writeError(w, http.StatusBadRequest, "bad_state", err.Error())
	case errors.Is(err, easyproject.ErrBadCursor):
		writeError(w, http.StatusBadRequest, "bad_cursor", err.Error())
	case err != nil:
		writeError(w, http.StatusInternalServerError, "read_failed", "the project list could not be read")
	default:
		writeJSON(w, http.StatusOK, page)
	}
}

func (e *easyProjectsAPI) get(w http.ResponseWriter, r *http.Request) {
	p, err := e.creator.Store.Get(r.PathValue("id"))
	if errors.Is(err, easyproject.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such project")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", "the project could not be read")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": p, "board": e.boardOf(p)})
}
