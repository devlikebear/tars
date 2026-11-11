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
//
// follow_up (tars#1002) is the arrival case's first branch: on the first
// arrival of the day with work left unfinished on an earlier day, it is
// said in place of the greet, not after it. Both would land on the same
// tick (a second speak would only wait out the cooldown), the suggestion is
// the more useful of the two, and it counts as that day's greet
// (History.LastGreetAt) so #1269's once-a-day rule holds as is — a later
// arrival the same day is neither greeted nor asked again. Its match
// condition is the greet case's plus FollowUpPending, so it takes only
// ticks that would otherwise greet; quiet_requested and every hard gate
// still come first, and special_day only ever changed the greet's reason.
//
// share_result (tars#1002) is unrequested news, so it sits after every
// hard gate, the arrival case and Busy (it waits while the user types), but
// before the care cases: a concrete result is more use than asking how the
// user is, and once it is said the cooldown spaces any check-in after it.
// It needs someone reachable, like a strained check-in. Its condition reads
// Go signals only, so on a tick it matches every text combination but
// quiet_requested returns it — the read still happens, the same as for
// greet — and on a tick it does not match, no text combination can reach
// it either (only quiet_requested and special_day come earlier), so
// textSignalsMatter is unchanged there (TestNewCoworkerCasesKeepTextSignalsReasoning).
//
// evening_recap (tars#1001 brief) is unrequested too, so it sits after every
// hard gate, the arrival case and Busy, right after share_result (a concrete
// result first; the recap names today's work anyway) and before the care
// cases, and needs someone reachable. Like share_result it reads Go signals
// only: a tick it matches is one where quiet_requested still turns it into
// none, so the read happens, and on a tick it does not match no text
// combination can reach it (TestEveningRecapKeepsTextSignalsReasoning).
// The morning brief is not a case: it rides on the arrival case's greet or
// follow_up, which is already once a day.
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
	case g.JustArrived && !g.GreetedToday && g.FollowUpPending:
		return speak(IntentFollowUp, "unfinished_work")
	case g.JustArrived && !g.GreetedToday:
		if t.SpecialDay {
			return speak(IntentGreet, "special_day")
		}
		return speak(IntentGreet, "arrived")
	case g.Busy:
		return none("busy")
	case g.ResultReady && g.Reachable:
		return speak(IntentShareResult, "result_ready")
	case g.RecapDue && g.Reachable:
		return speak(IntentEveningRecap, "end_of_day")
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
