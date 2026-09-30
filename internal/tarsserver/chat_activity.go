package tarsserver

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/devlikebear/tars/internal/session"
)

// Chat activity (#972): which sessions have a turn running and which of
// those turns are waiting for someone to approve a tool call.
//
// The approval questions themselves stream only to the console request that
// runs the turn. A client that is not that request — the desktop shell's
// tray, or the console showing another session — needs the same facts
// globally: GET /v1/chat/activity lists them, and a notification on
// /v1/events/stream announces each new question, so the tray can say
// "needs input" and a native notification can offer to approve.
//
// The tracker learns about questions by watching the permission events a
// turn's stream sends, so every path that asks (Claude Code's permission
// prompts and the native tool gate) is covered without being changed.

type chatActivity struct {
	mu       sync.Mutex
	running  map[string]*chatRunningTurn
	pending  map[string]chatPendingApproval
	sessions *session.Store
	notify   func(context.Context, notificationEvent)
	now      func() time.Time
}

type chatRunningTurn struct {
	started time.Time
	// turns counts overlapping requests for one session; the session is
	// running until the last one ends.
	turns int
}

// chatPendingApproval is one unanswered permission_request.
type chatPendingApproval struct {
	RequestID string    `json:"request_id"`
	SessionID string    `json:"session_id"`
	Session   string    `json:"session_title,omitempty"`
	ToolName  string    `json:"tool_name"`
	Preview   string    `json:"preview,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	AskedAt   time.Time `json:"asked_at"`
	// Decisions lists the answers POST /v1/chat/permissions/{id} accepts
	// for this question.
	Decisions []string `json:"decisions"`
}

type chatRunningSession struct {
	SessionID string    `json:"session_id"`
	Session   string    `json:"session_title,omitempty"`
	StartedAt time.Time `json:"started_at"`
}

type chatActivitySnapshot struct {
	Running []chatRunningSession  `json:"running"`
	Pending []chatPendingApproval `json:"pending_approvals"`
}

func newChatActivity(sessions *session.Store, notify func(context.Context, notificationEvent)) *chatActivity {
	return &chatActivity{
		running:  map[string]*chatRunningTurn{},
		pending:  map[string]chatPendingApproval{},
		sessions: sessions,
		notify:   notify,
		now:      time.Now,
	}
}

// begin marks a session's turn as running. The returned func ends it and
// drops any question the turn left unanswered.
func (a *chatActivity) begin(sessionID string) func() {
	if a == nil || strings.TrimSpace(sessionID) == "" {
		return func() {}
	}
	a.mu.Lock()
	turn := a.running[sessionID]
	if turn == nil {
		turn = &chatRunningTurn{started: a.now().UTC()}
		a.running[sessionID] = turn
	}
	turn.turns++
	a.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			a.mu.Lock()
			defer a.mu.Unlock()
			if t := a.running[sessionID]; t != nil {
				t.turns--
				if t.turns > 0 {
					return
				}
				delete(a.running, sessionID)
			}
			for id, p := range a.pending {
				if p.SessionID == sessionID {
					delete(a.pending, id)
				}
			}
		})
	}
}

// observe reads one event a turn's stream sent.
func (a *chatActivity) observe(sessionID string, payload map[string]any) {
	if a == nil {
		return
	}
	kind, _ := payload["type"].(string)
	requestID, _ := payload["request_id"].(string)
	if requestID == "" {
		return
	}
	switch kind {
	case "permission_request":
		p := chatPendingApproval{
			RequestID: requestID,
			SessionID: sessionID,
			ToolName:  activityString(payload, "tool_name"),
			Preview:   approvalPreview(payload),
			Reason:    activityString(payload, "reason"),
			AskedAt:   a.now().UTC(),
			Decisions: chatApprovalDecisions(payload),
		}
		p.Session = a.sessionTitle(sessionID)
		a.mu.Lock()
		a.pending[requestID] = p
		a.mu.Unlock()
		a.announce(p)
	case "permission_resolved":
		a.mu.Lock()
		delete(a.pending, requestID)
		a.mu.Unlock()
	}
}

func (a *chatActivity) snapshot() chatActivitySnapshot {
	snap := chatActivitySnapshot{Running: []chatRunningSession{}, Pending: []chatPendingApproval{}}
	if a == nil {
		return snap
	}
	a.mu.Lock()
	running := make(map[string]time.Time, len(a.running))
	for id, t := range a.running {
		running[id] = t.started
	}
	for _, p := range a.pending {
		snap.Pending = append(snap.Pending, p)
	}
	a.mu.Unlock()
	for id, started := range running {
		snap.Running = append(snap.Running, chatRunningSession{SessionID: id, Session: a.sessionTitle(id), StartedAt: started})
	}
	slices.SortFunc(snap.Running, func(x, y chatRunningSession) int { return x.StartedAt.Compare(y.StartedAt) })
	slices.SortFunc(snap.Pending, func(x, y chatPendingApproval) int { return x.AskedAt.Compare(y.AskedAt) })
	return snap
}

// announce tells every event-stream client that a turn is waiting. The
// question itself is answered through the chat API, never here.
func (a *chatActivity) announce(p chatPendingApproval) {
	if a.notify == nil {
		return
	}
	title := "Approval needed"
	if p.Session != "" {
		title += ": " + p.Session
	}
	message := p.ToolName
	if p.Preview != "" {
		message += ": " + p.Preview
	}
	evt := newNotificationEvent("approval", "warning", title, message)
	evt.SessionID = p.SessionID
	evt.RequestID = p.RequestID
	evt.OpenPath = "/console/chat/" + p.SessionID
	a.notify(context.Background(), evt)
}

func (a *chatActivity) sessionTitle(sessionID string) string {
	if a.sessions == nil {
		return ""
	}
	sess, err := a.sessions.Get(sessionID)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(sess.Title)
}

func activityString(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}

// approvalPreviewRunes caps the preview; it is a hint for a notification,
// the full input is in the console card.
const approvalPreviewRunes = 160

// approvalPreview picks the input field that says what the call does: the
// command, the file, or the URL.
func approvalPreview(payload map[string]any) string {
	var input map[string]any
	switch raw := payload["input"].(type) {
	case json.RawMessage:
		_ = json.Unmarshal(raw, &input)
	case map[string]any:
		input = raw
	}
	preview := ""
	for _, key := range []string{"command", "cmd", "file_path", "path", "url", "pattern", "query"} {
		if value, ok := input[key].(string); ok && strings.TrimSpace(value) != "" {
			preview = strings.TrimSpace(value)
			break
		}
	}
	if preview == "" {
		preview = activityString(payload, "description")
	}
	if utf8.RuneCountInString(preview) > approvalPreviewRunes {
		r := []rune(preview)
		preview = string(r[:approvalPreviewRunes-1]) + "…"
	}
	return preview
}

func handleChatActivity(w http.ResponseWriter, r *http.Request, activity *chatActivity) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, activity.snapshot())
}

// chatApprovalDecisions lists the answers a question takes. allow_session is
// always accepted: the server falls back to allowing once when no rule can
// be remembered. allow_always is offered only when the prompt names the
// folder it would cover.
func chatApprovalDecisions(payload map[string]any) []string {
	if activityString(payload, "always_dir") != "" {
		return []string{"allow_once", "allow_session", "allow_always", "deny"}
	}
	return []string{"allow_once", "allow_session", "deny"}
}
