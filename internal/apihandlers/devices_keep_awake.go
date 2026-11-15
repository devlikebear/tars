package apihandlers

import (
	"net/http"

	"github.com/devlikebear/tars/internal/httpapi"
)

// KeepAwakeStatus is the Phones card's "keep this computer awake" line:
// what the owner asked for and what holds now.
type KeepAwakeStatus struct {
	// Enabled is the owner's choice (consoleauth.Store.KeepAwake).
	Enabled bool `json:"enabled"`
	// Supported is whether this system can be kept awake at all.
	Supported bool `json:"supported"`
	// Active is whether the assertion is held now.
	Active bool `json:"active"`
	// Reason says why it is not active while enabled: "unsupported",
	// "no_phone" (no paired phone) or "failed"; "" otherwise.
	Reason string `json:"reason,omitempty"`
}

// KeepAwakeControl is the server's keep-awake loop: its status, and Apply
// to act on a changed choice at once instead of on the next tick.
type KeepAwakeControl interface {
	KeepAwakeStatus() (KeepAwakeStatus, error)
	ApplyKeepAwake()
}

// DevicesOption configures NewDevicesHandler.
type DevicesOption func(*devicesAPIHandler)

// WithKeepAwake serves GET/PUT /v1/admin/devices/keep-awake through control.
func WithKeepAwake(control KeepAwakeControl) DevicesOption {
	return func(h *devicesAPIHandler) { h.keepAwake = control }
}

func (h *devicesAPIHandler) getKeepAwake(w http.ResponseWriter, _ *http.Request) {
	h.writeKeepAwake(w)
}

// setKeepAwake stores the choice and has the loop act on it now.
func (h *devicesAPIHandler) setKeepAwake(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if !httpapi.DecodeJSONBody(w, r, &req) {
		return
	}
	if req.Enabled == nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_keep_awake", "enabled is required")
		return
	}
	if err := h.store.SetKeepAwake(*req.Enabled); err != nil {
		h.logger.Error().Err(err).Msg("save keep awake")
		httpapi.WriteError(w, http.StatusInternalServerError, "keep_awake_unavailable", "failed to save keep awake")
		return
	}
	if h.keepAwake != nil {
		h.keepAwake.ApplyKeepAwake()
	}
	h.writeKeepAwake(w)
}

func (h *devicesAPIHandler) writeKeepAwake(w http.ResponseWriter) {
	var status KeepAwakeStatus
	var err error
	if h.keepAwake != nil {
		status, err = h.keepAwake.KeepAwakeStatus()
	} else {
		status.Enabled, err = h.store.KeepAwake()
		if status.Enabled {
			status.Reason = "unsupported"
		}
	}
	if err != nil {
		h.logger.Error().Err(err).Msg("read keep awake")
		httpapi.WriteError(w, http.StatusInternalServerError, "keep_awake_unavailable", "failed to read keep awake")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, status)
}
