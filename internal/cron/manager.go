package cron

import (
	"context"
	"strings"
	"time"

	cronv3 "github.com/robfig/cron/v3"
)

type Manager struct {
	store    *Store
	runJob   func(ctx context.Context, job Job) (string, error)
	interval time.Duration
	nowFn    func() time.Time
}

func NewManager(
	store *Store,
	runJob func(ctx context.Context, job Job) (string, error),
	interval time.Duration,
	nowFn func() time.Time,
) *Manager {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if nowFn == nil {
		nowFn = time.Now
	}
	return &Manager{
		store:    store,
		runJob:   runJob,
		interval: interval,
		nowFn:    nowFn,
	}
}

func (m *Manager) Start(ctx context.Context) error {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			_ = m.Tick(ctx)
		}
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
	for _, job := range jobs {
		if !shouldRunAt(job, now) {
			continue
		}
		if !m.store.TryStartRun(job.ID) {
			continue
		}
		response, runErr := m.runJob(ctx, job)
		m.store.FinishRun(job.ID)
		_, _ = m.store.MarkRunResult(job.ID, now, response, runErr)
	}
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
	next := sched.Next(base)
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
