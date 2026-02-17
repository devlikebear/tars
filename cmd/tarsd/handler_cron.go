package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/cron"
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
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/cron/jobs", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			jobs, err := store.List()
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
				SessionTarget  string          `json:"session_target,omitempty"`
				WakeMode       string          `json:"wake_mode,omitempty"`
				DeliveryMode   string          `json:"delivery_mode,omitempty"`
				Payload        json.RawMessage `json:"payload,omitempty"`
				DeleteAfterRun *bool           `json:"delete_after_run,omitempty"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
				return
			}
			enabled := true
			hasEnable := false
			if req.Enabled != nil {
				enabled = *req.Enabled
				hasEnable = true
			}
			job, err := store.CreateWithOptions(cron.CreateInput{
				Name:              req.Name,
				Prompt:            req.Prompt,
				Schedule:          req.Schedule,
				Enabled:           enabled,
				HasEnable:         hasEnable,
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
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/v1/cron/jobs/", func(w http.ResponseWriter, r *http.Request) {
		pathRemainder := strings.TrimPrefix(r.URL.Path, "/v1/cron/jobs/")
		pathParts := strings.Split(pathRemainder, "/")
		if len(pathParts) < 1 || pathParts[0] == "" {
			http.NotFound(w, r)
			return
		}
		jobID := pathParts[0]
		if len(pathParts) == 1 {
			switch r.Method {
			case http.MethodGet:
				job, err := store.Get(jobID)
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
					SessionTarget  *string          `json:"session_target,omitempty"`
					WakeMode       *string          `json:"wake_mode,omitempty"`
					DeliveryMode   *string          `json:"delivery_mode,omitempty"`
					Payload        *json.RawMessage `json:"payload,omitempty"`
					DeleteAfterRun *bool            `json:"delete_after_run,omitempty"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
					return
				}
				job, err := store.Update(jobID, cron.UpdateInput{
					Name:           req.Name,
					Prompt:         req.Prompt,
					Schedule:       req.Schedule,
					Enabled:        req.Enabled,
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
				if err := store.Delete(jobID); err != nil {
					if strings.Contains(err.Error(), "job not found") {
						writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
						return
					}
					logger.Error().Err(err).Str("job_id", jobID).Msg("delete cron job failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete cron job failed"})
					return
				}
				w.WriteHeader(http.StatusNoContent)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}
		if len(pathParts) != 2 || pathParts[1] != "run" {
			if len(pathParts) == 2 && pathParts[1] == "runs" {
				if r.Method != http.MethodGet {
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				if _, err := store.Get(jobID); err != nil {
					if strings.Contains(err.Error(), "job not found") {
						writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
						return
					}
					logger.Error().Err(err).Str("job_id", jobID).Msg("get cron job failed")
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get cron job failed"})
					return
				}
				limit := 50
				if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
					v, err := strconv.Atoi(raw)
					if err != nil || v <= 0 {
						writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be a positive integer"})
						return
					}
					limit = v
				}
				runs, err := store.ListRuns(jobID, limit)
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
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if runJob == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cron runner is not configured"})
			return
		}
		job, err := store.Get(jobID)
		if err != nil {
			if strings.Contains(err.Error(), "job not found") {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
				return
			}
			logger.Error().Err(err).Str("job_id", jobID).Msg("get cron job failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get cron job failed"})
			return
		}
		if !store.TryStartRun(jobID) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "job is already running"})
			return
		}
		defer store.FinishRun(jobID)

		response, err := runJob(r.Context(), job)
		_, _ = store.MarkRunResult(jobID, time.Now().UTC(), response, err)
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
