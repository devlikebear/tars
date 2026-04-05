package reflection

import "time"

// Config is the runtime configuration for a reflection Runtime. It is
// populated from the main server config at startup; restarting the
// server is required to pick up changes.
type Config struct {
	// Enabled globally controls whether reflection runs at all. When
	// false, the runtime still answers HTTP status queries but its tick
	// loop is a no-op.
	Enabled bool

	// SleepWindow is the "HH:MM-HH:MM" range (in Timezone) during which
	// reflection is allowed to run its nightly jobs. Defaults to
	// "02:00-05:00". Wrap-around windows (e.g. "22:00-02:00") are
	// supported.
	SleepWindow string

	// Timezone is the IANA zone name used for SleepWindow evaluation.
	// Empty or "Local" uses the host's local time.
	Timezone string

	// TickInterval is how often the scheduler wakes up to check whether
	// the current moment is inside SleepWindow and whether today's run
	// has happened. Defaults to 5 minutes. Keeping it short is cheap
	// (the inner check has no LLM calls) and ensures jobs start within
	// a few minutes of the window opening even after system sleep.
	TickInterval time.Duration

	// EmptySessionAge is the minimum age a zero-message session must
	// reach before the KB cleanup job will delete it. Defaults to 24h
	// so fresh sessions that happen to be empty don't disappear under
	// the user while they're still composing a first turn.
	EmptySessionAge time.Duration

	// MemoryLookbackHours controls how far back the memory cleanup job
	// reads session history when extracting experiences and compiling
	// knowledge. Defaults to 24 hours — one reflection run per night
	// with a 24-hour window covers everything between runs.
	MemoryLookbackHours int

	// MaxTurnsPerSession caps how many turns the memory job processes
	// per session to keep nightly runs bounded even for extremely
	// chatty sessions.
	MaxTurnsPerSession int
}

// EffectiveTickInterval returns TickInterval with sensible defaults.
func (c Config) EffectiveTickInterval() time.Duration {
	if c.TickInterval <= 0 {
		return 5 * time.Minute
	}
	return c.TickInterval
}

// EffectiveSleepWindow returns SleepWindow with the default applied when
// the caller passed an empty string.
func (c Config) EffectiveSleepWindow() string {
	if c.SleepWindow == "" {
		return "02:00-05:00"
	}
	return c.SleepWindow
}

// EffectiveEmptySessionAge returns EmptySessionAge with the 24h default.
func (c Config) EffectiveEmptySessionAge() time.Duration {
	if c.EmptySessionAge <= 0 {
		return 24 * time.Hour
	}
	return c.EmptySessionAge
}

// EffectiveMemoryLookback returns the lookback window as a Duration.
func (c Config) EffectiveMemoryLookback() time.Duration {
	hours := c.MemoryLookbackHours
	if hours <= 0 {
		hours = 24
	}
	return time.Duration(hours) * time.Hour
}

// EffectiveMaxTurnsPerSession returns MaxTurnsPerSession clamped to a
// reasonable range.
func (c Config) EffectiveMaxTurnsPerSession() int {
	if c.MaxTurnsPerSession <= 0 {
		return 20
	}
	return c.MaxTurnsPerSession
}
