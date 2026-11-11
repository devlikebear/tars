package tarsserver

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/initiative"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
)

// initiativeStatusResponse is initiative.Runtime.Snapshot() plus the
// console presence facts (tars#1249) — timestamps only, never message text
// or a session id (#1003's rule).
type initiativeStatusResponse struct {
	initiative.Snapshot
	Presence initiativeStatusPresence `json:"presence"`
}

type initiativeStatusPresence struct {
	LastAt    *time.Time `json:"last_at,omitempty"`
	ArrivedAt *time.Time `json:"arrived_at,omitempty"`
	Present   bool       `json:"present"`
}

func initiativeStatusPresenceFrom(state initiative.PresenceState, now time.Time) initiativeStatusPresence {
	out := initiativeStatusPresence{Present: state.Present(now)}
	if !state.LastAt.IsZero() {
		lastAt := state.LastAt
		out.LastAt = &lastAt
	}
	if !state.ArrivedAt.IsZero() {
		arrivedAt := state.ArrivedAt
		out.ArrivedAt = &arrivedAt
	}
	return out
}

// newInitiativeAPIHandler serves GET /v1/initiative/status. presence and now
// may be nil (setup-only mode, or initiative disabled entirely) — the
// response then always reports presence.present=false with no timestamps,
// same as before this field existed.
func newInitiativeAPIHandler(runtime *initiative.Runtime, presence *initiative.PresenceStore, now func() time.Time) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/initiative/status", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		resp := initiativeStatusResponse{Snapshot: runtime.Snapshot()}
		if presence != nil && now != nil {
			resp.Presence = initiativeStatusPresenceFrom(presence.Current(), now())
		}
		writeJSON(w, http.StatusOK, resp)
	})
	return mux
}

// initiativeMemoryAnswerDeps is what the confirm_memory answer route needs
// (tars#1001): the main session the question was written to, and the
// memory inbox it reviews.
type initiativeMemoryAnswerDeps struct {
	WorkspaceDir  string
	Store         *session.Store
	MainSessionID string
	Backend       memory.Backend
}

// newInitiativeAPIHandlerWithMemory is newInitiativeAPIHandler plus
// /v1/initiative/memory/answer, the console's remember / don't-remember
// buttons on a confirm_memory question (tars#1001). The answer is the
// button, not the user's words: nothing reads a reply to decide approve or
// reject, so no model is ever asked to judge it.
//
//	GET  ?entry_id=…                   → {entry_id, status}
//	POST {entry_id, action: approve|reject} → {entry_id, status}
//
// status is the candidate's inbox status (pending, approved, rejected,
// merged) or "missing" when the candidate is gone from the inbox. A POST on
// a candidate already reviewed (here or on the Memory screen) is 409 with
// the current status. Approving uses the same memory.ReviewMemoryCandidate
// the Memory screen does, then resolves the question's reply window as
// replied.
func newInitiativeAPIHandlerWithMemory(runtime *initiative.Runtime, presence *initiative.PresenceStore, now func() time.Time, deps initiativeMemoryAnswerDeps) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/v1/initiative/status", newInitiativeAPIHandler(runtime, presence, now))
	mux.HandleFunc("/v1/initiative/memory/answer", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet, http.MethodPost) {
			return
		}
		var req struct {
			EntryID string `json:"entry_id"`
			Action  string `json:"action"`
		}
		if r.Method == http.MethodGet {
			req.EntryID = r.URL.Query().Get("entry_id")
		} else if !decodeJSONBody(w, r, &req) {
			return
		}
		req.EntryID = strings.TrimSpace(req.EntryID)
		if req.EntryID == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "entry_id is required")
			return
		}
		candidateID, err := confirmMemoryCandidateID(deps, req.EntryID)
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "no confirm_memory question with that entry_id")
			return
		}
		status, err := memoryCandidateStatus(deps.WorkspaceDir, candidateID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "memory_inbox_error", "memory inbox unreadable")
			return
		}
		if r.Method == http.MethodGet {
			writeJSON(w, http.StatusOK, map[string]string{"entry_id": req.EntryID, "status": status})
			return
		}
		action := memory.MemoryCandidateAction(strings.ToLower(strings.TrimSpace(req.Action)))
		if action != memory.MemoryCandidateActionApprove && action != memory.MemoryCandidateActionReject {
			writeError(w, http.StatusBadRequest, "invalid_request", "action must be approve or reject")
			return
		}
		if status != string(memory.MemoryCandidateStatusPending) {
			writeJSON(w, http.StatusConflict, map[string]string{"entry_id": req.EntryID, "status": status})
			return
		}
		reviewed, err := memory.ReviewMemoryCandidate(r.Context(), deps.WorkspaceDir, deps.Backend, candidateID, memory.MemoryCandidateReview{Action: action})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "memory_review_failed", "memory review failed")
			return
		}
		runtime.RecordAnswer(req.EntryID)
		writeJSON(w, http.StatusOK, map[string]string{"entry_id": req.EntryID, "status": string(reviewed.Status)})
	})
	return mux
}

// confirmMemoryCandidateID finds the confirm_memory message entryID names in
// the main session and returns the candidate id it asked about.
func confirmMemoryCandidateID(deps initiativeMemoryAnswerDeps, entryID string) (string, error) {
	if deps.Store == nil || deps.MainSessionID == "" {
		return "", errors.New("no main session")
	}
	msgs, err := session.ReadMessages(deps.Store.TranscriptPath(deps.MainSessionID))
	if err != nil {
		return "", err
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		in := msgs[i].Initiative
		if in != nil && in.EntryID == entryID && in.Intent == string(initiative.IntentConfirmMemory) && strings.TrimSpace(in.Ref) != "" {
			return strings.TrimSpace(in.Ref), nil
		}
	}
	return "", errors.New("not found")
}

func memoryCandidateStatus(workspaceDir, id string) (string, error) {
	candidates, err := memory.ListMemoryCandidates(workspaceDir, memory.MemoryCandidateListOptions{})
	if err != nil {
		return "", err
	}
	for _, c := range candidates {
		if c.ID == id {
			return string(c.Status), nil
		}
	}
	return "missing", nil
}
