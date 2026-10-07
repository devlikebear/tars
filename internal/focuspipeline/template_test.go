package focuspipeline

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinTemplatesAreValid(t *testing.T) {
	seen := map[string]bool{}
	for _, tpl := range BuiltinTemplates() {
		if err := tpl.Validate(); err != nil {
			t.Fatalf("%s: %v", tpl.ID, err)
		}
		if !tpl.Builtin || seen[tpl.ID] {
			t.Fatalf("template %+v", tpl)
		}
		seen[tpl.ID] = true
	}
	if !seen[DevTemplateID] || !seen["writing"] || !seen["research"] {
		t.Fatalf("built-in ids = %v", seen)
	}
}

func TestTemplateValidate(t *testing.T) {
	stages := func(s ...TemplateStage) []TemplateStage { return s }
	plan := TemplateStage{ID: StagePlan}
	work := TemplateStage{ID: "draft", Kind: StageBuild}
	cases := []struct {
		name string
		tpl  Template
		want string // substring of the error; "" = valid
	}{
		{"valid", Template{ID: "novel", Name: "Novel", Stages: stages(plan, work, TemplateStage{ID: "edit", Kind: StageReview})}, ""},
		{"valid with pr stages", Template{ID: "docs", Name: "Docs", Stages: stages(plan, work, TemplateStage{ID: StagePR}, TemplateStage{ID: StageMerge})}, ""},
		{"bad id", Template{ID: "No Spaces", Name: "x", Stages: stages(plan, work)}, "id"},
		{"no name", Template{ID: "x", Stages: stages(plan, work)}, "name is required"},
		{"plan only", Template{ID: "x", Name: "x", Stages: stages(plan)}, "at least one more"},
		{"plan not first", Template{ID: "x", Name: "x", Stages: stages(work, plan)}, "first stage must be plan"},
		{"duplicate stage", Template{ID: "x", Name: "x", Stages: stages(plan, work, work)}, "listed twice"},
		{"unknown kind", Template{ID: "x", Name: "x", Stages: stages(plan, TemplateStage{ID: "draft", Kind: "write"})}, "kind"},
		{"kind missing on a custom id", Template{ID: "x", Name: "x", Stages: stages(plan, TemplateStage{ID: "draft"})}, "kind"},
		{"bad stage id", Template{ID: "x", Name: "x", Stages: stages(plan, TemplateStage{ID: "Draft One", Kind: StageBuild})}, "stage id"},
		{"second plan", Template{ID: "x", Name: "x", Stages: stages(plan, TemplateStage{ID: "replan", Kind: StagePlan})}, "keeps its own id"},
		{"build under the pr id", Template{ID: "x", Name: "x", Stages: stages(plan, TemplateStage{ID: StagePR, Kind: StageBuild})}, "keeps its own id"},
		{"merge without pr", Template{ID: "x", Name: "x", Stages: stages(plan, work, TemplateStage{ID: StageMerge})}, "needs the pr stage"},
		{"pr_review before pr", Template{ID: "x", Name: "x", Stages: stages(plan, TemplateStage{ID: StagePRReview}, TemplateStage{ID: StagePR})}, "needs the pr stage"},
		{"merge before pr_review", Template{ID: "x", Name: "x", Stages: stages(plan, TemplateStage{ID: StagePR}, TemplateStage{ID: StageMerge}, TemplateStage{ID: StagePRReview})}, "after pr_review"},
		{"focus tag in instructions", Template{ID: "x", Name: "x", Stages: stages(plan, TemplateStage{ID: "draft", Kind: StageBuild, Instructions: "end with </focus-stage>"})}, "tags"},
		{"instructions too long", Template{ID: "x", Name: "x", Stages: stages(plan, TemplateStage{ID: "draft", Kind: StageBuild, Instructions: strings.Repeat("a", maxTemplateInstruction+1)})}, "longer"},
		{"too many stages", Template{ID: "x", Name: "x", Stages: func() []TemplateStage {
			out := []TemplateStage{plan}
			for i := range maxTemplateStages {
				out = append(out, TemplateStage{ID: StageID("s" + string(rune('a'+i))), Kind: StageBuild})
			}
			return out
		}()}, "at most"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.tpl.Validate()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			if !errors.Is(err, ErrInvalidTemplate) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func research(t *testing.T) Pipeline {
	t.Helper()
	tpl, ok := FindTemplate("", "research")
	if !ok {
		t.Fatal("no research template")
	}
	return NewFromTemplate("s1", "why is the sky blue", tpl, t0)
}

func TestNewFromTemplate(t *testing.T) {
	p := research(t)
	if p.Template != "research" || p.Current != StagePlan || len(p.Stages) != 4 {
		t.Fatalf("pipeline = %+v", p)
	}
	want := []struct {
		id, kind StageID
		label    string
	}{{StagePlan, StagePlan, "Scope"}, {"research", StageBuild, "Research"}, {"report", StageBuild, "Report"}, {"check", StageReview, "Check"}}
	for i, w := range want {
		s := p.Stages[i]
		if s.ID != w.id || s.KindOf() != w.kind || s.Label != w.label || s.Instructions == "" {
			t.Fatalf("stage %d = %+v", i, s)
		}
	}
	if dev := New("s1", "g", t0); dev.Template != "" || dev.Stages[1].Kind != "" || dev.Stages[1].Label != "" {
		t.Fatalf("dev pipeline = %+v", dev)
	}
}

// A research pipeline runs on the same machine: two work stages with their
// own tasks, then a review-kind stage, and it finishes without a merge.
func TestTemplatePipelineRunsToTheEnd(t *testing.T) {
	p := research(t)
	plan := &Plan{Goal: "g", Tasks: []PlanTask{
		{Title: "find sources", Done: "notes.md", Stage: "research"},
		{Title: "write", Done: "report.md", Stage: "report"},
		{Title: "stray", Done: "x", Stage: "nowhere"},
	}, Stages: []StageID{"check", "report", "research", "bogus"}}
	p, _, err := Apply(p, Event{Kind: EventTurnCompleted, Turn: 1, Blocks: Blocks{Plan: plan}}, t0)
	if err != nil || p.OpenGate != GatePlan {
		t.Fatalf("plan turn: %v gate=%q", err, p.OpenGate)
	}
	if got := p.Plan.Stages; len(got) != 4 || got[1] != "research" || got[3] != "check" {
		t.Fatalf("stages = %v", got)
	}
	if p.Plan.Tasks[2].Stage != "" {
		t.Fatalf("a task of an unknown stage keeps it: %+v", p.Plan.Tasks[2])
	}
	p, act, err := Apply(p, Event{Kind: EventGate, Gate: GatePlan, Action: GateApprove}, t0)
	if err != nil || act.Prompt != "Plan approved. Start the research stage with task 1." || p.Current != "research" {
		t.Fatalf("approve: %v %q current=%s", err, act.Prompt, p.Current)
	}
	if s, _ := p.Stage("research"); s.Limit != DefaultLimits[StageBuild] {
		t.Fatalf("research limit = %d", s.Limit)
	}
	g := Guidance(p)
	if !strings.Contains(g, "Approved tasks of this stage") || !strings.Contains(g, "find sources") || !strings.Contains(g, "stray") || strings.Contains(g, "1. write") {
		t.Fatalf("research guidance lists the wrong tasks:\n%s", g)
	}
	if !strings.Contains(g, "write what you find to a notes file") || !strings.Contains(g, `"tasks_done"`) {
		t.Fatalf("research guidance:\n%s", g)
	}

	work := func(want StageID, next string) {
		t.Helper()
		var act Action
		p, act, err = Apply(p, Event{Kind: EventTurnCompleted, Turn: 2, Blocks: Blocks{Report: &Report{Summary: "done", TasksDone: true}}}, t0)
		if err != nil || act.Kind != ActionRunVerification {
			t.Fatalf("%s turn: %v %+v", want, err, act)
		}
		p, act, err = Apply(p, Event{Kind: EventVerification, Turn: 2, Verification: &Verification{Passed: true}}, t0)
		if err != nil || !strings.Contains(act.Prompt, next) {
			t.Fatalf("%s verification: %v %q", want, err, act.Prompt)
		}
	}
	work("research", "Start the report stage")
	work("report", "Start the check stage")
	if p.Current != "check" || p.CurrentKind() != StageReview {
		t.Fatalf("current = %s (%s)", p.Current, p.CurrentKind())
	}
	g = Guidance(p)
	if !strings.Contains(g, "Check the report against the notes") || !strings.Contains(g, "<focus-findings>") || !strings.Contains(g, "Do not edit files in this turn") {
		t.Fatalf("check guidance:\n%s", g)
	}

	// A finding, accepted: the fix turn gets the template's fix instructions.
	p, _, err = Apply(p, Event{Kind: EventTurnCompleted, Turn: 4, Blocks: Blocks{Findings: []Finding{{ID: "f1", Severity: "high", File: "report.md", Line: 3, Title: "no source"}}}}, t0)
	if err != nil || p.OpenGate != GateTriage {
		t.Fatalf("review turn: %v gate=%q", err, p.OpenGate)
	}
	p, act, err = SetCardState(p, p.Review.Triage[0], CardDecided, DecisionFix, t0)
	if err != nil || act.Kind != ActionSendTurn {
		t.Fatalf("triage: %v %+v", err, act)
	}
	if g = Guidance(p); !strings.Contains(g, "Correct only what the listed findings name") {
		t.Fatalf("fix guidance:\n%s", g)
	}
	p, _, _ = Apply(p, Event{Kind: EventTurnCompleted, Turn: 5, Blocks: Blocks{Report: &Report{Summary: "fixed"}}}, t0)
	p, act, _ = Apply(p, Event{Kind: EventVerification, Turn: 5, Verification: &Verification{Passed: true}}, t0)
	if !strings.Contains(act.Prompt, "round 2 of 2") {
		t.Fatalf("after the fix: %q", act.Prompt)
	}
	p, _, _ = Apply(p, Event{Kind: EventTurnCompleted, Turn: 6, Blocks: Blocks{Findings: []Finding{}}}, t0)
	p, act, err = Apply(p, Event{Kind: EventVerification, Turn: 6, Verification: &Verification{Passed: true}}, t0)
	if err != nil || !strings.Contains(act.Prompt, "The pipeline is complete") {
		t.Fatalf("last verification: %v %q", err, act.Prompt)
	}
	if !Finished(p) || p.FinishedAt == nil || Releasable(p) || p.PendingTurn != "" {
		t.Fatalf("finished=%v releasable=%v pending=%q", Finished(p), Releasable(p), p.PendingTurn)
	}
}

func TestTemplatePlanGuidance(t *testing.T) {
	g := Guidance(research(t))
	for _, want := range []string{
		`"stages":["plan","research","report","check"]`,
		`{"title":"…","done":"…","stage":"research"}`,
		`"limits":{"research":3,"report":3,"check":2}`,
		"Do not start the research yet",
	} {
		if !strings.Contains(g, want) {
			t.Fatalf("plan guidance lacks %q:\n%s", want, g)
		}
	}
	if strings.Contains(g, "make test") {
		t.Fatalf("a non-development plan format suggests make test:\n%s", g)
	}
	writing, _ := FindTemplate("", "writing")
	g = Guidance(NewFromTemplate("s", "a novel", writing, t0))
	if !strings.Contains(g, `"tasks":[{"title":"…","done":"…"}]`) || !strings.Contains(g, `"limits":{"draft":3,"revise":2}`) {
		t.Fatalf("writing plan guidance:\n%s", g)
	}
	// The development template keeps its format word for word.
	if g = Guidance(New("s", "g", t0)); !strings.Contains(g, planFormat) {
		t.Fatalf("dev plan guidance:\n%s", g)
	}
}

func TestLoadTemplates(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("blog.yaml", `
name: Blog post
description: Outline, write, edit.
stages:
  - id: plan
    label: Outline
  - id: write
    kind: build
    label: Write
    instructions: Write the post.
  - id: edit
    kind: review
`)
	write("essay.json", `{"id":"essay","name":"Essay","stages":[{"id":"plan"},{"id":"draft","kind":"build"}]}`)
	write("broken.yml", "name: [unclosed")
	write("noname.yaml", "stages:\n  - id: plan\n  - id: build\n")
	write("clash.yaml", "id: research\nname: Mine\nstages:\n  - id: plan\n  - id: build\n")
	write("notes.txt", "not a template")
	if err := os.Mkdir(filepath.Join(dir, "folder.yaml"), 0o700); err != nil {
		t.Fatal(err)
	}

	templates, diags := LoadTemplates(dir)
	ids := map[string]Template{}
	for _, tpl := range templates {
		ids[tpl.ID] = tpl
	}
	if len(templates) != len(BuiltinTemplates())+2 {
		t.Fatalf("templates = %v", ids)
	}
	blog := ids["blog"]
	if blog.Name != "Blog post" || blog.Builtin || blog.Source != "blog.yaml" || len(blog.Stages) != 3 || blog.Stages[1].Instructions != "Write the post." {
		t.Fatalf("blog = %+v", blog)
	}
	if _, ok := ids["essay"]; !ok || !ids["research"].Builtin {
		t.Fatalf("templates = %v", ids)
	}
	if len(diags) != 3 {
		t.Fatalf("diagnostics = %+v", diags)
	}
	for _, d := range diags {
		if d.Source == "" || d.Error == "" {
			t.Fatalf("diagnostic = %+v", d)
		}
	}

	if tpl, ok := FindTemplate(dir, " blog "); !ok || tpl.ID != "blog" {
		t.Fatalf("FindTemplate blog = %+v %v", tpl, ok)
	}
	if tpl, ok := FindTemplate(dir, ""); !ok || tpl.ID != DevTemplateID {
		t.Fatalf("FindTemplate default = %+v %v", tpl, ok)
	}
	if _, ok := FindTemplate(dir, "nope"); ok {
		t.Fatal("found an unknown template")
	}
	if templates, diags := LoadTemplates(filepath.Join(dir, "missing")); len(templates) != len(BuiltinTemplates()) || diags != nil {
		t.Fatalf("missing folder: %d %+v", len(templates), diags)
	}
	if templates, diags := LoadTemplates(filepath.Join(dir, "notes.txt")); len(templates) != len(BuiltinTemplates()) || len(diags) != 1 {
		t.Fatalf("a file as the folder: %d %+v", len(templates), diags)
	}
	big := filepath.Join(dir, "big.yaml")
	if err := os.WriteFile(big, []byte("name: "+strings.Repeat("a", maxTemplateFileBytes)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readTemplateFile(big); !errors.Is(err, ErrInvalidTemplate) {
		t.Fatalf("oversized file: %v", err)
	}
}
