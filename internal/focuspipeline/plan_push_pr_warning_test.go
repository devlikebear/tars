package focuspipeline

import (
	"strings"
	"testing"
)

func TestTaskPushesOrOpensPR(t *testing.T) {
	tests := []struct {
		name string
		task PlanTask
		want bool
	}{
		{"english push and gh pr create", PlanTask{Title: "push and gh pr create", Done: "PR is open"}, true},
		{"english open the pull request", PlanTask{Title: "Open the pull request", Done: "PR exists on GitHub"}, true},
		{"korean push and PR 생성", PlanTask{Title: "push하고 PR 생성", Done: "PR이 열려 있다"}, true},
		{"korean 풀 리퀘스트 열기", PlanTask{Title: "변경사항 풀 리퀘스트 열기", Done: "리뷰 가능"}, true},
		{"bare push", PlanTask{Title: "Push the branch", Done: "branch is on origin"}, true},
		{"bare korean 푸시", PlanTask{Title: "브랜치 푸시", Done: "원격에 올라감"}, true},
		{"normal plan task", PlanTask{Title: "Add the config field", Done: "field parses and round-trips"}, false},
		{"describes the normal flow, names the pr stage", PlanTask{Title: "Note", Done: "push and open the PR in the pr stage"}, false},
		{"describes the normal flow in Korean", PlanTask{Title: "메모", Done: "pr 단계에서 push하고 PR 생성"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := taskPushesOrOpensPR(tt.task); got != tt.want {
				t.Errorf("taskPushesOrOpensPR(%+v) = %v, want %v", tt.task, got, tt.want)
			}
		})
	}
}

// TestPlanGateNoticesPushOrPRTasks is tars dogfooding fix #3: a plan whose
// task pushes or opens the pull request itself gets a notice card on the
// plan gate, in a template that has a pr stage; the gate itself is
// unaffected (no block, developer still approves or edits as before), and
// a normal plan or a template with no pr stage gets no notice at all.
func TestPlanGateNoticesPushOrPRTasks(t *testing.T) {
	t.Run("a task that pushes and opens the PR gets a notice", func(t *testing.T) {
		plan := testPlan(StagePlan, StageBuild, StagePR)
		plan.Tasks = []PlanTask{
			{Title: "implement the feature", Done: "tests pass"},
			{Title: "push and gh pr create", Done: "PR is open"},
		}
		p, act, err := Apply(New("s1", "goal", t0), Event{Kind: EventTurnCompleted, Turn: 1, Blocks: Blocks{Plan: plan}}, t0)
		if err != nil {
			t.Fatal(err)
		}
		if p.OpenGate != GatePlan || act.Kind != ActionNone {
			t.Fatalf("plan gate should open normally: gate=%q act=%+v", p.OpenGate, act)
		}
		var notice *Card
		for i := range p.Cards {
			if p.Cards[i].Title == NoticePlanPushesOrOpensPR {
				notice = &p.Cards[i]
			}
		}
		if notice == nil {
			t.Fatalf("no notice card: %+v", p.Cards)
		}
		var payload struct {
			Errors []string `json:"errors"`
		}
		if !decodePayload(notice.Payload, &payload) || len(payload.Errors) != 1 || !strings.Contains(payload.Errors[0], "gh pr create") {
			t.Fatalf("notice payload = %+v (decoded %+v)", notice.Payload, payload)
		}

		// The plan gate still works as before: approving it is unaffected.
		p, act, err = Apply(p, Event{Kind: EventGate, Gate: GatePlan, Action: GateApprove}, t0)
		if err != nil || p.Current != StageBuild || act.Kind != ActionSendTurn {
			t.Fatalf("approve after notice: err=%v current=%s act=%+v", err, p.Current, act)
		}
	})

	t.Run("a normal plan gets no notice", func(t *testing.T) {
		plan := testPlan(StagePlan, StageBuild, StagePR)
		p, _, err := Apply(New("s1", "goal", t0), Event{Kind: EventTurnCompleted, Turn: 1, Blocks: Blocks{Plan: plan}}, t0)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range p.Cards {
			if c.Title == NoticePlanPushesOrOpensPR {
				t.Fatalf("unexpected notice on a normal plan: %+v", c)
			}
		}
	})

	t.Run("describing the pr stage's own job is not flagged", func(t *testing.T) {
		plan := testPlan(StagePlan, StageBuild, StagePR)
		plan.Tasks = []PlanTask{
			{Title: "implement the feature", Done: "push and open the PR in the pr stage"},
		}
		p, _, err := Apply(New("s1", "goal", t0), Event{Kind: EventTurnCompleted, Turn: 1, Blocks: Blocks{Plan: plan}}, t0)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range p.Cards {
			if c.Title == NoticePlanPushesOrOpensPR {
				t.Fatalf("unexpected notice when the task names the pr stage: %+v", c)
			}
		}
	})

	t.Run("a template with no pr stage never gets the notice", func(t *testing.T) {
		noPR := Template{ID: "x", Name: "x", Stages: []TemplateStage{{ID: StagePlan}, {ID: "draft", Kind: StageBuild}}}
		plan := &Plan{Goal: "g", Tasks: []PlanTask{{Title: "push and gh pr create", Done: "done"}}, Stages: []StageID{StagePlan, "draft"}}
		p, _, err := Apply(NewFromTemplate("s1", "goal", noPR, t0), Event{Kind: EventTurnCompleted, Turn: 1, Blocks: Blocks{Plan: plan}}, t0)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range p.Cards {
			if c.Title == NoticePlanPushesOrOpensPR {
				t.Fatalf("unexpected notice with no pr stage: %+v", c)
			}
		}
	})
}
