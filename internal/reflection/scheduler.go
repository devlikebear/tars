package reflection

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// schedulerDecision describes what the scheduler thinks should happen at
// a given moment. runtime.loop consults this on every tick.
type schedulerDecision struct {
	Run    bool
	Reason string
}

// decideTick reports whether a reflection run should start right now,
// given the current time, the configured sleep window, and the last
// time a run was recorded.
//
// The rules are:
//
//  1. If reflection is disabled → skip with "disabled".
//  2. If the current local time is outside the sleep window → skip with
//     "outside_sleep_window".
//  3. If the last run already happened in the current "reflection day"
//     (the calendar day, in Timezone, that the sleep window opens on) →
//     skip with "already_ran_today".
//  4. Otherwise → run.
//
// For wrap-around windows like 22:00-02:00 the "day" is anchored to the
// start of the window. 01:30 on April 6 belongs to the April 5 window.
func decideTick(now time.Time, cfg Config, lastRun time.Time) schedulerDecision {
	if !cfg.Enabled {
		return schedulerDecision{Reason: "disabled"}
	}

	loc := resolveLocation(cfg.Timezone)
	local := now.In(loc)

	startMin, endMin, err := parseSleepWindow(cfg.EffectiveSleepWindow())
	if err != nil {
		return schedulerDecision{Reason: "invalid_sleep_window"}
	}

	nowMin := local.Hour()*60 + local.Minute()
	inWindow := windowContains(startMin, endMin, nowMin)
	if !inWindow {
		return schedulerDecision{Reason: "outside_sleep_window"}
	}

	anchor := windowAnchorDay(local, startMin, endMin)
	if !lastRun.IsZero() {
		lastLocal := lastRun.In(loc)
		lastAnchor := windowAnchorDay(lastLocal, startMin, endMin)
		if !anchor.After(lastAnchor) {
			return schedulerDecision{Reason: "already_ran_today"}
		}
	}
	return schedulerDecision{Run: true}
}

// windowAnchorDay returns the calendar day (in local tz, midnight) that
// "owns" the current moment with respect to the sleep window. For
// non-wrapping windows this is trivially the same day. For wrapping
// windows (22:00-02:00), minutes after midnight belong to the previous
// day's window.
func windowAnchorDay(local time.Time, startMin, endMin int) time.Time {
	y, m, d := local.Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, local.Location())
	if startMin > endMin {
		nowMin := local.Hour()*60 + local.Minute()
		if nowMin < endMin {
			// we're in the tail of yesterday's window
			day = day.AddDate(0, 0, -1)
		}
	}
	return day
}

// windowContains reports whether nowMin falls within [startMin, endMin).
// Supports wrap-around (startMin > endMin) windows.
func windowContains(startMin, endMin, nowMin int) bool {
	if startMin <= endMin {
		return nowMin >= startMin && nowMin < endMin
	}
	return nowMin >= startMin || nowMin < endMin
}

// parseSleepWindow parses "HH:MM-HH:MM" strictly. Matches pulse's parser
// but lives here to avoid cross-package dependencies in the core types.
func parseSleepWindow(s string) (int, int, error) {
	s = strings.TrimSpace(s)
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return 0, 0, errors.New("expected HH:MM-HH:MM")
	}
	start, err := parseClockMinutes(parts[0])
	if err != nil {
		return 0, 0, err
	}
	end, err := parseClockMinutes(parts[1])
	if err != nil {
		return 0, 0, err
	}
	return start, end, nil
}

func parseClockMinutes(s string) (int, error) {
	s = strings.TrimSpace(s)
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid time %q", s)
	}
	var h, m int
	if _, err := fmt.Sscanf(parts[0], "%d", &h); err != nil {
		return 0, fmt.Errorf("invalid hour in %q", s)
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &m); err != nil {
		return 0, fmt.Errorf("invalid minute in %q", s)
	}
	if h < 0 || h > 24 || m < 0 || m >= 60 {
		return 0, fmt.Errorf("out of range: %q", s)
	}
	if h == 24 {
		h = 24
		m = 0
	}
	return h*60 + m, nil
}

func resolveLocation(tz string) *time.Location {
	tz = strings.TrimSpace(tz)
	if tz == "" || tz == "Local" {
		return time.Local
	}
	if loc, err := time.LoadLocation(tz); err == nil {
		return loc
	}
	return time.Local
}
