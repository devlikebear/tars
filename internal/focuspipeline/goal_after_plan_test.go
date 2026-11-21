package focuspipeline

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGoalDueOnApproval(t *testing.T) {
	p := New("s", "g", t0)
	if GoalDueOnApproval(p, GatePlan, GateApprove) {
		t.Fatal("due without asking")
	}
	p.GoalAfterPlan = true
	if !GoalDueOnApproval(p, GatePlan, GateApprove) {
		t.Fatal("not due on the plan's approval")
	}
	for _, c := range []struct{ gate, action string }{
		{GatePlan, "stop"}, {GatePR, GateApprove}, {GateMerge, GateApprove}, {GateBlocked, "retry"},
	} {
		if GoalDueOnApproval(p, c.gate, c.action) {
			t.Errorf("due on %s/%s", c.gate, c.action)
		}
	}
	// Goal mode a person turned off before stays off.
	p.GoalMode = &GoalMode{Enabled: false, EndReason: GoalEndDisabled}
	if GoalDueOnApproval(p, GatePlan, GateApprove) {
		t.Fatal("due after goal mode was turned off")
	}
}

func TestGoalAfterPlanIsNotStoredUnlessAsked(t *testing.T) {
	raw, err := json.Marshal(New("s", "g", t0))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "goal_after_plan") {
		t.Fatalf("stored:\n%s", raw)
	}
}
