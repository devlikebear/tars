package initiative

import "time"

// Pacing suggestion verdicts (SuggestPacing).
const (
	PacingInsufficient = "insufficient"
	PacingKeep         = "keep"
	PacingBackOff      = "back_off"
	PacingLeanIn       = "lean_in"
)

const (
	defaultPacingWindow      = 14 * 24 * time.Hour
	defaultPacingMinOutcomes = 10
	pacingMinCooldown        = 15 * time.Minute
	pacingMaxCooldown        = 8 * time.Hour
	pacingBackOffIgnoredRate = 0.6
	pacingBackOffStopped     = 2
	pacingLeanInRepliedRate  = 0.7
)

// PacingInput is the configured pacing a suggestion starts from.
type PacingInput struct {
	DailyCap int
	Cooldown time.Duration
	Location *time.Location
	// Window is how far back outcomes count (default 14 days);
	// MinOutcomes is how many resolved outcomes it takes to suggest
	// anything (default 10).
	Window      time.Duration
	MinOutcomes int
}

// PacingSuggestion is SuggestPacing's verdict: the outcomes it was based
// on, how often each limit actually held TARS back, and the daily_cap and
// cooldown it would set. It is advice for `tars initiative adapt` to
// print; nothing applies it.
type PacingSuggestion struct {
	Verdict           string        `json:"verdict"`
	Reason            string        `json:"reason"`
	Outcomes          OutcomeCounts `json:"outcomes"`
	CapHitDays        int           `json:"cap_hit_days"`
	CooldownBlocks    int           `json:"cooldown_blocks"`
	CurrentDailyCap   int           `json:"current_daily_cap"`
	CurrentCooldown   time.Duration `json:"current_cooldown"`
	SuggestedDailyCap int           `json:"suggested_daily_cap"`
	SuggestedCooldown time.Duration `json:"suggested_cooldown"`
}

// SuggestPacing reads reply-tracking outcomes (tars#1221) over a window and
// suggests the user's own daily_cap and cooldown (tars#1003's "per-user
// adaptation"). It complements, and never replaces, what the runtime
// already does on its own: EffectiveCooldown doubles the cooldown for
// every ignored speak in a row and a single reply resets it, and a
// "stopped" reply silences the rest of the day. Those react to a streak;
// this looks at the rate over weeks, which a streak cannot see (replies
// and ignores alternating never back off, and nothing ever leans in).
//
//   - back_off: two or more "stopped" replies, or at least 60% ignored —
//     double the cooldown (at most 8h) and lower the daily cap by one (at
//     least 1).
//   - lean_in: at least 70% replied and never stopped — raise the cap by
//     one only if the cap was actually reached on some day, and shorten
//     the cooldown by a quarter (at least 15m) only if it actually held a
//     decision back; with neither limit binding there is nothing to change
//     and the verdict is keep.
//   - insufficient: fewer than MinOutcomes resolved outcomes.
//
// Pure: entries in, advice out.
func SuggestPacing(entries []Entry, now time.Time, in PacingInput) PacingSuggestion {
	if in.Window <= 0 {
		in.Window = defaultPacingWindow
	}
	if in.MinOutcomes <= 0 {
		in.MinOutcomes = defaultPacingMinOutcomes
	}
	if in.Location == nil {
		in.Location = time.UTC
	}
	s := PacingSuggestion{
		CurrentDailyCap: in.DailyCap, CurrentCooldown: in.Cooldown,
		SuggestedDailyCap: in.DailyCap, SuggestedCooldown: in.Cooldown,
	}
	since := now.Add(-in.Window)
	spokenPerDay := map[string]int{}
	for _, e := range entries {
		if e.At.Before(since) || e.At.After(now) {
			continue
		}
		switch {
		case e.Kind == entryKindOutcome:
			s.Outcomes.add(e.Outcome)
		case e.spoken():
			spokenPerDay[e.At.In(in.Location).Format("2006-01-02")]++
		case e.Intent == IntentNone && e.Reason == "cooldown":
			s.CooldownBlocks++
		}
	}
	for _, n := range spokenPerDay {
		if in.DailyCap > 0 && n >= in.DailyCap {
			s.CapHitDays++
		}
	}
	total := s.Outcomes.Replied + s.Outcomes.Ignored + s.Outcomes.Stopped
	if total < in.MinOutcomes {
		s.Verdict, s.Reason = PacingInsufficient, "too_few_outcomes"
		return s
	}
	ignoredRate := float64(s.Outcomes.Ignored) / float64(total)
	repliedRate := float64(s.Outcomes.Replied) / float64(total)
	switch {
	case s.Outcomes.Stopped >= pacingBackOffStopped || ignoredRate >= pacingBackOffIgnoredRate:
		s.Verdict = PacingBackOff
		s.Reason = "mostly_ignored"
		if s.Outcomes.Stopped >= pacingBackOffStopped {
			s.Reason = "asked_to_stop"
		}
		if c := min(in.Cooldown*2, pacingMaxCooldown); c > in.Cooldown {
			s.SuggestedCooldown = c
		}
		s.SuggestedDailyCap = max(in.DailyCap-1, 1)
	case repliedRate >= pacingLeanInRepliedRate && s.Outcomes.Stopped == 0:
		if s.CapHitDays > 0 {
			s.SuggestedDailyCap = in.DailyCap + 1
		}
		if c := max(in.Cooldown*3/4, pacingMinCooldown); s.CooldownBlocks > 0 && c < in.Cooldown {
			s.SuggestedCooldown = c
		}
		if s.SuggestedDailyCap == in.DailyCap && s.SuggestedCooldown == in.Cooldown {
			s.Verdict, s.Reason = PacingKeep, "replied_but_limits_not_binding"
			return s
		}
		s.Verdict, s.Reason = PacingLeanIn, "mostly_replied"
	default:
		s.Verdict, s.Reason = PacingKeep, "mixed"
	}
	return s
}
