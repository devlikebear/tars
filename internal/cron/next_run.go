package cron

import (
	"strings"
	"time"

	cronv3 "github.com/robfig/cron/v3"
)

// NextRunIn reports when an enabled job next runs in [from, to): its first
// scheduled time in that span, or, for an every:/@every job, its interval
// (at is then zero). ok is false when the job does not run in the span — it
// is disabled, a one-shot outside the span or already run, or its schedule
// does not parse. Read-only: what the initiative morning brief lists as
// today's cron runs (tars#1001), never what decides a run (shouldRunAt).
func (j Job) NextRunIn(from, to time.Time) (at time.Time, every time.Duration, ok bool) {
	if !j.Enabled || !from.Before(to) {
		return time.Time{}, 0, false
	}
	atTime, isAt, err := parseAtTime(j.Schedule)
	if isAt {
		if err != nil || j.LastRunAt != nil || atTime.Before(from) || !atTime.Before(to) {
			return time.Time{}, 0, false
		}
		return atTime, 0, true
	}
	if interval, isEvery := parseEveryDuration(j.Schedule); isEvery {
		if interval <= 0 {
			return time.Time{}, 0, false
		}
		return time.Time{}, interval, true
	}
	sched, err := cronv3.ParseStandard(strings.TrimSpace(j.Schedule))
	if err != nil {
		return time.Time{}, 0, false
	}
	// Next is strictly after its argument; a second back keeps a run due
	// exactly at from.
	next := sched.Next(from.Add(-time.Second).In(j.location()))
	if next.IsZero() || !next.Before(to) {
		return time.Time{}, 0, false
	}
	return next, 0, true
}
