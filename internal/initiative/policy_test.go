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
		{"long busy session checks in", GoSignals{Busy: true, LongSession: true, Reachable: true}, TextSignals{}, IntentCheckIn, "long_session"},
		{"long session already checked in", GoSignals{Busy: true, LongSession: true, CheckedInRecently: true, Reachable: true}, TextSignals{}, IntentNone, "busy"},
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
