package tarsserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/devlikebear/tars/internal/easyproject"
	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/rs/zerolog"
)

// Easy-mode projects (docs/decisions/vibe-coder-identity.md, phase 2):
//
//	POST /v1/easy/projects        {idea} → 202 {project}; 400 idea_required|idea_too_long, 503 node_missing|cannot_run
//	GET  /v1/easy/projects        → {projects} newest first
//	GET  /v1/easy/projects/{id}   → {project}; 404
//
// One sentence is all the caller sends. The server picks the folder
// (~/TARS Projects/<name>), sets the web app stack up in it, starts version
// control and opens a webapp-template focus pipeline there whose first turn
// it sends itself. The folder is the server's own choice, so — unlike a
// pipeline in a folder the caller names — a user token is enough; a paired
// phone (role focus) never reaches these routes.

type easyProjectsAPI struct {
	creator *easyproject.Creator
	// canRun reports whether the server can send a pipeline's first turn.
	canRun func() bool
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
		canRun: func() bool { return api.driver != nil && api.driver.canRunTurns() },
	}
	mux.HandleFunc("POST /v1/easy/projects", easy.create)
	mux.HandleFunc("GET /v1/easy/projects", easy.list)
	mux.HandleFunc("GET /v1/easy/projects/{id}", easy.get)
}

// easyOpenPipeline opens the project's pipeline through the focus API's
// create path: the webapp template, in the project folder, not isolated
// (the folder is the project's own), the first turn sent by the server and
// run as the role that asked.
func easyOpenPipeline(api *focusAPI) easyproject.Opener {
	return func(ctx context.Context, folder, idea string) (string, error) {
		role := serverauth.RoleFromContext(ctx)
		if role == "" {
			role = serverauth.RoleUser
		}
		status, resp := api.open(ctx, focusCreateRequest{
			Goal: idea, Cwd: folder, Template: focuspipeline.WebAppTemplateID, Start: true,
			saveVersion: true,
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
	if role == serverauth.RoleFocus {
		writeError(w, http.StatusForbidden, "forbidden", "a paired phone cannot start a project")
		return
	}
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

func (e *easyProjectsAPI) list(w http.ResponseWriter, _ *http.Request) {
	list, err := e.creator.Store.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", "the project list could not be read")
		return
	}
	if list == nil {
		list = []easyproject.Project{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": list})
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
	writeJSON(w, http.StatusOK, map[string]any{"project": p})
}
