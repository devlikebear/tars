package tarsserver

import (
	"net/http"
	"strings"

	"github.com/devlikebear/tarsncase/internal/gateway"
	"github.com/rs/zerolog"
)

func newGatewayAPIHandler(runtime *gateway.Runtime, logger zerolog.Logger, reloadHook func()) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/gateway/status", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if runtime == nil {
			writeJSON(w, http.StatusOK, gateway.GatewayStatus{Enabled: false})
			return
		}
		writeJSON(w, http.StatusOK, runtime.Status())
	})
	mux.HandleFunc("/v1/gateway/reload", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if runtime == nil {
			writeUnavailable(w, "gateway runtime is not configured")
			return
		}
		if reloadHook != nil {
			reloadHook()
		}
		writeJSON(w, http.StatusOK, runtime.Reload())
	})
	mux.HandleFunc("/v1/gateway/restart", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if runtime == nil {
			writeUnavailable(w, "gateway runtime is not configured")
			return
		}
		status := runtime.Restart()
		logger.Info().Msg("gateway runtime restarted")
		writeJSON(w, http.StatusOK, status)
	})
	mux.HandleFunc("/v1/gateway/reports/summary", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		handleGatewaySummaryReport(w, runtime)
	})
	mux.HandleFunc("/v1/gateway/reports/runs", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		handleGatewayDetailedReport(w, r, runtime, func(limit int) (any, error) {
			return runtime.ReportsRuns(limit)
		})
	})
	mux.HandleFunc("/v1/gateway/reports/channels", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		handleGatewayDetailedReport(w, r, runtime, func(limit int) (any, error) {
			return runtime.ReportsChannels(limit)
		})
	})
	return mux
}

func handleGatewaySummaryReport(w http.ResponseWriter, runtime *gateway.Runtime) {
	if runtime == nil {
		writeUnavailable(w, "gateway runtime is not configured")
		return
	}
	report, err := runtime.ReportsSummary()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	if !report.SummaryEnabled {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "gateway summary report is disabled"})
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func handleGatewayDetailedReport(
	w http.ResponseWriter,
	r *http.Request,
	runtime *gateway.Runtime,
	fetch func(limit int) (any, error),
) {
	if runtime == nil {
		writeUnavailable(w, "gateway runtime is not configured")
		return
	}
	limit, ok := parsePositiveLimit(w, r, 50)
	if !ok {
		return
	}
	report, err := fetch(limit)
	if err != nil {
		writeJSON(w, gatewayReportErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func gatewayReportErrorStatus(err error) int {
	if strings.Contains(strings.ToLower(err.Error()), "disabled") {
		return http.StatusNotFound
	}
	return http.StatusServiceUnavailable
}
