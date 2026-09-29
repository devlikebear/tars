package initiative

import (
	"strings"
	"time"
)

const (
	ModeShadow = "shadow"

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
}

// WithDefaults fills zero values. P1 only knows shadow mode.
func (c Config) WithDefaults() Config {
	c.Mode = ModeShadow
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
	if strings.TrimSpace(c.QuietHours) == "" {
		c.QuietHours = "23:00-07:00"
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
