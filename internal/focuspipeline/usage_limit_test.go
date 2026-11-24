package focuspipeline

import (
	"encoding/json"
	"testing"
	"time"
)

func TestUsageLimitResets(t *testing.T) {
	epoch := time.Unix(1760000000, 0).UTC()
	for _, c := range []struct {
		text, words string
		at          *time.Time
	}{
		{"claude-code-cli request: You've hit your session limit · resets 2:40pm (Asia/Seoul)", "2:40pm (Asia/Seoul)", nil},
		{"5-hour limit reached ∙ resets 3pm", "3pm", nil},
		{"You've hit your weekly limit · resets Nov 28 at 9am.\nRun /usage for more.", "Nov 28 at 9am", nil},
		{`openai status 400: {"error":{"message":"You've hit your session limit · resets at 2:40pm"}}`, "2:40pm", nil},
		{"Claude AI usage limit reached|1760000000", "", &epoch},
		{"You've hit your usage limit. Upgrade or try again later.", "", nil},
		{"usage limit · resets a very long sentence that goes on well past any time a provider would write here", "a very long sentence that goes on well past any", nil},
	} {
		words, at := UsageLimitResets(c.text)
		if words != c.words {
			t.Errorf("UsageLimitResets(%q) words = %q, want %q", c.text, words, c.words)
		}
		if (at == nil) != (c.at == nil) || (at != nil && !at.Equal(*c.at)) {
			t.Errorf("UsageLimitResets(%q) at = %v, want %v", c.text, at, c.at)
		}
	}
}

func TestFailTurnRecordsTheUsageLimit(t *testing.T) {
	p := goalBuilding(t, StagePlan, StageBuild)
	failed, ok := FailTurn(p, "You've hit your session limit · resets 2:40pm (Asia/Seoul)", t0)
	if !ok {
		t.Fatal("no turn was owed")
	}
	stop := failed.UsageLimitStop()
	if stop == nil || !stop.UsageLimit || stop.Resets != "2:40pm (Asia/Seoul)" {
		t.Fatalf("stop = %+v", stop)
	}

	// Any other failure is not a usage limit, and says nothing about one.
	other, _ := FailTurn(p, "provider timeout", t0)
	if other.UsageLimitStop() != nil {
		t.Fatal("a timeout read as a usage limit")
	}
	raw, _ := json.Marshal(other.Cards[len(other.Cards)-1].Payload)
	var fact map[string]any
	_ = json.Unmarshal(raw, &fact)
	for _, key := range []string{"usage_limit", "resets", "resets_at"} {
		if _, ok := fact[key]; ok {
			t.Errorf("%s stored on a timeout: %s", key, raw)
		}
	}

	// A gate closed by the retry is no longer a stop.
	retried, _, err := Apply(failed, Event{Kind: EventGate, Gate: GateBlocked, Action: GateRetry}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if retried.UsageLimitStop() != nil {
		t.Fatal("still a stop after the retry")
	}
}

// A fact recorded before the usage limit was read keeps only the error.
func TestUsageLimitStopReadsAnOlderFact(t *testing.T) {
	p := goalBuilding(t, StagePlan, StageBuild)
	failed, _ := FailTurn(p, "Claude AI usage limit reached|1760000000", t0)
	i := failed.openGateCard()
	var fact BlockedFact
	_ = json.Unmarshal(failed.Cards[i].Payload, &fact)
	fact.UsageLimit, fact.Resets, fact.ResetsAt = false, "", nil
	failed.Cards[i].Payload, _ = json.Marshal(fact)

	stop := failed.UsageLimitStop()
	if stop == nil || stop.ResetsAt == nil || stop.ResetsAt.Unix() != 1760000000 {
		t.Fatalf("stop = %+v", stop)
	}
}

func TestGoalComesBackAfterTheUsageLimit(t *testing.T) {
	p := goalBuilding(t, StagePlan, StageBuild)
	p.GoalAfterPlan = true
	failed, _ := FailTurn(p, "You've hit your session limit · resets 2:40pm", t0)
	if GoalDueOnApproval(failed, GateBlocked, GateRetry) {
		t.Fatal("due while goal mode is still on")
	}
	ended, _ := EndGoal(failed, GoalEndUsageLimit, t0)
	if !GoalDueOnApproval(ended, GateBlocked, GateRetry) {
		t.Fatal("not due on the retry after the usage limit")
	}
	if GoalDueOnApproval(ended, GateBlocked, GateStop) {
		t.Fatal("due on stop")
	}

	// Only the pipeline that asked for goal mode after the plan.
	asked := ended
	asked.GoalAfterPlan = false
	if GoalDueOnApproval(asked, GateBlocked, GateRetry) {
		t.Fatal("due on a pipeline that never asked")
	}
	// Goal mode that ended for another reason stays off.
	other, _ := EndGoal(failed, GoalEndDisabled, t0)
	if GoalDueOnApproval(other, GateBlocked, GateRetry) {
		t.Fatal("due after a person turned goal mode off")
	}
	// A blocked gate that is not the usage limit.
	timeout, _ := FailTurn(p, "provider timeout", t0)
	timeoutEnded, _ := EndGoal(timeout, GoalEndUsageLimit, t0)
	if GoalDueOnApproval(timeoutEnded, GateBlocked, GateRetry) {
		t.Fatal("due on a gate the limit did not raise")
	}
}
