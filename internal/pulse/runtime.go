package pulse

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/pulse/autofix"
)

// Config is the runtime configuration for pulse. It is populated from
// the main server config and passed once at startup. Changes require a
// restart — pulse does not watch its config file.
type Config struct {
	Enabled     bool
	Interval    time.Duration // default 1m
	Timeout     time.Duration // decider LLM call timeout, default 2m
	ActiveHours string        // "HH:MM-HH:MM" in Timezone, default "00:00-24:00"
	Timezone    string        // IANA name or "Local"; default "Local"
}

func (c Config) effectiveInterval() time.Duration {
	if c.Interval <= 0 {
		return time.Minute
	}
	return c.Interval
}

func (c Config) effectiveTimeout() time.Duration {
	if c.Timeout <= 0 {
		return 2 * time.Minute
	}
	return c.Timeout
}

// Dependencies bundles the wired-up collaborators that a Runtime needs.
// Any field may be nil; a Runtime constructed with nil scanner or nil
// decider is a no-op runtime, which is convenient for wiring code that
// hasn't finished building its dependencies yet.
type Dependencies struct {
	Scanner   *Scanner
	Decider   *Decider
	Router    *NotifyRouter
	Autofixes *autofix.Registry
	State     *State
}

// Runtime owns the pulse tick loop. It is single-instance per server;
// constructing more than one is allowed but uncommon.
type Runtime struct {
	cfg  Config
	deps Dependencies

	mu       sync.Mutex
	started  bool
	stopCh   chan struct{}
	doneCh   chan struct{}
	tickHook func(outcome TickOutcome) // test hook
}

// NewRuntime constructs a runtime. Call Start to begin the tick loop.
func NewRuntime(cfg Config, deps Dependencies) *Runtime {
	return &Runtime{cfg: cfg, deps: deps}
}

// Start begins the tick loop in a goroutine. Start is idempotent;
// calling it twice has no effect. The runtime stops when Stop is called
// or when the provided parent context is canceled.
//
// A disabled runtime (cfg.Enabled == false) is a no-op: Start returns
// immediately without launching a goroutine.
func (r *Runtime) Start(ctx context.Context) {
	if r == nil || !r.cfg.Enabled {
		return
	}
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return
	}
	r.started = true
	r.stopCh = make(chan struct{})
	r.doneCh = make(chan struct{})
	r.mu.Unlock()

	interval := r.cfg.effectiveInterval()
	stopCh := r.stopCh
	doneCh := r.doneCh
	go r.loop(ctx, interval, stopCh, doneCh)
}

// Stop signals the tick loop to exit and waits for it to drain. Stop is
// safe to call multiple times.
func (r *Runtime) Stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if !r.started {
		r.mu.Unlock()
		return
	}
	stopCh := r.stopCh
	doneCh := r.doneCh
	r.started = false
	r.stopCh = nil
	r.mu.Unlock()

	close(stopCh)
	<-doneCh
}

// SetTickHook installs a callback invoked after every tick with the
// tick's outcome. It is intended for tests and observability; the hook
// must not block the loop.
func (r *Runtime) SetTickHook(fn func(outcome TickOutcome)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tickHook = fn
}

// Snapshot returns the current runtime state. Safe to call from any
// goroutine, including while the loop is running.
func (r *Runtime) Snapshot() Snapshot {
	if r == nil || r.deps.State == nil {
		return Snapshot{}
	}
	return r.deps.State.Snapshot()
}

func (r *Runtime) loop(parentCtx context.Context, interval time.Duration, stopCh <-chan struct{}, doneCh chan<- struct{}) {
	defer close(doneCh)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-parentCtx.Done():
			return
		case <-stopCh:
			return
		case <-ticker.C:
			outcome := r.runTick(parentCtx)
			r.record(outcome)
		}
	}
}

// RunOnce executes a single tick synchronously and returns its outcome.
// It is safe to call even when the loop is not running; tests use it to
// exercise the orchestration without waiting for a ticker.
func (r *Runtime) RunOnce(ctx context.Context) TickOutcome {
	if r == nil {
		return TickOutcome{}
	}
	outcome := r.runTick(ctx)
	r.record(outcome)
	return outcome
}

func (r *Runtime) record(outcome TickOutcome) {
	if r.deps.State != nil {
		r.deps.State.RecordTick(outcome)
	}
	r.mu.Lock()
	hook := r.tickHook
	r.mu.Unlock()
	if hook != nil {
		hook(outcome)
	}
}

// runTick is the synchronous per-tick pipeline. It never panics; any
// error becomes a field on the returned outcome. The decider call is
// wrapped in a timeout so a hung LLM does not block subsequent ticks.
func (r *Runtime) runTick(parentCtx context.Context) TickOutcome {
	now := time.Now()
	outcome := TickOutcome{At: now}

	// 1. Active hours gate.
	if ok, reason := r.withinActiveHours(now); !ok {
		outcome.Skipped = true
		outcome.SkipReason = reason
		return outcome
	}

	// 2. Scan signals.
	if r.deps.Scanner == nil {
		outcome.Skipped = true
		outcome.SkipReason = "no_scanner"
		return outcome
	}
	signals := r.deps.Scanner.Scan(parentCtx)
	outcome.Signals = signals
	if len(signals) == 0 {
		return outcome
	}

	// 3. Decide (with timeout).
	if r.deps.Decider == nil {
		outcome.Err = "no_decider"
		return outcome
	}
	decideCtx, cancel := context.WithTimeout(parentCtx, r.cfg.effectiveTimeout())
	defer cancel()
	outcome.DeciderInvoked = true
	decision, err := r.deps.Decider.Decide(decideCtx, signals)
	if err != nil {
		outcome.Err = fmt.Sprintf("decider: %s", err.Error())
		return outcome
	}
	dc := decision
	outcome.Decision = &dc

	// 4. Act on decision.
	switch decision.Action {
	case ActionIgnore:
		// nothing to do
	case ActionNotify:
		if r.deps.Router != nil {
			delivered, nerr := r.deps.Router.Route(parentCtx, decision)
			outcome.NotifyDelivered = delivered
			if nerr != nil {
				outcome.Err = fmt.Sprintf("notify: %s", nerr.Error())
			}
		}
	case ActionAutofix:
		r.runAutofix(parentCtx, &outcome, decision)
	}
	return outcome
}

func (r *Runtime) runAutofix(ctx context.Context, outcome *TickOutcome, decision Decision) {
	outcome.AutofixAttempt = decision.AutofixName
	if r.deps.Autofixes == nil {
		outcome.AutofixErr = "no_registry"
		return
	}
	result, err := r.deps.Autofixes.Run(ctx, decision.AutofixName)
	if err != nil {
		outcome.AutofixErr = err.Error()
		return
	}
	outcome.AutofixOK = true
	if outcome.Decision.Details == nil {
		outcome.Decision.Details = map[string]any{}
	}
	outcome.Decision.Details["autofix_result"] = result
}

// withinActiveHours returns (true, "") if now is inside the configured
// window, or (false, reason) otherwise. A missing or malformed window
// is treated as "always active" so operators can omit it from config.
func (r *Runtime) withinActiveHours(now time.Time) (bool, string) {
	window := strings.TrimSpace(r.cfg.ActiveHours)
	if window == "" || window == "00:00-24:00" || window == "00:00-00:00" {
		return true, ""
	}
	loc := time.Local
	if tz := strings.TrimSpace(r.cfg.Timezone); tz != "" && tz != "Local" {
		if parsed, err := time.LoadLocation(tz); err == nil {
			loc = parsed
		}
	}
	local := now.In(loc)
	startMin, endMin, err := parseActiveHours(window)
	if err != nil {
		return true, ""
	}
	nowMin := local.Hour()*60 + local.Minute()
	// Window may wrap past midnight (e.g. 22:00-02:00).
	if startMin <= endMin {
		if nowMin >= startMin && nowMin < endMin {
			return true, ""
		}
	} else {
		if nowMin >= startMin || nowMin < endMin {
			return true, ""
		}
	}
	return false, "outside_active_hours"
}

// parseActiveHours parses "HH:MM-HH:MM" and returns (startMin, endMin).
// Any deviation from the exact format is an error — unlike the previous
// heartbeat implementation, we do not silently fall back to defaults.
func parseActiveHours(s string) (int, int, error) {
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return 0, 0, errors.New("expected HH:MM-HH:MM")
	}
	start, err := parseClockMinutes(parts[0])
	if err != nil {
		return 0, 0, err
	}
	end, err := parseClockMinutes(parts[1])
	if err != nil {
		return 0, 0, err
	}
	return start, end, nil
}

func parseClockMinutes(s string) (int, error) {
	s = strings.TrimSpace(s)
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid time %q", s)
	}
	var h, m int
	if _, err := fmt.Sscanf(parts[0], "%d", &h); err != nil {
		return 0, fmt.Errorf("invalid hour in %q", s)
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &m); err != nil {
		return 0, fmt.Errorf("invalid minute in %q", s)
	}
	if h < 0 || h > 24 || m < 0 || m >= 60 {
		return 0, fmt.Errorf("out of range: %q", s)
	}
	if h == 24 {
		h = 24
		m = 0
	}
	return h*60 + m, nil
}
