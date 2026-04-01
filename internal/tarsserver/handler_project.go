package tarsserver

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/project"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func newProjectAPIHandler(
	store *project.Store,
	sessionStore *session.Store,
	mainSessionID string,
	taskRunner project.TaskRunner,
	githubAuthChecker project.GitHubAuthChecker,
	skillResolver project.SkillResolver,
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
			if !requireMethod(w, r, http.MethodGet, http.MethodPatch) {
				return
			}
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
				if !decodeJSONBody(w, r, &req) {
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
			}
			return
		}
		if len(parts) == 2 && parts[1] == "finalize" {
			if !requireMethod(w, r, http.MethodPost) {
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
			state, err := store.GetState(created.ID)
			if err != nil {
				logger.Error().Err(err).Str("project_id", created.ID).Msg("get project state after brief finalize failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "load project state failed"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"project":        created,
				"brief":          brief,
				"state":          state,
				"planning_ready": true,
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
		if !requireMethod(w, r, http.MethodGet, http.MethodPost, http.MethodDelete) {
			return
		}
		switch r.Method {
		case http.MethodDelete:
			count, err := store.DeleteAll()
			if err != nil {
				logger.Error().Err(err).Msg("delete all projects failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete all projects failed"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]int{"deleted": count})
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
				Name            string                      `json:"name"`
				Type            string                      `json:"type,omitempty"`
				GitRepo         string                      `json:"git_repo,omitempty"`
				Objective       string                      `json:"objective,omitempty"`
				WorkflowProfile string                      `json:"workflow_profile,omitempty"`
				WorkflowRules   []project.WorkflowRule      `json:"workflow_rules,omitempty"`
				Instructions    string                      `json:"instructions,omitempty"`
				CloneRepo       bool                        `json:"clone_repo,omitempty"`
				ExecutionMode   string                      `json:"execution_mode,omitempty"`
				MaxPhases       int                         `json:"max_phases,omitempty"`
				SubAgents       []project.SubAgentConfig    `json:"sub_agents,omitempty"`
				SkillsAllow     []string                    `json:"skills_allow,omitempty"`
			}
			if !decodeJSONBody(w, r, &req) {
				return
			}
			created, err := store.Create(project.CreateInput{
				Name:            req.Name,
				Type:            req.Type,
				GitRepo:         req.GitRepo,
				Objective:       req.Objective,
				WorkflowProfile: req.WorkflowProfile,
				WorkflowRules:   req.WorkflowRules,
				Instructions:    req.Instructions,
				CloneRepo:       req.CloneRepo,
				ExecutionMode:   req.ExecutionMode,
				MaxPhases:       req.MaxPhases,
				SubAgents:       req.SubAgents,
				SkillsAllow:     req.SkillsAllow,
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
			// Auto-create a dedicated session for the project
			if sessionStore != nil && strings.TrimSpace(created.SessionID) == "" {
				if sess, err := sessionStore.Create(strings.TrimSpace(created.Name)); err == nil {
					_ = sessionStore.SetProjectID(sess.ID, created.ID)
					sessID := sess.ID
					created, _ = store.Update(created.ID, project.UpdateInput{SessionID: &sessID})
				}
			}
			writeJSON(w, http.StatusOK, created)

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
			if !requireMethod(w, r, http.MethodGet, http.MethodPatch, http.MethodDelete) {
				return
			}
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
				if !decodeJSONBody(w, r, &req) {
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

				// Sync STATE.md when project status changes
				syncProjectStateOnStatusChange(store, projectID, updated.Status)
				writeJSON(w, http.StatusOK, updated)
			case http.MethodDelete:
				if err := store.Delete(projectID); err != nil {
					if strings.Contains(strings.ToLower(err.Error()), "not found") {
						writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
						return
					}
					logger.Error().Err(err).Str("project_id", projectID).Msg("delete project failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete project failed"})
					return
				}

				w.WriteHeader(http.StatusNoContent)
			}
			return
		}

		if len(parts) == 2 && parts[1] == "state" {
			if !requireMethod(w, r, http.MethodGet, http.MethodPatch) {
				return
			}
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
				if !decodeJSONBody(w, r, &req) {
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

				writeJSON(w, http.StatusOK, updated)
			}
			return
		}

		if len(parts) == 2 && parts[1] == "activity" {
			if !requireMethod(w, r, http.MethodGet, http.MethodPost) {
				return
			}
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
				if !decodeJSONBody(w, r, &req) {
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

				writeJSON(w, http.StatusOK, item)
			}
			return
		}

		if len(parts) == 2 && parts[1] == "board" {
			if !requireMethod(w, r, http.MethodGet, http.MethodPatch) {
				return
			}
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
				if !decodeJSONBody(w, r, &req) {
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

				writeJSON(w, http.StatusOK, item)
			}
			return
		}

		if len(parts) == 2 && parts[1] == "dispatch" {
			if !requireMethod(w, r, http.MethodPost) {
				return
			}
			if taskRunner == nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "project task runner is not configured"})
				return
			}
			var req struct {
				Stage string `json:"stage,omitempty"`
			}
			if !decodeOptionalJSONBody(w, r, &req) {
				return
			}
			orchestrator := project.NewOrchestratorWithGitHubAuthChecker(store, taskRunner, githubAuthChecker)
			orchestrator.SetSkillResolver(skillResolver)
			stage, ok := project.DefaultWorkflowPolicy.NormalizeDispatchStage(req.Stage)
			if !ok {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "stage must be todo or review"})
				return
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
			}
			if err != nil {
				logger.Error().Err(err).Str("project_id", projectID).Str("stage", stage).Msg("dispatch project tasks failed")
				writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, report)
			return
		}

		if len(parts) == 2 && parts[1] == "activate" {
			if !requireMethod(w, r, http.MethodPost) {
				return
			}
			activeStatus := "active"
			updated, err := store.Update(projectID, project.UpdateInput{Status: &activeStatus})
			if err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "not found") {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
					return
				}
				logger.Error().Err(err).Str("project_id", projectID).Msg("activate project failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "activate project failed"})
				return
			}
			syncProjectStateOnStatusChange(store, projectID, "active")
			writeJSON(w, http.StatusOK, map[string]any{
				"activated":  true,
				"project_id": projectID,
				"session_id": updated.SessionID,
			})
			return
		}

		if len(parts) == 2 && parts[1] == "deactivate" {
			if !requireMethod(w, r, http.MethodPost) {
				return
			}
			archivedStatus := "archived"
			updated, err := store.Update(projectID, project.UpdateInput{Status: &archivedStatus})
			if err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "not found") {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
					return
				}
				logger.Error().Err(err).Str("project_id", projectID).Msg("deactivate project failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "deactivate project failed"})
				return
			}
			syncProjectStateOnStatusChange(store, projectID, "archived")
			writeJSON(w, http.StatusOK, map[string]any{
				"deactivated": true,
				"project_id":  updated.ID,
			})
			return
		}

		// Project session endpoints
		if len(parts) == 2 && parts[1] == "session" {
			if !requireMethod(w, r, http.MethodGet) {
				return
			}
			item, err := store.Get(projectID)
			if err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "not found") {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get project failed"})
				return
			}
			sessionID := resolveProjectSessionID(item, sessionStore)
			if sessionID == "" {
				writeJSON(w, http.StatusOK, map[string]any{
					"project_id": projectID,
					"session_id": "",
					"messages":   0,
					"tokens":     0,
				})
				return
			}
			messages, _ := session.ReadMessages(sessionStore.TranscriptPath(sessionID))
			tokens := 0
			for _, m := range messages {
				cost := len(m.Content) / 4
				if cost < 1 {
					cost = 1
				}
				tokens += cost
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"project_id": projectID,
				"session_id": sessionID,
				"messages":   len(messages),
				"tokens":     tokens,
			})
			return
		}

		if len(parts) == 3 && parts[1] == "session" && parts[2] == "clear" {
			if !requireMethod(w, r, http.MethodPost) {
				return
			}
			item, err := store.Get(projectID)
			if err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "not found") {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get project failed"})
				return
			}
			sessionID := resolveProjectSessionID(item, sessionStore)
			if sessionID == "" {
				writeJSON(w, http.StatusOK, map[string]any{"cleared": false, "reason": "no session"})
				return
			}
			path := sessionStore.TranscriptPath(sessionID)
			if err := session.RewriteMessages(path, nil); err != nil {
				logger.Error().Err(err).Str("session_id", sessionID).Msg("clear session failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "clear session failed"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"cleared":    true,
				"project_id": projectID,
				"session_id": sessionID,
			})
			return
		}

		if len(parts) == 3 && parts[1] == "session" && parts[2] == "compact" {
			if !requireMethod(w, r, http.MethodPost) {
				return
			}
			item, err := store.Get(projectID)
			if err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "not found") {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get project failed"})
				return
			}
			sessionID := resolveProjectSessionID(item, sessionStore)
			if sessionID == "" {
				writeJSON(w, http.StatusOK, map[string]any{"compacted": false, "reason": "no session"})
				return
			}
			path := sessionStore.TranscriptPath(sessionID)
			messages, _ := session.ReadMessages(path)
			originalCount := len(messages)
			compacted, err := session.CompactTranscriptWithOptions(path, 0, time.Now().UTC(), session.CompactOptions{
				KeepRecentTokens:    12000,
				KeepRecentFraction:  0.30,
			})
			if err != nil {
				logger.Error().Err(err).Str("session_id", sessionID).Msg("compact session failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "compact session failed"})
				return
			}
			finalMessages, _ := session.ReadMessages(path)
			writeJSON(w, http.StatusOK, map[string]any{
				"compacted":      compacted,
				"project_id":     projectID,
				"session_id":     sessionID,
				"original_count": originalCount,
				"final_count":    len(finalMessages),
			})
			return
		}

		// Project files browser
		if len(parts) >= 2 && parts[1] == "files" {
			if !requireMethod(w, r, http.MethodGet) {
				return
			}
			projectDir := store.ProjectDir(projectID)
			if projectDir == "" {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "project directory not found"})
				return
			}

			if len(parts) == 2 {
				// List files
				entries, err := os.ReadDir(projectDir)
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read project directory failed"})
					return
				}
				systemFiles := map[string]bool{
					"PROJECT.md": true, "STATE.md": true, "KANBAN.md": true,
					"ACTIVITY.jsonl": true, "AUTOPILOT.json": true,
				}
				type fileEntry struct {
					Name   string `json:"name"`
					Size   int64  `json:"size"`
					System bool   `json:"system"`
				}
				files := make([]fileEntry, 0)
				for _, e := range entries {
					if e.IsDir() {
						continue
					}
					info, err := e.Info()
					if err != nil {
						continue
					}
					files = append(files, fileEntry{
						Name:   e.Name(),
						Size:   info.Size(),
						System: systemFiles[e.Name()],
					})
				}
				writeJSON(w, http.StatusOK, files)
				return
			}

			// Read file content: /v1/projects/{id}/files/{filename}
			filename := strings.Join(parts[2:], "/")
			if strings.Contains(filename, "..") {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid filename"})
				return
			}
			filePath := filepath.Join(projectDir, filename)
			data, err := os.ReadFile(filePath)
			if err != nil {
				if os.IsNotExist(err) {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read file failed"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"name":    filename,
				"content": string(data),
				"size":    len(data),
			})
			return
		}

		http.NotFound(w, r)
	})

	return mux
}

func syncProjectStateOnStatusChange(store *project.Store, projectID, status string) {
	if store == nil {
		return
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "archived":
		donePhase := "done"
		doneStatus := "done"
		stopReason := "Project archived"
		_, _ = store.UpdateState(projectID, project.ProjectStateUpdateInput{
			Phase: &donePhase, Status: &doneStatus, StopReason: &stopReason,
		})
	case "active":
		planningPhase := "planning"
		activeStatus := "active"
		_, _ = store.UpdateState(projectID, project.ProjectStateUpdateInput{
			Phase: &planningPhase, Status: &activeStatus,
		})
	}
}

func resolveProjectSessionID(item project.Project, sessionStore *session.Store) string {
	if id := strings.TrimSpace(item.SessionID); id != "" {
		return id
	}
	if sessionStore == nil {
		return ""
	}
	allSessions, err := sessionStore.List()
	if err != nil {
		return ""
	}
	for _, s := range allSessions {
		if strings.TrimSpace(s.ProjectID) == strings.TrimSpace(item.ID) {
			return s.ID
		}
	}
	return ""
}
