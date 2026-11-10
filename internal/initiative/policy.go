package initiative

// Decide turns signals into one companion intent. The order is the policy:
// an explicit request for quiet and the hard limits (including typing or a
// turn in flight) first, then arrival, then care, then a silent body
// expression. It is pure so every rule is table-tested.
//
// GreetedToday (tars#1269) only narrows the arrival case's match condition
// (g.JustArrived && !g.GreetedToday) rather than returning its own "none" —
// a second arrival the same local day is not entitled to a fresh greet, but
// it is still entitled to whatever else the rest of the switch would decide
// (check_in for a strained or long-absent user, body_only for sitting idle
// at the desk, or plain "busy"/"nothing"). Narrowing the case match instead
// of adding an early "none(greeted_today)" return is what lets that
// fallthrough happen for free: an unmatched case simply defers to the next
// one, exactly as if JustArrived had been false, without duplicating the
// rest of the switch. The fact that a greet was suppressed is still visible
// — every Entry carries its own GoSignals (including GreetedToday) in
// Signals, regardless of what Decide ultimately returned.
//
// remind (tars#1001) is the first case: a reminder is the user's own
// request with a time attached, so none of the rules that hold back
// unrequested speech — a quiet request, a stop, quiet hours, cooldown, the
// daily cap — apply to it. Only typing or a turn in flight (Busy) defers it
// a tick, and then the switch reads exactly as if no reminder were due. It
// depends on Go signals alone, so for every tick it matches, all text
// signal combinations return the same decision (textSignalsMatter stays
// false and no text call is made for a reminder).
//
// confirm_memory (tars#1001) is unrequested, so it sits after every hard
// gate, arrival, Busy and every care case, and only takes the moment
// body_only would otherwise use (sitting idle at the console), at most once
// a day. Every text-dependent case comes before it, so it never changes
// what a text signal could do on any tick where it does not match.
func Decide(g GoSignals, t TextSignals) Decision {
	switch {
	case g.ReminderDue && !g.Busy:
		return speak(IntentRemind, "reminder_due")
	case t.QuietRequested:
		return none("quiet_requested")
	case g.StoppedToday:
		return none("stopped_today")
	case g.QuietHours:
		return none("quiet_hours")
	case g.CooldownActive:
		return none("cooldown")
	case g.DailyCapReached:
		return none("daily_cap")
	case g.JustArrived && !g.GreetedToday:
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
	case g.MemoryPending && !g.MemoryAskedToday && g.IdleAtDesk:
		return speak(IntentConfirmMemory, "memory_pending")
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
//
// GreetedToday (tars#1269) needs no special case here either: when it
// blocks the arrival case and nothing else in the switch is true,
// user_strained still cannot turn "none" into a check_in (CheckedInRecently
// or Reachable already rule it out in that fixture) and special_day never
// mattered once the arrival case is unmatched — so every combination keeps
// returning the same Intent/Speak and the function naturally reports false.
// When CheckedInRecently/Reachable do allow a strained check_in, the
// function naturally reports true instead, exactly as it already does for
// any other GoSignals combination where some text signal can change the
// outcome.
//
// The two tars#1001 cases keep this reasoning intact. remind is first and
// reads only Go signals, so a tick it matches returns remind for all eight
// combinations (false: no read). confirm_memory is placed after every case a
// text signal can reach; a tick it matches is one where quiet_requested
// would still turn it into none and user_strained into a check_in, so the
// function reports true and the read happens before TARS asks — the same
// way a body_only tick is read today.
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
