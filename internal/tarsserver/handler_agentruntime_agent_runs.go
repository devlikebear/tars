package tarsserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/workstore"
	"github.com/rs/zerolog"
)

func newAgentRunsAPIHandler(runtime *agentruntime.Runtime, logger zerolog.Logger) http.Handler {
	return newAgentRunsAPIHandlerWithWorkLedgerAndInflightLimit(runtime, nil, logger, 4)
}

func newAgentRunsAPIHandlerWithInflightLimit(runtime *agentruntime.Runtime, logger zerolog.Logger, maxInflightAgentRuns int) http.Handler {
	return newAgentRunsAPIHandlerWithWorkLedgerAndInflightLimit(runtime, nil, logger, maxInflightAgentRuns)
}

func newAgentRunsAPIHandlerWithWorkLedger(runtime *agentruntime.Runtime, ledger *workstore.Store, logger zerolog.Logger) http.Handler {
	return newAgentRunsAPIHandlerWithWorkLedgerAndInflightLimit(runtime, ledger, logger, 4)
}

func newAgentRunsAPIHandlerWithWorkLedgerAndInflightLimit(runtime *agentruntime.Runtime, ledger *workstore.Store, logger zerolog.Logger, maxInflightAgentRuns int) http.Handler {
	inflight := newInflightLimiter(maxInflightAgentRuns, 4)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/agentruntime/agents", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		handleAgentList(w, runtime)
	})
	mux.HandleFunc("/v1/agentruntime/runs", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet, http.MethodPost) {
			return
		}
		if r.Method == http.MethodPost {
			handleAgentRunSpawn(w, r, runtime, inflight)
			return
		}
		handleAgentRunList(w, r, runtime, ledger, logger)
	})
	mux.HandleFunc("/v1/agentruntime/runs/", func(w http.ResponseWriter, r *http.Request) {
		handleAgentRunByID(w, r, runtime, ledger, logger)
	})
	return mux
}

func handleAgentList(w http.ResponseWriter, runtime *agentruntime.Runtime) {
	if runtime == nil {
		writeJSON(w, http.StatusOK, map[string]any{"count": 0, "agents": []map[string]any{}})
		return
	}
	agents := runtime.Agents()
	writeJSON(w, http.StatusOK, map[string]any{"count": len(agents), "agents": agents})
}

type agentRunSpawnRequest struct {
	SessionID string `json:"session_id"`
	TaskID    string `json:"task_id"`
	Title     string `json:"title"`
	Message   string `json:"message"`
	Prompt    string `json:"prompt"`
	Agent     string `json:"agent"`
}

type agentRunRestartRequest struct {
	CheckpointID     string                         `json:"checkpoint_id"`
	Agent            string                         `json:"agent"`
	Tier             string                         `json:"tier"`
	ProviderOverride *agentruntime.ProviderOverride `json:"provider_override"`
	PromptAdjustment string                         `json:"prompt_adjustment"`
	Title            string                         `json:"title"`
}

func handleAgentRunSpawn(w http.ResponseWriter, r *http.Request, runtime *agentruntime.Runtime, inflight *inflightLimiter) {
	if runtime == nil {
		writeUnavailable(w, "agent runtime is not configured")
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

	run, err := runtime.Spawn(r.Context(), agentruntime.SpawnRequest{
		WorkspaceID: defaultWorkspaceID,
		SessionID:   req.SessionID,
		TaskID:      req.TaskID,
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

func handleAgentRunList(w http.ResponseWriter, r *http.Request, runtime *agentruntime.Runtime, ledger *workstore.Store, logger zerolog.Logger) {
	limit, ok := parsePositiveLimit(w, r, 50)
	if !ok {
		return
	}
	filters, ok := parseAgentRunListFilters(w, r)
	if !ok {
		return
	}
	fetchLimit := limit
	if filters.active() {
		fetchLimit = max(fetchLimit, 1000)
	}
	runs, ledgerErr := mergedAgentRuntimeRuns(r, runtime, ledger, fetchLimit)
	if ledgerErr != nil {
		logger.Warn().Err(ledgerErr).Msg("read agent runtime work ledger projections failed")
		if runtime == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read agent runtime projections failed"})
			return
		}
	}
	runs = filterAgentRuntimeRuns(runs, filters)
	if len(runs) > limit {
		runs = runs[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(runs), "runs": runs})
}

func mergedAgentRuntimeRuns(r *http.Request, runtime *agentruntime.Runtime, ledger *workstore.Store, limit int) ([]agentruntime.Run, error) {
	runs := make([]agentruntime.Run, 0)
	seen := make(map[string]struct{})
	if runtime != nil {
		for _, run := range runtime.List(limit) {
			runs = append(runs, run)
			seen[run.ID] = struct{}{}
		}
	}
	if ledger == nil {
		return runs, nil
	}
	projections, err := ledger.ListLegacyAgentRuntimeRunProjections(r.Context(), workspaceIDFromRequest(r), limit)
	if err != nil {
		return runs, err
	}
	for _, projection := range projections {
		var run agentruntime.Run
		if err := json.Unmarshal(projection, &run); err != nil {
			return runs, fmt.Errorf("decode agent runtime work ledger projection: %w", err)
		}
		if _, exists := seen[run.ID]; exists {
			continue
		}
		runs = append(runs, run)
		seen[run.ID] = struct{}{}
	}
	return runs, nil
}

type agentRunListFilters struct {
	status string
	search string
	since  *time.Time
}

func (f agentRunListFilters) active() bool {
	return f.status != "" || f.search != "" || f.since != nil
}

func parseAgentRunListFilters(w http.ResponseWriter, r *http.Request) (agentRunListFilters, bool) {
	query := r.URL.Query()
	filters := agentRunListFilters{
		status: strings.ToLower(strings.TrimSpace(query.Get("status"))),
		search: strings.ToLower(strings.TrimSpace(query.Get("search"))),
	}
	if filters.status == "all" {
		filters.status = ""
	}

	since, ok := parseAgentRunSinceFilter(w, strings.TrimSpace(query.Get("since")))
	if !ok {
		return agentRunListFilters{}, false
	}
	filters.since = since
	return filters, true
}

func parseAgentRunSinceFilter(w http.ResponseWriter, raw string) (*time.Time, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" || value == "all" {
		return nil, true
	}
	now := time.Now().UTC()
	var cutoff time.Time
	switch value {
	case "24h":
		cutoff = now.Add(-24 * time.Hour)
	case "7d":
		cutoff = now.Add(-7 * 24 * time.Hour)
	default:
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "since must be 24h, 7d, all, or RFC3339"})
			return nil, false
		}
		cutoff = parsed.UTC()
	}
	return &cutoff, true
}

func filterAgentRuntimeRuns(runs []agentruntime.Run, filters agentRunListFilters) []agentruntime.Run {
	if !filters.active() {
		return runs
	}
	out := make([]agentruntime.Run, 0, len(runs))
	for _, run := range runs {
		if !agentRunMatchesStatus(run, filters.status) {
			continue
		}
		if filters.since != nil && agentRunTime(run).Before(*filters.since) {
			continue
		}
		if filters.search != "" && !agentRunMatchesSearch(run, filters.search) {
			continue
		}
		out = append(out, run)
	}
	return out
}

func agentRunMatchesStatus(run agentruntime.Run, status string) bool {
	if status == "" {
		return true
	}
	actual := strings.ToLower(strings.TrimSpace(string(run.Status)))
	switch status {
	case "running":
		return actual == string(agentruntime.RunStatusAccepted) || actual == string(agentruntime.RunStatusRunning)
	case "done", "completed":
		return actual == string(agentruntime.RunStatusCompleted)
	case "failed":
		return actual == string(agentruntime.RunStatusFailed) || actual == string(agentruntime.RunStatusCanceled)
	default:
		return actual == status
	}
}

func agentRunMatchesSearch(run agentruntime.Run, search string) bool {
	haystack := strings.ToLower(strings.Join([]string{
		run.ID,
		run.SessionID,
		run.Agent,
		run.Prompt,
		run.Response,
		run.Error,
		run.Tier,
		run.ResolvedAlias,
		run.ResolvedModel,
	}, "\n"))
	return strings.Contains(haystack, search)
}

func agentRunTime(run agentruntime.Run) time.Time {
	for _, value := range []string{run.CreatedAt, run.StartedAt, run.UpdatedAt, run.CompletedAt} {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func handleAgentRunByID(w http.ResponseWriter, r *http.Request, runtime *agentruntime.Runtime, ledger *workstore.Store, logger zerolog.Logger) {
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
		run, found, err := findAgentRuntimeRunProjection(r, runtime, ledger, runID)
		if err != nil {
			logger.Warn().Err(err).Str("run_id", runID).Msg("read agent runtime work ledger projection failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read agent runtime projection failed"})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
			return
		}
		writeJSON(w, http.StatusOK, run)
	case "cancel":
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if runtime == nil {
			writeUnavailable(w, "agent runtime is not configured")
			return
		}
		run, err := runtime.Cancel(runID)
		if err != nil {
			logger.Error().Err(err).Str("run_id", runID).Msg("cancel run failed")
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
			return
		}
		writeJSON(w, http.StatusOK, run)
	case "restart":
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		if runtime == nil {
			writeUnavailable(w, "agent runtime is not configured")
			return
		}
		var req agentRunRestartRequest
		if !decodeOptionalJSONBody(w, r, &req) {
			return
		}
		run, err := runtime.RestartFromCheckpoint(r.Context(), agentruntime.RestartRequest{
			WorkspaceID:      defaultWorkspaceID,
			RunID:            runID,
			CheckpointID:     req.CheckpointID,
			Agent:            req.Agent,
			Tier:             req.Tier,
			ProviderOverride: req.ProviderOverride,
			PromptAdjustment: req.PromptAdjustment,
			Title:            req.Title,
		})
		if err != nil {
			logger.Error().Err(err).Str("run_id", runID).Msg("restart run failed")
			writeJSON(w, restartErrorStatus(err), map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, run)
	case "events":
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if runtime == nil {
			writeUnavailable(w, "agent runtime is not configured")
			return
		}
		handleAgentRunEvents(w, r, runtime, runID)
	default:
		http.NotFound(w, r)
	}
}

func findAgentRuntimeRunProjection(r *http.Request, runtime *agentruntime.Runtime, ledger *workstore.Store, runID string) (agentruntime.Run, bool, error) {
	if runtime != nil {
		if run, found := runtime.Get(runID); found {
			return run, true, nil
		}
	}
	if ledger == nil {
		return agentruntime.Run{}, false, nil
	}
	projection, found, err := ledger.GetLegacyAgentRuntimeRunProjection(r.Context(), workspaceIDFromRequest(r), runID)
	if err != nil || !found {
		return agentruntime.Run{}, found, err
	}
	var run agentruntime.Run
	if err := json.Unmarshal(projection, &run); err != nil {
		return agentruntime.Run{}, false, fmt.Errorf("decode agent runtime work ledger projection: %w", err)
	}
	return run, true, nil
}

func handleAgentRunEvents(w http.ResponseWriter, r *http.Request, runtime *agentruntime.Runtime, runID string) {
	if _, found := runtime.Get(runID); !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming is not supported"})
		return
	}
	ch, unsubscribe := runtime.SubscribeRunEvents(runID)
	defer unsubscribe()
	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			if err := writeSSEData(w, event); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func writeSSEData(w http.ResponseWriter, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", data)
	return err
}

func parseAgentRunPath(path string) (runID string, action string, ok bool) {
	trimmed := strings.TrimSpace(path)
	trimmed = strings.TrimPrefix(trimmed, "/v1/agentruntime/runs/")
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

func restartErrorStatus(err error) int {
	if err == nil {
		return http.StatusAccepted
	}
	lower := strings.ToLower(strings.TrimSpace(err.Error()))
	if strings.Contains(lower, "not found") {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}
