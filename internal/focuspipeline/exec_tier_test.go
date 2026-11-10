package focuspipeline

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestExecTurnTierFollowsTheTurnKind walks the golden test's development
// pipeline with the execution tier on: plan and review turns stay on the
// session's tier, the build turn and the review fix turn take the
// execution tier.
func TestExecTurnTierFollowsTheTurnKind(t *testing.T) {
	p := New("s1", "goal", t0)
	p.ExecTier = &ExecTier{Tier: "light", At: t0}
	check := func(step string, want string) {
		t.Helper()
		if got := p.ExecTurnTier(); got != want {
			t.Fatalf("%s: tier %q, want %q (stage %s, fixing %v)", step, got, want, p.Current, p.Review.Fixing)
		}
	}
	check("plan", "")
	var err error
	p, _, err = Apply(p, Event{Kind: EventTurnCompleted, Turn: 1, Blocks: Blocks{Plan: testPlan(StagePlan, StageBuild, StageReview, StagePR)}}, t0)
	if err != nil {
		t.Fatal(err)
	}
	check("plan proposed", "")
	if p, _, err = Apply(p, Event{Kind: EventGate, Gate: GatePlan, Action: GateApprove}, t0); err != nil {
		t.Fatal(err)
	}
	check("build turn", "light")
	p, _ = turn(p, t, &Report{Summary: "built", TasksDone: true})
	p, _ = verify(p, t, passed())
	check("first review turn", "")
	p, _ = reviewTurn(t, p, twoFindings())
	p, _ = decide(t, p, findingCards(p)[0].ID, DecisionFix)
	p, _ = decide(t, p, findingCards(p)[1].ID, DecisionDismiss)
	check("review fix turn", "light")
	p, _ = turn(p, t, &Report{Summary: "fixed"})
	p, _ = verify(p, t, passed())
	check("second review round", "")
	p, _ = reviewTurn(t, p, []Finding{})
	p, _ = verify(p, t, passed())
	check("pr stage", "")
}

func TestExecTurnTierCases(t *testing.T) {
	stagesAt := func(current StageID) Pipeline {
		p := New("s1", "goal", t0)
		for i := range p.Stages {
			p.Stages[i].Status = StatusDone
			if p.Stages[i].ID == current {
				p.Stages[i].Status = StatusActive
				break
			}
		}
		p.Current = current
		p.ExecTier = &ExecTier{Tier: "standard"}
		return p
	}
	cases := []struct {
		name string
		p    func() Pipeline
		want string
	}{
		{"no opt-in", func() Pipeline { p := stagesAt(StageBuild); p.ExecTier = nil; return p }, ""},
		{"empty tier", func() Pipeline { p := stagesAt(StageBuild); p.ExecTier.Tier = ""; return p }, ""},
		{"build", func() Pipeline { return stagesAt(StageBuild) }, "standard"},
		{"stopped build", func() Pipeline {
			p := stagesAt(StageBuild)
			p.Stages[1].Status = StatusBlocked
			return p
		}, ""},
		{"review turn", func() Pipeline { return stagesAt(StageReview) }, ""},
		{"review fix turn", func() Pipeline { p := stagesAt(StageReview); p.Review.Fixing = true; return p }, "standard"},
		{"pr_review waiting for checks", func() Pipeline { return stagesAt(StagePRReview) }, ""},
		{"pr_review fix turn", func() Pipeline { p := stagesAt(StagePRReview); p.PRWait = PRWaitFix; return p }, "standard"},
		{"pr", func() Pipeline { return stagesAt(StagePR) }, ""},
		{"merge", func() Pipeline { return stagesAt(StageMerge) }, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.p().ExecTurnTier(); got != tc.want {
				t.Fatalf("tier %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExecTierSwitchSafe(t *testing.T) {
	resumes := func(kind string) bool { return kind == "claude-code-cli" || kind == "antigravity-cli" }
	cases := []struct {
		session, exec string
		want          bool
	}{
		{"anthropic", "anthropic", true},
		{"anthropic", "openai", true},
		{"claude-code-cli", "claude-code-cli", true},
		{"claude-code-cli", "anthropic", false},
		{"anthropic", "claude-code-cli", false},
		{"claude-code-cli", "antigravity-cli", false},
	}
	for _, tc := range cases {
		if got := ExecTierSwitchSafe(tc.session, tc.exec, resumes); got != tc.want {
			t.Errorf("%s → %s: %v, want %v", tc.session, tc.exec, got, tc.want)
		}
	}
}

func TestAddExecTierFallbackOnce(t *testing.T) {
	p := New("s1", "goal", t0)
	p.ExecTier = &ExecTier{Tier: "light"}
	p, ok := AddExecTierFallback(p, ExecTierFallbackResume, "claude-code-cli", "anthropic", t0)
	if !ok || len(p.Cards) != 1 || p.Cards[0].Title != NoticeExecTierFallback {
		t.Fatalf("first fallback: ok %v cards %+v", ok, p.Cards)
	}
	var payload map[string]string
	if err := json.Unmarshal(p.Cards[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["tier"] != "light" || payload["reason"] != ExecTierFallbackResume || payload["session_provider"] != "claude-code-cli" || payload["exec_provider"] != "anthropic" {
		t.Fatalf("payload %v", payload)
	}
	if _, ok := AddExecTierFallback(p, ExecTierFallbackResume, "claude-code-cli", "anthropic", t0.Add(time.Minute)); ok {
		t.Fatal("second fallback added a card")
	}
}

func TestExecTierCloneAndJSON(t *testing.T) {
	p := New("s1", "goal", t0)
	if raw, _ := json.Marshal(p); strings.Contains(string(raw), "exec_tier") {
		t.Fatalf("a pipeline without the opt-in carries exec_tier: %s", raw)
	}
	p.ExecTier = &ExecTier{Tier: "light", Repo: "/repo", Lessons: 2, At: t0}
	c := p.clone()
	c.ExecTier.Tier = "heavy"
	if p.ExecTier.Tier != "light" {
		t.Fatal("clone shares ExecTier")
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var back Pipeline
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if *back.ExecTier != *p.ExecTier {
		t.Fatalf("round trip %+v, want %+v", back.ExecTier, p.ExecTier)
	}
}
