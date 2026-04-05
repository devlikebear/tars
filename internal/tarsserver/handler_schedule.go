package tarsserver

import (
	"net/http"
	"strings"

	"github.com/devlikebear/tars/internal/schedule"
	"github.com/rs/zerolog"
)

func newScheduleAPIHandler(store *schedule.Store, logger zerolog.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/schedules", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "schedule store is not configured"})
			return
		}
		if !requireMethod(w, r, http.MethodGet, http.MethodPost) {
			return
		}
		switch r.Method {
		case http.MethodGet:
			items, err := store.List()
			if err != nil {
				logger.Error().Err(err).Msg("list schedules failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list schedules failed"})
				return
			}
			writeJSON(w, http.StatusOK, items)
		case http.MethodPost:
			var req struct {
				Natural  string `json:"natural"`
				Title    string `json:"title,omitempty"`
				Prompt   string `json:"prompt,omitempty"`
				Schedule string `json:"schedule,omitempty"`
				Timezone string `json:"timezone,omitempty"`
			}
			if !decodeJSONBody(w, r, &req) {
				return
			}
			created, err := store.Create(schedule.CreateInput{
				Natural:  req.Natural,
				Title:    req.Title,
				Prompt:   req.Prompt,
				Schedule: req.Schedule,
				Timezone: req.Timezone,
			})
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, created)
		}
	})

	mux.HandleFunc("/v1/schedules/", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "schedule store is not configured"})
			return
		}
		id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/v1/schedules/"))
		if id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}
		if !requireMethod(w, r, http.MethodPatch, http.MethodDelete) {
			return
		}
		switch r.Method {
		case http.MethodPatch:
			var req struct {
				Title    *string `json:"title,omitempty"`
				Prompt   *string `json:"prompt,omitempty"`
				Schedule *string `json:"schedule,omitempty"`
				Status   *string `json:"status,omitempty"`
				Timezone *string `json:"timezone,omitempty"`
			}
			if !decodeJSONBody(w, r, &req) {
				return
			}
			updated, err := store.Update(id, schedule.UpdateInput{
				Title:    req.Title,
				Prompt:   req.Prompt,
				Schedule: req.Schedule,
				Status:   req.Status,
				Timezone: req.Timezone,
			})
			if err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "not found") {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule not found"})
					return
				}
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, updated)
		case http.MethodDelete:
			if err := store.Delete(id); err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "not found") {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule not found"})
					return
				}
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}
	})

	return mux
}
