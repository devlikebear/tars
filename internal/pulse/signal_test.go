package pulse

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/cron"
	"github.com/devlikebear/tars/internal/gateway"
	"github.com/devlikebear/tars/internal/ops"
)

// --- test doubles ---

type fakeCronLister struct {
	jobs []cron.Job
	err  error
}

func (f *fakeCronLister) List() ([]cron.Job, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.jobs, nil
}

type fakeGatewayLister struct {
	runs []gateway.Run
}

func (f *fakeGatewayLister) List(limit int) []gateway.Run {
	return f.runs
}

type fakeOpsStatus struct {
	status ops.Status
	err    error
}

func (f *fakeOpsStatus) Status(ctx context.Context) (ops.Status, error) {
	if f.err != nil {
		return ops.Status{}, f.err
	}
	return f.status, nil
}

type fakeDeliveryCounter struct {
	failures int
	window   time.Duration
}

func (f *fakeDeliveryCounter) FailuresWithin(w time.Duration) int {
	f.window = w
	return f.failures
}

// --- helpers ---

func buildScanner(src ScannerSources, th Thresholds, now time.Time) *Scanner {
	s := NewScanner(src, th)
	s.now = func() time.Time { return now }
	return s
}

// --- cron ---

func TestScanner_CronBelowThreshold(t *testing.T) {
	src := ScannerSources{
		Cron: &fakeCronLister{jobs: []cron.Job{
			{ID: "j1", Name: "alpha", ConsecutiveFailures: 1},
			{ID: "j2", Name: "beta", ConsecutiveFailures: 2},
		}},
	}
	th := Thresholds{CronConsecutiveFailures: 3}
	sc := buildScanner(src, th, time.Now())
	if got := sc.Scan(context.Background()); len(got) != 0 {
		t.Errorf("expected no signals, got %d", len(got))
	}
}

func TestScanner_CronAtThreshold(t *testing.T) {
	src := ScannerSources{
		Cron: &fakeCronLister{jobs: []cron.Job{
			{ID: "j1", Name: "alpha", ConsecutiveFailures: 3, LastRunError: "boom"},
			{ID: "j2", Name: "beta", ConsecutiveFailures: 4},
		}},
	}
	th := Thresholds{CronConsecutiveFailures: 3}
	sc := buildScanner(src, th, time.Now())
	got := sc.Scan(context.Background())
	if len(got) != 1 {
		t.Fatalf("want 1 signal, got %d", len(got))
	}
	sig := got[0]
	if sig.Kind != SignalKindCronFailures {
		t.Errorf("kind = %v, want cron_failures", sig.Kind)
	}
	if sig.Severity != SeverityWarn {
		t.Errorf("severity = %v, want warn", sig.Severity)
	}
	if sig.Details["worst_job_name"] != "beta" {
		t.Errorf("worst job = %v, want beta", sig.Details["worst_job_name"])
	}
}

func TestScanner_CronEscalatesToError(t *testing.T) {
	// worst at 2*threshold → error severity
	src := ScannerSources{
		Cron: &fakeCronLister{jobs: []cron.Job{
			{ID: "j1", Name: "alpha", ConsecutiveFailures: 6},
		}},
	}
	sc := buildScanner(src, Thresholds{CronConsecutiveFailures: 3}, time.Now())
	got := sc.Scan(context.Background())
	if len(got) != 1 || got[0].Severity != SeverityError {
		t.Fatalf("want error severity, got %+v", got)
	}
}

func TestScanner_CronListErrorIsSuppressed(t *testing.T) {
	src := ScannerSources{Cron: &fakeCronLister{err: errors.New("io")}}
	sc := buildScanner(src, Thresholds{CronConsecutiveFailures: 1}, time.Now())
	if got := sc.Scan(context.Background()); len(got) != 0 {
		t.Errorf("expected suppression, got %d signals", len(got))
	}
}

func TestScanner_CronDisabled(t *testing.T) {
	src := ScannerSources{Cron: &fakeCronLister{jobs: []cron.Job{{ConsecutiveFailures: 99}}}}
	sc := buildScanner(src, Thresholds{CronConsecutiveFailures: 0}, time.Now())
	if got := sc.Scan(context.Background()); len(got) != 0 {
		t.Errorf("disabled cron should not emit: got %d", len(got))
	}
}

// --- stuck runs ---

func TestScanner_StuckRunsDetectsOldRunning(t *testing.T) {
	now := time.Date(2026, 4, 5, 10, 0, 0, 0, time.UTC)
	oldStart := now.Add(-75 * time.Minute).Format(time.RFC3339)
	recentStart := now.Add(-5 * time.Minute).Format(time.RFC3339)

	src := ScannerSources{
		Gateway: &fakeGatewayLister{runs: []gateway.Run{
			{ID: "r1", Status: gateway.RunStatusRunning, StartedAt: oldStart},
			{ID: "r2", Status: gateway.RunStatusRunning, StartedAt: recentStart},
			{ID: "r3", Status: gateway.RunStatusCompleted, StartedAt: oldStart}, // not running
		}},
	}
	sc := buildScanner(src, Thresholds{StuckRunMinutes: 60}, now)
	got := sc.Scan(context.Background())
	if len(got) != 1 {
		t.Fatalf("want 1 signal, got %d", len(got))
	}
	if got[0].Kind != SignalKindStuckGatewayRun {
		t.Errorf("kind = %v", got[0].Kind)
	}
	if got[0].Details["stuck_count"] != 1 {
		t.Errorf("stuck_count = %v, want 1", got[0].Details["stuck_count"])
	}
}

func TestScanner_StuckRunsEscalatesAtThreeOrMore(t *testing.T) {
	now := time.Date(2026, 4, 5, 10, 0, 0, 0, time.UTC)
	oldStart := now.Add(-90 * time.Minute).Format(time.RFC3339)
	runs := []gateway.Run{
		{ID: "r1", Status: gateway.RunStatusRunning, StartedAt: oldStart},
		{ID: "r2", Status: gateway.RunStatusRunning, StartedAt: oldStart},
		{ID: "r3", Status: gateway.RunStatusRunning, StartedAt: oldStart},
	}
	sc := buildScanner(ScannerSources{Gateway: &fakeGatewayLister{runs: runs}},
		Thresholds{StuckRunMinutes: 60}, now)
	got := sc.Scan(context.Background())
	if len(got) != 1 || got[0].Severity != SeverityError {
		t.Fatalf("want error severity, got %+v", got)
	}
}

func TestScanner_StuckRunsSkipsUnparseableTimestamp(t *testing.T) {
	now := time.Now()
	runs := []gateway.Run{
		{ID: "r1", Status: gateway.RunStatusRunning, StartedAt: "not-a-date"},
	}
	sc := buildScanner(ScannerSources{Gateway: &fakeGatewayLister{runs: runs}},
		Thresholds{StuckRunMinutes: 60}, now)
	if got := sc.Scan(context.Background()); len(got) != 0 {
		t.Errorf("expected skip, got %d signals", len(got))
	}
}

// --- disk ---

func TestScanner_DiskWarn(t *testing.T) {
	now := time.Now()
	src := ScannerSources{Ops: &fakeOpsStatus{status: ops.Status{DiskUsedPercent: 87}}}
	th := Thresholds{DiskUsedPercentWarn: 85, DiskUsedPercentCritical: 95}
	sc := buildScanner(src, th, now)
	got := sc.Scan(context.Background())
	if len(got) != 1 || got[0].Severity != SeverityWarn {
		t.Fatalf("want 1 warn signal, got %+v", got)
	}
}

func TestScanner_DiskCritical(t *testing.T) {
	now := time.Now()
	src := ScannerSources{Ops: &fakeOpsStatus{status: ops.Status{DiskUsedPercent: 97}}}
	th := Thresholds{DiskUsedPercentWarn: 85, DiskUsedPercentCritical: 95}
	sc := buildScanner(src, th, now)
	got := sc.Scan(context.Background())
	if len(got) != 1 || got[0].Severity != SeverityCritical {
		t.Fatalf("want critical, got %+v", got)
	}
}

func TestScanner_DiskBelowWarn(t *testing.T) {
	now := time.Now()
	src := ScannerSources{Ops: &fakeOpsStatus{status: ops.Status{DiskUsedPercent: 50}}}
	th := Thresholds{DiskUsedPercentWarn: 85, DiskUsedPercentCritical: 95}
	sc := buildScanner(src, th, now)
	if got := sc.Scan(context.Background()); len(got) != 0 {
		t.Errorf("expected no signals, got %d", len(got))
	}
}

func TestScanner_DiskStatusErrorSuppressed(t *testing.T) {
	src := ScannerSources{Ops: &fakeOpsStatus{err: errors.New("statfs fail")}}
	sc := buildScanner(src, Thresholds{DiskUsedPercentWarn: 1}, time.Now())
	if got := sc.Scan(context.Background()); len(got) != 0 {
		t.Errorf("expected suppression, got %d", len(got))
	}
}

// --- delivery ---

func TestScanner_DeliveryAboveThreshold(t *testing.T) {
	now := time.Now()
	dc := &fakeDeliveryCounter{failures: 5}
	sc := buildScanner(
		ScannerSources{Delivery: dc},
		Thresholds{DeliveryFailuresWithinWindow: 3, DeliveryFailureWindow: 15 * time.Minute},
		now,
	)
	got := sc.Scan(context.Background())
	if len(got) != 1 || got[0].Kind != SignalKindDeliveryFailures {
		t.Fatalf("want 1 delivery signal, got %+v", got)
	}
	if dc.window != 15*time.Minute {
		t.Errorf("scanner passed window %v, want 15m", dc.window)
	}
}

func TestScanner_DeliveryBelowThreshold(t *testing.T) {
	dc := &fakeDeliveryCounter{failures: 2}
	sc := buildScanner(
		ScannerSources{Delivery: dc},
		Thresholds{DeliveryFailuresWithinWindow: 3},
		time.Now(),
	)
	if got := sc.Scan(context.Background()); len(got) != 0 {
		t.Errorf("expected no signals, got %d", len(got))
	}
}

func TestScanner_DeliveryUsesDefaultWindow(t *testing.T) {
	dc := &fakeDeliveryCounter{failures: 5}
	sc := buildScanner(
		ScannerSources{Delivery: dc},
		Thresholds{DeliveryFailuresWithinWindow: 3}, // no window set
		time.Now(),
	)
	sc.Scan(context.Background())
	if dc.window != 10*time.Minute {
		t.Errorf("default window = %v, want 10m", dc.window)
	}
}

func TestScanner_DeliveryEscalates(t *testing.T) {
	dc := &fakeDeliveryCounter{failures: 10}
	sc := buildScanner(
		ScannerSources{Delivery: dc},
		Thresholds{DeliveryFailuresWithinWindow: 3},
		time.Now(),
	)
	got := sc.Scan(context.Background())
	if len(got) != 1 || got[0].Severity != SeverityError {
		t.Fatalf("want error severity, got %+v", got)
	}
}

// --- combined ---

func TestScanner_AllSourcesNilProducesNoSignals(t *testing.T) {
	sc := NewScanner(ScannerSources{}, Thresholds{
		CronConsecutiveFailures:      3,
		StuckRunMinutes:              60,
		DiskUsedPercentWarn:          85,
		DeliveryFailuresWithinWindow: 3,
	})
	if got := sc.Scan(context.Background()); len(got) != 0 {
		t.Errorf("nil sources should produce no signals, got %d", len(got))
	}
}

func TestScanner_MultipleSourcesEmit(t *testing.T) {
	now := time.Date(2026, 4, 5, 10, 0, 0, 0, time.UTC)
	src := ScannerSources{
		Cron: &fakeCronLister{jobs: []cron.Job{
			{ID: "j1", Name: "alpha", ConsecutiveFailures: 3},
		}},
		Ops:      &fakeOpsStatus{status: ops.Status{DiskUsedPercent: 90}},
		Delivery: &fakeDeliveryCounter{failures: 4},
	}
	th := Thresholds{
		CronConsecutiveFailures:      3,
		DiskUsedPercentWarn:          85,
		DeliveryFailuresWithinWindow: 3,
	}
	sc := buildScanner(src, th, now)
	got := sc.Scan(context.Background())
	if len(got) != 3 {
		t.Fatalf("want 3 signals, got %d: %+v", len(got), got)
	}
}

func TestParseRunTimestamp(t *testing.T) {
	cases := []struct {
		in string
		ok bool
	}{
		{"2026-04-05T10:00:00Z", true},
		{"2026-04-05T10:00:00.123456789Z", true},
		{"", false},
		{"not-a-date", false},
	}
	for _, c := range cases {
		_, ok := parseRunTimestamp(c.in)
		if ok != c.ok {
			t.Errorf("parseRunTimestamp(%q) ok=%v, want %v", c.in, ok, c.ok)
		}
	}
}

func TestScannerNilReceiverSafe(t *testing.T) {
	var sc *Scanner
	if got := sc.Scan(context.Background()); got != nil {
		t.Errorf("nil scanner should return nil, got %v", got)
	}
}
