package tarsserver

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/devlikebear/tarsncase/internal/project"
	"github.com/devlikebear/tarsncase/internal/session"
	"github.com/rs/zerolog"
)

func newProjectAPIHandler(store *project.Store, sessionStore *session.Store, mainSessionID string, logger zerolog.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/projects", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "project store is not configured"})
			return
		}
		switch r.Method {
		case http.MethodGet:
			items, err := store.List()
			if err != nil {
				logger.Error().Err(err).Msg("list projects failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list projects failed"})
				return
			}
			writeJSON(w, http.StatusOK, items)
		case http.MethodPost:
			var req struct {
				Name         string `json:"name"`
				Type         string `json:"type,omitempty"`
				GitRepo      string `json:"git_repo,omitempty"`
				Objective    string `json:"objective,omitempty"`
				Instructions string `json:"instructions,omitempty"`
				CloneRepo    bool   `json:"clone_repo,omitempty"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
				return
			}
			created, err := store.Create(project.CreateInput{
				Name:         req.Name,
				Type:         req.Type,
				GitRepo:      req.GitRepo,
				Objective:    req.Objective,
				Instructions: req.Instructions,
				CloneRepo:    req.CloneRepo,
			})
			if err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "required") || strings.Contains(strings.ToLower(err.Error()), "invalid") {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
				logger.Error().Err(err).Msg("create project failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create project failed"})
				return
			}
			writeJSON(w, http.StatusOK, created)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/v1/projects/", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "project store is not configured"})
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/v1/projects/")
		parts := strings.Split(path, "/")
		if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
			http.NotFound(w, r)
			return
		}
		projectID := strings.TrimSpace(parts[0])

		if len(parts) == 1 {
			switch r.Method {
			case http.MethodGet:
				item, err := store.Get(projectID)
				if err != nil {
					if strings.Contains(strings.ToLower(err.Error()), "not found") {
						writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
						return
					}
					logger.Error().Err(err).Str("project_id", projectID).Msg("get project failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get project failed"})
					return
				}
				writeJSON(w, http.StatusOK, item)
			case http.MethodPatch:
				var req project.UpdatePayload
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
					return
				}
				updated, err := store.Update(projectID, req.ToUpdateInput())
				if err != nil {
					if strings.Contains(strings.ToLower(err.Error()), "not found") {
						writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
						return
					}
					if strings.Contains(strings.ToLower(err.Error()), "required") || strings.Contains(strings.ToLower(err.Error()), "invalid") {
						writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
						return
					}
					logger.Error().Err(err).Str("project_id", projectID).Msg("update project failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update project failed"})
					return
				}
				writeJSON(w, http.StatusOK, updated)
			case http.MethodDelete:
				if _, err := store.Archive(projectID); err != nil {
					if strings.Contains(strings.ToLower(err.Error()), "not found") {
						writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
						return
					}
					logger.Error().Err(err).Str("project_id", projectID).Msg("archive project failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "archive project failed"})
					return
				}
				w.WriteHeader(http.StatusNoContent)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}

		if len(parts) == 2 && parts[1] == "activate" {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if sessionStore == nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "session store is not configured"})
				return
			}
			if _, err := store.Get(projectID); err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "not found") {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
					return
				}
				logger.Error().Err(err).Str("project_id", projectID).Msg("get project failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get project failed"})
				return
			}
			var req struct {
				SessionID string `json:"session_id,omitempty"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
				return
			}
			sessionID := strings.TrimSpace(req.SessionID)
			if sessionID == "" {
				sessionID = strings.TrimSpace(mainSessionID)
			}
			if sessionID == "" {
				latest, err := sessionStore.Latest()
				if err == nil {
					sessionID = strings.TrimSpace(latest.ID)
				}
			}
			if sessionID == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session_id is required"})
				return
			}
			if _, err := sessionStore.Get(sessionID); err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "not found") {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
					return
				}
				logger.Error().Err(err).Str("session_id", sessionID).Msg("get session failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get session failed"})
				return
			}
			if err := sessionStore.SetProjectID(sessionID, projectID); err != nil {
				logger.Error().Err(err).Str("session_id", sessionID).Str("project_id", projectID).Msg("set session project failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "activate project failed"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"activated":  true,
				"project_id": projectID,
				"session_id": sessionID,
			})
			return
		}

		http.NotFound(w, r)
	})

	return mux
}
