package reflection

import (
	"testing"
	"time"
)

func TestParseSleepWindow(t *testing.T) {
	start, end, err := parseSleepWindow("02:00-05:00")
	if err != nil || start != 120 || end != 300 {
		t.Fatalf("parseSleepWindow = %d,%d,%v", start, end, err)
	}
	if _, _, err := parseSleepWindow(""); err == nil {
		t.Error("empty should error")
	}
	if _, _, err := parseSleepWindow("25:00-05:00"); err == nil {
		t.Error("out of range hour should error")
	}
	if _, _, err := parseSleepWindow("02:60-05:00"); err == nil {
		t.Error("out of range minute should error")
	}
}

func TestWindowContains(t *testing.T) {
	// Non-wrapping window 09:00-17:00
	cases := []struct {
		start, end, now int
		want            bool
	}{
		{540, 1020, 600, true},   // 10:00 in 09:00-17:00
		{540, 1020, 540, true},   // exactly start
		{540, 1020, 1020, false}, // exactly end (exclusive)
		{540, 1020, 300, false},  // before
		{1320, 120, 30, true},    // 00:30 in 22:00-02:00 (wrap)
		{1320, 120, 1400, true},  // 23:20 in 22:00-02:00
		{1320, 120, 200, false},  // 03:20 outside 22:00-02:00
	}
	for _, c := range cases {
		if got := windowContains(c.start, c.end, c.now); got != c.want {
			t.Errorf("windowContains(%d,%d,%d) = %v, want %v", c.start, c.end, c.now, got, c.want)
		}
	}
}

func TestDecideTickDisabled(t *testing.T) {
	d := decideTick(time.Now(), Config{Enabled: false}, time.Time{})
	if d.Run || d.Reason != "disabled" {
		t.Errorf("disabled = %+v", d)
	}
}

func TestDecideTickInvalidWindow(t *testing.T) {
	cfg := Config{Enabled: true, SleepWindow: "nonsense", Timezone: "UTC"}
	d := decideTick(time.Now(), cfg, time.Time{})
	if d.Run || d.Reason != "invalid_sleep_window" {
		t.Errorf("invalid = %+v", d)
	}
}

func TestDecideTickOutsideWindow(t *testing.T) {
	cfg := Config{Enabled: true, SleepWindow: "02:00-05:00", Timezone: "UTC"}
	now := time.Date(2026, 4, 5, 10, 0, 0, 0, time.UTC)
	d := decideTick(now, cfg, time.Time{})
	if d.Run || d.Reason != "outside_sleep_window" {
		t.Errorf("10:00 outside = %+v", d)
	}
}

func TestDecideTickInsideWindowFirstRun(t *testing.T) {
	cfg := Config{Enabled: true, SleepWindow: "02:00-05:00", Timezone: "UTC"}
	now := time.Date(2026, 4, 5, 3, 0, 0, 0, time.UTC)
	d := decideTick(now, cfg, time.Time{})
	if !d.Run {
		t.Errorf("03:00 first run = %+v", d)
	}
}

func TestDecideTickAlreadyRanToday(t *testing.T) {
	cfg := Config{Enabled: true, SleepWindow: "02:00-05:00", Timezone: "UTC"}
	now := time.Date(2026, 4, 5, 3, 30, 0, 0, time.UTC)
	last := time.Date(2026, 4, 5, 2, 10, 0, 0, time.UTC)
	d := decideTick(now, cfg, last)
	if d.Run || d.Reason != "already_ran_today" {
		t.Errorf("already ran = %+v", d)
	}
}

func TestDecideTickNewDayAfterLastRun(t *testing.T) {
	cfg := Config{Enabled: true, SleepWindow: "02:00-05:00", Timezone: "UTC"}
	now := time.Date(2026, 4, 6, 3, 30, 0, 0, time.UTC)
	last := time.Date(2026, 4, 5, 2, 10, 0, 0, time.UTC)
	d := decideTick(now, cfg, last)
	if !d.Run {
		t.Errorf("next day = %+v", d)
	}
}

func TestDecideTickWrapAroundWindow(t *testing.T) {
	cfg := Config{Enabled: true, SleepWindow: "22:00-02:00", Timezone: "UTC"}

	// 23:00 on April 5 — first reflection day starts
	now1 := time.Date(2026, 4, 5, 23, 0, 0, 0, time.UTC)
	if d := decideTick(now1, cfg, time.Time{}); !d.Run {
		t.Errorf("23:00 first run: %+v", d)
	}

	// 01:00 on April 6 — still the same reflection day (April 5 anchor)
	last := time.Date(2026, 4, 5, 23, 10, 0, 0, time.UTC)
	now2 := time.Date(2026, 4, 6, 1, 30, 0, 0, time.UTC)
	if d := decideTick(now2, cfg, last); d.Run {
		t.Errorf("01:00 next day, same window = %+v", d)
	}

	// 23:00 on April 6 — next reflection day, should run
	now3 := time.Date(2026, 4, 6, 23, 0, 0, 0, time.UTC)
	if d := decideTick(now3, cfg, last); !d.Run {
		t.Errorf("23:00 next day = %+v", d)
	}
}

func TestWindowAnchorDayWrapBeforeMidnight(t *testing.T) {
	local := time.Date(2026, 4, 5, 23, 30, 0, 0, time.UTC)
	anchor := windowAnchorDay(local, 22*60, 2*60)
	want := time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC)
	if !anchor.Equal(want) {
		t.Errorf("anchor = %v, want %v", anchor, want)
	}
}

func TestWindowAnchorDayWrapAfterMidnight(t *testing.T) {
	local := time.Date(2026, 4, 6, 1, 30, 0, 0, time.UTC)
	anchor := windowAnchorDay(local, 22*60, 2*60)
	want := time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC)
	if !anchor.Equal(want) {
		t.Errorf("anchor = %v, want %v", anchor, want)
	}
}
