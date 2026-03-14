package tarsserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/devlikebear/tars/internal/project"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

type projectAutopilotManager interface {
	Start(context.Context, string) (project.AutopilotRun, error)
	Status(string) (project.AutopilotRun, bool)
}

func newProjectAPIHandler(
	store *project.Store,
	sessionStore *session.Store,
	mainSessionID string,
	taskRunner project.TaskRunner,
	githubAuthChecker project.GitHubAuthChecker,
	autopilot projectAutopilotManager,
	dashboardBroker *projectDashboardBroker,
	logger zerolog.Logger,
) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/project-briefs/", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "project store is not configured"})
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/v1/project-briefs/")
		parts := strings.Split(path, "/")
		if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
			http.NotFound(w, r)
			return
		}
		briefID := strings.TrimSpace(parts[0])
		if len(parts) == 1 {
			switch r.Method {
			case http.MethodGet:
				item, err := store.GetBrief(briefID)
				if err != nil {
					if strings.Contains(strings.ToLower(err.Error()), "not found") {
						writeJSON(w, http.StatusNotFound, map[string]string{"error": "brief not found"})
						return
					}
					logger.Error().Err(err).Str("brief_id", briefID).Msg("get brief failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get brief failed"})
					return
				}
				writeJSON(w, http.StatusOK, item)
			case http.MethodPatch:
				var req struct {
					Title              *string  `json:"title,omitempty"`
					Goal               *string  `json:"goal,omitempty"`
					Kind               *string  `json:"kind,omitempty"`
					Genre              *string  `json:"genre,omitempty"`
					TargetLength       *string  `json:"target_length,omitempty"`
					Cadence            *string  `json:"cadence,omitempty"`
					TargetInstallments *string  `json:"target_installments,omitempty"`
					Premise            *string  `json:"premise,omitempty"`
					PlotSeed           *string  `json:"plot_seed,omitempty"`
					StylePreferences   *string  `json:"style_preferences,omitempty"`
					Constraints        []string `json:"constraints,omitempty"`
					MustHave           []string `json:"must_have,omitempty"`
					MustAvoid          []string `json:"must_avoid,omitempty"`
					OpenQuestions      []string `json:"open_questions,omitempty"`
					Decisions          []string `json:"decisions,omitempty"`
					Status             *string  `json:"status,omitempty"`
					Summary            *string  `json:"summary,omitempty"`
					Body               *string  `json:"body,omitempty"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
					return
				}
				updated, err := store.UpdateBrief(briefID, project.BriefUpdateInput{
					Title:              req.Title,
					Goal:               req.Goal,
					Kind:               req.Kind,
					Genre:              req.Genre,
					TargetLength:       req.TargetLength,
					Cadence:            req.Cadence,
					TargetInstallments: req.TargetInstallments,
					Premise:            req.Premise,
					PlotSeed:           req.PlotSeed,
					StylePreferences:   req.StylePreferences,
					Constraints:        req.Constraints,
					MustHave:           req.MustHave,
					MustAvoid:          req.MustAvoid,
					OpenQuestions:      req.OpenQuestions,
					Decisions:          req.Decisions,
					Status:             req.Status,
					Summary:            req.Summary,
					Body:               req.Body,
				})
				if err != nil {
					if strings.Contains(strings.ToLower(err.Error()), "required") || strings.Contains(strings.ToLower(err.Error()), "invalid") {
						writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
						return
					}
					logger.Error().Err(err).Str("brief_id", briefID).Msg("update brief failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update brief failed"})
					return
				}
				writeJSON(w, http.StatusOK, updated)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}
		if len(parts) == 2 && parts[1] == "finalize" {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			created, brief, err := store.FinalizeBrief(briefID, sessionStore)
			if err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "not found") {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "brief not found"})
					return
				}
				if strings.Contains(strings.ToLower(err.Error()), "already finalized") {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
				logger.Error().Err(err).Str("brief_id", briefID).Msg("finalize brief failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "finalize brief failed"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"project": created,
				"brief":   brief,
				"seeded":  true,
			})
			return
		}
		http.NotFound(w, r)
	})

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
			dashboardBroker.publish(newProjectDashboardEvent(created.ID, "activity"))
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
				dashboardBroker.publish(newProjectDashboardEvent(projectID, "activity"))
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
				dashboardBroker.publish(newProjectDashboardEvent(projectID, "activity"))
				w.WriteHeader(http.StatusNoContent)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}

		if len(parts) == 2 && parts[1] == "state" {
			switch r.Method {
			case http.MethodGet:
				item, err := store.GetState(projectID)
				if err != nil {
					if strings.Contains(strings.ToLower(err.Error()), "not found") {
						writeJSON(w, http.StatusNotFound, map[string]string{"error": "project state not found"})
						return
					}
					logger.Error().Err(err).Str("project_id", projectID).Msg("get project state failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get project state failed"})
					return
				}
				writeJSON(w, http.StatusOK, item)
			case http.MethodPatch:
				var req struct {
					Goal              *string  `json:"goal,omitempty"`
					Phase             *string  `json:"phase,omitempty"`
					Status            *string  `json:"status,omitempty"`
					NextAction        *string  `json:"next_action,omitempty"`
					RemainingTasks    []string `json:"remaining_tasks,omitempty"`
					CompletionSummary *string  `json:"completion_summary,omitempty"`
					LastRunSummary    *string  `json:"last_run_summary,omitempty"`
					LastRunAt         *string  `json:"last_run_at,omitempty"`
					StopReason        *string  `json:"stop_reason,omitempty"`
					Body              *string  `json:"body,omitempty"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
					return
				}
				updated, err := store.UpdateState(projectID, project.ProjectStateUpdateInput{
					Goal:              req.Goal,
					Phase:             req.Phase,
					Status:            req.Status,
					NextAction:        req.NextAction,
					RemainingTasks:    req.RemainingTasks,
					CompletionSummary: req.CompletionSummary,
					LastRunSummary:    req.LastRunSummary,
					LastRunAt:         req.LastRunAt,
					StopReason:        req.StopReason,
					Body:              req.Body,
				})
				if err != nil {
					if strings.Contains(strings.ToLower(err.Error()), "required") || strings.Contains(strings.ToLower(err.Error()), "invalid") || strings.Contains(strings.ToLower(err.Error()), "not found") {
						status := http.StatusBadRequest
						if strings.Contains(strings.ToLower(err.Error()), "not found") {
							status = http.StatusNotFound
						}
						writeJSON(w, status, map[string]string{"error": err.Error()})
						return
					}
					logger.Error().Err(err).Str("project_id", projectID).Msg("update project state failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update project state failed"})
					return
				}
				dashboardBroker.publish(newProjectDashboardEvent(projectID, "activity"))
				writeJSON(w, http.StatusOK, updated)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}

		if len(parts) == 2 && parts[1] == "activity" {
			switch r.Method {
			case http.MethodGet:
				limit, ok := parsePositiveLimit(w, r, 50)
				if !ok {
					return
				}
				items, err := store.ListActivity(projectID, limit)
				if err != nil {
					if strings.Contains(strings.ToLower(err.Error()), "not found") {
						writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
						return
					}
					logger.Error().Err(err).Str("project_id", projectID).Msg("list project activity failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list project activity failed"})
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{"count": len(items), "items": items})
			case http.MethodPost:
				var req struct {
					TaskID    string            `json:"task_id,omitempty"`
					Source    string            `json:"source"`
					Agent     string            `json:"agent,omitempty"`
					Kind      string            `json:"kind"`
					Status    string            `json:"status,omitempty"`
					Message   string            `json:"message,omitempty"`
					Timestamp string            `json:"timestamp,omitempty"`
					Meta      map[string]string `json:"meta,omitempty"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
					return
				}
				item, err := store.AppendActivity(projectID, project.ActivityAppendInput{
					TaskID:    req.TaskID,
					Source:    req.Source,
					Agent:     req.Agent,
					Kind:      req.Kind,
					Status:    req.Status,
					Message:   req.Message,
					Timestamp: req.Timestamp,
					Meta:      req.Meta,
				})
				if err != nil {
					lower := strings.ToLower(err.Error())
					if strings.Contains(lower, "not found") {
						writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
						return
					}
					if strings.Contains(lower, "required") || strings.Contains(lower, "invalid") {
						writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
						return
					}
					logger.Error().Err(err).Str("project_id", projectID).Msg("append project activity failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "append project activity failed"})
					return
				}
				dashboardBroker.publish(newProjectDashboardEvent(projectID, "activity"))
				writeJSON(w, http.StatusOK, item)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}

		if len(parts) == 2 && parts[1] == "board" {
			switch r.Method {
			case http.MethodGet:
				item, err := store.GetBoard(projectID)
				if err != nil {
					if strings.Contains(strings.ToLower(err.Error()), "not found") {
						writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
						return
					}
					logger.Error().Err(err).Str("project_id", projectID).Msg("get project board failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get project board failed"})
					return
				}
				writeJSON(w, http.StatusOK, item)
			case http.MethodPatch:
				var req struct {
					Columns []string            `json:"columns,omitempty"`
					Tasks   []project.BoardTask `json:"tasks,omitempty"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
					return
				}
				item, err := store.UpdateBoard(projectID, project.BoardUpdateInput{
					Columns: req.Columns,
					Tasks:   req.Tasks,
				})
				if err != nil {
					lower := strings.ToLower(err.Error())
					if strings.Contains(lower, "not found") {
						writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
						return
					}
					if strings.Contains(lower, "required") || strings.Contains(lower, "invalid") {
						writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
						return
					}
					logger.Error().Err(err).Str("project_id", projectID).Msg("update project board failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update project board failed"})
					return
				}
				dashboardBroker.publish(newProjectDashboardEvent(projectID, "board"))
				writeJSON(w, http.StatusOK, item)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}

		if len(parts) == 2 && parts[1] == "dispatch" {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if taskRunner == nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "project task runner is not configured"})
				return
			}
			var req struct {
				Stage string `json:"stage,omitempty"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
				return
			}
			orchestrator := project.NewOrchestratorWithGitHubAuthChecker(store, taskRunner, githubAuthChecker)
			stage := strings.ToLower(strings.TrimSpace(req.Stage))
			if stage == "" {
				stage = "todo"
			}

			var (
				report project.DispatchReport
				err    error
			)
			switch stage {
			case "todo":
				report, err = orchestrator.DispatchTodo(r.Context(), projectID)
			case "review":
				report, err = orchestrator.DispatchReview(r.Context(), projectID)
			default:
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "stage must be todo or review"})
				return
			}
			if err != nil {
				logger.Error().Err(err).Str("project_id", projectID).Str("stage", stage).Msg("dispatch project tasks failed")
				writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
				return
			}
			dashboardBroker.publish(newProjectDashboardEvent(projectID, "board"))
			dashboardBroker.publish(newProjectDashboardEvent(projectID, "activity"))
			writeJSON(w, http.StatusOK, report)
			return
		}

		if len(parts) == 2 && parts[1] == "autopilot" {
			if autopilot == nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "project autopilot manager is not configured"})
				return
			}
			switch r.Method {
			case http.MethodGet:
				item, ok := autopilot.Status(projectID)
				if !ok {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "project autopilot run not found"})
					return
				}
				writeJSON(w, http.StatusOK, item)
			case http.MethodPost:
				item, err := autopilot.Start(r.Context(), projectID)
				if err != nil {
					lower := strings.ToLower(err.Error())
					if strings.Contains(lower, "not found") {
						writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
						return
					}
					if strings.Contains(lower, "required") || strings.Contains(lower, "invalid") {
						writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
						return
					}
					logger.Error().Err(err).Str("project_id", projectID).Msg("start project autopilot failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "start project autopilot failed"})
					return
				}
				writeJSON(w, http.StatusOK, item)
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
