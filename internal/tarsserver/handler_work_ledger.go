package tarsserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/workscheduler"
	"github.com/devlikebear/tars/internal/workstore"
	"github.com/rs/zerolog"
)

const (
	defaultWorkLedgerListLimit = 100
	maxWorkLedgerListLimit     = 1000
)

func newWorkLedgerAPIHandler(store *workstore.Store, logger zerolog.Logger, schedulers ...*workscheduler.Scheduler) http.Handler {
	var scheduler *workscheduler.Scheduler
	if len(schedulers) > 0 {
		scheduler = schedulers[0]
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeUnavailable(w, "work ledger is not configured")
			return
		}

		workspaceID := normalizeWorkspaceID(serverauth.WorkspaceIDFromContext(r.Context()))
		path := strings.TrimSuffix(r.URL.Path, "/")
		switch {
		case path == "/v1/work/works":
			if !requireMethod(w, r, http.MethodGet) {
				return
			}
			handleWorkLedgerList(w, r, store, workspaceID, logger)
		case strings.HasPrefix(path, "/v1/work/works/") && strings.HasSuffix(path, "/timeline"):
			if !requireMethod(w, r, http.MethodGet) {
				return
			}
			handleWorkLedgerTimeline(w, r, store, workspaceID, path, logger)
		case strings.HasPrefix(path, "/v1/work/works/") && strings.HasSuffix(path, "/wait"):
			if !requireMethod(w, r, http.MethodGet) {
				return
			}
			handleWorkLedgerWait(w, r, store, scheduler, workspaceID, path, logger)
		case strings.HasPrefix(path, "/v1/work/works/") && strings.HasSuffix(path, "/watch"):
			if !requireMethod(w, r, http.MethodGet) {
				return
			}
			handleWorkLedgerWatch(w, r, store, scheduler, workspaceID, path, logger)
		case strings.HasPrefix(path, "/v1/work/legacy/sessions/"):
			if !requireMethod(w, r, http.MethodGet) {
				return
			}
			handleWorkLedgerLegacyTasks(w, r, store, workspaceID, path, logger)
		case strings.HasPrefix(path, "/v1/admin/work/works/") && strings.HasSuffix(path, "/cancel"):
			if !requireMethod(w, r, http.MethodPost) {
				return
			}
			handleWorkLedgerCancel(w, r, scheduler, workspaceID, path, logger)
		case strings.HasPrefix(path, "/v1/admin/work/works/") && strings.HasSuffix(path, "/resume"):
			if !requireMethod(w, r, http.MethodPost) {
				return
			}
			handleWorkLedgerResume(w, r, scheduler, workspaceID, path, logger)
		default:
			http.NotFound(w, r)
		}
	})
}

func handleWorkLedgerWait(w http.ResponseWriter, r *http.Request, store *workstore.Store, scheduler *workscheduler.Scheduler, workspaceID, path string, logger zerolog.Logger) {
	if !requireWorkScheduler(w, scheduler, workspaceID) {
		return
	}
	workID, ok := workIDFromActionPath(w, path, "/v1/work/works/", "/wait")
	if !ok {
		return
	}
	timeout, err := boundedQueryDuration(r.URL.Query(), "timeout_ms", 30000, 300000)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	waitCtx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	projection, err := scheduler.Wait(waitCtx, workID)
	if errors.Is(err, context.DeadlineExceeded) {
		projection, err = store.GetWorkProjection(r.Context(), workspaceID, workID)
		if err == nil {
			normalizeWorkProjectionSlices(&projection)
			writeJSON(w, http.StatusAccepted, map[string]any{"terminal": false, "projection": projection})
			return
		}
	}
	if errors.Is(err, workstore.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "work not found"})
		return
	}
	if err != nil {
		logger.Error().Err(err).Str("work_id", workID).Msg("wait for durable work failed")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "wait for durable work failed"})
		return
	}
	normalizeWorkProjectionSlices(&projection)
	writeJSON(w, http.StatusOK, projection)
}

func handleWorkLedgerWatch(w http.ResponseWriter, r *http.Request, store *workstore.Store, scheduler *workscheduler.Scheduler, workspaceID, path string, logger zerolog.Logger) {
	if !requireWorkScheduler(w, scheduler, workspaceID) {
		return
	}
	workID, ok := workIDFromActionPath(w, path, "/v1/work/works/", "/watch")
	if !ok {
		return
	}
	if _, err := store.GetWork(r.Context(), workspaceID, workID); errors.Is(err, workstore.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "work not found"})
		return
	} else if err != nil {
		logger.Error().Err(err).Str("work_id", workID).Msg("validate watched durable work failed")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "validate watched durable work failed"})
		return
	}
	afterSequence := int64(0)
	if raw := strings.TrimSpace(r.URL.Query().Get("after_sequence")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "after_sequence must be a non-negative integer"})
			return
		}
		afterSequence = parsed
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming is unavailable"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	events, errs := scheduler.Watch(r.Context(), workID, afterSequence)
	for event := range events {
		raw, err := json.Marshal(event)
		if err != nil {
			logger.Error().Err(err).Str("work_id", workID).Msg("encode durable work event failed")
			return
		}
		_, _ = fmt.Fprintf(w, "event: work_event\ndata: %s\n\n", raw)
		flusher.Flush()
	}
	if err := <-errs; err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, workscheduler.ErrClosed) {
		logger.Error().Err(err).Str("workspace_id", workspaceID).Str("work_id", workID).Msg("watch durable work failed")
	}
}

func handleWorkLedgerCancel(w http.ResponseWriter, r *http.Request, scheduler *workscheduler.Scheduler, workspaceID, path string, logger zerolog.Logger) {
	if !requireWorkScheduler(w, scheduler, workspaceID) {
		return
	}
	workID, ok := workIDFromActionPath(w, path, "/v1/admin/work/works/", "/cancel")
	if !ok {
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&body); err != nil || strings.TrimSpace(body.Reason) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reason is required"})
		return
	}
	projection, err := scheduler.Cancel(r.Context(), workID, workAPIActor(r), strings.TrimSpace(body.Reason))
	if errors.Is(err, workstore.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "work not found"})
		return
	}
	if err != nil {
		logger.Error().Err(err).Str("work_id", workID).Msg("cancel durable work failed")
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	normalizeWorkProjectionSlices(&projection)
	writeJSON(w, http.StatusOK, projection)
}

func handleWorkLedgerResume(w http.ResponseWriter, r *http.Request, scheduler *workscheduler.Scheduler, workspaceID, path string, logger zerolog.Logger) {
	if !requireWorkScheduler(w, scheduler, workspaceID) {
		return
	}
	remainder := strings.TrimSuffix(strings.TrimPrefix(path, "/v1/admin/work/works/"), "/resume")
	parts := strings.Split(strings.Trim(remainder, "/"), "/steps/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	workID, err := url.PathUnescape(parts[0])
	if err != nil || strings.TrimSpace(workID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid work id"})
		return
	}
	stepID, err := url.PathUnescape(parts[1])
	if err != nil || strings.TrimSpace(stepID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid step id"})
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&body); err != nil || strings.TrimSpace(body.Reason) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reason is required"})
		return
	}
	projection, err := scheduler.Resume(r.Context(), workID, stepID, workAPIActor(r), strings.TrimSpace(body.Reason))
	if errors.Is(err, workstore.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "work or step not found"})
		return
	}
	if err != nil {
		logger.Error().Err(err).Str("workspace_id", workspaceID).Str("work_id", workID).Str("step_id", stepID).Msg("resume durable work failed")
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	normalizeWorkProjectionSlices(&projection)
	writeJSON(w, http.StatusOK, projection)
}

func requireWorkScheduler(w http.ResponseWriter, scheduler *workscheduler.Scheduler, workspaceID string) bool {
	if scheduler == nil {
		writeUnavailable(w, "work scheduler is not configured")
		return false
	}
	if scheduler.WorkspaceID() != workspaceID {
		writeUnavailable(w, "work scheduler is not configured for this workspace")
		return false
	}
	return true
}

func workIDFromActionPath(w http.ResponseWriter, path, prefix, suffix string) (string, bool) {
	remainder := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	if strings.Contains(strings.Trim(remainder, "/"), "/") {
		http.Error(w, "404 page not found", http.StatusNotFound)
		return "", false
	}
	workID, err := url.PathUnescape(strings.Trim(remainder, "/"))
	if err != nil || strings.TrimSpace(workID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid work id"})
		return "", false
	}
	return workID, true
}

func boundedQueryDuration(query url.Values, key string, defaultMS, maxMS int) (time.Duration, error) {
	value := defaultMS
	if raw := strings.TrimSpace(query.Get(key)); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 || parsed > maxMS {
			return 0, fmt.Errorf("%s must be between 1 and %d", key, maxMS)
		}
		value = parsed
	}
	return time.Duration(value) * time.Millisecond, nil
}

func workAPIActor(r *http.Request) string {
	role := strings.TrimSpace(serverauth.RoleFromContext(r.Context()))
	if role == "" {
		role = "local"
	}
	return "work-api:" + role
}

func handleWorkLedgerList(w http.ResponseWriter, r *http.Request, store *workstore.Store, workspaceID string, logger zerolog.Logger) {
	source := strings.TrimSpace(r.URL.Query().Get("source"))
	sourceID := strings.TrimSpace(r.URL.Query().Get("source_id"))
	if sourceID != "" && source == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "source is required when source_id is set"})
		return
	}
	states, err := parseWorkLedgerStates(r.URL.Query())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	limit, offset, err := parseWorkLedgerPagination(r.URL.Query())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	works, err := store.ListWorks(r.Context(), workstore.ListWorksFilter{
		WorkspaceID: workspaceID,
		Source:      source,
		SourceID:    sourceID,
		States:      states,
		Limit:       limit,
		Offset:      offset,
	})
	if err != nil {
		logger.Error().Err(err).Str("workspace_id", workspaceID).Msg("list work ledger projections failed")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list work ledger projections failed"})
		return
	}
	if works == nil {
		works = []workstore.Work{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"works":  works,
		"limit":  limit,
		"offset": offset,
	})
}

func handleWorkLedgerTimeline(w http.ResponseWriter, r *http.Request, store *workstore.Store, workspaceID, path string, logger zerolog.Logger) {
	remainder := strings.TrimPrefix(path, "/v1/work/works/")
	parts := strings.Split(remainder, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "timeline" {
		http.NotFound(w, r)
		return
	}
	workID, err := url.PathUnescape(parts[0])
	if err != nil || strings.TrimSpace(workID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid work id"})
		return
	}
	projection, err := store.GetWorkProjection(r.Context(), workspaceID, workID)
	if errors.Is(err, workstore.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "work not found"})
		return
	}
	if err != nil {
		logger.Error().Err(err).Str("workspace_id", workspaceID).Str("work_id", workID).Msg("get work ledger timeline failed")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get work ledger timeline failed"})
		return
	}
	normalizeWorkProjectionSlices(&projection)
	writeJSON(w, http.StatusOK, projection)
}

func handleWorkLedgerLegacyTasks(w http.ResponseWriter, r *http.Request, store *workstore.Store, workspaceID, path string, logger zerolog.Logger) {
	remainder := strings.TrimPrefix(path, "/v1/work/legacy/sessions/")
	parts := strings.Split(remainder, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "tasks" {
		http.NotFound(w, r)
		return
	}
	sessionID, err := url.PathUnescape(parts[0])
	if err != nil || strings.TrimSpace(sessionID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid session id"})
		return
	}
	projection, found, err := store.GetLegacySessionTasksProjection(r.Context(), workspaceID, sessionID)
	if err != nil {
		logger.Error().Err(err).Str("workspace_id", workspaceID).Str("session_id", sessionID).Msg("get legacy tasks projection failed")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get legacy tasks projection failed"})
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "legacy tasks projection not found"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(projection)
}

func parseWorkLedgerStates(query url.Values) ([]workstore.WorkState, error) {
	var states []workstore.WorkState
	for _, raw := range query["state"] {
		for _, value := range strings.Split(raw, ",") {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			state := workstore.WorkState(value)
			switch state {
			case workstore.WorkStateTriage, workstore.WorkStateBacklog, workstore.WorkStateTodo,
				workstore.WorkStateReady, workstore.WorkStateRunning, workstore.WorkStateReview,
				workstore.WorkStateBlocked, workstore.WorkStateDone, workstore.WorkStateCancelled:
				states = append(states, state)
			default:
				return nil, errors.New("invalid work state")
			}
		}
	}
	return states, nil
}

func parseWorkLedgerPagination(query url.Values) (int, int, error) {
	limit := defaultWorkLedgerListLimit
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 || parsed > maxWorkLedgerListLimit {
			return 0, 0, errors.New("limit must be between 1 and 1000")
		}
		limit = parsed
	}
	offset := 0
	if raw := strings.TrimSpace(query.Get("offset")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			return 0, 0, errors.New("offset must be a non-negative integer")
		}
		offset = parsed
	}
	return limit, offset, nil
}

func normalizeWorkProjectionSlices(projection *workstore.WorkProjection) {
	if projection.Steps == nil {
		projection.Steps = []workstore.Step{}
	}
	if projection.Schedules == nil {
		projection.Schedules = []workstore.StepSchedule{}
	}
	if projection.Dependencies == nil {
		projection.Dependencies = []workstore.StepDependency{}
	}
	if projection.Attempts == nil {
		projection.Attempts = []workstore.Attempt{}
	}
	if projection.Events == nil {
		projection.Events = []workstore.Event{}
	}
	if projection.Proofs == nil {
		projection.Proofs = []workstore.Proof{}
	}
	if projection.Artifacts == nil {
		projection.Artifacts = []workstore.Artifact{}
	}
	if projection.Approvals == nil {
		projection.Approvals = []workstore.Approval{}
	}
}
