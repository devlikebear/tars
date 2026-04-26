package tarsserver

import (
	"net/http"
	"strings"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/rs/zerolog"
)

func newAgentRuntimeAPIHandler(runtime *agentruntime.Runtime, logger zerolog.Logger, reloadHook func()) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/agentruntime/status", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if runtime == nil {
			writeJSON(w, http.StatusOK, agentruntime.AgentRuntimeStatus{Enabled: false})
			return
		}
		writeJSON(w, http.StatusOK, runtime.Status())
	})
	mux.HandleFunc("/v1/agentruntime/reload", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if runtime == nil {
			writeUnavailable(w, "agent runtime is not configured")
			return
		}
		if reloadHook != nil {
			reloadHook()
		}
		writeJSON(w, http.StatusOK, runtime.Reload())
	})
	mux.HandleFunc("/v1/agentruntime/restart", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if runtime == nil {
			writeUnavailable(w, "agent runtime is not configured")
			return
		}
		status := runtime.Restart()
		logger.Info().Msg("agent runtime restarted")
		writeJSON(w, http.StatusOK, status)
	})
	mux.HandleFunc("/v1/agentruntime/reports/summary", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		handleAgentRuntimeSummaryReport(w, runtime)
	})
	mux.HandleFunc("/v1/agentruntime/reports/runs", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		handleAgentRuntimeDetailedReport(w, r, runtime, func(limit int) (any, error) {
			return runtime.ReportsRuns(limit)
		})
	})
	mux.HandleFunc("/v1/agentruntime/reports/channels", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		handleAgentRuntimeDetailedReport(w, r, runtime, func(limit int) (any, error) {
			return runtime.ReportsChannels(limit)
		})
	})
	return mux
}

func handleAgentRuntimeSummaryReport(w http.ResponseWriter, runtime *agentruntime.Runtime) {
	if runtime == nil {
		writeUnavailable(w, "agent runtime is not configured")
		return
	}
	report, err := runtime.ReportsSummary()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	if !report.SummaryEnabled {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent runtime summary report is disabled"})
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func handleAgentRuntimeDetailedReport(
	w http.ResponseWriter,
	r *http.Request,
	runtime *agentruntime.Runtime,
	fetch func(limit int) (any, error),
) {
	if runtime == nil {
		writeUnavailable(w, "agent runtime is not configured")
		return
	}
	limit, ok := parsePositiveLimit(w, r, 50)
	if !ok {
		return
	}
	report, err := fetch(limit)
	if err != nil {
		writeJSON(w, agentRuntimeReportErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func agentRuntimeReportErrorStatus(err error) int {
	if strings.Contains(strings.ToLower(err.Error()), "disabled") {
		return http.StatusNotFound
	}
	return http.StatusServiceUnavailable
}
