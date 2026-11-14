package tarsserver

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/devlikebear/tars/internal/focuspipeline"
)

// Phase 3 of docs/decisions/mobile-remote.md: how much is answered from a
// phone. Every gate and card answer records where it came from
// (focusAnswerSource, AnswerGoal for goal mode); this reads them back.
const (
	focusAnswerReportDays    = 30
	focusAnswerReportMaxDays = 365
)

// answerReport serves GET /v1/focus/answer-report?days=N: gates and cards
// answered on a phone, at the desk and by goal mode over the last N days
// (30 by default), from the pipelines' own cards. A read; no model.
func (a *focusAPI) answerReport(w http.ResponseWriter, r *http.Request) {
	days := focusAnswerReportDays
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > focusAnswerReportMaxDays {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("days must be 1 to %d", focusAnswerReportMaxDays)})
			return
		}
		days = n
	}
	pipelines, err := a.store().List()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read focus pipelines"})
		return
	}
	since := a.now().UTC().AddDate(0, 0, -days)
	writeJSON(w, http.StatusOK, map[string]any{
		"days":   days,
		"report": focuspipeline.AnswerSummary(pipelines, since),
	})
}
