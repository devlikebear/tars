package initiative

import (
	"context"
	"sync"
	"time"

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

type BackendInfo struct {
	Configured bool   `json:"configured"`
	Loopback   bool   `json:"loopback"`
	Host       string `json:"host,omitempty"`
}

type Dependencies struct {
	Observer    Observer
	SystemOne   SystemOne
	IncludeText bool
	Body        BodyActor
	Ledger      *Ledger
	Logger      zerolog.Logger
	Now         func() time.Time
	Backend     BackendInfo
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

	mu         sync.Mutex
	hist       History
	histLoaded bool
	lastReason string
	lastError  string
	recent     []Entry

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
	return &Runtime{
		cfg:    cfg,
		deps:   deps,
		text:   newTextReader(deps.SystemOne, cfg.Thresholds),
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

	text := TextSignals{Source: "skipped"}
	if !g.QuietHours && !g.CooldownActive && !g.DailyCapReached {
		state, key := RenderState(r.cfg, obs, r.deps.IncludeText)
		text, err = r.text.Read(ctx, key, state, textQuestionsFor(now.In(r.cfg.Location)))
		if err != nil {
			entry.Error = err.Error()
			r.noteErrorLocked(entry.Error)
		} else if text.Source == "systemone" {
			r.lastError = ""
		}
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
	if e.Intent == IntentBodyOnly && e.Body == "delivered" {
		r.hist.LastBodyAt = e.At
	}
	if e.Intent != IntentNone || e.Text.Source == "systemone" || e.Reason != r.lastReason {
		if err := r.deps.Ledger.Append(e); err != nil {
			r.deps.Logger.Warn().Err(err).Msg("initiative: ledger append failed")
		}
	}
	r.rememberLocked(e)
}

func (r *Runtime) rememberLocked(e Entry) {
	r.lastReason = e.Reason
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
	if msg != r.lastError {
		r.deps.Logger.Warn().Str("error", msg).Msg("initiative: tick degraded")
	}
	r.lastError = msg
}

func (r *Runtime) localDay(t time.Time) string { return t.In(r.cfg.Location).Format("2006-01-02") }

func (r *Runtime) Snapshot() Snapshot {
	if r == nil {
		return Snapshot{Recent: []Entry{}}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return Snapshot{
		Enabled:   r.cfg.Enabled,
		Mode:      r.cfg.Mode,
		Backend:   r.deps.Backend,
		LastError: r.lastError,
		Recent:    append([]Entry{}, r.recent...),
	}
}
