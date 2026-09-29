package initiative

// Decide turns signals into one companion intent. The order is the policy:
// an explicit request for quiet and the hard limits first, then arrival,
// then care, then a silent body expression. It is pure so every rule is
// table-tested.
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
		if g.LongSession && !g.CheckedInRecently {
			return speak(IntentCheckIn, "long_session")
		}
		return none("busy")
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
