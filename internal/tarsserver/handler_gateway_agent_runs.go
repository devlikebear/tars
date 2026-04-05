package tarsserver

import (
	"net/http"
	"strings"

	"github.com/devlikebear/tars/internal/gateway"
	"github.com/rs/zerolog"
)

func newAgentRunsAPIHandler(runtime *gateway.Runtime, logger zerolog.Logger) http.Handler {
	return newAgentRunsAPIHandlerWithInflightLimit(runtime, logger, 4)
}

func newAgentRunsAPIHandlerWithInflightLimit(runtime *gateway.Runtime, logger zerolog.Logger, maxInflightAgentRuns int) http.Handler {
	inflight := newInflightLimiter(maxInflightAgentRuns, 4)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/agent/agents", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		handleAgentList(w, runtime)
	})
	mux.HandleFunc("/v1/agent/runs", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet, http.MethodPost) {
			return
		}
		if r.Method == http.MethodPost {
			handleAgentRunSpawn(w, r, runtime, inflight)
			return
		}
		handleAgentRunList(w, r, runtime)
	})
	mux.HandleFunc("/v1/agent/runs/", func(w http.ResponseWriter, r *http.Request) {
		handleAgentRunByID(w, r, runtime, logger)
	})
	return mux
}

func handleAgentList(w http.ResponseWriter, runtime *gateway.Runtime) {
	if runtime == nil {
		writeJSON(w, http.StatusOK, map[string]any{"count": 0, "agents": []map[string]any{}})
		return
	}
	agents := runtime.Agents()
	writeJSON(w, http.StatusOK, map[string]any{"count": len(agents), "agents": agents})
}

type agentRunSpawnRequest struct {
	SessionID string `json:"session_id"`
	Title     string `json:"title"`
	Message   string `json:"message"`
	Prompt    string `json:"prompt"`
	Agent     string `json:"agent"`
}

func handleAgentRunSpawn(w http.ResponseWriter, r *http.Request, runtime *gateway.Runtime, inflight *inflightLimiter) {
	if runtime == nil {
		writeUnavailable(w, "gateway runtime is not configured")
		return
	}
	release, ok := inflight.tryAcquire()
	if !ok {
		writeError(w, http.StatusTooManyRequests, "overloaded", "overloaded")
		return
	}
	defer release()

	var req agentRunSpawnRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	message := agentRunPrompt(req)
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
		writeJSON(w, spawnErrorStatus(err), map[string]string{
			"error": err.Error(),
			"code":  classifySpawnErrorCode(err),
		})
		return
	}
	writeJSON(w, http.StatusAccepted, run)
}

func agentRunPrompt(req agentRunSpawnRequest) string {
	message := strings.TrimSpace(req.Message)
	if message != "" {
		return message
	}
	return strings.TrimSpace(req.Prompt)
}

func spawnErrorStatus(err error) int {
	if strings.Contains(strings.ToLower(err.Error()), "not found") {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}

func handleAgentRunList(w http.ResponseWriter, r *http.Request, runtime *gateway.Runtime) {
	if runtime == nil {
		writeJSON(w, http.StatusOK, map[string]any{"count": 0, "runs": []gateway.Run{}})
		return
	}
	limit, ok := parsePositiveLimit(w, r, 50)
	if !ok {
		return
	}
	runs := runtime.List(limit)
	writeJSON(w, http.StatusOK, map[string]any{"count": len(runs), "runs": runs})
}

func handleAgentRunByID(w http.ResponseWriter, r *http.Request, runtime *gateway.Runtime, logger zerolog.Logger) {
	if runtime == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "gateway runtime is not configured"})
		return
	}
	runID, action, ok := parseAgentRunPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if runID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run_id is required"})
		return
	}
	switch action {
	case "":
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		run, found := runtime.Get(runID)
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
			return
		}
		writeJSON(w, http.StatusOK, run)
	case "cancel":
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		run, err := runtime.Cancel(runID)
		if err != nil {
			logger.Error().Err(err).Str("run_id", runID).Msg("cancel run failed")
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
			return
		}
		writeJSON(w, http.StatusOK, run)
	default:
		http.NotFound(w, r)
	}
}

func parseAgentRunPath(path string) (runID string, action string, ok bool) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(path, "/v1/agent/runs/"))
	if trimmed == "" {
		return "", "", false
	}
	parts := strings.Split(trimmed, "/")
	runID = strings.TrimSpace(parts[0])
	if len(parts) == 1 {
		return runID, "", true
	}
	if len(parts) == 2 {
		return runID, strings.TrimSpace(parts[1]), true
	}
	return "", "", false
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
