package cron

import (
	"context"
	"strings"
	"sync"
	"time"

	cronv3 "github.com/robfig/cron/v3"
)

type Manager struct {
	store  *Store
	runJob func(ctx context.Context, job Job) (string, error)
	nowFn  func() time.Time
}

func NewManager(
	store *Store,
	runJob func(ctx context.Context, job Job) (string, error),
	nowFn func() time.Time,
) *Manager {
	if nowFn == nil {
		nowFn = time.Now
	}
	return &Manager{
		store:  store,
		runJob: runJob,
		nowFn:  nowFn,
	}
}

func (m *Manager) Tick(ctx context.Context) error {
	if m == nil || m.store == nil || m.runJob == nil {
		return nil
	}
	now := m.nowFn().UTC()
	jobs, err := m.store.List()
	if err != nil {
		return err
	}
	dueJobs := make([]Job, 0, len(jobs))
	for _, job := range jobs {
		if !shouldRunAt(job, now) {
			continue
		}
		if !m.store.TryStartRun(job.ID) {
			continue
		}
		dueJobs = append(dueJobs, job)
	}
	var wg sync.WaitGroup
	for _, job := range dueJobs {
		wg.Add(1)
		go func(job Job) {
			defer wg.Done()
			response, runErr := m.runJob(ctx, job)
			m.store.FinishRun(job.ID)
			_, _ = m.store.MarkRunResult(job.ID, now, response, runErr)
		}(job)
	}
	wg.Wait()
	return nil
}

func shouldRunAt(job Job, now time.Time) bool {
	if !job.Enabled {
		return false
	}
	if job.BackoffUntil != nil && now.Before(job.BackoffUntil.UTC()) {
		return false
	}
	atTime, isAt, err := parseAtTime(job.Schedule)
	if isAt {
		if err != nil {
			return false
		}
		if job.LastRunAt != nil {
			return false
		}
		return !atTime.After(now)
	}

	interval, ok := parseEveryDuration(job.Schedule)
	if ok {
		if interval <= 0 {
			return false
		}
		if job.LastRunAt == nil {
			return true
		}
		return now.Sub(job.LastRunAt.UTC()) >= interval
	}

	sched, err := cronv3.ParseStandard(strings.TrimSpace(job.Schedule))
	if err != nil {
		return false
	}
	base := job.CreatedAt.UTC()
	if job.LastRunAt != nil {
		base = job.LastRunAt.UTC()
	}
	// robfig/cron reads the expression in base's location (a CRON_TZ= prefix
	// overrides it), so base must be in the job's zone, not UTC.
	next := sched.Next(base.In(job.location()))
	return !next.After(now)
}

func parseEveryDuration(schedule string) (time.Duration, bool) {
	s := strings.TrimSpace(schedule)
	if s == "" {
		return 0, false
	}
	lower := strings.ToLower(s)
	switch {
	case strings.HasPrefix(lower, "every:"):
		v := strings.TrimSpace(s[len("every:"):])
		d, err := time.ParseDuration(v)
		if err != nil {
			return 0, false
		}
		return d, true
	case strings.HasPrefix(lower, "@every "):
		v := strings.TrimSpace(s[len("@every "):])
		d, err := time.ParseDuration(v)
		if err != nil {
			return 0, false
		}
		return d, true
	default:
		return 0, false
	}
}

// LocalTimezone is the Job.Timezone value for the server's local time zone,
// resolved at each tick (the same "Local" convention as pulse and reflection).
const LocalTimezone = "Local"

// location is the zone the job's cron expression is evaluated in.
func (j Job) location() *time.Location {
	tz := strings.TrimSpace(j.Timezone)
	switch tz {
	case "":
		return time.UTC
	case LocalTimezone:
		return time.Local
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.Local
	}
	return loc
}
