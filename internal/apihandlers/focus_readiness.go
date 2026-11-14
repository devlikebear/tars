package apihandlers

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/httpapi"
	"github.com/devlikebear/tars/internal/remoteaccess"
)

// Readiness is what a phone shows under a desktop besides whether it
// answers (docs/decisions/mobile-remote.md §2): can a focus task run there
// end to end. Three facts, each a fixed state — never a tool's own output:
//
//   - model: the chat tier's provider; a CLI provider (claude-code-cli,
//     antigravity-cli) also needs its binary on this desktop.
//   - gh: installed and signed in, for the PR stages.
//   - tailscale: installed, signed in, and serve pointing at TARS — the
//     address the phone and the Telegram alert's link use.
const (
	ReadinessOK         = "ok"
	ReadinessMissing    = "missing"
	ReadinessSignedOut  = "signed_out"
	ReadinessNotServing = "not_serving"
	ReadinessUnknown    = "unknown"
)

// readinessTTL keeps the answer for this long: gh and tailscale are
// processes, and a phone reads every desktop again every half minute.
const readinessTTL = 2 * time.Minute

type ReadinessCheck struct {
	ID    string `json:"id"`
	State string `json:"state"`
	// Kind is the chat provider's kind on the model check.
	Kind string `json:"kind,omitempty"`
}

type readinessAnswer struct {
	Checks    []ReadinessCheck `json:"checks"`
	CheckedAt time.Time        `json:"checked_at"`
}

// ReadinessProbes reads the facts. Each is injected so the decision is
// tested without a gh, a tailscale or a claude on the machine.
type ReadinessProbes struct {
	// ModelKind is the chat tier's provider kind, "" when none resolves.
	ModelKind func() string
	// FindCLI finds the binary a CLI provider kind runs; nil error when found.
	FindCLI func(kind string) error
	// GH is focusprobe.GHAuthState.
	GH func(ctx context.Context) string
	// Tailscale is remoteaccess.Detect with the server's options.
	Tailscale func(ctx context.Context) (remoteaccess.Status, error)
	Now       func() time.Time
}

// cliProviderKinds run a binary on this desktop.
var cliProviderKinds = map[string]bool{"claude-code-cli": true, "antigravity-cli": true}

// ReadinessOf turns the facts into the three checks.
func ReadinessOf(kind string, cliErr error, gh string, ts remoteaccess.Status, tsErr error) []ReadinessCheck {
	kind = strings.ToLower(strings.TrimSpace(kind))
	model := ReadinessCheck{ID: "model", State: ReadinessOK, Kind: kind}
	switch {
	case kind == "":
		model.State = ReadinessMissing
	case cliProviderKinds[kind] && cliErr != nil:
		model.State = ReadinessMissing
	}

	ghCheck := ReadinessCheck{ID: "gh", State: gh}
	switch gh {
	case ReadinessOK, ReadinessMissing, ReadinessSignedOut:
	default:
		ghCheck.State = ReadinessUnknown
	}

	tailscale := ReadinessCheck{ID: "tailscale", State: ReadinessOK}
	switch {
	case !ts.Installed && tsErr == nil:
		tailscale.State = ReadinessMissing
	case tsErr != nil:
		tailscale.State = ReadinessUnknown
	case !ts.LoggedIn:
		tailscale.State = ReadinessSignedOut
	case ts.URL() == "":
		tailscale.State = ReadinessNotServing
	}
	return []ReadinessCheck{model, ghCheck, tailscale}
}

type focusReadiness struct {
	probes ReadinessProbes
	mu     sync.Mutex
	cached *readinessAnswer
}

// NewFocusReadinessHandler serves GET /v1/focus/readiness to the console's
// roles and to paired phones. The answer is cached for readinessTTL; while
// one read runs, others wait for it rather than starting their own.
func NewFocusReadinessHandler(probes ReadinessProbes) http.Handler {
	if probes.Now == nil {
		probes.Now = time.Now
	}
	h := &focusReadiness{probes: probes}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/focus/readiness", h.serve)
	return mux
}

func (h *focusReadiness) serve(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.probes.Now()
	if h.cached == nil || now.Sub(h.cached.CheckedAt) >= readinessTTL {
		answer := h.read(r.Context(), now)
		h.cached = &answer
	}
	httpapi.WriteJSON(w, http.StatusOK, h.cached)
}

func (h *focusReadiness) read(ctx context.Context, now time.Time) readinessAnswer {
	kind := ""
	if h.probes.ModelKind != nil {
		kind = h.probes.ModelKind()
	}
	var cliErr error
	if cliProviderKinds[strings.ToLower(strings.TrimSpace(kind))] && h.probes.FindCLI != nil {
		cliErr = h.probes.FindCLI(kind)
	}
	gh := ReadinessUnknown
	if h.probes.GH != nil {
		gh = h.probes.GH(ctx)
	}
	var ts remoteaccess.Status
	tsErr := error(errNoTailscaleProbe)
	if h.probes.Tailscale != nil {
		ts, tsErr = h.probes.Tailscale(ctx)
	}
	return readinessAnswer{Checks: ReadinessOf(kind, cliErr, gh, ts, tsErr), CheckedAt: now}
}

type readinessError string

func (e readinessError) Error() string { return string(e) }

const errNoTailscaleProbe = readinessError("no tailscale probe")
