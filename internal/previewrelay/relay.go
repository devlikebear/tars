// Package previewrelay serves a vibe project's running app to the paired
// phone on an origin of its own (docs/decisions/vibe-phone-preview.md,
// option 1b): a second loopback listener that proxies, at its root, to the
// project's development server, behind Tailscale serve's second HTTPS port.
//
// What it serves is model-written code run on the owner's desk, so the
// relay is the gate in front of it:
//
//   - nothing is forwarded without a relay session, and a session is only
//     made from a ticket the API handed a paired device (60 s, single use,
//     bound to project and device) — the bearer never reaches the page;
//   - the session cookie is HttpOnly, Secure, SameSite=Strict and host-only;
//   - forwarded requests lose the console's cookie and this relay's own,
//     and the app's responses cannot set either;
//   - /cdn-cgi/ and /__debug (the Cloudflare plugin's unauthenticated D1,
//     KV and R2 explorer), /@fs/ outside the project folder, and service
//     worker scripts (one origin serves every project in turn) are refused.
//
// Standard library only; the server wires the previews and the device
// store in through Options.
package previewrelay

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DefaultAddr is where the server listens for the relay. Tailscale serve's
// relay port points here (remoteaccess.DefaultRelayTargetURL).
const DefaultAddr = "127.0.0.1:43181"

// CookieName is the relay session's cookie. ConsoleCookieName is the
// console's, which never leaves the relay towards the app.
const (
	CookieName        = "tars_preview"
	ConsoleCookieName = "tars_session"
)

// EnterPath trades a ticket for a session cookie.
const EnterPath = "/__tars/enter"

const (
	// DefaultTicketTTL is how long a ticket can be traded.
	DefaultTicketTTL = 60 * time.Second
	// DefaultSessionIdle ends a relay session nobody used for this long.
	DefaultSessionIdle = time.Hour
)

// PreviewState is the project's preview as the relay needs it.
type PreviewState string

const (
	StateRunning  PreviewState = "running"
	StateStarting PreviewState = "starting"
	// StateStopped also stands for failed: either way nothing answers.
	StateStopped PreviewState = "stopped"
)

// Options wires the relay to the server.
type Options struct {
	// Preview answers the project's preview state and, while it runs, the
	// development server's base URL. Calling it counts as someone looking
	// (easyproject.Previews.Status), which keeps the idle sweep from
	// stopping a preview the phone is using.
	Preview func(projectID string) (PreviewState, string)
	// DeviceActive reports whether a paired device may still use the relay.
	// It is asked on every request, so revoking the phone ends its sessions
	// at their next request. Tickets issued with no device skip it.
	DeviceActive func(deviceID string) bool
	Now          func() time.Time
	TicketTTL    time.Duration
	SessionIdle  time.Duration
	// Transport reaches the development server; nil uses a default one.
	Transport http.RoundTripper
}

// Relay is the handler behind the relay listener, and the ticket office
// the API asks.
type Relay struct {
	opts      Options
	transport http.RoundTripper

	mu       sync.Mutex
	tickets  map[string]ticket   // sha256(ticket) → ticket
	sessions map[string]*session // sha256(cookie) → session
}

type ticket struct {
	project string
	folder  string
	device  string
	expires time.Time
}

type session struct {
	project  string
	folders  []string
	device   string
	lastUsed time.Time
}

// Ticket is what the API hands back: the value goes in EnterPath's query.
type Ticket struct {
	Value     string
	ExpiresAt time.Time
}

// New makes a relay. Preview is required.
func New(opts Options) *Relay {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.TicketTTL <= 0 {
		opts.TicketTTL = DefaultTicketTTL
	}
	if opts.SessionIdle <= 0 {
		opts.SessionIdle = DefaultSessionIdle
	}
	transport := opts.Transport
	if transport == nil {
		transport = &http.Transport{
			Proxy:                 nil,
			MaxIdleConnsPerHost:   16,
			IdleConnTimeout:       90 * time.Second,
			ResponseHeaderTimeout: 2 * time.Minute,
		}
	}
	return &Relay{
		opts:      opts,
		transport: transport,
		tickets:   map[string]ticket{},
		sessions:  map[string]*session{},
	}
}

// ErrNoProject is returned for a ticket with no project.
var ErrNoProject = errors.New("previewrelay: no project")

// IssueTicket makes a single-use ticket for project, served from folder,
// bound to device ("" for a desk caller with no device).
func (r *Relay) IssueTicket(project, folder, device string) (Ticket, error) {
	if strings.TrimSpace(project) == "" {
		return Ticket{}, ErrNoProject
	}
	value, err := randomToken()
	if err != nil {
		return Ticket{}, err
	}
	now := r.opts.Now()
	t := ticket{project: project, folder: folder, device: device, expires: now.Add(r.opts.TicketTTL)}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked(now)
	r.tickets[hashKey(value)] = t
	return Ticket{Value: value, ExpiresAt: t.expires}, nil
}

// consume trades a ticket for a new session: the ticket is gone whatever
// the outcome.
func (r *Relay) consume(value string) (string, bool) {
	if value == "" {
		return "", false
	}
	now := r.opts.Now()
	r.mu.Lock()
	key := hashKey(value)
	t, ok := r.tickets[key]
	delete(r.tickets, key)
	r.pruneLocked(now)
	r.mu.Unlock()
	if !ok || !now.Before(t.expires) || !r.deviceActive(t.device) {
		return "", false
	}
	cookie, err := randomToken()
	if err != nil {
		return "", false
	}
	r.mu.Lock()
	r.sessions[hashKey(cookie)] = &session{project: t.project, folders: projectFolders(t.folder), device: t.device, lastUsed: now}
	r.mu.Unlock()
	return cookie, true
}

// session answers the request's live relay session, touching it; a session
// whose device was revoked or that sat idle too long is ended here.
func (r *Relay) session(req *http.Request) (session, bool) {
	c, err := req.Cookie(CookieName)
	if err != nil || c.Value == "" {
		return session{}, false
	}
	key := hashKey(c.Value)
	now := r.opts.Now()
	r.mu.Lock()
	s, ok := r.sessions[key]
	if ok && now.Sub(s.lastUsed) >= r.opts.SessionIdle {
		delete(r.sessions, key)
		ok = false
	}
	var out session
	if ok {
		s.lastUsed = now
		out = *s
	}
	r.mu.Unlock()
	if !ok {
		return session{}, false
	}
	if !r.deviceActive(out.device) {
		r.endSession(key)
		return session{}, false
	}
	return out, true
}

func (r *Relay) endSession(key string) {
	r.mu.Lock()
	delete(r.sessions, key)
	r.mu.Unlock()
}

// RevokeDevice ends every ticket and session of a device at once.
func (r *Relay) RevokeDevice(device string) {
	if device == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, t := range r.tickets {
		if t.device == device {
			delete(r.tickets, k)
		}
	}
	for k, s := range r.sessions {
		if s.device == device {
			delete(r.sessions, k)
		}
	}
}

// Sessions counts live relay sessions (status reporting).
func (r *Relay) Sessions() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked(r.opts.Now())
	return len(r.sessions)
}

func (r *Relay) deviceActive(device string) bool {
	if device == "" || r.opts.DeviceActive == nil {
		return true
	}
	return r.opts.DeviceActive(device)
}

// pruneLocked drops expired tickets and idle sessions. r.mu is held.
func (r *Relay) pruneLocked(now time.Time) {
	for k, t := range r.tickets {
		if !now.Before(t.expires) {
			delete(r.tickets, k)
		}
	}
	for k, s := range r.sessions {
		if now.Sub(s.lastUsed) >= r.opts.SessionIdle {
			delete(r.sessions, k)
		}
	}
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// hashKey keys the maps by a digest, so a lookup's timing says nothing
// about how much of a guessed value matched.
func hashKey(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// projectFolders is the folder as given and, when it differs, with its
// symbolic links resolved: Vite's /@fs/ paths may use either.
func projectFolders(folder string) []string {
	folder = strings.TrimSpace(folder)
	if folder == "" {
		return nil
	}
	clean := filepath.Clean(folder)
	out := []string{clean}
	if resolved, err := filepath.EvalSymlinks(clean); err == nil && resolved != clean {
		out = append(out, resolved)
	}
	return out
}

// normalizedPath decodes the path until it stops changing (a double-encoded
// %252e must not slip a rule that the development server would decode
// again), cleans it and lower-cases it for the prefix rules — the desk's
// file system ignores case.
func normalizedPath(p string) string {
	for i := 0; i < 4; i++ {
		decoded, err := url.PathUnescape(p)
		if err != nil || decoded == p {
			break
		}
		p = decoded
	}
	p = strings.ReplaceAll(p, "\\", "/")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return path.Clean(p)
}

// refusal names why a request is never forwarded, "" when it may be.
func refusal(req *http.Request, folders []string) string {
	if strings.EqualFold(strings.TrimSpace(req.Header.Get("Service-Worker")), "script") {
		return "service_worker"
	}
	p := normalizedPath(req.URL.EscapedPath())
	lower := strings.ToLower(p)
	for _, prefix := range []string{"/cdn-cgi", "/__debug", "/__tars"} {
		if lower == prefix || strings.HasPrefix(lower, prefix+"/") {
			return "reserved_path"
		}
	}
	if lower == "/@fs" || strings.HasPrefix(lower, "/@fs/") {
		if !insideFolders(p[len("/@fs"):], folders) {
			return "outside_project"
		}
	}
	return ""
}

func insideFolders(file string, folders []string) bool {
	if file == "" {
		return false
	}
	// Vite writes a Windows path as /@fs/C:/…: drop the slash before the
	// drive letter.
	if len(file) >= 3 && file[0] == '/' && file[2] == ':' && filepath.VolumeName(file[1:]) != "" {
		file = file[1:]
	}
	file = filepath.Clean(filepath.FromSlash(file))
	for _, folder := range folders {
		rel, err := filepath.Rel(folder, file)
		if err != nil {
			continue
		}
		// A path spelled in another case reads as outside and is refused:
		// the safe side of a case-insensitive file system.
		if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}
