package initiative

import (
	"strings"
	"time"
)

const (
	ModeShadow = "shadow"
	ModeLive   = "live"

	// defaultBackoffStart/defaultBackoffMax bound the exponential backoff a
	// failed speak attempt or a failed text-signal read falls back to, so a
	// stuck backend is retried on a widening schedule instead of every tick
	// (tars#1220).
	defaultBackoffStart = 5 * time.Minute
	defaultBackoffMax   = 60 * time.Minute

	typingWindow     = 3 * time.Minute
	arrivalGap       = 4 * time.Hour
	arrivalFreshness = 10 * time.Minute
	sessionGap       = 30 * time.Minute
	longSession      = 3 * time.Hour
	longSessionTail  = 10 * time.Minute
	longAbsence      = 24 * time.Hour
	idleMin          = 15 * time.Minute
	idleMax          = 2 * time.Hour
	checkInSpacing   = 3 * time.Hour
	bodySpacing      = 30 * time.Minute
)

// ReminderGrace is how long a queued reminder may wait for the speak path
// (typing, a turn in flight, nobody reachable, a failed compose) before the
// server hands it back to cron's own delivery, so a reminder is late by at
// most this much and never lost (tars#1001).
const ReminderGrace = 10 * time.Minute

// ResultGrace is how long a queued background result may wait for the
// speak path before the server gives it back (tars#1002): an agent run's
// [RUN SUMMARY] note is written where it would have gone, a result with no
// note of its own is dropped (the notification history still has it). A
// result is not time-critical like a reminder, so this covers a cooldown
// and a short absence, not a night away.
const ResultGrace = 2 * time.Hour

// FollowUpLookback is how far back before today an unfinished goal or
// pipeline still counts as "left yesterday" for follow_up (tars#1002): long
// enough to span a weekend, short enough that week-old work is not raised
// every morning.
const FollowUpLookback = 72 * time.Hour

// ArrivalFreshness is how long after a console arrival the arrival still
// counts (GoSignals.JustArrived) — exported so an adapter can skip work
// only an arrival tick reads (follow_up's lookup).
const ArrivalFreshness = arrivalFreshness

// Thresholds turn System One probabilities into signals. Defaults come from
// the Kev-0.8B measurements in tars#998.
type Thresholds struct {
	QuietRequested float64
	UserStrained   float64
	SpecialDay     float64
}

type Config struct {
	Enabled      bool
	Mode         string
	Tick         time.Duration
	QuietHours   string
	Location     *time.Location
	DailyCap     int
	Cooldown     time.Duration
	BodyProvider string
	Thresholds   Thresholds
	// DailyTextCalls caps how many text-signal backend calls (jev or llm)
	// the runtime makes per local day, independent of DailyCap (spoken
	// initiatives). tars#1219.
	DailyTextCalls int
	// DailySpeakCalls caps how many System 2 speak-composer LLM calls live
	// mode makes per local day. tars#1220.
	DailySpeakCalls int
	// ReplyWindow is how long after a delivered speak a main-session user
	// message still counts as a reply to it (tars#1221): within the
	// window, replied (or stopped, when that message reads as a quiet
	// request) — past it with no message, ignored.
	ReplyWindow time.Duration
	// Telegram opts in to delivering a live-mode speak over Telegram's
	// default paired chat when the console is not connected (tars#1221
	// P2c). False (the default) means deliverLocked behaves exactly as
	// before this field existed.
	Telegram bool
	// Remind routes cron reminders through the speak path (tars#1001).
	// Off (the default), cron keeps writing its own [REMINDER] note.
	Remind bool
	// ConfirmMemory lets TARS ask whether to keep a pending reflection
	// inbox candidate (tars#1001). Off by default.
	ConfirmMemory bool
	// ShareResult reports background results that left no conversational
	// trace (tars#1002). Off by default; live mode only, since the server
	// queues results only while it can deliver them.
	ShareResult bool
	// FollowUp suggests picking up work left unfinished on an earlier day
	// at the first arrival of the day, in place of that greet (tars#1002).
	// Off by default.
	FollowUp bool
	// MorningBrief adds the morning brief (today's cron runs, yesterday's
	// daily log in counts, the work still open) to the day's first arrival
	// utterance in CASE's bubble (tars#1001 brief). Off by default. The
	// runtime only reports it; the speaker builds the lines.
	MorningBrief bool
	// EveningRecap says what finished, failed and is still open today, once
	// a day, while RecapWindow is open (tars#1001 brief). Off by default.
	EveningRecap bool
	// RecapWindow is the "HH:MM-HH:MM" window the evening recap may go out
	// in. Empty means the hour before QuietHours starts
	// (DefaultRecapWindow).
	RecapWindow string
}

// WithDefaults fills zero values. Mode defaults to shadow when unset, but an
// explicit live is kept — callers are expected to have already validated
// Mode is shadow or live (config.Load does this); WithDefaults itself never
// rejects an unknown mode, it simply does not treat it as live.
func (c Config) WithDefaults() Config {
	if strings.TrimSpace(c.Mode) == "" {
		c.Mode = ModeShadow
	}
	if c.Tick <= 0 {
		c.Tick = time.Minute
	}
	if c.Location == nil {
		c.Location = time.Local
	}
	if c.DailyCap <= 0 {
		c.DailyCap = 6
	}
	if c.Cooldown <= 0 {
		c.Cooldown = 45 * time.Minute
	}
	if c.DailyTextCalls <= 0 {
		c.DailyTextCalls = 60
	}
	if c.DailySpeakCalls <= 0 {
		c.DailySpeakCalls = 12
	}
	if c.ReplyWindow <= 0 {
		c.ReplyWindow = 30 * time.Minute
	}
	if strings.TrimSpace(c.QuietHours) == "" {
		c.QuietHours = "23:00-07:00"
	}
	if strings.TrimSpace(c.RecapWindow) == "" {
		c.RecapWindow = DefaultRecapWindow(c.QuietHours)
	}
	if c.Thresholds.QuietRequested <= 0 {
		c.Thresholds.QuietRequested = 0.55
	}
	if c.Thresholds.UserStrained <= 0 {
		c.Thresholds.UserStrained = 0.60
	}
	if c.Thresholds.SpecialDay <= 0 {
		c.Thresholds.SpecialDay = 0.55
	}
	c.BodyProvider = strings.TrimSpace(c.BodyProvider)
	return c
}

// Live reports whether the runtime should actually compose and deliver
// speech (greet/check_in), as opposed to shadow mode's record-only
// behavior. Any mode other than exactly ModeLive is treated as shadow.
func (c Config) Live() bool { return c.Mode == ModeLive }

// RecapWindowOpen reports whether t falls in the evening recap window.
// Call it on a Config that went through WithDefaults.
func (c Config) RecapWindowOpen(t time.Time) bool {
	loc := c.Location
	if loc == nil {
		loc = time.Local
	}
	return inQuietWindow(c.RecapWindow, t.In(loc))
}

// inQuietWindow reports whether t falls in an "HH:MM-HH:MM" window that may
// wrap past midnight. An empty or malformed window is never quiet.
func inQuietWindow(window string, t time.Time) bool {
	start, end, ok := parseWindow(window)
	if !ok || start == end {
		return false
	}
	m := t.Hour()*60 + t.Minute()
	if start < end {
		return m >= start && m < end
	}
	return m >= start || m < end
}

func parseWindow(window string) (int, int, bool) {
	parts := strings.Split(strings.TrimSpace(window), "-")
	if len(parts) != 2 {
		return 0, 0, false
	}
	start, ok1 := parseClock(parts[0])
	end, ok2 := parseClock(parts[1])
	return start, end, ok1 && ok2
}

func parseClock(s string) (int, bool) {
	ts, err := time.Parse("15:04", strings.TrimSpace(s))
	if err != nil {
		return 0, false
	}
	return ts.Hour()*60 + ts.Minute(), true
}
