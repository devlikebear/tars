package tarsserver

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/workstore"
	"github.com/rs/zerolog"
)

const (
	defaultWorkLedgerListLimit = 100
	maxWorkLedgerListLimit     = 1000
)

// newWorkLedgerAPIHandler exposes projections only. Mutations continue to use
// the legacy APIs until the migration compatibility window is complete.
func newWorkLedgerAPIHandler(store *workstore.Store, logger zerolog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeUnavailable(w, "work ledger is not configured")
			return
		}
		if !requireMethod(w, r, http.MethodGet) {
			return
		}

		workspaceID := normalizeWorkspaceID(serverauth.WorkspaceIDFromContext(r.Context()))
		path := strings.TrimSuffix(r.URL.Path, "/")
		switch {
		case path == "/v1/work/works":
			handleWorkLedgerList(w, r, store, workspaceID, logger)
		case strings.HasPrefix(path, "/v1/work/works/"):
			handleWorkLedgerTimeline(w, r, store, workspaceID, path, logger)
		case strings.HasPrefix(path, "/v1/work/legacy/sessions/"):
			handleWorkLedgerLegacyTasks(w, r, store, workspaceID, path, logger)
		default:
			http.NotFound(w, r)
		}
	})
}

func handleWorkLedgerList(w http.ResponseWriter, r *http.Request, store *workstore.Store, workspaceID string, logger zerolog.Logger) {
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
