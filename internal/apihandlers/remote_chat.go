package apihandlers

import (
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/devlikebear/tars/internal/consoleauth"
	"github.com/devlikebear/tars/internal/httpapi"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// deviceChatPermissionMode is the only permission mode a device-chat session
// is ever given (docs/decisions/mobile-remote.md §8): nobody is at a desk to
// have pre-approved anything riskier, so the phone is never offered Accept
// Edits or Plan for this session.
const deviceChatPermissionMode = "manual"

// NewRemoteChatHandler serves the one chat session a paired phone talks to
// for a plain question outside a focus task (docs/decisions/mobile-remote.md
// §8) — not the chat workbench: no session list, no switching sessions, no
// history beyond this one session.
//
// POST /v1/remote/chat/session finds or makes the device's session;
// GET /v1/remote/chat/history reads its transcript. Neither route takes a
// session id in the request: the device never names one, the server always
// resolves "my session" from the device id serverauth put on the request
// context (serverauth.DeviceIDFromRequest). Sending or streaming a turn
// goes through the ordinary /v1/chat routes, which focusDeviceScope
// (internal/tarsserver/focus_device_scope.go) scopes to this same session.
func NewRemoteChatHandler(sessions *session.Store, auth *consoleauth.Store, logger zerolog.Logger) http.Handler {
	h := &remoteChatHandler{sessions: sessions, auth: auth, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/remote/chat/session", h.ensureSession)
	mux.HandleFunc("GET /v1/remote/chat/history", h.history)
	return mux
}

type remoteChatHandler struct {
	sessions *session.Store
	auth     *consoleauth.Store
	logger   zerolog.Logger
	// ensureMu makes find-or-create one step, so two requests from the same
	// phone (a double tap, two launches) cannot each make a session and
	// leave one of them orphaned.
	ensureMu sync.Mutex
}

type remoteChatSessionView struct {
	SessionID string `json:"session_id"`
}

func (h *remoteChatHandler) ensureSession(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := h.requireDevice(w, r)
	if !ok {
		return
	}
	sess, err := h.ensure(deviceID)
	if err != nil {
		h.logger.Error().Err(err).Str("device_id", deviceID).Msg("ensure device chat session failed")
		httpapi.WriteError(w, http.StatusInternalServerError, "", "failed to prepare chat session")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, remoteChatSessionView{SessionID: sess.ID})
}

func (h *remoteChatHandler) history(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := h.requireDevice(w, r)
	if !ok {
		return
	}
	sess, err := h.ensure(deviceID)
	if err != nil {
		h.logger.Error().Err(err).Str("device_id", deviceID).Msg("ensure device chat session failed")
		httpapi.WriteError(w, http.StatusInternalServerError, "", "failed to prepare chat session")
		return
	}
	messages, err := session.ReadMessages(h.sessions.TranscriptPath(sess.ID))
	if err != nil {
		h.logger.Error().Err(err).Str("session_id", sess.ID).Msg("read device chat session history failed")
		httpapi.WriteError(w, http.StatusInternalServerError, "", "read session history failed")
		return
	}
	if messages == nil {
		messages = []session.Message{}
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"session_id": sess.ID, "messages": messages})
}

func (h *remoteChatHandler) requireDevice(w http.ResponseWriter, r *http.Request) (string, bool) {
	if h.sessions == nil || h.auth == nil {
		httpapi.WriteUnavailable(w, "device chat is not available")
		return "", false
	}
	deviceID := strings.TrimSpace(serverauth.DeviceIDFromRequest(r))
	if deviceID == "" {
		httpapi.WriteError(w, http.StatusForbidden, "forbidden", "a device chat session needs a paired device token")
		return "", false
	}
	return deviceID, true
}

// ensure returns deviceID's chat session, creating one when there is none
// recorded, when the recorded session is gone, or when it was auto-archived
// (idle long enough that session retention tagged it) — a session that
// stale starts fresh rather than quietly resuming days-old context the
// phone's owner has likely forgotten.
func (h *remoteChatHandler) ensure(deviceID string) (session.Session, error) {
	h.ensureMu.Lock()
	defer h.ensureMu.Unlock()
	if sessionID, ok, err := h.auth.DeviceChatSession(deviceID); err != nil {
		return session.Session{}, err
	} else if ok {
		sess, err := h.sessions.Get(sessionID)
		switch {
		case err == nil && sess.ArchivedAt == nil:
			return sess, nil
		case err != nil && !errors.Is(err, session.ErrSessionNotFound):
			return session.Session{}, err
		}
		// Gone, or archived: fall through and make a new one.
	}
	return h.create(deviceID)
}

func (h *remoteChatHandler) create(deviceID string) (session.Session, error) {
	title := "Phone · " + deviceID
	if device, ok, err := h.auth.DeviceByID(deviceID); err == nil && ok && strings.TrimSpace(device.Name) != "" {
		title = "Phone · " + device.Name
	}
	created, err := h.sessions.Create(title)
	if err != nil {
		return session.Session{}, err
	}
	if err := h.sessions.SetPermissionMode(created.ID, deviceChatPermissionMode); err != nil {
		return session.Session{}, err
	}
	created.PermissionMode = deviceChatPermissionMode
	if err := h.auth.SetDeviceChatSession(deviceID, created.ID); err != nil {
		return session.Session{}, err
	}
	return created, nil
}
