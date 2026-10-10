package initiative

// Decide turns signals into one companion intent. The order is the policy:
// an explicit request for quiet and the hard limits (including typing or a
// turn in flight) first, then arrival, then care, then a silent body
// expression. It is pure so every rule is table-tested.
func Decide(g GoSignals, t TextSignals) Decision {
	switch {
	case t.QuietRequested:
		return none("quiet_requested")
	case g.QuietHours:
		return none("quiet_hours")
	case g.CooldownActive:
		return none("cooldown")
	case g.DailyCapReached:
		return none("daily_cap")
	case g.JustArrived:
		if t.SpecialDay {
			return speak(IntentGreet, "special_day")
		}
		return speak(IntentGreet, "arrived")
	case g.Busy:
		return none("busy")
	case g.LongSession && !g.CheckedInRecently:
		return speak(IntentCheckIn, "long_session")
	case t.UserStrained && !g.CheckedInRecently && g.Reachable:
		return speak(IntentCheckIn, "strained")
	case g.LongAbsence:
		if !g.Reachable {
			return none("unreachable")
		}
		return speak(IntentCheckIn, "long_absence")
	case g.IdleAtDesk && g.BodyAvailable && !g.BodyRecently:
		return Decision{Intent: IntentBodyOnly, Reason: "idle_at_desk"}
	default:
		return none("nothing")
	}
}

func none(reason string) Decision { return Decision{Intent: IntentNone, Reason: reason} }

func speak(intent Intent, reason string) Decision {
	return Decision{Intent: intent, Speak: true, Reason: reason}
}

// textSignalsMatter reports whether some combination of the three atomic
// text-signal booleans would change what Decide actually does (Intent and
// Speak — not the Reason string, which may legitimately vary between two
// "none" outcomes such as quiet_requested vs quiet_hours without changing
// behavior) compared to assuming every text signal is false (what an
// unread tick already falls back to). When nothing would change, reading
// the user's own words would only relabel an already-fixed outcome, so the
// text-signal backend is not worth calling this tick (tars#1219) — this is
// what lets a quiet-hours, cooldown, daily-cap or (when not arriving)
// merely-typing tick skip the call without hard-coding any of those cases
// here: Decide already returns the same Intent/Speak for every text
// combination in each of them.
//
// JustArrived deliberately is NOT given the same treatment as Busy: it is
// checked before Busy in Decide, so on a tick that is both just-arrived and
// busy, a true quiet_requested still overrides the greet (Decide checks
// quiet_requested first of all) — skipping the read because of Busy would
// silently drop that override and greet through a requested quiet.
func textSignalsMatter(g GoSignals) bool {
	base := Decide(g, TextSignals{})
	for mask := 1; mask < 8; mask++ {
		t := TextSignals{
			QuietRequested: mask&1 != 0,
			UserStrained:   mask&2 != 0,
			SpecialDay:     mask&4 != 0,
		}
		d := Decide(g, t)
		if d.Intent != base.Intent || d.Speak != base.Speak {
			return true
		}
	}
	return false
}
