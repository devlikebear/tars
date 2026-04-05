package tarsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/cron"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/rs/zerolog"
)

func newCronAPIHandler(
	store *cron.Store,
	runPrompt func(ctx context.Context, prompt string) (string, error),
	logger zerolog.Logger,
) http.Handler {
	var runJob func(ctx context.Context, job cron.Job) (string, error)
	if runPrompt != nil {
		runJob = func(ctx context.Context, job cron.Job) (string, error) {
			return runPrompt(ctx, job.Prompt)
		}
	}
	return newCronAPIHandlerWithRunner(store, runJob, logger)
}

func newCronAPIHandlerWithRunner(
	store *cron.Store,
	runJob func(ctx context.Context, job cron.Job) (string, error),
	logger zerolog.Logger,
) http.Handler {
	baseWorkspaceDir := ""
	runHistoryLimit := 0
	if store != nil {
		baseWorkspaceDir = store.WorkspaceDir()
		runHistoryLimit = store.RunHistoryLimit()
	}
	resolver := newWorkspaceCronStoreResolver(baseWorkspaceDir, runHistoryLimit, store)
	return newCronAPIHandlerWithRunnerAndResolver(resolver, runJob, logger)
}

func newCronAPIHandlerWithRunnerAndResolver(
	resolver *workspaceCronStoreResolver,
	runJob func(ctx context.Context, job cron.Job) (string, error),
	logger zerolog.Logger,
) http.Handler {
	mux := http.NewServeMux()
	resolveStore := func(r *http.Request) (*cron.Store, string, error) {
		if resolver == nil {
			return nil, "", fmt.Errorf("cron store resolver is not configured")
		}
		reqStore, workspaceID, err := resolver.ResolveFromRequest(r)
		if err != nil {
			return nil, "", err
		}
		return reqStore, workspaceID, nil
	}

	mux.HandleFunc("/v1/cron/jobs", func(w http.ResponseWriter, r *http.Request) {
		reqStore, _, err := resolveStore(r)
		if err != nil {
			logger.Error().Err(err).Msg("resolve workspace cron store failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "resolve workspace failed"})
			return
		}
		if !requireMethod(w, r, http.MethodGet, http.MethodPost) {
			return
		}
		switch r.Method {
		case http.MethodGet:
			jobs, err := reqStore.List()
			if err != nil {
				logger.Error().Err(err).Msg("list cron jobs failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list cron jobs failed"})
				return
			}
			writeJSON(w, http.StatusOK, jobs)
		case http.MethodPost:
			var req struct {
				Name           string          `json:"name"`
				Prompt         string          `json:"prompt"`
				Schedule       string          `json:"schedule"`
				Enabled        *bool           `json:"enabled,omitempty"`
				SessionID      string          `json:"session_id,omitempty"`
				SessionTarget  string          `json:"session_target,omitempty"`
				WakeMode       string          `json:"wake_mode,omitempty"`
				DeliveryMode   string          `json:"delivery_mode,omitempty"`
				Payload        json.RawMessage `json:"payload,omitempty"`
				DeleteAfterRun *bool           `json:"delete_after_run,omitempty"`
			}
			if !decodeJSONBody(w, r, &req) {
				return
			}
			enabled := true
			hasEnable := false
			if req.Enabled != nil {
				enabled = *req.Enabled
				hasEnable = true
			}
			job, err := reqStore.CreateWithOptions(cron.CreateInput{
				Name:              req.Name,
				Prompt:            req.Prompt,
				Schedule:          req.Schedule,
				Enabled:           enabled,
				HasEnable:         hasEnable,
				SessionID:         req.SessionID,
				SessionTarget:     req.SessionTarget,
				WakeMode:          req.WakeMode,
				DeliveryMode:      req.DeliveryMode,
				Payload:           req.Payload,
				DeleteAfterRun:    req.DeleteAfterRun != nil && *req.DeleteAfterRun,
				HasDeleteAfterRun: req.DeleteAfterRun != nil,
			})
			if err != nil {
				if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "payload") {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
				logger.Error().Err(err).Msg("create cron job failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create cron job failed"})
				return
			}
			writeJSON(w, http.StatusOK, job)
		}
	})

	mux.HandleFunc("/v1/cron/jobs/", func(w http.ResponseWriter, r *http.Request) {
		reqStore, workspaceID, err := resolveStore(r)
		if err != nil {
			logger.Error().Err(err).Msg("resolve workspace cron store failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "resolve workspace failed"})
			return
		}
		pathRemainder := strings.TrimPrefix(r.URL.Path, "/v1/cron/jobs/")
		pathParts := strings.Split(pathRemainder, "/")
		if len(pathParts) < 1 || pathParts[0] == "" {
			http.NotFound(w, r)
			return
		}
		jobID := pathParts[0]
		if len(pathParts) == 1 {
			if !requireMethod(w, r, http.MethodGet, http.MethodPut, http.MethodDelete) {
				return
			}
			switch r.Method {
			case http.MethodGet:
				job, err := reqStore.Get(jobID)
				if err != nil {
					if strings.Contains(err.Error(), "job not found") {
						writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
						return
					}
					logger.Error().Err(err).Str("job_id", jobID).Msg("get cron job failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get cron job failed"})
					return
				}
				writeJSON(w, http.StatusOK, job)
			case http.MethodPut:
				var req struct {
					Name           *string          `json:"name,omitempty"`
					Prompt         *string          `json:"prompt,omitempty"`
					Schedule       *string          `json:"schedule,omitempty"`
					Enabled        *bool            `json:"enabled,omitempty"`
					SessionID      *string          `json:"session_id,omitempty"`
					SessionTarget  *string          `json:"session_target,omitempty"`
					WakeMode       *string          `json:"wake_mode,omitempty"`
					DeliveryMode   *string          `json:"delivery_mode,omitempty"`
					Payload        *json.RawMessage `json:"payload,omitempty"`
					DeleteAfterRun *bool            `json:"delete_after_run,omitempty"`
				}
				if !decodeJSONBody(w, r, &req) {
					return
				}
				job, err := reqStore.Update(jobID, cron.UpdateInput{
					Name:           req.Name,
					Prompt:         req.Prompt,
					Schedule:       req.Schedule,
					Enabled:        req.Enabled,
					SessionID:      req.SessionID,
					SessionTarget:  req.SessionTarget,
					WakeMode:       req.WakeMode,
					DeliveryMode:   req.DeliveryMode,
					Payload:        req.Payload,
					DeleteAfterRun: req.DeleteAfterRun,
				})
				if err != nil {
					switch {
					case strings.Contains(err.Error(), "job not found"):
						writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
					case strings.Contains(err.Error(), "required"), strings.Contains(err.Error(), "invalid"), strings.Contains(err.Error(), "payload"):
						writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					default:
						logger.Error().Err(err).Str("job_id", jobID).Msg("update cron job failed")
						writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update cron job failed"})
					}
					return
				}
				writeJSON(w, http.StatusOK, job)
			case http.MethodDelete:
				if err := reqStore.Delete(jobID); err != nil {
					if strings.Contains(err.Error(), "job not found") {
						writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
						return
					}
					logger.Error().Err(err).Str("job_id", jobID).Msg("delete cron job failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete cron job failed"})
					return
				}
				w.WriteHeader(http.StatusNoContent)
			}
			return
		}
		if len(pathParts) != 2 || pathParts[1] != "run" {
			if len(pathParts) == 2 && pathParts[1] == "runs" {
				if !requireMethod(w, r, http.MethodGet) {
					return
				}
				if _, err := reqStore.Get(jobID); err != nil {
					if strings.Contains(err.Error(), "job not found") {
						writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
						return
					}
					logger.Error().Err(err).Str("job_id", jobID).Msg("get cron job failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get cron job failed"})
					return
				}
				limit, ok := parsePositiveLimit(w, r, 50)
				if !ok {
					return
				}
				runs, err := reqStore.ListRuns(jobID, limit)
				if err != nil {
					logger.Error().Err(err).Str("job_id", jobID).Msg("list cron runs failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list cron runs failed"})
					return
				}
				writeJSON(w, http.StatusOK, runs)
				return
			}
			http.NotFound(w, r)
			return
		}
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if runJob == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cron runner is not configured"})
			return
		}
		job, err := reqStore.Get(jobID)
		if err != nil {
			if strings.Contains(err.Error(), "job not found") {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
				return
			}
			logger.Error().Err(err).Str("job_id", jobID).Msg("get cron job failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get cron job failed"})
			return
		}
		if !reqStore.TryStartRun(jobID) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "job is already running"})
			return
		}
		defer reqStore.FinishRun(jobID)

		runCtx := serverauth.WithWorkspaceID(r.Context(), workspaceID)
		response, err := runJob(runCtx, job)
		_, _ = reqStore.MarkRunResult(jobID, time.Now().UTC(), response, err)
		if err != nil {
			logger.Error().Err(err).Str("job_id", jobID).Msg("run cron job failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "run cron job failed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"job_id":         job.ID,
			"response":       response,
			"job_name":       job.Name,
			"job_prompt":     job.Prompt,
			"session_target": job.SessionTarget,
			"wake_mode":      job.WakeMode,
			"delivery_mode":  job.DeliveryMode,
		})
	})

	return mux
}
