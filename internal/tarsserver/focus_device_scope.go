package tarsserver

import (
	"errors"
	"net/http"
	"strings"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
)

// A paired phone (role focus, docs/decisions/mobile-remote.md §4) reaches
// the chat turn, turn feed, cancel, permission-answer and ops-approval
// routes only for focus pipeline sessions. serverauth's focus table lets the
// request in; the session it names is checked here, by the handler, because
// only the handler knows which session that is (a body field, a query
// parameter, or the session an approval belongs to).
//
// Which sessions a phone reaches:
//   - a pipeline's own session, for everything;
//   - a pipeline's hidden Q&A session, for reading its turn feed, cancelling
//     it and answering its queued approvals. The phone asks Q&A through
//     POST /v1/focus/pipelines/{id}/qa, never by sending a chat turn there.

// focusDeviceScope answers which sessions a phone may reach.
type focusDeviceScope struct {
	pipelines *focuspipeline.Store
}

func newFocusDeviceScope(sessions *session.Store) focusDeviceScope {
	return focusDeviceScope{pipelines: focusStoreFor(sessions)}
}

// isFocusDevice reports whether r comes from a paired phone.
func isFocusDevice(r *http.Request) bool {
	return serverauth.RoleFromRequest(r) == serverauth.RoleFocus
}

// sessions lists every session a phone may reach: each pipeline's session
// and its Q&A session.
func (s focusDeviceScope) sessions() (map[string]bool, error) {
	out := map[string]bool{}
	if s.pipelines == nil {
		return out, nil
	}
	list, err := s.pipelines.List()
	if err != nil {
		return nil, err
	}
	for _, p := range list {
		if id := strings.TrimSpace(p.SessionID); id != "" {
			out[id] = true
		}
		if id := strings.TrimSpace(p.QASessionID); id != "" {
			out[id] = true
		}
	}
	return out, nil
}

// allows reports whether a phone may reach sessionID. pipelineOnly leaves
// out the Q&A sessions.
func (s focusDeviceScope) allows(sessionID string, pipelineOnly bool) (bool, error) {
	id := strings.TrimSpace(sessionID)
	if s.pipelines == nil || id == "" {
		return false, nil
	}
	_, ok, err := s.pipelines.Get(id)
	if errors.Is(err, focuspipeline.ErrInvalidSessionID) {
		return false, nil
	}
	if err != nil || ok || pipelineOnly {
		return ok, err
	}
	all, err := s.sessions()
	if err != nil {
		return false, err
	}
	return all[id], nil
}

// refuse answers for a phone that may not reach sessionID and reports
// whether it did. Every other caller passes untouched.
func (s focusDeviceScope) refuse(w http.ResponseWriter, r *http.Request, sessionID string, pipelineOnly bool) bool {
	if !isFocusDevice(r) {
		return false
	}
	ok, err := s.allows(sessionID, pipelineOnly)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "", "failed to read focus pipelines")
		return true
	}
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden", "a paired device reaches only focus pipeline sessions")
		return true
	}
	return false
}

// refuseChatRequest also refuses what a phone's turn may not carry: a tier
// pin, which changes the session for every later turn.
func (s focusDeviceScope) refuseChatRequest(w http.ResponseWriter, r *http.Request, req chatRequestPayload) bool {
	if s.refuse(w, r, req.SessionID, true) {
		return true
	}
	if isFocusDevice(r) && req.TierRecommendation != nil && req.TierRecommendation.Pin {
		writeError(w, http.StatusForbidden, "forbidden", "a paired device cannot pin a session's tier")
		return true
	}
	return false
}

// refusePermissionAnswer also refuses the answers that outlive the
// question: allow_always (a rule for the whole folder) and a mode to
// continue a plan in (the session's mode for every later turn).
func (s focusDeviceScope) refusePermissionAnswer(w http.ResponseWriter, r *http.Request, sessionID string, answer chatPermissionAnswer) bool {
	if s.refuse(w, r, sessionID, true) {
		return true
	}
	if isFocusDevice(r) && (answer.Decision == "allow_always" || answer.Mode != "") {
		writeError(w, http.StatusForbidden, "forbidden", "a paired device answers only allow_once, allow_session or deny")
		return true
	}
	return false
}

// filterActivity keeps only the phone's sessions in a chat activity
// snapshot.
func (s focusDeviceScope) filterActivity(snap chatActivitySnapshot) (chatActivitySnapshot, error) {
	allowed, err := s.sessions()
	if err != nil {
		return chatActivitySnapshot{}, err
	}
	out := chatActivitySnapshot{Running: []chatRunningSession{}, Pending: []chatPendingApproval{}, Queued: []chatQueuedApproval{}}
	for _, item := range snap.Running {
		if allowed[item.SessionID] {
			out.Running = append(out.Running, item)
		}
	}
	for _, item := range snap.Pending {
		if allowed[item.SessionID] {
			out.Pending = append(out.Pending, item)
		}
	}
	for _, item := range snap.Queued {
		if allowed[item.SessionID] {
			out.Queued = append(out.Queued, item)
		}
	}
	return out, nil
}

// refuseDeviceApproval answers for a phone that may not review approvalID
// and reports whether it did. A phone answers only an unattended turn's tool
// call (a tool_permission approval) in a session it reaches: never a cleanup
// or a git mutation, which act on the desk outside any focus session.
func refuseDeviceApproval(w http.ResponseWriter, r *http.Request, manager *ops.Manager, sessions *session.Store, approvalID string) bool {
	if !isFocusDevice(r) {
		return false
	}
	approval, err := manager.GetApproval(approvalID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "approval not found")
		return true
	}
	if approval.Type != ops.ApprovalTypeToolPermission || approval.ToolPermission == nil {
		writeError(w, http.StatusForbidden, "forbidden", "a paired device answers only tool permission questions")
		return true
	}
	return newFocusDeviceScope(sessions).refuse(w, r, approval.ToolPermission.SessionID, false)
}
