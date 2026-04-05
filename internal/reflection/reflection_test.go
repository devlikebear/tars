package reflection

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubJob struct {
	name   string
	result JobResult
	err    error
	called int
}

func (s *stubJob) Name() string { return s.name }
func (s *stubJob) Run(ctx context.Context) (JobResult, error) {
	s.called++
	if s.err != nil {
		return JobResult{}, s.err
	}
	return s.result, nil
}

type panicJob struct{ name string }

func (p *panicJob) Name() string                               { return p.name }
func (p *panicJob) Run(ctx context.Context) (JobResult, error) { panic("boom") }

func TestRunJobSuccess(t *testing.T) {
	job := &stubJob{name: "memory", result: JobResult{Changed: true, Summary: "ok"}}
	r := runJob(context.Background(), job)
	if !r.Success || !r.Changed || r.Name != "memory" {
		t.Errorf("unexpected result: %+v", r)
	}
	if r.Duration < 0 {
		t.Error("duration should be non-negative")
	}
}

func TestRunJobError(t *testing.T) {
	job := &stubJob{name: "broken", err: errors.New("disk full")}
	r := runJob(context.Background(), job)
	if r.Success || r.Err != "disk full" {
		t.Errorf("unexpected result: %+v", r)
	}
}

func TestRunJobPanicRecovers(t *testing.T) {
	r := runJob(context.Background(), &panicJob{name: "crasher"})
	if r.Success {
		t.Error("panic should mark job as failed")
	}
	if r.Name != "crasher" {
		t.Errorf("name = %q", r.Name)
	}
}

func TestRunJobNilJob(t *testing.T) {
	r := runJob(context.Background(), nil)
	if r.Success || r.Err == "" {
		t.Errorf("nil job result = %+v", r)
	}
}

func TestRunJobResultErrSetMarksFailure(t *testing.T) {
	// Job returns nil error but sets Err on the result.
	job := &stubJob{name: "x", result: JobResult{Err: "soft fail"}}
	r := runJob(context.Background(), job)
	if r.Success {
		t.Error("Err set should mark as failed")
	}
}

// --- Runtime tests ---

func newTestRuntime(jobs ...Job) (*Runtime, *State) {
	state := NewState(5)
	cfg := Config{Enabled: true, SleepWindow: "02:00-05:00", Timezone: "UTC"}
	rt := NewRuntime(cfg, jobs, state)
	return rt, state
}

func TestRuntimeRunOnceRecordsSummary(t *testing.T) {
	job := &stubJob{name: "memory", result: JobResult{Changed: true}}
	rt, state := newTestRuntime(job)
	summary := rt.RunOnce(context.Background())
	if !summary.Success || len(summary.Results) != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	if state.Snapshot().TotalRuns != 1 {
		t.Errorf("state not updated")
	}
}

func TestRuntimeRunOncePartialFailureMarksRunFailed(t *testing.T) {
	good := &stubJob{name: "good", result: JobResult{Changed: true}}
	bad := &stubJob{name: "bad", err: errors.New("nope")}
	rt, state := newTestRuntime(good, bad)
	summary := rt.RunOnce(context.Background())
	if summary.Success {
		t.Error("partial failure should mark run failed")
	}
	if len(summary.Results) != 2 {
		t.Errorf("want 2 results, got %d", len(summary.Results))
	}
	if good.called != 1 || bad.called != 1 {
		t.Error("all jobs should run even on failure")
	}
	if state.Snapshot().ConsecutiveFailures != 1 {
		t.Error("failure should bump consecutive counter")
	}
}

func TestRuntimeRunOnceDisabledReturnsEmpty(t *testing.T) {
	rt := NewRuntime(Config{Enabled: false}, []Job{&stubJob{name: "x"}}, NewState(5))
	summary := rt.RunOnce(context.Background())
	if len(summary.Results) != 0 {
		t.Errorf("disabled runtime should not run jobs: %+v", summary)
	}
}

func TestRuntimeNilReceiverSafe(t *testing.T) {
	var rt *Runtime
	rt.Start(context.Background())
	rt.Stop()
	if summary := rt.RunOnce(context.Background()); len(summary.Results) != 0 {
		t.Errorf("nil RunOnce should be empty")
	}
	if rt.State() != nil {
		t.Error("nil State")
	}
	if snap := rt.Snapshot(); snap.TotalRuns != 0 {
		t.Error("nil Snapshot")
	}
}

func TestRuntimeStartStop(t *testing.T) {
	rt, _ := newTestRuntime(&stubJob{name: "noop"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Use a tiny interval so we don't wait 5 minutes.
	rt.cfg.TickInterval = 5 * time.Millisecond
	rt.Start(ctx)
	time.Sleep(20 * time.Millisecond) // let a few ticks fire (all skipped - outside window)
	rt.Stop()
	rt.Stop() // second call must not panic
}

func TestRuntimeStartDisabledIsNoop(t *testing.T) {
	rt := NewRuntime(Config{Enabled: false}, nil, NewState(5))
	rt.Start(context.Background())
	rt.Stop() // must not panic
}

func TestRuntimeSetTickHookObservesExecute(t *testing.T) {
	job := &stubJob{name: "memory"}
	rt, _ := newTestRuntime(job)
	received := 0
	rt.SetTickHook(func(summary RunSummary) { received++ })
	rt.RunOnce(context.Background())
	rt.RunOnce(context.Background())
	if received != 2 {
		t.Errorf("hook received %d, want 2", received)
	}
}

func TestRuntimeTickGateSkipsOutsideWindow(t *testing.T) {
	job := &stubJob{name: "memory"}
	rt, state := newTestRuntime(job)
	// Inject a clock outside the 02:00-05:00 window
	rt.now = func() time.Time { return time.Date(2026, 4, 5, 10, 0, 0, 0, time.UTC) }
	rt.tick(context.Background())
	if job.called != 0 {
		t.Error("job should not run outside window")
	}
	if state.Snapshot().TotalRuns != 0 {
		t.Error("outside window tick should not record a run")
	}
}

func TestRuntimeTickGateRunsInsideWindow(t *testing.T) {
	job := &stubJob{name: "memory"}
	rt, state := newTestRuntime(job)
	rt.now = func() time.Time { return time.Date(2026, 4, 5, 3, 0, 0, 0, time.UTC) }
	rt.tick(context.Background())
	if job.called != 1 {
		t.Errorf("job called %d times, want 1", job.called)
	}
	if state.Snapshot().TotalRuns != 1 {
		t.Errorf("run not recorded")
	}
}

func TestRuntimeTickGateOncePerDay(t *testing.T) {
	job := &stubJob{name: "memory"}
	rt, _ := newTestRuntime(job)
	// Two ticks in the same window, same day.
	rt.now = func() time.Time { return time.Date(2026, 4, 5, 3, 0, 0, 0, time.UTC) }
	rt.tick(context.Background())
	rt.now = func() time.Time { return time.Date(2026, 4, 5, 4, 0, 0, 0, time.UTC) }
	rt.tick(context.Background())
	if job.called != 1 {
		t.Errorf("job called %d times, want 1 (second tick should be blocked)", job.called)
	}
}
