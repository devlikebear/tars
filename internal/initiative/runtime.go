package initiative

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
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
	// Speaker composes and delivers live-mode speech (tars#1220). nil
	// means live mode can never speak (every Speak-intent tick while
	// reachable records Delivery=skipped, Reason=speak_unavailable). Never
	// consulted in shadow mode.
	Speaker Speaker
	// SpeakSendsText mirrors IncludeText but for the speak role: whether
	// the speak composer's provider resolves to the same provider pool
	// alias as chat (tars#1219 §3's rule, applied to the speak role).
	SpeakSendsText bool
	// SpeakBackend describes the configured speak backend (tier, provider,
	// model, sends_text, reason) for GET /v1/initiative/status and `tars
	// doctor`, same shape as Backend. CallsToday/DailyCallCap are refreshed
	// on every Snapshot from DailySpeakCalls/the ledger-backed count.
	SpeakBackend BackendInfo
}

// DeliverySummary is the live-mode outcome of the most recent Speak-intent
// tick, surfaced by Snapshot/doctor without ever holding the composed
// words (tars#1220, tars#1003's rule).
type DeliverySummary struct {
	At       time.Time `json:"at"`
	Intent   string    `json:"intent"`
	Delivery string    `json:"delivery"`
	Reason   string    `json:"reason,omitempty"`
}

type Snapshot struct {
	Enabled      bool             `json:"enabled"`
	Mode         string           `json:"mode"`
	Backend      BackendInfo      `json:"backend"`
	SpeakBackend BackendInfo      `json:"speak_backend,omitempty"`
	LastDelivery *DeliverySummary `json:"last_delivery,omitempty"`
	// SpokenToday is how many initiatives counted as spoken today —
	// delivered-only in live mode, every Speak decision in shadow (mirrors
	// History.Today/Entry.spoken()).
	SpokenToday int     `json:"spoken_today"`
	LastError   string  `json:"last_error,omitempty"`
	Recent      []Entry `json:"recent"`
}

// Runtime evaluates one tick at a time. In shadow mode it records what it
// would say and only ever acts through a silent body expression.
type Runtime struct {
	cfg  Config
	deps Dependencies
	text *textReader

	// mu serializes ticks and guards pacing state. snapMu guards only what
	// Snapshot reads, so status stays responsive while a tick waits on I/O.
	mu                 sync.Mutex
	hist               History
	histLoaded         bool
	lastReason         string
	lastDelivery       string
	lastDeliveryReason string

	snapMu          sync.Mutex
	lastError       string
	recent          []Entry
	textCallsToday  int
	speakCallsToday int
	spokenToday     int
	// lastEntry is the most recently recorded entry, kept for parity with
	// speakCallsToday/textCallsToday though Snapshot reads lastDelivery
	// below for its reporting (tars#1220).
	lastEntry Entry
	// lastDeliverySummary is the most recent Speak-intent tick's outcome
	// (any entry with a non-empty Delivery), surfaced by Snapshot/doctor.
	// nil until the first such tick.
	lastDeliverySummary *DeliverySummary

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
	return r.runOnce(ctx, r.deps.Now())
}

// RunOnceAt is RunOnce with an explicit "now" instead of deps.Now(), for a
// test or e2e harness that needs to drive a tick deterministically without
// controlling the real clock (tars#1220's console E2E tick hook). Normal
// production code always uses RunOnce.
func (r *Runtime) RunOnceAt(ctx context.Context, now time.Time) Entry {
	return r.runOnce(ctx, now)
}

func (r *Runtime) runOnce(ctx context.Context, now time.Time) Entry {
	start := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loadHistoryLocked(now)
	if !r.hist.LastSpokeAt.IsZero() && r.localDay(r.hist.LastSpokeAt) != r.localDay(now) {
		r.hist.Today = 0
	}
	if !r.hist.LastTextCallAt.IsZero() && r.localDay(r.hist.LastTextCallAt) != r.localDay(now) {
		r.hist.TextCallsToday = 0
	}
	if !r.hist.LastSpeakCallAt.IsZero() && r.localDay(r.hist.LastSpeakCallAt) != r.localDay(now) {
		r.hist.SpeakCallsToday = 0
	}

	entry := Entry{ID: nextEntryID(now), At: now, Mode: r.cfg.Mode}
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
	// signals could change this tick's actual outcome (textSignalsMatter),
	// the daily call cap still has room, and a run of consecutive failures
	// has not put the read on backoff — all three naturally skip a tick a
	// hard rule (quiet hours, cooldown, daily cap), plain busy typing, or a
	// stuck backend has already fixed the outcome for, without
	// special-casing any of them here. Calling the backend at all is what
	// the caller must opt into (textSignalsMatter also returns false under
	// the zero-value Config).
	text := TextSignals{Source: "skipped"}
	if textSignalsMatter(g) && r.hist.TextCallsToday < r.cfg.DailyTextCalls && r.textReadEligible(now) {
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
	switch {
	case entry.Intent == IntentBodyOnly:
		entry.Body = r.express(ctx)
		if r.cfg.Live() {
			r.notifyBodyOnly(ctx)
		}
	case entry.Speak && r.cfg.Live():
		r.deliverLocked(ctx, &entry, obs)
	}
	entry.LatencyMS = time.Since(start).Milliseconds()
	r.applyLocked(entry)
	return entry
}

// textReadEligible reports whether the text-signal backend may be called
// this tick, given any consecutive-failure backoff in effect. A zero
// TextReadFailCount (never failed, or the last read succeeded) is always
// eligible.
func (r *Runtime) textReadEligible(now time.Time) bool {
	if r.hist.TextReadFailCount <= 0 {
		return true
	}
	return !now.Before(r.hist.LastTextCallAt.Add(backoffDuration(r.hist.TextReadFailCount)))
}

// deliverLocked decides whether to call the speak composer this tick and
// records the outcome on entry.Delivery/DeliveryReason/Composed. Called
// only when entry.Speak and live mode. It never calls Speak when the
// console is not connected (Observation.ConsoleConnectedAt) — the words
// are never composed for a tick nobody can see.
func (r *Runtime) deliverLocked(ctx context.Context, entry *Entry, obs Observation) {
	if obs.ConsoleConnectedAt.IsZero() {
		entry.Delivery = DeliveryUnreachable
		entry.DeliveryReason = deliveryReasonNoConsole
		return
	}
	if r.deps.Speaker == nil {
		entry.Delivery = DeliverySkipped
		entry.DeliveryReason = deliveryReasonSpeakUnavailable
		return
	}
	if r.hist.SpeakFailCount > 0 && entry.At.Before(r.hist.LastSpeakCallAt.Add(backoffDuration(r.hist.SpeakFailCount))) {
		entry.Delivery = DeliverySkipped
		entry.DeliveryReason = deliveryReasonBackoff
		return
	}
	if r.hist.SpeakCallsToday >= r.cfg.DailySpeakCalls {
		entry.Delivery = DeliverySkipped
		entry.DeliveryReason = deliveryReasonDailySpeakCap
		return
	}
	req := SpeakRequest{
		Intent:     entry.Intent,
		EntryID:    entry.ID,
		Now:        entry.At,
		SendsText:  r.deps.SpeakSendsText,
		RecentUser: obs.RecentUser,
		Profile:    obs.Profile,
	}
	outcome, err := r.deps.Speaker.Speak(ctx, req)
	entry.Composed = outcome.Composed
	switch {
	case outcome.Delivered:
		entry.Delivery = DeliveryDelivered
		entry.DeliveryReason = deliveryReasonDelivered
		r.clearError()
	case outcome.Composed:
		entry.Delivery = DeliveryError
		entry.DeliveryReason = outcome.Reason
		if entry.DeliveryReason == "" {
			entry.DeliveryReason = deliveryReasonComposeError
		}
	default:
		entry.Delivery = DeliverySkipped
		entry.DeliveryReason = outcome.Reason
		if entry.DeliveryReason == "" {
			entry.DeliveryReason = deliveryReasonBusy
		}
	}
	if err != nil {
		r.noteErrorLocked(err.Error())
	}
}

// notifyBodyOnly sends the companion-event half of a body_only tick
// (tars#1220): best-effort, logged on failure, never recorded on the Entry
// (a body_only tick's only recorded outcome stays entry.Body, as before).
func (r *Runtime) notifyBodyOnly(ctx context.Context) {
	if r.deps.Speaker == nil {
		return
	}
	if err := r.deps.Speaker.NotifyBodyOnly(ctx); err != nil {
		r.deps.Logger.Warn().Err(err).Msg("initiative: body_only companion notify failed")
	}
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
// toward pacing too (e.spoken() is true for every Speak in shadow), so the
// shadow ledger mirrors what live mode would do; in live mode, only an
// actually delivered speak (Delivery == DeliveryDelivered) counts, so an
// unreachable console or a skipped/failed attempt never starts a cooldown
// or eats into the daily cap for a sentence nobody heard.
func (r *Runtime) applyLocked(e Entry) {
	if e.spoken() {
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
		if e.Text.Source == "error" {
			r.hist.TextReadFailCount++
		} else {
			r.hist.TextReadFailCount = 0
		}
	}
	if e.Composed {
		r.hist.SpeakCallsToday++
		r.hist.LastSpeakCallAt = e.At
		if e.Delivery == DeliveryDelivered {
			r.hist.SpeakFailCount = 0
		} else {
			r.hist.SpeakFailCount++
		}
	}
	// A tick worth a ledger line is one where something changed (the
	// decision's reason, or live mode's delivery outcome/reason) or where
	// a real backend call happened (text-signal read or speak compose) —
	// each call is worth recording even with an unchanged outcome (tuning
	// data, tars#1003), but an unchanged reason/delivery with no call
	// (e.g. the same "unreachable" tick after tick) is not: that would log
	// forever for a console that stays disconnected, since live mode does
	// not advance cooldown/daily-cap pacing for a non-delivered Speak the
	// way shadow's unconditional "Intent != none" used to.
	changed := e.Reason != r.lastReason || e.Delivery != r.lastDelivery || e.DeliveryReason != r.lastDeliveryReason
	if changed || e.Called || e.Composed {
		if err := r.deps.Ledger.Append(e); err != nil {
			r.deps.Logger.Warn().Err(err).Msg("initiative: ledger append failed")
		}
	}
	r.rememberLocked(e)
}

func (r *Runtime) rememberLocked(e Entry) {
	r.lastReason = e.Reason
	r.lastDelivery = e.Delivery
	r.lastDeliveryReason = e.DeliveryReason
	r.snapMu.Lock()
	defer r.snapMu.Unlock()
	r.textCallsToday = r.hist.TextCallsToday
	r.speakCallsToday = r.hist.SpeakCallsToday
	r.spokenToday = r.hist.Today
	r.lastEntry = e
	if e.Delivery != "" {
		r.lastDeliverySummary = &DeliverySummary{
			At: e.At, Intent: string(e.Intent), Delivery: e.Delivery, Reason: e.DeliveryReason,
		}
	}
	r.recent = append(r.recent, e)
	if len(r.recent) > recentKept {
		r.recent = r.recent[len(r.recent)-recentKept:]
	}
}

// entryIDCounter disambiguates entries created within the same
// nanosecond — tests (and a very fast clock) can call RunOnce repeatedly
// with a frozen Now().
var entryIDCounter atomic.Uint64

// nextEntryID returns a new, unique-enough id for one Entry: the id a
// delivered speak's assistant message references in its Initiative
// metadata (tars#1220), without storing the words themselves twice.
func nextEntryID(now time.Time) string {
	n := entryIDCounter.Add(1)
	return fmt.Sprintf("init_%d_%d", now.UnixNano(), n)
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
	// Restore the last delivery summary too, so a restart's first
	// Snapshot/doctor report isn't blank until the next Speak-intent tick.
	for i := len(entries) - 1; i >= 0; i-- {
		if e := entries[i]; e.Delivery != "" {
			r.snapMu.Lock()
			r.lastDeliverySummary = &DeliverySummary{
				At: e.At, Intent: string(e.Intent), Delivery: e.Delivery, Reason: e.DeliveryReason,
			}
			r.snapMu.Unlock()
			break
		}
	}
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
	speakBackend := r.deps.SpeakBackend
	speakBackend.CallsToday = r.speakCallsToday
	speakBackend.DailyCallCap = r.cfg.DailySpeakCalls
	return Snapshot{
		Enabled:      r.cfg.Enabled,
		Mode:         r.cfg.Mode,
		Backend:      backend,
		SpeakBackend: speakBackend,
		LastDelivery: r.lastDeliverySummary,
		SpokenToday:  r.spokenToday,
		LastError:    r.lastError,
		Recent:       append([]Entry{}, r.recent...),
	}
}
