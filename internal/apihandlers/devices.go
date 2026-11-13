package apihandlers

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/consoleauth"
	"github.com/devlikebear/tars/internal/httpapi"
	"github.com/rs/zerolog"
)

// Pairing codes for a phone live long enough to walk from the desk to the
// phone and scan, and no longer (docs/decisions/mobile-remote.md §4).
const (
	DefaultDevicePairingTTL = 10 * time.Minute
	MaxDevicePairingTTL     = time.Hour
)

type devicesAPIHandler struct {
	store  *consoleauth.Store
	logger zerolog.Logger
	mux    *http.ServeMux
}

type deviceView struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Role      string     `json:"role"`
	Active    bool       `json:"active"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

// NewDevicesHandler serves the admin side of mobile pairing: list paired
// devices, issue a one-time pairing code, revoke a device. Every route is
// under /v1/admin/, so only the admin role reaches it. The device side of
// pairing (exchanging the code for a token) is /v1/auth/device-pair.
func NewDevicesHandler(store *consoleauth.Store, logger zerolog.Logger) http.Handler {
	h := &devicesAPIHandler{store: store, logger: logger, mux: http.NewServeMux()}
	h.mux.HandleFunc("GET /v1/admin/devices", h.list)
	h.mux.HandleFunc("POST /v1/admin/devices/pairing-code", h.pairingCode)
	h.mux.HandleFunc("DELETE /v1/admin/devices/{id}", h.revoke)
	h.mux.HandleFunc("GET /v1/admin/devices/folders", h.folders)
	h.mux.HandleFunc("PUT /v1/admin/devices/folders", h.setFolders)
	return h
}

// folders returns the folders a paired device may start a focus pipeline in.
func (h *devicesAPIHandler) folders(w http.ResponseWriter, _ *http.Request) {
	folders, err := h.store.DeviceFolders()
	if err != nil {
		h.logger.Error().Err(err).Msg("read device folders")
		httpapi.WriteError(w, http.StatusInternalServerError, "device_folders_unavailable", "failed to read device folders")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"folders": folders})
}

// setFolders replaces the list. Every folder must exist now, so a typo is
// caught at the desk rather than on the phone; one that disappears later is
// refused when a pipeline is created in it.
func (h *devicesAPIHandler) setFolders(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Folders []string `json:"folders"`
	}
	if !httpapi.DecodeJSONBody(w, r, &req) {
		return
	}
	for _, folder := range req.Folders {
		normalized, err := consoleauth.NormalizeDeviceFolder(folder)
		if err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_folder", err.Error())
			return
		}
		info, err := os.Stat(normalized)
		if err != nil || !info.IsDir() {
			httpapi.WriteError(w, http.StatusBadRequest, "folder_not_found", "folder not found: "+normalized)
			return
		}
	}
	stored, err := h.store.SetDeviceFolders(req.Folders)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_folders", err.Error())
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"folders": stored})
}

func (h *devicesAPIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *devicesAPIHandler) list(w http.ResponseWriter, _ *http.Request) {
	devices, err := h.store.ListDevices()
	if err != nil {
		h.logger.Error().Err(err).Msg("list devices")
		httpapi.WriteError(w, http.StatusInternalServerError, "devices_unavailable", "failed to read devices")
		return
	}
	views := make([]deviceView, 0, len(devices))
	for _, device := range devices {
		views = append(views, deviceView{
			ID:        device.ID,
			Name:      device.Name,
			Role:      device.Role,
			Active:    device.Active(),
			CreatedAt: device.CreatedAt,
			RevokedAt: device.RevokedAt,
		})
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"devices": views})
}

func (h *devicesAPIHandler) pairingCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TTLSeconds int `json:"ttl_seconds,omitempty"`
	}
	if !httpapi.DecodeOptionalJSONBody(w, r, &req) {
		return
	}
	ttl, err := devicePairingTTL(req.TTLSeconds)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_ttl", err.Error())
		return
	}
	code, err := h.store.CreatePairingCode(consoleauth.RoleFocus, ttl)
	if err != nil {
		h.logger.Error().Err(err).Msg("create device pairing code")
		httpapi.WriteError(w, http.StatusInternalServerError, "pairing_code_failed", "failed to create pairing code")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{
		"code":       code.Code,
		"role":       code.Role,
		"expires_at": code.ExpiresAt,
	})
}

func (h *devicesAPIHandler) revoke(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if err := h.store.RevokeDevice(id); err != nil {
		if errors.Is(err, consoleauth.ErrDeviceNotFound) {
			httpapi.WriteError(w, http.StatusNotFound, "device_not_found", "device not found")
			return
		}
		h.logger.Error().Err(err).Msg("revoke device")
		httpapi.WriteError(w, http.StatusInternalServerError, "revoke_failed", "failed to revoke device")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

func devicePairingTTL(seconds int) (time.Duration, error) {
	if seconds == 0 {
		return DefaultDevicePairingTTL, nil
	}
	if seconds < 0 {
		return 0, errors.New("ttl_seconds must be positive")
	}
	ttl := time.Duration(seconds) * time.Second
	if ttl > MaxDevicePairingTTL {
		return 0, errors.New("ttl_seconds must be at most 3600")
	}
	return ttl, nil
}
