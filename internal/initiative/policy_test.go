package initiative

import "testing"

func TestDecide(t *testing.T) {
	tests := []struct {
		name   string
		g      GoSignals
		t      TextSignals
		intent Intent
		reason string
	}{
		{"quiet request wins over everything", GoSignals{JustArrived: true, Reachable: true}, TextSignals{QuietRequested: true}, IntentNone, "quiet_requested"},
		{"stopped today wins over arrival", GoSignals{StoppedToday: true, JustArrived: true, Reachable: true}, TextSignals{}, IntentNone, "stopped_today"},
		{"quiet request still wins over stopped today", GoSignals{StoppedToday: true, JustArrived: true, Reachable: true}, TextSignals{QuietRequested: true}, IntentNone, "quiet_requested"},
		{"quiet hours", GoSignals{QuietHours: true, JustArrived: true, Reachable: true}, TextSignals{}, IntentNone, "quiet_hours"},
		{"cooldown", GoSignals{CooldownActive: true, JustArrived: true, Reachable: true}, TextSignals{}, IntentNone, "cooldown"},
		{"daily cap", GoSignals{DailyCapReached: true, JustArrived: true, Reachable: true}, TextSignals{}, IntentNone, "daily_cap"},
		{"arrival greets", GoSignals{JustArrived: true, Reachable: true}, TextSignals{}, IntentGreet, "arrived"},
		{"arrival on a special day", GoSignals{JustArrived: true, Reachable: true}, TextSignals{SpecialDay: true}, IntentGreet, "special_day"},
		{"busy stays quiet", GoSignals{Busy: true, Reachable: true}, TextSignals{UserStrained: true}, IntentNone, "busy"},
		{"typing through a long session stays quiet", GoSignals{Busy: true, LongSession: true, Reachable: true}, TextSignals{}, IntentNone, "busy"},
		{"a pause in a long session checks in", GoSignals{LongSession: true, Reachable: true}, TextSignals{}, IntentCheckIn, "long_session"},
		{"long session already checked in", GoSignals{LongSession: true, CheckedInRecently: true, Reachable: true}, TextSignals{}, IntentNone, "nothing"},
		{"strained user gets a check-in", GoSignals{Reachable: true}, TextSignals{UserStrained: true}, IntentCheckIn, "strained"},
		{"strain already checked in", GoSignals{Reachable: true, CheckedInRecently: true}, TextSignals{UserStrained: true}, IntentNone, "nothing"},
		{"long absence checks in", GoSignals{LongAbsence: true, Reachable: true}, TextSignals{}, IntentCheckIn, "long_absence"},
		{"unreachable stays silent", GoSignals{LongAbsence: true}, TextSignals{}, IntentNone, "unreachable"},
		{"idle at desk gets a body expression", GoSignals{IdleAtDesk: true, BodyAvailable: true, Reachable: true}, TextSignals{}, IntentBodyOnly, "idle_at_desk"},
		{"body expressions are spaced", GoSignals{IdleAtDesk: true, BodyAvailable: true, BodyRecently: true, Reachable: true}, TextSignals{}, IntentNone, "nothing"},
		{"idle without a body does nothing", GoSignals{IdleAtDesk: true, Reachable: true}, TextSignals{}, IntentNone, "nothing"},
		{"nothing to do", GoSignals{Reachable: true}, TextSignals{}, IntentNone, "nothing"},

		// tars#1269: a greet already counted as spoken today narrows the
		// arrival case's match condition only — it does not add a new
		// "none" return of its own, so once the arrival case is unmatched,
		// the rest of the switch decides exactly as if JustArrived had been
		// false all along.
		{"greeted today with nothing else pending does nothing", GoSignals{JustArrived: true, GreetedToday: true, Reachable: true}, TextSignals{}, IntentNone, "nothing"},
		{"greeted today blocks a special day greet too", GoSignals{JustArrived: true, GreetedToday: true, Reachable: true}, TextSignals{SpecialDay: true}, IntentNone, "nothing"},
		{"greeted today with typing falls through to busy", GoSignals{JustArrived: true, GreetedToday: true, Busy: true, Reachable: true}, TextSignals{}, IntentNone, "busy"},
		{"greeted today still allows a strained check-in", GoSignals{JustArrived: true, GreetedToday: true, Reachable: true}, TextSignals{UserStrained: true}, IntentCheckIn, "strained"},
		{"quiet request still wins when greeted today too", GoSignals{JustArrived: true, GreetedToday: true, Reachable: true}, TextSignals{QuietRequested: true}, IntentNone, "quiet_requested"},
		{"greeted today does not block a long-session check-in", GoSignals{GreetedToday: true, LongSession: true, Reachable: true}, TextSignals{}, IntentCheckIn, "long_session"},
		{"greeted today does not block a long-absence check-in", GoSignals{GreetedToday: true, LongAbsence: true, Reachable: true}, TextSignals{}, IntentCheckIn, "long_absence"},
		{"greeted today does not block a body expression", GoSignals{GreetedToday: true, IdleAtDesk: true, BodyAvailable: true, Reachable: true}, TextSignals{}, IntentBodyOnly, "idle_at_desk"},
		{"not greeted today still greets on arrival", GoSignals{JustArrived: true, GreetedToday: false, Reachable: true}, TextSignals{}, IntentGreet, "arrived"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Decide(tt.g, tt.t)
			if d.Intent != tt.intent || d.Reason != tt.reason {
				t.Fatalf("Decide = %+v, want %s/%s", d, tt.intent, tt.reason)
			}
			if d.Speak != (tt.intent == IntentGreet || tt.intent == IntentCheckIn) {
				t.Fatalf("speak = %v for %s", d.Speak, d.Intent)
			}
		})
	}
}

// TestJustArrivedAndBusyStillHonorsQuietRequest guards the exact scenario
// the no-separate-Busy-gate design relies on: Decide checks quiet_requested
// before JustArrived, so a text read that discovers the user asked for
// quiet must still turn a greet into silence even while the user is typing
// (Busy) and just connected (JustArrived) — nothing may skip reading text
// here just because the user looks busy.
func TestJustArrivedAndBusyStillHonorsQuietRequest(t *testing.T) {
	g := GoSignals{JustArrived: true, Busy: true, Reachable: true}
	d := Decide(g, TextSignals{QuietRequested: true})
	if d.Intent != IntentNone || d.Speak || d.Reason != "quiet_requested" {
		t.Fatalf("Decide = %+v, want none/quiet_requested", d)
	}
}

func TestTextSignalsMatter(t *testing.T) {
	tests := []struct {
		name   string
		g      GoSignals
		matter bool
	}{
		{"quiet hours blocks regardless of text", GoSignals{QuietHours: true}, false},
		{"cooldown blocks regardless of text", GoSignals{CooldownActive: true}, false},
		{"daily cap blocks regardless of text", GoSignals{DailyCapReached: true}, false},
		{"busy with no arrival blocks regardless of text", GoSignals{Busy: true, Reachable: true}, false},
		{"unreachable idle stays none regardless of text", GoSignals{IdleAtDesk: true}, false},
		{"just arrived depends on quiet_requested even while busy", GoSignals{JustArrived: true, Busy: true, Reachable: true}, true},
		{"just arrived depends on text", GoSignals{JustArrived: true, Reachable: true}, true},
		{"long session depends on quiet_requested", GoSignals{LongSession: true, Reachable: true}, true},
		{"reachable idle depends on strain and quiet_requested", GoSignals{Reachable: true}, true},
		{"idle at desk with a body depends on text", GoSignals{IdleAtDesk: true, BodyAvailable: true, Reachable: true}, true},
		{"long absence depends on quiet_requested", GoSignals{LongAbsence: true, Reachable: true}, true},

		// tars#1269: GreetedToday needs no special case in textSignalsMatter
		// itself — when it blocks the arrival case and nothing else in the
		// fixture is true, a strained check-in is already ruled out by
		// CheckedInRecently, so every text combination still returns the
		// same "none(nothing)" and the function reports false; when
		// CheckedInRecently/Reachable do allow a strained check-in, the
		// function reports true exactly as it already would without
		// GreetedToday in the picture.
		{"greeted today blocks arrival but nothing else can change the outcome", GoSignals{JustArrived: true, GreetedToday: true, Reachable: true, CheckedInRecently: true}, false},
		{"greeted today blocks arrival but a strained check-in still depends on text", GoSignals{JustArrived: true, GreetedToday: true, Reachable: true}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := textSignalsMatter(tt.g); got != tt.matter {
				t.Fatalf("textSignalsMatter(%+v) = %v, want %v", tt.g, got, tt.matter)
			}
		})
	}
}
