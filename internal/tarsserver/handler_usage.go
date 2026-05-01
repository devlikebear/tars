package tarsserver

import (
	"net/http"
	"strconv"
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
		if !requireMethod(w, r, http.MethodGet) {
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
		if !requireMethod(w, r, http.MethodGet, http.MethodPatch) {
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
				DailyUSD    *float64 `json:"daily_usd,omitempty"`
				WeeklyUSD   *float64 `json:"weekly_usd,omitempty"`
				MonthlyUSD  *float64 `json:"monthly_usd,omitempty"`
				DailyTokens *int     `json:"daily_tokens,omitempty"`
				Mode        *string  `json:"mode,omitempty"`
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
			if req.DailyTokens != nil {
				next.DailyTokens = *req.DailyTokens
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
		}
	})

	mux.HandleFunc("/v1/admin/usage/today", func(w http.ResponseWriter, r *http.Request) {
		if tracker == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "usage tracker is not configured"})
			return
		}
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		today, err := tracker.TodayTokens()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "usage today failed"})
			return
		}
		writeJSON(w, http.StatusOK, today)
	})

	mux.HandleFunc("/v1/admin/analytics", func(w http.ResponseWriter, r *http.Request) {
		if tracker == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "usage tracker is not configured"})
			return
		}
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		days, ok := parseAnalyticsDays(w, r)
		if !ok {
			return
		}
		analytics, err := tracker.Analytics(days)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "analytics failed"})
			return
		}
		writeJSON(w, http.StatusOK, analytics)
	})

	mux.HandleFunc("/v1/usage/signals", func(w http.ResponseWriter, r *http.Request) {
		if tracker == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "usage tracker is not configured"})
			return
		}
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		period := strings.TrimSpace(r.URL.Query().Get("period"))
		signals, err := tracker.Signals(period)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"signals": signals,
		})
	})

	return mux
}

func parseAnalyticsDays(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("days"))
	if raw == "" {
		return 7, true
	}
	days, err := strconv.Atoi(raw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "days must be one of 7, 30, or 90"})
		return 0, false
	}
	switch days {
	case 7, 30, 90:
		return days, true
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "days must be one of 7, 30, or 90"})
		return 0, false
	}
}
