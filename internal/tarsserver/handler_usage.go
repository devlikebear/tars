package tarsserver

import (
	"net/http"
	"strings"

	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/usage"
	"github.com/rs/zerolog"
)

func newUsageAPIHandler(tracker *usage.Tracker, authMode string, logger zerolog.Logger) http.Handler {
	normalizedAuthMode := serverauth.NormalizeMode(strings.TrimSpace(strings.ToLower(authMode)))
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/usage/summary", func(w http.ResponseWriter, r *http.Request) {
		if tracker == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "usage tracker is not configured"})
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		period := strings.TrimSpace(r.URL.Query().Get("period"))
		groupBy := strings.TrimSpace(r.URL.Query().Get("group_by"))
		summary, err := tracker.Summary(period, groupBy)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		status, _ := tracker.CheckLimitStatus()
		writeJSON(w, http.StatusOK, map[string]any{
			"summary":      summary,
			"limits":       tracker.Limits(),
			"limit_status": status,
		})
	})

	mux.HandleFunc("/v1/usage/limits", func(w http.ResponseWriter, r *http.Request) {
		if tracker == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "usage tracker is not configured"})
			return
		}
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, tracker.Limits())
		case http.MethodPatch:
			if normalizedAuthMode != serverauth.ModeOff && strings.TrimSpace(serverauth.RoleFromContext(r.Context())) != serverauth.RoleAdmin {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
				return
			}
			var req struct {
				DailyUSD   *float64 `json:"daily_usd,omitempty"`
				WeeklyUSD  *float64 `json:"weekly_usd,omitempty"`
				MonthlyUSD *float64 `json:"monthly_usd,omitempty"`
				Mode       *string  `json:"mode,omitempty"`
			}
			if !decodeJSONBody(w, r, &req) {
				return
			}
			next := tracker.Limits()
			if req.DailyUSD != nil {
				next.DailyUSD = *req.DailyUSD
			}
			if req.WeeklyUSD != nil {
				next.WeeklyUSD = *req.WeeklyUSD
			}
			if req.MonthlyUSD != nil {
				next.MonthlyUSD = *req.MonthlyUSD
			}
			if req.Mode != nil {
				next.Mode = strings.TrimSpace(strings.ToLower(*req.Mode))
			}
			updated, err := tracker.UpdateLimits(next)
			if err != nil {
				logger.Error().Err(err).Msg("update usage limits failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update usage limits failed"})
				return
			}
			writeJSON(w, http.StatusOK, updated)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	return mux
}
