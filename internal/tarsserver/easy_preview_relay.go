package tarsserver

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devlikebear/tars/internal/consoleauth"
	"github.com/devlikebear/tars/internal/easyproject"
	"github.com/devlikebear/tars/internal/previewrelay"
	"github.com/devlikebear/tars/internal/remoteaccess"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/rs/zerolog"
)

// The running app of a vibe project on the paired phone (vibe redesign
// unit 7, docs/decisions/vibe-phone-preview.md): the preview relay
// (internal/previewrelay) listens on its own loopback port, Tailscale
// serve's second HTTPS port (8443) points at it whenever remote access is
// on, and
//
//	POST /v1/easy/projects/{id}/preview/ticket
//
// starts the project's preview and answers a one-use, 60-second link into
// the relay for the device that asked:
//
//	200 {url, expires_at, preview}
//	404 not_found · 409 not_ready (project not made yet)
//	409 preview_failed {preview} (the development server would not start)
//	503 relay_down            — the relay is not listening on this desk
//	503 remote_access_off     — Tailscale serve does not point at TARS
//	503 relay_not_served      — serve has no relay port (taken by another target, or not mapped yet)
//	503 tailscale_unavailable — tailscale is missing, signed out or unreadable
//
// No URL is handed out unless the relay is listening and Tailscale serves
// both TARS and the relay.

// Reason codes of a ticket refused because the relay cannot be reached.
const (
	relayReasonDown       = "relay_down"
	relayReasonRemoteOff  = "remote_access_off"
	relayReasonNotServed  = "relay_not_served"
	relayReasonTailscale  = "tailscale_unavailable"
	relayStatusTTL        = 30 * time.Second
	relayStatusFailureTTL = 5 * time.Second
)

// easyPhonePreview is the server's half of the relay: the relay itself,
// whether its listener runs, and whether Tailscale serves it.
type easyPhonePreview struct {
	relay     *previewrelay.Relay
	listening atomic.Bool
	previews  atomic.Pointer[easyproject.Previews]
	// detect is remoteaccess.Detect with the relay's target.
	detect func(ctx context.Context) (remoteaccess.Status, error)
	now    func() time.Time

	// srv is the relay listener's server once it listens.
	srvMu sync.Mutex
	srv   *http.Server

	mu       sync.Mutex
	cachedAt time.Time
	cacheTTL time.Duration
	url      string
	reason   string
}

// newEasyPhonePreview makes the relay for the workspace's paired devices.
func newEasyPhonePreview(workspace string, detect func(ctx context.Context) (remoteaccess.Status, error)) *easyPhonePreview {
	p := &easyPhonePreview{detect: detect, now: time.Now}
	devices := consoleauth.NewStore(workspace)
	p.relay = previewrelay.New(previewrelay.Options{
		Preview: p.previewOf,
		DeviceActive: func(id string) bool {
			d, ok, err := devices.DeviceByID(id)
			return err == nil && ok && d.Active() && d.Role == serverauth.RoleFocus
		},
	})
	return p
}

// relayDetect is the server's Tailscale detection with the relay named.
func relayDetect(runner remoteaccess.Runner, httpsPort int, target string) func(ctx context.Context) (remoteaccess.Status, error) {
	return func(ctx context.Context) (remoteaccess.Status, error) {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		return remoteaccess.Detect(ctx, remoteaccess.Options{
			Runner: runner, HTTPSPort: httpsPort, TargetURL: target,
			RelayTargetURL: remoteaccess.DefaultRelayTargetURL,
		})
	}
}

// previewOf is the relay's view of a project's preview. Status counts as
// someone looking, so a phone using the app keeps the idle sweep away.
func (p *easyPhonePreview) previewOf(id string) (previewrelay.PreviewState, string) {
	previews := p.previews.Load()
	if previews == nil {
		return previewrelay.StateStopped, ""
	}
	v := previews.Status(id)
	switch v.State {
	case easyproject.PreviewRunning:
		return previewrelay.StateRunning, v.URL
	case easyproject.PreviewStarting:
		return previewrelay.StateStarting, ""
	default:
		return previewrelay.StateStopped, ""
	}
}

// listen serves the relay on addr until close. A port already taken (a
// second server on this desk) leaves the relay down with a warning: the
// ticket route then answers relay_down.
func (p *easyPhonePreview) listen(addr string, logger zerolog.Logger) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Warn().Err(err).Str("addr", addr).Msg("preview relay: not listening; the phone cannot open vibe apps")
		return
	}
	srv := &http.Server{Handler: p.relay, ReadHeaderTimeout: 10 * time.Second}
	p.srvMu.Lock()
	p.srv = srv
	p.srvMu.Unlock()
	p.listening.Store(true)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Warn().Err(err).Msg("preview relay stopped")
		}
		p.listening.Store(false)
	}()
	logger.Info().Str("addr", addr).Msg("preview relay listening")
}

// close stops the relay listener, if it listens.
func (p *easyPhonePreview) close() {
	p.srvMu.Lock()
	srv := p.srv
	p.srv = nil
	p.srvMu.Unlock()
	if srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	p.listening.Store(false)
}

// relayURL is the HTTPS origin the phone opens, or the reason it cannot.
// Tailscale's answer is kept for 30 seconds (5 when it said no): it is two
// processes, and a phone may ask for a ticket on every Try it.
func (p *easyPhonePreview) relayURL(ctx context.Context) (string, string) {
	if !p.listening.Load() {
		return "", relayReasonDown
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	if !p.cachedAt.IsZero() && now.Sub(p.cachedAt) < p.cacheTTL {
		return p.url, p.reason
	}
	p.url, p.reason = relayURLOf(p.detect(ctx))
	p.cachedAt, p.cacheTTL = now, relayStatusTTL
	if p.reason != "" {
		p.cacheTTL = relayStatusFailureTTL
	}
	return p.url, p.reason
}

// relayURLOf turns a Tailscale detection into the relay's origin or a
// reason code.
func relayURLOf(status remoteaccess.Status, err error) (string, string) {
	switch {
	case err != nil, !status.Installed, !status.LoggedIn:
		return "", relayReasonTailscale
	case status.URL() == "":
		return "", relayReasonRemoteOff
	case status.RelayURL() == "":
		return "", relayReasonNotServed
	}
	return status.RelayURL(), ""
}

func (e *easyProjectsAPI) previewTicket(w http.ResponseWriter, r *http.Request) {
	p, ok := e.previewProject(w, r)
	if !ok {
		return
	}
	if e.phone == nil {
		writeError(w, http.StatusServiceUnavailable, relayReasonDown, "the preview relay is not running")
		return
	}
	base, reason := e.phone.relayURL(r.Context())
	if reason != "" {
		writeError(w, http.StatusServiceUnavailable, reason, "the app cannot be opened from a phone right now")
		return
	}
	preview := e.previews.Start(p.ID, p.Folder)
	if preview.State == easyproject.PreviewFailed {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "the app could not start", "code": "preview_failed",
			"preview": previewFor(r, preview),
		})
		return
	}
	tk, err := e.phone.relay.IssueTicket(p.ID, p.Folder, serverauth.DeviceIDFromRequest(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ticket_failed", "no ticket could be made")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"url":        base + previewrelay.EnterPath + "?t=" + tk.Value,
		"expires_at": tk.ExpiresAt.UTC().Format(time.RFC3339),
		"preview":    previewFor(r, preview),
	})
}

// previewFor leaves the app's loopback address out of what a phone reads:
// it leads nowhere from there, and the relay is the phone's way in.
func previewFor(r *http.Request, v easyproject.Preview) easyproject.Preview {
	if serverauth.RoleFromRequest(r) == serverauth.RoleFocus {
		v.URL = ""
	}
	return v
}
