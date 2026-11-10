package initiative

import "fmt"

// DecideScenario is one line of the fixed evaluation set for Decide
// (testdata/eval/decide.jsonl, tars#1003): the Go signals and text signals
// of one tick and what the policy must decide for them. A policy change
// has to keep every scenario passing (eval_test.go) — the regression gate
// a model or threshold change is measured against before it ships.
// `tars initiative scenarios` turns labeled export records into lines of
// this shape.
type DecideScenario struct {
	Name    string       `json:"name"`
	Signals GoSignals    `json:"signals"`
	Text    ScenarioText `json:"text"`
	Want    ScenarioWant `json:"want"`
}

// ScenarioText is the three atomic text signals a scenario feeds Decide.
type ScenarioText struct {
	QuietRequested bool `json:"quiet_requested,omitempty"`
	UserStrained   bool `json:"user_strained,omitempty"`
	SpecialDay     bool `json:"special_day,omitempty"`
}

// ScenarioWant is what a scenario expects. An empty Intent, a nil Speak or
// an empty Reason is not checked, so a user correction that only says
// "should not have spoken" pins exactly that and nothing more.
type ScenarioWant struct {
	Intent Intent `json:"intent,omitempty"`
	Speak  *bool  `json:"speak,omitempty"`
	Reason string `json:"reason,omitempty"`
}

func (w ScenarioWant) empty() bool { return w.Intent == "" && w.Speak == nil && w.Reason == "" }

// Run feeds the scenario to Decide and reports the decision and, when it
// does not match Want, why.
func (s DecideScenario) Run() (Decision, error) {
	d := Decide(s.Signals, TextSignals{
		QuietRequested: s.Text.QuietRequested,
		UserStrained:   s.Text.UserStrained,
		SpecialDay:     s.Text.SpecialDay,
	})
	if s.Want.empty() {
		return d, fmt.Errorf("scenario %q has no expectation", s.Name)
	}
	if s.Want.Intent != "" && d.Intent != s.Want.Intent {
		return d, fmt.Errorf("intent = %s, want %s", d.Intent, s.Want.Intent)
	}
	if s.Want.Speak != nil && d.Speak != *s.Want.Speak {
		return d, fmt.Errorf("speak = %t, want %t", d.Speak, *s.Want.Speak)
	}
	if s.Want.Reason != "" && d.Reason != s.Want.Reason {
		return d, fmt.Errorf("reason = %s, want %s", d.Reason, s.Want.Reason)
	}
	return d, nil
}

// ScenariosFromDataset turns exported records into Decide scenarios.
//
// A record with a corrected intent or speak in its label becomes a
// scenario expecting that correction, fed the recorded text signals with
// any corrected text booleans laid over them — what the policy should
// have done had it read the user right. With all set, a record without
// such a correction becomes a scenario pinning the decision it actually
// recorded, unless its label corrects a text signal (the recorded decision
// was made on the uncorrected reading, so it is no expectation for the
// corrected one).
func ScenariosFromDataset(records []DatasetRecord, all bool) []DecideScenario {
	var out []DecideScenario
	for _, r := range records {
		text := ScenarioText{
			QuietRequested: r.Text.QuietRequested,
			UserStrained:   r.Text.UserStrained,
			SpecialDay:     r.Text.SpecialDay,
		}
		textCorrected := false
		if v := r.Label.QuietRequested; v != nil {
			text.QuietRequested, textCorrected = *v, textCorrected || *v != r.Text.QuietRequested
		}
		if v := r.Label.UserStrained; v != nil {
			text.UserStrained, textCorrected = *v, textCorrected || *v != r.Text.UserStrained
		}
		if v := r.Label.SpecialDay; v != nil {
			text.SpecialDay, textCorrected = *v, textCorrected || *v != r.Text.SpecialDay
		}
		var want ScenarioWant
		name := "ledger " + r.ID
		switch {
		case r.Label.Intent != nil || r.Label.Speak != nil:
			if r.Label.Intent != nil {
				want.Intent = *r.Label.Intent
			}
			want.Speak = r.Label.Speak
			name += " (corrected)"
		case all && !textCorrected && r.Decision.Intent != "":
			speak := r.Decision.Speak
			want = ScenarioWant{Intent: r.Decision.Intent, Speak: &speak, Reason: r.Decision.Reason}
		default:
			continue
		}
		out = append(out, DecideScenario{Name: name, Signals: r.Signals, Text: text, Want: want})
	}
	return out
}
