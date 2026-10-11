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
	// Channel/ChannelError mirror Entry's fields (tars#1221 P2c) — which
	// transport the delivery used, and a fixed code for a channel send
	// failure after the main-session write already succeeded. Never the
	// composed words, a chat id, or a token.
	Channel      string `json:"channel,omitempty"`
	ChannelError string `json:"channel_error,omitempty"`
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
	// Outcomes is reply tracking's aggregate (tars#1221): today's and the
	// recent window's replied/ignored/stopped counts, plus the cooldown
	// currently in force and whether a "stopped" outcome blocks the rest
	// of today. Always present (zero-valued when reply tracking has
	// nothing yet, or in shadow mode, which never records outcomes).
	Outcomes OutcomeSummary `json:"outcomes"`
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
	// consecutiveIgnored and lastStoppedAt mirror the authoritative
	// hist fields of the same name (tars#1221), the same way
	// textCallsToday/speakCallsToday/spokenToday already mirror History —
	// so Snapshot can report them without touching hist under mu.
	consecutiveIgnored int
	lastStoppedAt      time.Time
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
	// signals could change this tick's actual outcome (textSignalsMatter)
	// or a pending reply would resolve this tick and needs this message's
	// quiet_requested reading to tell replied from stopped apart
	// (pendingReplyNeedsText, tars#1221) — and, either way, only when the
	// daily call cap still has room and a run of consecutive failures has
	// not put the read on backoff. These naturally skip a tick a hard rule
	// (quiet hours, cooldown, daily cap), plain busy typing, or a stuck
	// backend has already fixed the outcome for, without special-casing
	// any of them here. Calling the backend at all is what the caller
	// must opt into (textSignalsMatter also returns false under the
	// zero-value Config).
	text := TextSignals{Source: "skipped"}
	// textStatus classifies what this tick's read actually achieved —
	// consulted only by resolveRepliesLocked, for a pending reply within
	// its window (tars#1221 review): TextReadUnavailable by default (no
	// read attempted at all), refined below to
	// TextReadTemporarilyUnavailable or TextReadOK whenever a read is
	// actually attempted. A tick where no pending reply needs this
	// distinction (pendingReplyNeedsText false and textSignalsMatter
	// false) never looks at it.
	textStatus := TextReadUnavailable
	needsText := textSignalsMatter(g) || pendingReplyNeedsText(r.hist.PendingReplies, obs.MainUserAt, r.cfg.ReplyWindow)
	switch {
	case !needsText:
		// No combination of text signals matters and no pending reply is
		// within its window — textStatus is not consulted either way.
	case r.hist.TextCallsToday >= r.cfg.DailyTextCalls:
		// Daily call cap exhausted: structurally unavailable for the rest
		// of today, same as not sendsText — waiting won't help.
	case !r.textReadEligible(now):
		// Read-failure backoff in effect: may succeed once it lifts.
		textStatus = TextReadTemporarilyUnavailable
	default:
		state, key := RenderState(r.cfg, obs, r.deps.IncludeText)
		text, err = r.text.Read(ctx, key, state, textQuestionsFor(now.In(r.cfg.Location)))
		switch {
		case err != nil:
			entry.Error = describeSystemOneError(err)
			r.noteErrorLocked(entry.Error)
			textStatus = TextReadTemporarilyUnavailable
		case text.Source == r.text.source:
			r.clearError()
			textStatus = TextReadOK
		case text.Source == "cache":
			// A cached answer is still a trustworthy prior read, even
			// though it does not count as a fresh call below.
			textStatus = TextReadOK
		default:
			// "skipped" from within Read itself: either sendsText is
			// false (RenderState never includes text) or there was
			// nothing to render this tick — either way, structurally no
			// quiet_requested answer is coming from this tick's attempt.
			textStatus = TextReadUnavailable
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
	r.resolveRepliesLocked(now, obs, text, textStatus)
	entry.LatencyMS = time.Since(start).Milliseconds()
	r.applyLocked(entry)
	return entry
}

// resolveRepliesLocked resolves every pending delivered speak that this
// tick's observation (a main-session reply, or the reply window simply
// elapsing) now settles, records each as a separate outcome Entry, and
// updates the consecutive-ignored/last-stopped pacing state (tars#1221).
// textStatus (tars#1221 review) tells ResolveReplies whether this tick's
// quietRequested is trustworthy, so a transient read failure or backoff
// leaves a reply within its window pending rather than locking in the
// wrong outcome. Always a no-op in shadow mode: shadow never delivers, so
// r.hist.PendingReplies never gains an entry (see applyLocked).
func (r *Runtime) resolveRepliesLocked(now time.Time, obs Observation, text TextSignals, textStatus TextReadStatus) {
	resolved, remaining := ResolveReplies(r.hist.PendingReplies, obs.MainUserAt, textStatus, text.QuietRequested, now, r.cfg.ReplyWindow)
	r.hist.PendingReplies = remaining
	for _, res := range resolved {
		r.applyOutcomeLocked(Entry{
			ID:      nextEntryID(now),
			At:      now,
			Mode:    r.cfg.Mode,
			Kind:    entryKindOutcome,
			Ref:     res.EntryID,
			Outcome: res.Outcome,
		})
	}
}

// applyOutcomeLocked records one resolved reply outcome: updates the
// consecutive-ignored streak and last-stopped timestamp the same way
// ReplyState would replay them from the ledger, appends the entry to the
// ledger, and keeps it in Snapshot's recent window. Unlike applyLocked,
// it never touches lastReason/lastDelivery/lastDeliveryReason — those
// drive the *next tick entry's* own dedup check, which an outcome entry
// (Kind == entryKindOutcome, no Reason/Delivery of its own) must not
// affect.
func (r *Runtime) applyOutcomeLocked(e Entry) {
	switch e.Outcome {
	case OutcomeReplied, OutcomeStopped:
		r.hist.ConsecutiveIgnored = 0
	case OutcomeIgnored:
		r.hist.ConsecutiveIgnored++
	}
	if e.Outcome == OutcomeStopped && e.At.After(r.hist.LastStoppedAt) {
		r.hist.LastStoppedAt = e.At
	}
	if err := r.deps.Ledger.Append(e); err != nil {
		r.deps.Logger.Warn().Err(err).Msg("initiative: ledger append failed")
	}
	r.snapMu.Lock()
	r.consecutiveIgnored = r.hist.ConsecutiveIgnored
	r.lastStoppedAt = r.hist.LastStoppedAt
	r.recent = append(r.recent, e)
	if len(r.recent) > recentKept {
		r.recent = r.recent[len(r.recent)-recentKept:]
	}
	r.snapMu.Unlock()
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
// records the outcome on entry.Delivery/DeliveryReason/Composed/Channel.
// Called only when entry.Speak and live mode. The channel is picked before
// any compose call: console whenever connected, else telegram only when
// Config.Telegram is on and Observation.TelegramReady — never both, and
// never telegram when Config.Telegram is off (that leaves the entry
// byte-for-byte what it was before this field existed: unreachable,
// no_console, no Channel). Neither channel composes for a tick nobody can
// see.
func (r *Runtime) deliverLocked(ctx context.Context, entry *Entry, obs Observation) {
	channel := ""
	switch {
	case !obs.ConsoleConnectedAt.IsZero():
		channel = ChannelConsole
	case r.cfg.Telegram && obs.TelegramReady:
		channel = ChannelTelegram
	}
	if channel == "" {
		entry.Delivery = DeliveryUnreachable
		if r.cfg.Telegram {
			entry.DeliveryReason = deliveryReasonTelegramUnavailable
		} else {
			entry.DeliveryReason = deliveryReasonNoConsole
		}
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
		Channel:    channel,
	}
	outcome, err := r.deps.Speaker.Speak(ctx, req)
	entry.Composed = outcome.Composed
	switch {
	case outcome.Delivered:
		entry.Delivery = DeliveryDelivered
		entry.DeliveryReason = deliveryReasonDelivered
		entry.Channel = channel
		entry.ChannelError = outcome.ChannelError
		r.clearError()
	case outcome.Superseded:
		// Compose ran (and counts below toward SpeakCallsToday like any
		// other Composed attempt) but the text was deliberately discarded
		// unwritten — the claim was unavailable, or the user's own
		// message already landed first. Skipped, not Error: applyLocked
		// must not treat this as a compose failure (no SpeakFailCount
		// increase) or as a delivered speak (no cooldown/daily-cap pacing).
		entry.Delivery = DeliverySkipped
		entry.DeliveryReason = outcome.Reason
		if entry.DeliveryReason == "" {
			entry.DeliveryReason = deliveryReasonBusy
		}
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
		switch e.Delivery {
		case DeliveryDelivered:
			r.hist.SpeakFailCount = 0
		case DeliveryError:
			r.hist.SpeakFailCount++
		case DeliverySkipped:
			// Composed is only ever true alongside DeliverySkipped for a
			// Superseded attempt (tars#1220 review): Compose ran and
			// produced real text, but the claim was unavailable or the
			// user had already spoken, so it was discarded unwritten —
			// not a compose failure. Leave SpeakFailCount exactly as is:
			// neither reset (nothing was actually delivered) nor
			// incremented (nothing actually failed), so a run of
			// superseded attempts does not fabricate backoff for a
			// backend that is composing successfully every time.
		}
	}
	if e.Delivery == DeliveryDelivered {
		// Starts this delivered speak's reply-tracking window (tars#1221):
		// resolveRepliesLocked (earlier this same tick, before this entry
		// existed) never sees it; the earliest it can resolve is next
		// tick.
		r.hist.PendingReplies = append(r.hist.PendingReplies, PendingReply{EntryID: e.ID, At: e.At})
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
	r.consecutiveIgnored = r.hist.ConsecutiveIgnored
	r.lastStoppedAt = r.hist.LastStoppedAt
	r.lastEntry = e
	if e.Delivery != "" {
		r.lastDeliverySummary = &DeliverySummary{
			At: e.At, Intent: string(e.Intent), Delivery: e.Delivery, Reason: e.DeliveryReason,
			Channel: e.Channel, ChannelError: e.ChannelError,
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
		// Keep what was read: an empty history would forget today's count
		// and the cooldown.
		r.deps.Logger.Warn().Err(err).Int("entries", len(entries)).Msg("initiative: read ledger history failed")
	}
	r.hist = historyFrom(entries, now, r.cfg.Location)
	r.snapMu.Lock()
	r.consecutiveIgnored = r.hist.ConsecutiveIgnored
	r.lastStoppedAt = r.hist.LastStoppedAt
	r.snapMu.Unlock()
	// Restore the last delivery summary too, so a restart's first
	// Snapshot/doctor report isn't blank until the next Speak-intent tick.
	for i := len(entries) - 1; i >= 0; i-- {
		if e := entries[i]; e.Delivery != "" {
			r.snapMu.Lock()
			r.lastDeliverySummary = &DeliverySummary{
				At: e.At, Intent: string(e.Intent), Delivery: e.Delivery, Reason: e.DeliveryReason,
				Channel: e.Channel, ChannelError: e.ChannelError,
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
	outcomes := SummarizeOutcomes(r.recent, r.deps.Now(), r.cfg.Location, r.cfg.Cooldown, r.consecutiveIgnored, r.lastStoppedAt)
	return Snapshot{
		Enabled:      r.cfg.Enabled,
		Mode:         r.cfg.Mode,
		Backend:      backend,
		SpeakBackend: speakBackend,
		LastDelivery: r.lastDeliverySummary,
		SpokenToday:  r.spokenToday,
		LastError:    r.lastError,
		Recent:       append([]Entry{}, r.recent...),
		Outcomes:     outcomes,
	}
}
