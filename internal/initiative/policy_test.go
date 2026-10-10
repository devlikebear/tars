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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := textSignalsMatter(tt.g); got != tt.matter {
				t.Fatalf("textSignalsMatter(%+v) = %v, want %v", tt.g, got, tt.matter)
			}
		})
	}
}
