package tarsdapp

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/devlikebear/tarsncase/internal/gateway"
	"github.com/rs/zerolog"
)

func newAgentRunsAPIHandler(runtime *gateway.Runtime, logger zerolog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/agent/agents", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if runtime == nil {
			writeJSON(w, http.StatusOK, map[string]any{"count": 0, "agents": []map[string]any{}})
			return
		}
		agents := runtime.Agents()
		writeJSON(w, http.StatusOK, map[string]any{"count": len(agents), "agents": agents})
	})
	mux.HandleFunc("/v1/agent/runs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.Method == http.MethodPost {
			if runtime == nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "gateway runtime is not configured"})
				return
			}
			var req struct {
				SessionID string `json:"session_id"`
				Title     string `json:"title"`
				Message   string `json:"message"`
				Prompt    string `json:"prompt"`
				Agent     string `json:"agent"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
				return
			}
			message := strings.TrimSpace(req.Message)
			if message == "" {
				message = strings.TrimSpace(req.Prompt)
			}
			if message == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
				return
			}
			run, err := runtime.Spawn(r.Context(), gateway.SpawnRequest{
				WorkspaceID: defaultWorkspaceID,
				SessionID:   req.SessionID,
				Title:       req.Title,
				Prompt:      message,
				Agent:       req.Agent,
			})
			if err != nil {
				status := http.StatusBadRequest
				if strings.Contains(strings.ToLower(err.Error()), "not found") {
					status = http.StatusNotFound
				}
				writeJSON(w, status, map[string]string{
					"error": err.Error(),
					"code":  classifySpawnErrorCode(err),
				})
				return
			}
			writeJSON(w, http.StatusAccepted, run)
			return
		}
		if runtime == nil {
			writeJSON(w, http.StatusOK, map[string]any{"count": 0, "runs": []gateway.Run{}})
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
		runs := runtime.List(limit)
		writeJSON(w, http.StatusOK, map[string]any{"count": len(runs), "runs": runs})
	})
	mux.HandleFunc("/v1/agent/runs/", func(w http.ResponseWriter, r *http.Request) {
		if runtime == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "gateway runtime is not configured"})
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/v1/agent/runs/")
		path = strings.TrimSpace(path)
		if path == "" {
			http.NotFound(w, r)
			return
		}
		parts := strings.Split(path, "/")
		runID := strings.TrimSpace(parts[0])
		if runID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run_id is required"})
			return
		}
		if len(parts) == 1 {
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			run, ok := runtime.Get(runID)
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
				return
			}
			writeJSON(w, http.StatusOK, run)
			return
		}
		if len(parts) == 2 && parts[1] == "cancel" {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			run, err := runtime.Cancel(runID)
			if err != nil {
				logger.Error().Err(err).Str("run_id", runID).Msg("cancel run failed")
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
				return
			}
			writeJSON(w, http.StatusOK, run)
			return
		}
		http.NotFound(w, r)
	})
	return mux
}

func newGatewayAPIHandler(runtime *gateway.Runtime, logger zerolog.Logger, reloadHook func()) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/gateway/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if runtime == nil {
			writeJSON(w, http.StatusOK, gateway.GatewayStatus{Enabled: false})
			return
		}
		writeJSON(w, http.StatusOK, runtime.Status())
	})
	mux.HandleFunc("/v1/gateway/reload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if runtime == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "gateway runtime is not configured"})
			return
		}
		if reloadHook != nil {
			reloadHook()
		}
		status := runtime.Reload()
		writeJSON(w, http.StatusOK, status)
	})
	mux.HandleFunc("/v1/gateway/restart", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if runtime == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "gateway runtime is not configured"})
			return
		}
		status := runtime.Restart()
		logger.Info().Msg("gateway runtime restarted")
		writeJSON(w, http.StatusOK, status)
	})
	mux.HandleFunc("/v1/gateway/reports/summary", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if runtime == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "gateway runtime is not configured"})
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
	})
	mux.HandleFunc("/v1/gateway/reports/runs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if runtime == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "gateway runtime is not configured"})
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
		report, err := runtime.ReportsRuns(limit)
		if err != nil {
			status := http.StatusServiceUnavailable
			if strings.Contains(strings.ToLower(err.Error()), "disabled") {
				status = http.StatusNotFound
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, report)
	})
	mux.HandleFunc("/v1/gateway/reports/channels", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if runtime == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "gateway runtime is not configured"})
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
		report, err := runtime.ReportsChannels(limit)
		if err != nil {
			status := http.StatusServiceUnavailable
			if strings.Contains(strings.ToLower(err.Error()), "disabled") {
				status = http.StatusNotFound
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, report)
	})
	return mux
}

func newChannelsAPIHandler(runtime *gateway.Runtime, logger zerolog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/channels/webhook/inbound/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if runtime == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "gateway runtime is not configured"})
			return
		}
		channelID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/v1/channels/webhook/inbound/"))
		if channelID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "channel_id is required"})
			return
		}
		payload := map[string]any{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil && err != io.EOF {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		text := extractInboundText(payload)
		threadID := strings.TrimSpace(asString(payload["thread_id"]))
		msg, err := runtime.InboundWebhook(channelID, threadID, text, payload)
		if err != nil {
			logger.Error().Err(err).Str("channel_id", channelID).Msg("webhook inbound failed")
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, msg)
	})
	mux.HandleFunc("/v1/channels/telegram/webhook/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if runtime == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "gateway runtime is not configured"})
			return
		}
		botID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/v1/channels/telegram/webhook/"))
		if botID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bot_id is required"})
			return
		}
		payload := map[string]any{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil && err != io.EOF {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		text := extractInboundText(payload)
		threadID := strings.TrimSpace(asString(payload["thread_id"]))
		msg, err := runtime.InboundTelegram(botID, threadID, text, payload)
		if err != nil {
			logger.Error().Err(err).Str("bot_id", botID).Msg("telegram inbound failed")
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, msg)
	})
	return mux
}

func extractInboundText(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}
	if v := strings.TrimSpace(asString(payload["text"])); v != "" {
		return v
	}
	if msg, ok := payload["message"].(map[string]any); ok {
		if v := strings.TrimSpace(asString(msg["text"])); v != "" {
			return v
		}
	}
	return ""
}

func asString(v any) string {
	switch value := v.(type) {
	case string:
		return value
	default:
		return ""
	}
}

func classifySpawnErrorCode(err error) string {
	if err == nil {
		return ""
	}
	lower := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(lower, "unknown agent"):
		return "agent_not_found"
	case strings.Contains(lower, "prompt is required"), strings.Contains(lower, "message is required"):
		return "validation_error"
	case strings.Contains(lower, "session routing"), strings.Contains(lower, "session_fixed_id"):
		return "agent_policy_invalid"
	case strings.Contains(lower, "session store"):
		return "runtime_not_configured"
	default:
		return "spawn_failed"
	}
}
