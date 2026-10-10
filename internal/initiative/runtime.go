package initiative

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/jev"
	"github.com/devlikebear/tars/pkg/llm"
	"github.com/rs/zerolog"
)

const (
	recentKept    = 50
	idleEmotion   = "happy"
	historyWindow = 500
)

type Observer interface {
	Observe(ctx context.Context, now time.Time) (Observation, error)
}

// BodyActor performs a silent body expression and reports the outcome:
// delivered, or dropped:<reason>.
type BodyActor interface {
	Express(ctx context.Context, provider, emotion string) (string, error)
}

// BackendInfo describes the configured text-signal backend for status and
// doctor reporting (tars#1219). Configured/Loopback/Host are jev's original
// fields (Configured now generalizes to "can be called at all" —
// BackendPlan.Usable — for the llm backend too); Kind/Provider/Model/Tier,
// SendsText/TextReason and the call-cap fields are new.
type BackendInfo struct {
	// Backend is "llm" or "jev".
	Backend    string `json:"backend,omitempty"`
	Configured bool   `json:"configured"`
	Loopback   bool   `json:"loopback"`
	Host       string `json:"host,omitempty"`

	Kind     string `json:"kind,omitempty"`
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	Tier     string `json:"tier,omitempty"`

	// SendsText and TextReason mirror BackendPlan: whether the user's own
	// words go with a call to this backend, and why.
	SendsText  bool   `json:"sends_text"`
	TextReason string `json:"text_reason,omitempty"`

	// CallsToday/DailyCallCap are refreshed on every Snapshot from live
	// runtime state (the ledger-backed daily text-signal call cap).
	CallsToday   int `json:"calls_today"`
	DailyCallCap int `json:"daily_call_cap"`
}

type Dependencies struct {
	Observer Observer
	// SystemOne is the jev backend; set when Backend == "jev".
	SystemOne SystemOne
	// TextLLM is the llm backend (the initiative role's resolved client);
	// set when Backend == "llm". Takes priority over SystemOne when both
	// are set.
	TextLLM      llm.Client
	TextLLMModel string
	IncludeText  bool
	Body         BodyActor
	Ledger       *Ledger
	Logger       zerolog.Logger
	Now          func() time.Time
	Backend      BackendInfo
}

type Snapshot struct {
	Enabled   bool        `json:"enabled"`
	Mode      string      `json:"mode"`
	Backend   BackendInfo `json:"backend"`
	LastError string      `json:"last_error,omitempty"`
	Recent    []Entry     `json:"recent"`
}

// Runtime evaluates one tick at a time. In shadow mode it records what it
// would say and only ever acts through a silent body expression.
type Runtime struct {
	cfg  Config
	deps Dependencies
	text *textReader

	// mu serializes ticks and guards pacing state. snapMu guards only what
	// Snapshot reads, so status stays responsive while a tick waits on I/O.
	mu         sync.Mutex
	hist       History
	histLoaded bool
	lastReason string

	snapMu         sync.Mutex
	lastError      string
	recent         []Entry
	textCallsToday int

	lifeMu  sync.Mutex
	started bool
	stopped bool
	stopCh  chan struct{}
	doneCh  chan struct{}
}

func NewRuntime(cfg Config, deps Dependencies) *Runtime {
	cfg = cfg.WithDefaults()
	if deps.Now == nil {
		deps.Now = time.Now
	}
	text := newJevTextReader(deps.SystemOne, cfg.Thresholds)
	if deps.TextLLM != nil {
		text = newLLMTextReader(deps.TextLLM, deps.TextLLMModel)
	}
	return &Runtime{
		cfg:    cfg,
		deps:   deps,
		text:   text,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

func (r *Runtime) Start(ctx context.Context) {
	if r == nil || !r.cfg.Enabled {
		return
	}
	r.lifeMu.Lock()
	defer r.lifeMu.Unlock()
	if r.started || r.stopped {
		return
	}
	r.started = true
	go func() {
		defer close(r.doneCh)
		ticker := time.NewTicker(r.cfg.Tick)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-r.stopCh:
				return
			case <-ticker.C:
				r.RunOnce(ctx)
			}
		}
	}()
}

func (r *Runtime) Stop() {
	if r == nil {
		return
	}
	r.lifeMu.Lock()
	if r.stopped {
		r.lifeMu.Unlock()
		return
	}
	r.stopped = true
	close(r.stopCh)
	started := r.started
	r.lifeMu.Unlock()
	if started {
		<-r.doneCh
	}
}

// RunOnce observes, decides and records one tick.
func (r *Runtime) RunOnce(ctx context.Context) Entry {
	start := time.Now()
	now := r.deps.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loadHistoryLocked(now)
	if !r.hist.LastSpokeAt.IsZero() && r.localDay(r.hist.LastSpokeAt) != r.localDay(now) {
		r.hist.Today = 0
	}
	if !r.hist.LastTextCallAt.IsZero() && r.localDay(r.hist.LastTextCallAt) != r.localDay(now) {
		r.hist.TextCallsToday = 0
	}

	entry := Entry{At: now, Mode: r.cfg.Mode}
	obs, err := r.deps.Observer.Observe(ctx, now)
	if err != nil {
		entry.Decision = none("observe_error")
		entry.Error = err.Error()
		r.noteErrorLocked(entry.Error)
		r.rememberLocked(entry)
		return entry
	}
	obs.Now = now
	g := deriveGoSignals(r.cfg, obs, r.hist)
	entry.Signals = g

	// The backend is worth calling only when some combination of text
	// signals could change this tick's actual outcome (textSignalsMatter)
	// and the daily call cap still has room; both naturally skip a tick a
	// hard rule (quiet hours, cooldown, daily cap) or plain busy typing has
	// already fixed the outcome for, without special-casing any of them
	// here. Calling the backend at all is what the caller must opt into
	// (textSignalsMatter also returns false under the zero-value Config).
	text := TextSignals{Source: "skipped"}
	if textSignalsMatter(g) && r.hist.TextCallsToday < r.cfg.DailyTextCalls {
		state, key := RenderState(r.cfg, obs, r.deps.IncludeText)
		text, err = r.text.Read(ctx, key, state, textQuestionsFor(now.In(r.cfg.Location)))
		if err != nil {
			entry.Error = describeSystemOneError(err)
			r.noteErrorLocked(entry.Error)
		} else if text.Source == r.text.source {
			r.clearError()
		}
		entry.Called = text.Source == r.text.source || text.Source == "error"
	}
	entry.Text = text
	entry.Decision = Decide(g, text)
	if entry.Intent == IntentBodyOnly {
		entry.Body = r.express(ctx)
	}
	entry.LatencyMS = time.Since(start).Milliseconds()
	r.applyLocked(entry)
	return entry
}

func (r *Runtime) express(ctx context.Context) string {
	if r.deps.Body == nil || r.cfg.BodyProvider == "" {
		return "dropped:no_body"
	}
	outcome, err := r.deps.Body.Express(ctx, r.cfg.BodyProvider, idleEmotion)
	if err != nil {
		r.deps.Logger.Warn().Err(err).Msg("initiative: body expression failed")
		return "error"
	}
	return outcome
}

// applyLocked updates pacing and records the entry. Shadow decisions count
// toward pacing too, so the shadow ledger mirrors what live mode would do.
func (r *Runtime) applyLocked(e Entry) {
	if e.Speak {
		r.hist.Today++
		r.hist.LastSpokeAt = e.At
		if e.Intent == IntentCheckIn {
			r.hist.LastCheckInAt = e.At
		}
	}
	if e.Intent == IntentBodyOnly {
		// A failed attempt is spaced like a delivered one, so an offline
		// body is not retried (and logged) every tick.
		r.hist.LastBodyAt = e.At
	}
	if e.Called {
		r.hist.TextCallsToday++
		r.hist.LastTextCallAt = e.At
	}
	if e.Intent != IntentNone || e.Called || e.Reason != r.lastReason {
		if err := r.deps.Ledger.Append(e); err != nil {
			r.deps.Logger.Warn().Err(err).Msg("initiative: ledger append failed")
		}
	}
	r.rememberLocked(e)
}

func (r *Runtime) rememberLocked(e Entry) {
	r.lastReason = e.Reason
	r.snapMu.Lock()
	defer r.snapMu.Unlock()
	r.textCallsToday = r.hist.TextCallsToday
	r.recent = append(r.recent, e)
	if len(r.recent) > recentKept {
		r.recent = r.recent[len(r.recent)-recentKept:]
	}
}

func (r *Runtime) loadHistoryLocked(now time.Time) {
	if r.histLoaded {
		return
	}
	r.histLoaded = true
	entries, err := r.deps.Ledger.Recent(historyWindow)
	if err != nil {
		r.deps.Logger.Warn().Err(err).Msg("initiative: read ledger history failed")
		return
	}
	r.hist = historyFrom(entries, now, r.cfg.Location)
}

// noteErrorLocked logs an error once per change, not on every tick.
func (r *Runtime) noteErrorLocked(msg string) {
	r.snapMu.Lock()
	changed := msg != r.lastError
	r.lastError = msg
	r.snapMu.Unlock()
	if changed {
		r.deps.Logger.Warn().Str("error", msg).Msg("initiative: tick degraded")
	}
}

func (r *Runtime) clearError() {
	r.snapMu.Lock()
	r.lastError = ""
	r.snapMu.Unlock()
}

// describeSystemOneError keeps only the status of an HTTP failure. A
// server's error body can echo the request state, which on a same-provider
// llm backend or a loopback jev backend holds the user's words, and those
// must not reach the ledger, the logs, or GET /v1/initiative/status.
func describeSystemOneError(err error) string {
	var httpErr *jev.HTTPError
	if errors.As(err, &httpErr) {
		return fmt.Sprintf("jev: http %d", httpErr.Status)
	}
	var providerErr *llm.ProviderError
	if errors.As(err, &providerErr) && providerErr.StatusCode > 0 {
		return fmt.Sprintf("%s: http %d", providerErr.Provider, providerErr.StatusCode)
	}
	return err.Error()
}

func (r *Runtime) localDay(t time.Time) string { return t.In(r.cfg.Location).Format("2006-01-02") }

func (r *Runtime) Snapshot() Snapshot {
	if r == nil {
		return Snapshot{Recent: []Entry{}}
	}
	r.snapMu.Lock()
	defer r.snapMu.Unlock()
	backend := r.deps.Backend
	backend.CallsToday = r.textCallsToday
	backend.DailyCallCap = r.cfg.DailyTextCalls
	return Snapshot{
		Enabled:   r.cfg.Enabled,
		Mode:      r.cfg.Mode,
		Backend:   backend,
		LastError: r.lastError,
		Recent:    append([]Entry{}, r.recent...),
	}
}
