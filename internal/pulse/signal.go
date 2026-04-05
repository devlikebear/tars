package pulse

import (
	"context"
	"fmt"
	"time"

	"github.com/devlikebear/tars/internal/cron"
	"github.com/devlikebear/tars/internal/gateway"
	"github.com/devlikebear/tars/internal/ops"
)

// CronJobLister is the narrow interface pulse requires from a cron store
// to count failing jobs. The real *cron.Store satisfies it.
type CronJobLister interface {
	List() ([]cron.Job, error)
}

// GatewayRunLister is the narrow interface pulse requires from the
// gateway runtime to find stuck runs. The real *gateway.Runtime satisfies
// it.
type GatewayRunLister interface {
	List(limit int) []gateway.Run
}

// DiskStatProvider is the narrow interface pulse requires from the ops
// manager to read disk usage. The real *ops.Manager satisfies it.
type DiskStatProvider interface {
	Status(ctx context.Context) (ops.Status, error)
}

// DeliveryFailureCounter is the narrow interface pulse requires from the
// telegram delivery counter. The real counter in internal/tarsserver
// satisfies it.
type DeliveryFailureCounter interface {
	FailuresWithin(window time.Duration) int
}

// Thresholds controls when signals are emitted. Zero values mean
// "disabled" for that signal (never emit).
type Thresholds struct {
	// CronConsecutiveFailures — emit when any job's consecutive failures
	// reaches or exceeds this value. 0 = disabled.
	CronConsecutiveFailures int

	// StuckRunMinutes — emit when any gateway run has been in Running
	// status for at least this many minutes. 0 = disabled.
	StuckRunMinutes int

	// DiskUsedPercentWarn — emit a warn signal when disk usage percent
	// reaches or exceeds this value. 0 = disabled.
	DiskUsedPercentWarn float64

	// DiskUsedPercentCritical — emit a critical signal above this value.
	// 0 = disabled.
	DiskUsedPercentCritical float64

	// DeliveryFailuresWithinWindow — emit when telegram delivery failures
	// in the last DeliveryFailureWindow duration reach this count.
	// 0 = disabled.
	DeliveryFailuresWithinWindow int

	// DeliveryFailureWindow — rolling window for counting delivery
	// failures. Zero defaults to 10 minutes.
	DeliveryFailureWindow time.Duration
}

// ScannerSources bundles the data sources a Scanner reads from. Any field
// may be nil; nil sources yield no signals for that domain.
type ScannerSources struct {
	Cron     CronJobLister
	Gateway  GatewayRunLister
	Ops      DiskStatProvider
	Delivery DeliveryFailureCounter
}

// Scanner collects Signals from the configured sources. It is stateless
// and safe to call concurrently from multiple ticks, though in practice
// the runtime serializes ticks.
type Scanner struct {
	sources    ScannerSources
	thresholds Thresholds
	now        func() time.Time
}

// NewScanner constructs a Scanner. Callers typically build one at server
// startup and reuse it across ticks.
func NewScanner(sources ScannerSources, thresholds Thresholds) *Scanner {
	return &Scanner{
		sources:    sources,
		thresholds: thresholds,
		now:        time.Now,
	}
}

// Scan runs each enabled signal source once and returns the resulting
// signals. Sources that fail are surfaced as a SignalKindInfo with
// severity Warn — we do not propagate errors, because a single broken
// source should not block the whole tick.
func (s *Scanner) Scan(ctx context.Context) []Signal {
	if s == nil {
		return nil
	}
	now := s.now()
	var signals []Signal

	if sig := s.scanCron(); sig != nil {
		signals = append(signals, *sig)
	}
	if sig := s.scanStuckRuns(now); sig != nil {
		signals = append(signals, *sig)
	}
	if sig := s.scanDisk(ctx, now); sig != nil {
		signals = append(signals, *sig)
	}
	if sig := s.scanDelivery(now); sig != nil {
		signals = append(signals, *sig)
	}
	return signals
}

func (s *Scanner) scanCron() *Signal {
	if s.sources.Cron == nil || s.thresholds.CronConsecutiveFailures <= 0 {
		return nil
	}
	jobs, err := s.sources.Cron.List()
	if err != nil {
		return nil
	}
	worst := 0
	var worstJob cron.Job
	total := 0
	for _, j := range jobs {
		if j.ConsecutiveFailures >= s.thresholds.CronConsecutiveFailures {
			total++
			if j.ConsecutiveFailures > worst {
				worst = j.ConsecutiveFailures
				worstJob = j
			}
		}
	}
	if total == 0 {
		return nil
	}
	sev := SeverityWarn
	if worst >= s.thresholds.CronConsecutiveFailures*2 {
		sev = SeverityError
	}
	return &Signal{
		Kind:     SignalKindCronFailures,
		Severity: sev,
		Summary: fmt.Sprintf(
			"%d cron job(s) are failing (worst: %q at %d consecutive failures)",
			total, worstJob.Name, worst,
		),
		Details: map[string]any{
			"jobs_failing":    total,
			"worst_job_id":    worstJob.ID,
			"worst_job_name":  worstJob.Name,
			"worst_failures":  worst,
			"worst_job_error": worstJob.LastRunError,
		},
		At: s.now(),
	}
}

func (s *Scanner) scanStuckRuns(now time.Time) *Signal {
	if s.sources.Gateway == nil || s.thresholds.StuckRunMinutes <= 0 {
		return nil
	}
	runs := s.sources.Gateway.List(100)
	if len(runs) == 0 {
		return nil
	}
	cutoff := now.Add(-time.Duration(s.thresholds.StuckRunMinutes) * time.Minute)
	var stuck []gateway.Run
	var worstStarted time.Time
	for _, r := range runs {
		if r.Status != gateway.RunStatusRunning {
			continue
		}
		started, ok := parseRunTimestamp(r.StartedAt)
		if !ok {
			continue
		}
		if started.Before(cutoff) {
			stuck = append(stuck, r)
			if worstStarted.IsZero() || started.Before(worstStarted) {
				worstStarted = started
			}
		}
	}
	if len(stuck) == 0 {
		return nil
	}
	sev := SeverityWarn
	if len(stuck) >= 3 {
		sev = SeverityError
	}
	return &Signal{
		Kind:     SignalKindStuckGatewayRun,
		Severity: sev,
		Summary: fmt.Sprintf(
			"%d gateway run(s) stuck in running for more than %d minute(s)",
			len(stuck), s.thresholds.StuckRunMinutes,
		),
		Details: map[string]any{
			"stuck_count":           len(stuck),
			"oldest_started_at":     worstStarted.Format(time.RFC3339),
			"stuck_minutes_minimum": s.thresholds.StuckRunMinutes,
		},
		At: now,
	}
}

func (s *Scanner) scanDisk(ctx context.Context, now time.Time) *Signal {
	if s.sources.Ops == nil {
		return nil
	}
	if s.thresholds.DiskUsedPercentWarn <= 0 && s.thresholds.DiskUsedPercentCritical <= 0 {
		return nil
	}
	status, err := s.sources.Ops.Status(ctx)
	if err != nil {
		return nil
	}
	pct := status.DiskUsedPercent
	critical := s.thresholds.DiskUsedPercentCritical
	warn := s.thresholds.DiskUsedPercentWarn
	var sev Severity
	switch {
	case critical > 0 && pct >= critical:
		sev = SeverityCritical
	case warn > 0 && pct >= warn:
		sev = SeverityWarn
	default:
		return nil
	}
	return &Signal{
		Kind:     SignalKindDiskUsage,
		Severity: sev,
		Summary:  fmt.Sprintf("disk usage at %.1f%%", pct),
		Details: map[string]any{
			"disk_used_percent":  pct,
			"disk_total_bytes":   status.DiskTotalBytes,
			"disk_free_bytes":    status.DiskFreeBytes,
			"warn_threshold":     warn,
			"critical_threshold": critical,
		},
		At: now,
	}
}

func (s *Scanner) scanDelivery(now time.Time) *Signal {
	if s.sources.Delivery == nil || s.thresholds.DeliveryFailuresWithinWindow <= 0 {
		return nil
	}
	window := s.thresholds.DeliveryFailureWindow
	if window <= 0 {
		window = 10 * time.Minute
	}
	count := s.sources.Delivery.FailuresWithin(window)
	if count < s.thresholds.DeliveryFailuresWithinWindow {
		return nil
	}
	sev := SeverityWarn
	if count >= s.thresholds.DeliveryFailuresWithinWindow*2 {
		sev = SeverityError
	}
	return &Signal{
		Kind:     SignalKindDeliveryFailures,
		Severity: sev,
		Summary: fmt.Sprintf(
			"%d telegram delivery failure(s) in the last %s",
			count, window,
		),
		Details: map[string]any{
			"failures":  count,
			"window":    window.String(),
			"threshold": s.thresholds.DeliveryFailuresWithinWindow,
		},
		At: now,
	}
}

// parseRunTimestamp parses the string-typed StartedAt/CreatedAt fields
// used by gateway.Run. Returns false for empty or malformed values.
func parseRunTimestamp(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	return time.Time{}, false
}
