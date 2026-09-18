package computeruse

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/jev"
)

// Asker is the typed-decision side of the loop; *jev.Client satisfies it.
type Asker interface {
	Ask(ctx context.Context, state string, qs map[string]jev.Question) (jev.Response, error)
}

// Config tunes the loop's budgets and the thresholds the gates compare against.
type Config struct {
	MaxSteps       int
	StepTimeout    time.Duration
	TotalTimeout   time.Duration
	ResumeTTL      time.Duration
	ExposeValues   bool
	DoneThreshold  float64
	ActConfidence  float64
	RiskyThreshold float64
	InputTokenUSD  float64
}

func DefaultConfig() Config {
	return Config{
		MaxSteps: 25, StepTimeout: 15 * time.Second, TotalTimeout: 5 * time.Minute, ResumeTTL: 10 * time.Minute,
		ExposeValues: true, DoneThreshold: 0.85, ActConfidence: 0.70, RiskyThreshold: 0.50, InputTokenUSD: 0.042 / 1e6,
	}
}

const (
	maxStuck      = 3
	maxNoChange   = 3
	lookMaxDepth  = 40
	maxLastScreen = 20
	hardMaxSteps  = 50
)

// run is one loop's mutable state; it is parked in Engine.pending while a
// risky action waits for confirmation.
type run struct {
	req       Request
	window    Window
	trace     []TraceStep
	step      int
	stuck     int
	noChange  int
	lastHash  string
	looked    bool
	nextOpts  SnapshotOpts
	usage     Usage
	started   time.Time
	proposed  *ProposedAction
	pendingD  Decision
	pendingEl Element
	lastShown []Element
	createdAt time.Time
}

// Engine drives one GUI window through observe → decide → gate → act cycles.
type Engine struct {
	driver   Driver
	jev      Asker
	cfg      Config
	mu       sync.Mutex
	pending  map[string]*run
	now      func() time.Time
	newToken func() string
}

func NewEngine(driver Driver, asker Asker, cfg Config) *Engine {
	def := DefaultConfig()
	if cfg.MaxSteps <= 0 {
		cfg.MaxSteps = def.MaxSteps
	}
	if cfg.StepTimeout <= 0 {
		cfg.StepTimeout = def.StepTimeout
	}
	if cfg.TotalTimeout <= 0 {
		cfg.TotalTimeout = def.TotalTimeout
	}
	if cfg.ResumeTTL <= 0 {
		cfg.ResumeTTL = def.ResumeTTL
	}
	if cfg.DoneThreshold <= 0 {
		cfg.DoneThreshold = def.DoneThreshold
	}
	if cfg.ActConfidence <= 0 {
		cfg.ActConfidence = def.ActConfidence
	}
	if cfg.RiskyThreshold <= 0 {
		cfg.RiskyThreshold = def.RiskyThreshold
	}
	if cfg.InputTokenUSD <= 0 {
		cfg.InputTokenUSD = def.InputTokenUSD
	}
	return &Engine{driver: driver, jev: asker, cfg: cfg, pending: map[string]*run{}, now: time.Now, newToken: randomToken}
}

func randomToken() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "cu_" + hex.EncodeToString(b)
}

// Run drives req to a terminal status, or stops at the first action that needs
// the caller's confirmation.
func (e *Engine) Run(ctx context.Context, req Request) Result {
	if e.driver == nil || e.jev == nil {
		return Result{Status: StatusUnavailable, Reason: "computer_use is not configured", Hint: "set jev.api_key and install cua-driver"}
	}
	if err := e.driver.Ping(ctx); err != nil {
		return Result{Status: StatusUnavailable, Reason: err.Error(), Hint: "start the driver with `cua-driver serve` and grant Accessibility via `cua-driver permissions grant`"}
	}
	maxSteps := req.MaxSteps
	if maxSteps <= 0 {
		maxSteps = e.cfg.MaxSteps
	}
	if maxSteps > hardMaxSteps {
		maxSteps = hardMaxSteps
	}
	req.MaxSteps = maxSteps
	w, err := e.driver.ResolveWindow(ctx, req.App)
	if err != nil {
		return Result{Status: StatusError, Reason: err.Error()}
	}
	r := &run{req: req, window: w, started: e.now()}
	return e.loop(ctx, r, false)
}

// Resume continues (or drops) the run parked under token. Tokens are
// single-use: the entry is removed before it is inspected.
func (e *Engine) Resume(ctx context.Context, token string, confirm bool) Result {
	e.mu.Lock()
	r, ok := e.pending[token]
	delete(e.pending, token)
	e.mu.Unlock()
	if !ok {
		return Result{Status: StatusError, Reason: "resume token unknown or expired"}
	}
	if e.now().Sub(r.createdAt) > e.cfg.ResumeTTL {
		return Result{Status: StatusError, Reason: "resume token expired"}
	}
	if !confirm {
		return e.finish(r, Result{Status: StatusCancelled, Reason: "caller declined the proposed action"})
	}
	return e.loop(ctx, r, true)
}

// loop runs steps until a terminal status. confirmedFirst executes the parked
// decision before observing again.
func (e *Engine) loop(ctx context.Context, r *run, confirmedFirst bool) Result {
	ctx, cancel := context.WithTimeout(ctx, e.cfg.TotalTimeout)
	defer cancel()
	if confirmedFirst {
		e.execute(ctx, r, r.pendingD, r.pendingEl)
		r.proposed = nil
	}
	for r.step < r.req.MaxSteps {
		if ctx.Err() != nil {
			return e.finish(r, Result{Status: StatusError, Reason: "timed out"})
		}
		snap, err := e.snapshot(ctx, r)
		if err != nil {
			if errors.Is(err, ErrDriverUnavailable) {
				return e.finish(r, Result{Status: StatusUnavailable, Reason: err.Error()})
			}
			return e.finish(r, Result{Status: StatusError, Reason: err.Error()})
		}
		if snap.Degraded != "" {
			return e.finish(r, Result{Status: StatusStuck, Reason: "accessibility tree unavailable: " + snap.Degraded})
		}
		if h := ElementHash(snap); r.lastHash != "" && h == r.lastHash {
			r.noChange++
		} else {
			r.noChange = 0
			r.lastHash = h
		}
		if r.noChange >= maxNoChange {
			return e.finish(r, Result{Status: StatusStuck, Reason: "no_change: screen did not change after 3 actions"})
		}
		state, shown := RenderState(r.req, snap, r.trace, RenderOptions{ExposeValues: e.cfg.ExposeValues})
		r.lastShown = shown
		start := e.now()
		resp, err := e.jev.Ask(ctx, state, BuildQuestions(shown, inputKeys(r.req.Inputs)))
		if err != nil {
			return e.finish(r, Result{Status: StatusError, Reason: "jev: " + err.Error()})
		}
		r.usage.JevInputTokens += resp.Usage.InputTokens
		d, err := ParseDecision(resp)
		if err != nil {
			return e.finish(r, Result{Status: StatusError, Reason: err.Error()})
		}
		r.step++
		ts := TraceStep{Step: r.step, Op: d.Op, InputKey: d.InputKey, Confidence: d.OpConfidence, Risky: d.Risky, Done: d.Done,
			LatencyMS: e.now().Sub(start).Milliseconds(), InputTokens: resp.Usage.InputTokens}
		var el Element
		// Jev may name an eN that was never shown; the index must be range
		// checked before shown[...] is touched.
		outOfRange := d.TargetIndex > len(shown)
		if d.TargetIndex > 0 && !outOfRange {
			el = shown[d.TargetIndex-1]
			ts.Target = fmt.Sprintf("%s '%s'", el.Role, truncateRunes(el.Label, maxLabelRunes))
		}

		switch {
		case d.Done >= e.cfg.DoneThreshold || d.Op == OpDone:
			ts.Effect = "done"
			r.trace = append(r.trace, ts)
			return e.finish(r, Result{Status: StatusDone})
		case outOfRange:
			ts.Effect, ts.Note = "skipped", "target_out_of_range"
			r.stuck++
		case d.Op == OpNone || (NeedsTarget(d.Op) && d.TargetIndex == 0):
			ts.Effect, ts.Note = "skipped", "no_action"
			r.stuck++
		case d.OpConfidence < e.cfg.ActConfidence || (NeedsTarget(d.Op) && d.TargetConfidence < e.cfg.ActConfidence):
			if !r.looked {
				r.looked = true
				r.nextOpts = SnapshotOpts{MaxDepth: lookMaxDepth}
				ts.Effect, ts.Note = "look", "low_confidence_look"
			} else {
				ts.Effect, ts.Note = "skipped", "low_confidence"
				r.stuck++
			}
		case d.Op == OpLook:
			r.nextOpts = SnapshotOpts{MaxDepth: lookMaxDepth}
			if d.TargetIndex > 0 {
				r.nextOpts.Query = el.Label
			}
			ts.Effect = "look"
		case NeedsTarget(d.Op) && !Compatible(d.Op, el.Role):
			ts.Effect, ts.Note = "skipped", "incompatible"
			r.stuck++
		case d.Op == OpType && d.InputKey == "":
			ts.Effect, ts.Note = "skipped", "no_input_key"
			r.stuck++
		case d.Risky >= e.cfg.RiskyThreshold || (d.Op == OpType && el.Secure):
			// Secure fields are typeable (Compatible says yes) but never without
			// the caller's say-so, regardless of Jev's risk reading.
			r.proposed = &ProposedAction{Op: d.Op, Target: ts.Target, InputKey: d.InputKey, Risky: d.Risky}
			r.pendingD, r.pendingEl, r.createdAt = d, el, e.now()
			ts.Effect, ts.Note = "pending", "needs_confirmation"
			r.trace = append(r.trace, ts)
			token := e.newToken()
			e.mu.Lock()
			e.pending[token] = r
			e.mu.Unlock()
			res := e.finish(r, Result{Status: StatusNeedsConfirmation, Reason: "next action looks hard to undo", Resume: token})
			res.ProposedAction = r.proposed
			return res
		default:
			r.trace = append(r.trace, ts)
			e.execute(ctx, r, d, el)
			if r.stuck >= maxStuck {
				return e.finish(r, Result{Status: StatusStuck, Reason: "stuck: repeated failed or empty actions"})
			}
			continue
		}
		r.trace = append(r.trace, ts)
		if r.stuck >= maxStuck {
			return e.finish(r, Result{Status: StatusStuck, Reason: "stuck: repeated failed or empty actions"})
		}
	}
	return e.finish(r, Result{Status: StatusMaxSteps, Reason: fmt.Sprintf("reached max_steps=%d", r.req.MaxSteps)})
}

func (e *Engine) snapshot(ctx context.Context, r *run) (Snapshot, error) {
	opts := r.nextOpts
	r.nextOpts = SnapshotOpts{}
	sctx, cancel := context.WithTimeout(ctx, e.cfg.StepTimeout)
	defer cancel()
	return e.driver.Snapshot(sctx, r.window, opts)
}

// execute performs the decided action and records its effect on the last
// trace entry. Input VALUES are read here and nowhere else.
func (e *Engine) execute(ctx context.Context, r *run, d Decision, el Element) {
	actx, cancel := context.WithTimeout(ctx, e.cfg.StepTimeout)
	defer cancel()
	var eff Effect
	var err error
	switch d.Op {
	case OpClick:
		eff, err = e.driver.Click(actx, r.window, el.Token)
	case OpType:
		eff, err = e.driver.TypeText(actx, r.window, el.Token, r.req.Inputs[d.InputKey])
	case OpSetValue:
		value := "true"
		if el.Selected != nil && *el.Selected {
			value = "false"
		}
		if d.InputKey != "" {
			value = r.req.Inputs[d.InputKey]
		}
		eff, err = e.driver.SetValue(actx, r.window, el.Token, value)
	case OpPressEnter:
		eff, err = e.driver.PressKey(actx, r.window, "return")
	case OpPressEscape:
		eff, err = e.driver.PressKey(actx, r.window, "escape")
	case OpScrollDown:
		eff, err = e.driver.Scroll(actx, r.window, "down")
	case OpScrollUp:
		eff, err = e.driver.Scroll(actx, r.window, "up")
	}
	last := &r.trace[len(r.trace)-1]
	if err != nil {
		last.Effect = "error"
		last.Note = truncateRunes(err.Error(), 120)
		r.stuck++
		return
	}
	last.Effect = string(eff)
	if eff == EffectRefused || eff == EffectDeliveryFailed {
		if n := len(r.trace); n >= 2 && r.trace[n-2].Target == last.Target && (r.trace[n-2].Effect == string(EffectRefused) || r.trace[n-2].Effect == string(EffectDeliveryFailed)) {
			r.stuck++
		}
	}
}

func (e *Engine) finish(r *run, res Result) Result {
	res.Steps = r.step
	res.Trace = r.trace
	if res.Trace == nil {
		res.Trace = []TraceStep{}
	}
	r.usage.ElapsedMS = e.now().Sub(r.started).Milliseconds()
	r.usage.EstUSD = float64(r.usage.JevInputTokens) * e.cfg.InputTokenUSD
	res.Usage = r.usage
	for i, el := range r.lastShown {
		if i >= maxLastScreen {
			break
		}
		res.LastScreen = append(res.LastScreen, ElementLine(el, e.cfg.ExposeValues))
	}
	return res
}

func inputKeys(in map[string]string) []string {
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
