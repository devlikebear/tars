package computeruse

import (
	"testing"

	"github.com/devlikebear/tars/internal/jev"
)

func TestBuildQuestions_ShapesAndNoneOption(t *testing.T) {
	shown := []Element{{Index: 1, Role: "AXButton", Label: "OK", Enabled: true}, {Index: 2, Role: "AXTextField", Label: "Name", Enabled: true}}
	qs := BuildQuestions(shown, []string{"name"})
	for _, k := range []string{"op", "target", "input_key", "risky", "done"} {
		if _, ok := qs[k]; !ok {
			t.Fatalf("missing question %s", k)
		}
	}
	target := qs["target"].Criteria.(map[string]string)
	if len(target) != 3 || target["none"] == "" || target["e1"] != ElementLine(shown[0], false) {
		t.Fatalf("target criteria = %v", target)
	}
	ops := qs["op"].Criteria.(map[string]string)
	for _, op := range []string{OpClick, OpType, OpSetValue, OpPressEnter, OpPressEscape, OpScrollDown, OpScrollUp, OpLook, OpDone, OpNone} {
		if ops[op] == "" {
			t.Errorf("op %s missing", op)
		}
	}
	if ik := qs["input_key"].Criteria.(map[string]string); ik["name"] == "" || ik["none"] == "" {
		t.Errorf("input_key criteria = %v", ik)
	}
	if qs["risky"].Type != "noul" || qs["done"].Type != "noul" {
		t.Errorf("risky/done must be noul")
	}
	if _, ok := BuildQuestions(shown, nil)["input_key"]; ok {
		t.Error("input_key must be omitted without inputs")
	}
}

func TestParseDecision(t *testing.T) {
	resp := jev.Response{Answers: map[string]jev.Answer{
		"op":        {Type: "choice", Choice: "type", Confidence: 0.9},
		"target":    {Type: "choice", Choice: "e2", Confidence: 0.8},
		"input_key": {Type: "choice", Choice: "name", Confidence: 0.95},
		"risky":     {Type: "noul", Noul: 0.1},
		"done":      {Type: "noul", Noul: 0.02},
	}}
	d, err := ParseDecision(resp)
	if err != nil {
		t.Fatal(err)
	}
	if d.Op != OpType || d.TargetIndex != 2 || d.InputKey != "name" || d.Risky != 0.1 || d.Done != 0.02 || d.OpConfidence != 0.9 {
		t.Fatalf("decision = %+v", d)
	}
	resp.Answers["target"] = jev.Answer{Type: "choice", Choice: "none", Confidence: 0.7}
	if d, _ := ParseDecision(resp); d.TargetIndex != 0 {
		t.Fatalf("none must map to 0, got %+v", d)
	}
	delete(resp.Answers, "op")
	if _, err := ParseDecision(resp); err == nil {
		t.Fatal("missing op must error")
	}
}

func TestCompatible(t *testing.T) {
	cases := []struct {
		op, role string
		want     bool
	}{
		{OpType, "AXTextField", true}, {OpType, "AXButton", false},
		{OpSetValue, "AXPopUpButton", true}, {OpSetValue, "AXCheckBox", true}, {OpSetValue, "AXButton", false},
		{OpClick, "AXButton", true}, {OpClick, "AXTextField", true}, {OpClick, "AXSecureTextField", true},
		// secure fields are typeable; the engine gates them behind confirmation
		{OpType, "AXSecureTextField", true},
		{OpPressEnter, "", true}, {OpLook, "", true},
	}
	for _, c := range cases {
		if got := Compatible(c.op, c.role); got != c.want {
			t.Errorf("Compatible(%s,%s)=%v want %v", c.op, c.role, got, c.want)
		}
	}
	if !NeedsTarget(OpClick) || NeedsTarget(OpPressEnter) || NeedsTarget(OpDone) {
		t.Error("NeedsTarget wrong")
	}
}

func TestParseDecision_TargetMargin(t *testing.T) {
	base := map[string]jev.Answer{
		"op":    {Type: "choice", Choice: "click", Confidence: 0.9},
		"risky": {Type: "noul", Noul: 0.1},
		"done":  {Type: "noul", Noul: 0.0},
	}
	with := func(a jev.Answer) jev.Response {
		ans := map[string]jev.Answer{"target": a}
		for k, v := range base {
			ans[k] = v
		}
		return jev.Response{Answers: ans}
	}
	cases := []struct {
		name   string
		answer jev.Answer
		want   float64
	}{
		{"lead over runner-up", jev.Answer{Type: "choice", Choice: "e3", Confidence: 0.3,
			Probabilities: map[string]float64{"e3": 0.45, "e7": 0.10, "none": 0.05}}, 0.35},
		{"no probabilities", jev.Answer{Type: "choice", Choice: "e3", Confidence: 0.3}, 0},
		{"single option", jev.Answer{Type: "choice", Choice: "e3", Probabilities: map[string]float64{"e3": 0.9}}, 0},
		{"chosen absent", jev.Answer{Type: "choice", Choice: "e3",
			Probabilities: map[string]float64{"e1": 0.5, "e2": 0.4}}, 0},
		{"chosen trails", jev.Answer{Type: "choice", Choice: "e3",
			Probabilities: map[string]float64{"e3": 0.2, "e1": 0.5}}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := ParseDecision(with(tc.answer))
			if err != nil {
				t.Fatal(err)
			}
			if diff := d.TargetMargin - tc.want; diff > 1e-9 || diff < -1e-9 {
				t.Fatalf("margin=%v want %v", d.TargetMargin, tc.want)
			}
		})
	}
}
