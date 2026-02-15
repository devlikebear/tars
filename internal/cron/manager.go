package cron

import (
	"context"
	"strings"
	"time"
)

type Manager struct {
	store     *Store
	runPrompt func(ctx context.Context, prompt string) (string, error)
	interval  time.Duration
	nowFn     func() time.Time
}

func NewManager(
	store *Store,
	runPrompt func(ctx context.Context, prompt string) (string, error),
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
		store:     store,
		runPrompt: runPrompt,
		interval:  interval,
		nowFn:     nowFn,
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
	if m == nil || m.store == nil || m.runPrompt == nil {
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
		_, runErr := m.runPrompt(ctx, job.Prompt)
		_, _ = m.store.MarkRunResult(job.ID, now, runErr)
	}
	return nil
}

func shouldRunAt(job Job, now time.Time) bool {
	if !job.Enabled {
		return false
	}
	interval, ok := parseEveryDuration(job.Schedule)
	if !ok || interval <= 0 {
		return false
	}
	if job.LastRunAt == nil {
		return true
	}
	return now.Sub(job.LastRunAt.UTC()) >= interval
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
