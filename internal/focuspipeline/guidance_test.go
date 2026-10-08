package focuspipeline

import (
	"strings"
	"testing"
)

// atStage returns an approved pipeline whose current stage is id, as if its
// plan had named exactly StageOrder's stages: a stage of the dev template
// outside that list (its release stage) is skipped, same as a real plan
// that left it out (skipUnplannedStages), not pending. Order is the
// pipeline's own (p.Stages), not the fixed StageOrder, since that stage is
// not in it.
func atStage(t *testing.T, id StageID) Pipeline {
	t.Helper()
	p := New("s1", "ship focus mode", t0)
	if id == StagePlan {
		return p
	}
	p.Plan = testPlan(StageOrder...)
	keep := map[StageID]bool{}
	for _, s := range StageOrder {
		keep[s] = true
	}
	target := indexOf(p, id)
	for i := range p.Stages {
		switch {
		case p.Stages[i].ID == id:
			p.Stages[i].Status = StatusActive
			p.Stages[i].Iteration = 1
			p.Stages[i].Limit = p.limitFor(id)
		case !keep[p.Stages[i].ID]:
			p.Stages[i].Status = StatusSkipped
		case indexOf(p, p.Stages[i].ID) < target:
			p.Stages[i].Status = StatusDone
		default:
			p.Stages[i].Status = StatusPending
		}
	}
	p.Current = id
	return p
}

func indexOf(p Pipeline, id StageID) int {
	for i, s := range p.Stages {
		if s.ID == id {
			return i
		}
	}
	return -1
}

func TestGuidance(t *testing.T) {
	tests := []struct {
		stage StageID
		want  []string
		not   []string
	}{
		{StagePlan, []string{"current stage: plan", "Do not edit files", "<focus-plan>", "</focus-plan>", "ship focus mode"}, []string{"<focus-report>"}},
		{StageBuild, []string{"current stage: build", "iteration 1 of 5", "1. t1 — done when: d1", "2. t2 — done when: d2", "- make test", "- make console-e2e", "<focus-report>", "</focus-report>"}, []string{"<focus-plan>"}},
		{StageReview, []string{"current stage: review", "do not edit files", "<focus-findings>", "<focus-report>", "- make test", "done when: d1"}, nil},
		{StagePR, []string{"current stage: pr", "<focus-pr>", "<focus-report>", "- make test"}, nil},
		{StagePRReview, []string{"current stage: pr_review", "<focus-findings>", "- make test"}, nil},
		{StageMerge, []string{"current stage: merge", "<focus-report>", "Do not merge"}, nil},
	}
	for _, tt := range tests {
		t.Run(string(tt.stage), func(t *testing.T) {
			got := Guidance(atStage(t, tt.stage))
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("guidance lacks %q:\n%s", w, got)
				}
			}
			for _, n := range tt.not {
				if strings.Contains(got, n) {
					t.Errorf("guidance has %q:\n%s", n, got)
				}
			}
			if !strings.Contains(got, "in the language the developer writes in") {
				t.Errorf("guidance lacks the language rule:\n%s", got)
			}
			if strings.Contains(got, "</focus-stage>") {
				t.Error("guidance must not close its own wrapper")
			}
		})
	}
}

func TestGuidanceEmptyWhenFinished(t *testing.T) {
	p := atStage(t, StageMerge)
	p.setStatus(StageMerge, StatusDone)
	if got := Guidance(p); got != "" {
		t.Fatalf("finished pipeline guidance = %q", got)
	}
}

func TestGuidanceExamplesParse(t *testing.T) {
	// The formats we show the agent must themselves be what we parse, once
	// the placeholders are filled in.
	filled := strings.NewReplacer(`"…"`, `"x"`, `"high|medium|low"`, `"high"`).Replace(planFormat + reportFormat + findingsFormat)
	b := ParseBlocks(filled)
	if b.Plan == nil || b.Report == nil || len(b.Findings) != 1 || len(b.Errors) != 0 {
		t.Fatalf("blocks = %+v", b)
	}
}

func TestGuidanceFollowsThePRWait(t *testing.T) {
	tests := []struct {
		name  string
		stage StageID
		wait  string
		want  []string
		not   []string
	}{
		{"drafting", StagePR, "", []string{"Draft the pull request", "<focus-pr>"}, []string{"gh pr create"}},
		{"opening", StagePR, PRWaitOpen, []string{"gh pr create", "<focus-report>"}, []string{"<focus-pr>"}},
		{"fixing", StagePRReview, PRWaitFix, []string{"push", "<focus-report>"}, []string{"Do not edit"}},
		{"pr_review by hand", StagePRReview, "", []string{"gh pr checks", "<focus-findings>"}, nil},
		{"summarizing", StageMerge, "", []string{"Do not merge", "<focus-report>"}, []string{"gh pr merge"}},
		{"merging", StageMerge, PRWaitMerge, []string{"gh pr merge", "--squash", "<focus-report>"}, []string{"Do not merge"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := atStage(t, tt.stage)
			p.PRWait = tt.wait
			got := Guidance(p)
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("guidance lacks %q:\n%s", w, got)
				}
			}
			for _, n := range tt.not {
				if strings.Contains(got, n) {
					t.Errorf("guidance has %q:\n%s", n, got)
				}
			}
		})
	}
}
