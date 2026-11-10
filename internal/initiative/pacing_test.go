package initiative

import (
	"testing"
	"time"
)

func pacingEntries(now time.Time, replied, ignored, stopped int) []Entry {
	var out []Entry
	add := func(n int, outcome string) {
		for i := 0; i < n; i++ {
			at := now.Add(-time.Duration(len(out)+1) * time.Hour)
			out = append(out, Entry{Kind: entryKindOutcome, At: at, Outcome: outcome})
		}
	}
	add(replied, OutcomeReplied)
	add(ignored, OutcomeIgnored)
	add(stopped, OutcomeStopped)
	return out
}

func TestSuggestPacing(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	base := PacingInput{DailyCap: 6, Cooldown: 45 * time.Minute}
	capDay := func() []Entry {
		var out []Entry
		for i := 0; i < 6; i++ {
			out = append(out, Entry{At: now.Add(-time.Duration(i) * time.Minute), Mode: ModeLive,
				Decision: speak(IntentCheckIn, "long_session"), Delivery: DeliveryDelivered})
		}
		return out
	}
	cooldownBlock := []Entry{{At: now.Add(-time.Minute), Decision: none("cooldown")}}
	old := []Entry{{Kind: entryKindOutcome, At: now.Add(-30 * 24 * time.Hour), Outcome: OutcomeStopped}}

	cases := []struct {
		name     string
		entries  []Entry
		in       PacingInput
		verdict  string
		reason   string
		cap      int
		cooldown time.Duration
	}{
		{"too few", pacingEntries(now, 3, 3, 0), base, PacingInsufficient, "too_few_outcomes", 6, 45 * time.Minute},
		{"old outcomes do not count", append(pacingEntries(now, 9, 0, 0), old...), base, PacingInsufficient, "too_few_outcomes", 6, 45 * time.Minute},
		{"mostly ignored", pacingEntries(now, 3, 7, 0), base, PacingBackOff, "mostly_ignored", 5, 90 * time.Minute},
		{"asked to stop twice", pacingEntries(now, 8, 0, 2), base, PacingBackOff, "asked_to_stop", 5, 90 * time.Minute},
		{"back off is bounded", pacingEntries(now, 0, 10, 0), PacingInput{DailyCap: 1, Cooldown: 6 * time.Hour}, PacingBackOff, "mostly_ignored", 1, 8 * time.Hour},
		{"back off never shortens a long cooldown", pacingEntries(now, 0, 10, 0), PacingInput{DailyCap: 3, Cooldown: 10 * time.Hour}, PacingBackOff, "mostly_ignored", 2, 10 * time.Hour},
		{"mixed", pacingEntries(now, 5, 5, 0), base, PacingKeep, "mixed", 6, 45 * time.Minute},
		{"replied but no limit binds", pacingEntries(now, 9, 1, 0), base, PacingKeep, "replied_but_limits_not_binding", 6, 45 * time.Minute},
		{"replied and cap reached", append(pacingEntries(now, 9, 1, 0), capDay()...), base, PacingLeanIn, "mostly_replied", 7, 45 * time.Minute},
		{"replied and cooldown held it back", append(pacingEntries(now, 9, 1, 0), cooldownBlock...), base, PacingLeanIn, "mostly_replied", 6, 33*time.Minute + 45*time.Second},
		{"lean in never lengthens a short cooldown", append(pacingEntries(now, 10, 0, 0), cooldownBlock...), PacingInput{DailyCap: 6, Cooldown: 10 * time.Minute}, PacingKeep, "replied_but_limits_not_binding", 6, 10 * time.Minute},
		{"one stop blocks leaning in", append(pacingEntries(now, 9, 0, 1), capDay()...), base, PacingKeep, "mixed", 6, 45 * time.Minute},
	}
	for _, c := range cases {
		got := SuggestPacing(c.entries, now, c.in)
		if got.Verdict != c.verdict || got.Reason != c.reason || got.SuggestedDailyCap != c.cap || got.SuggestedCooldown != c.cooldown {
			t.Errorf("%s: got %+v", c.name, got)
		}
	}
}

func TestSuggestPacingCountsCapHitDaysInLocalTime(t *testing.T) {
	loc := time.FixedZone("KST", 9*3600)
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, loc)
	var entries []Entry
	// Two speaks on each side of local midnight: no single local day hits
	// a cap of 3.
	for _, at := range []time.Time{
		time.Date(2026, 10, 19, 23, 0, 0, 0, loc), time.Date(2026, 10, 19, 23, 30, 0, 0, loc),
		time.Date(2026, 10, 20, 0, 10, 0, 0, loc), time.Date(2026, 10, 20, 0, 20, 0, 0, loc),
	} {
		entries = append(entries, Entry{At: at, Mode: ModeShadow, Decision: speak(IntentGreet, "arrived")})
	}
	got := SuggestPacing(entries, now, PacingInput{DailyCap: 3, Cooldown: time.Hour, Location: loc, MinOutcomes: 1})
	if got.CapHitDays != 0 || got.Verdict != PacingInsufficient {
		t.Fatalf("got %+v", got)
	}
}
