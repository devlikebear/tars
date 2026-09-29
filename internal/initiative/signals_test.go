package initiative

import (
	"testing"
	"time"
)

var seoul = time.FixedZone("KST", 9*3600)

func at(h, m int) time.Time { return time.Date(2026, 9, 29, h, m, 0, 0, seoul) }

func TestQuietWindowCrossesMidnight(t *testing.T) {
	cases := map[time.Time]bool{at(23, 30): true, at(6, 59): true, at(7, 0): false, at(12, 0): false}
	for ts, want := range cases {
		if got := inQuietWindow("23:00-07:00", ts); got != want {
			t.Errorf("inQuietWindow(%s) = %v, want %v", ts.Format("15:04"), got, want)
		}
	}
	if !inQuietWindow("12:00-13:00", at(12, 30)) || inQuietWindow("12:00-13:00", at(13, 0)) {
		t.Error("same-day window bounds are wrong")
	}
	if inQuietWindow("", at(3, 0)) || inQuietWindow("garbage", at(3, 0)) {
		t.Fatal("empty or invalid window must never be quiet")
	}
}

func TestDeriveGoSignals(t *testing.T) {
	cfg := Config{Location: seoul}.WithDefaults()
	now := at(14, 0)
	tests := []struct {
		name string
		obs  Observation
		hist History
		want func(GoSignals) bool
	}{
		{"typing within window is busy",
			Observation{Now: now, RecentUser: []UserMessage{{At: now.Add(-time.Minute)}}},
			History{}, func(g GoSignals) bool { return g.Busy }},
		{"chat in flight is busy",
			Observation{Now: now, ChatBusy: true},
			History{}, func(g GoSignals) bool { return g.Busy }},
		{"console opened after a long gap is arrival",
			Observation{Now: now, ConsoleConnectedAt: now.Add(-2 * time.Minute), RecentUser: []UserMessage{{At: now.Add(-13 * time.Hour)}}},
			History{}, func(g GoSignals) bool { return g.JustArrived && g.Reachable }},
		{"console opened with no history is arrival",
			Observation{Now: now, ConsoleConnectedAt: now.Add(-2 * time.Minute)},
			History{}, func(g GoSignals) bool { return g.JustArrived }},
		{"typing since the console opened is not arrival",
			Observation{Now: now, ConsoleConnectedAt: now.Add(-5 * time.Minute), RecentUser: []UserMessage{{At: now.Add(-6 * time.Hour)}, {At: now.Add(-time.Minute)}}},
			History{}, func(g GoSignals) bool { return !g.JustArrived }},
		{"a console open for an hour is no longer arrival",
			Observation{Now: now, ConsoleConnectedAt: now.Add(-time.Hour), RecentUser: []UserMessage{{At: now.Add(-13 * time.Hour)}}},
			History{}, func(g GoSignals) bool { return !g.JustArrived }},
		{"three hours of steady messages is a long session",
			Observation{Now: now, RecentUser: steady(now, 3*time.Hour+10*time.Minute, 20*time.Minute)},
			History{}, func(g GoSignals) bool { return g.LongSession }},
		{"a gap breaks the session",
			Observation{Now: now, RecentUser: append(steady(now.Add(-2*time.Hour), time.Hour, 10*time.Minute), steady(now, time.Hour, 10*time.Minute)...)},
			History{}, func(g GoSignals) bool { return !g.LongSession }},
		{"a day of silence is a long absence",
			Observation{Now: now, TelegramPaired: true, RecentUser: []UserMessage{{At: now.Add(-26 * time.Hour)}}},
			History{}, func(g GoSignals) bool { return g.LongAbsence && g.Reachable }},
		{"console idle 20m is idle at desk",
			Observation{Now: now, ConsoleConnectedAt: now.Add(-3 * time.Hour), RecentUser: []UserMessage{{At: now.Add(-20 * time.Minute)}}},
			History{}, func(g GoSignals) bool { return g.IdleAtDesk }},
		{"no console means not reachable",
			Observation{Now: now, RecentUser: []UserMessage{{At: now.Add(-26 * time.Hour)}}},
			History{}, func(g GoSignals) bool { return !g.Reachable }},
		{"cooldown and cap come from history",
			Observation{Now: now},
			History{Today: 6, LastSpokeAt: now.Add(-10 * time.Minute)}, func(g GoSignals) bool { return g.CooldownActive && g.DailyCapReached }},
		{"recent check-in and body are spaced",
			Observation{Now: now},
			History{LastCheckInAt: now.Add(-time.Hour), LastBodyAt: now.Add(-10 * time.Minute)}, func(g GoSignals) bool { return g.CheckedInRecently && g.BodyRecently }},
		{"quiet hours use the configured zone",
			Observation{Now: time.Date(2026, 9, 29, 15, 0, 0, 0, time.UTC)}, // 00:00 KST
			History{}, func(g GoSignals) bool { return g.QuietHours }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if g := deriveGoSignals(cfg, tt.obs, tt.hist); !tt.want(g) {
				t.Fatalf("signals = %+v", g)
			}
		})
	}
}

// steady returns user messages every step, ending at end and spanning span.
func steady(end time.Time, span, step time.Duration) []UserMessage {
	var out []UserMessage
	for ts := end.Add(-span); !ts.After(end); ts = ts.Add(step) {
		out = append(out, UserMessage{At: ts})
	}
	return out
}
