package reflection

import (
	"context"
	"sync"
	"time"
)

// Runtime orchestrates the reflection tick loop. Unlike pulse, which
// ticks every minute, reflection ticks slowly (default every 5 minutes)
// because each tick does nothing unless a narrow sleep window is open
// AND today's run has not yet happened.
//
// The runtime is safe to construct with nil jobs/state — Start becomes a
// no-op and RunOnce returns a summary describing why it skipped.
type Runtime struct {
	cfg   Config
	jobs  []Job
	state *State
	now   func() time.Time

	mu      sync.Mutex
	started bool
	stopCh  chan struct{}
	doneCh  chan struct{}
	hook    func(summary RunSummary) // test observer
}

// NewRuntime constructs a Runtime with the given configuration, ordered
// job list, and shared state. Callers typically build one at server
// startup and call Start immediately.
func NewRuntime(cfg Config, jobs []Job, state *State) *Runtime {
	if state == nil {
		state = NewState(0)
	}
	return &Runtime{
		cfg:   cfg,
		jobs:  jobs,
		state: state,
		now:   time.Now,
	}
}

// Start begins the tick loop in a goroutine. Idempotent. A disabled
// runtime returns immediately without launching a goroutine.
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
	stopCh := r.stopCh
	doneCh := r.doneCh
	r.mu.Unlock()

	interval := r.cfg.EffectiveTickInterval()
	go r.loop(ctx, interval, stopCh, doneCh)
}

// Stop signals the tick loop to exit and waits for it to drain. Safe
// to call multiple times; the second call is a no-op.
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

// State returns the state handle used by the runtime. Pulse reads this
// directly to implement its reflection-failure signal.
func (r *Runtime) State() *State {
	if r == nil {
		return nil
	}
	return r.state
}

// Snapshot is a convenience wrapper around state.Snapshot.
func (r *Runtime) Snapshot() Snapshot {
	if r == nil {
		return Snapshot{}
	}
	return r.state.Snapshot()
}

// SetTickHook installs a callback invoked after every tick that actually
// ran jobs. Intended for tests; the hook must not block.
func (r *Runtime) SetTickHook(fn func(summary RunSummary)) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hook = fn
}

// RunOnce forces a reflection run now, bypassing the sleep-window gate.
// Tests and the HTTP run-once endpoint use this. It still returns an
// empty summary when reflection is disabled.
func (r *Runtime) RunOnce(ctx context.Context) RunSummary {
	if r == nil || !r.cfg.Enabled {
		return RunSummary{}
	}
	return r.execute(ctx)
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
			r.tick(parentCtx)
		}
	}
}

// tick is the slow-tick body. It consults the scheduler and only runs
// jobs when the scheduler says "run now".
func (r *Runtime) tick(parentCtx context.Context) {
	now := r.now()
	decision := decideTick(now, r.cfg, r.state.LastRunAt())
	if !decision.Run {
		return
	}
	r.state.MarkAttemptStarted(now)
	r.execute(parentCtx)
}

// execute runs every job sequentially and records the summary.
func (r *Runtime) execute(parentCtx context.Context) RunSummary {
	start := r.now()
	summary := RunSummary{StartedAt: start, Success: true}

	for _, job := range r.jobs {
		result := runJob(parentCtx, job)
		summary.Results = append(summary.Results, result)
		if !result.Success {
			summary.Success = false
		}
	}

	summary.FinishedAt = r.now()
	r.state.RecordRun(summary)

	r.mu.Lock()
	hook := r.hook
	r.mu.Unlock()
	if hook != nil {
		hook(summary)
	}
	return summary
}
