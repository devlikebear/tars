package focuspipeline

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
)

const blueprintBlock = `<focus-plan>{"goal":"A book list","tasks":[{"title":"A page that lists books","done":"route renders"},{"title":"Write the checks","done":"scripts exist"}],
"verify":["npm run build","sh checks/add.sh"],
"checks":[{"label":" Adding a book shows it in the list ","command":"sh checks/add.sh"},{"label":"The list stays after a reload","command":"sh checks/reload.sh"},{"label":"same command again","command":"sh checks/add.sh"},{"label":"","command":"sh x.sh"},{"label":"no command","command":" "}],
"screens":[{"name":"Books","line":"Your reading list","parts":[{"kind":"Heading","text":"My books"},{"kind":"field","text":"Title"},{"kind":"button","text":"Add"},{"kind":"carousel","text":"dropped"},{"kind":"list"}]},{"name":" ","line":"no name"}],
"saved":["Books you added"," "],"not_now":["Sharing the list"]}</focus-plan>`

func webAppPipeline(t *testing.T) Pipeline {
	t.Helper()
	tpl, ok := FindTemplate("", WebAppTemplateID)
	if !ok {
		t.Fatal("no webapp template")
	}
	return NewFromTemplate("s1", "a book list", tpl, t0)
}

func parsedPlan(t *testing.T, text string) *Plan {
	t.Helper()
	b := ParseBlocks(text)
	if b.Plan == nil {
		t.Fatalf("no plan: %v", b.Errors)
	}
	return b.Plan
}

func TestParseBlueprintTrimsAndBounds(t *testing.T) {
	plan := parsedPlan(t, blueprintBlock)
	want := []PlanCheck{{"Adding a book shows it in the list", "sh checks/add.sh"}, {"The list stays after a reload", "sh checks/reload.sh"}}
	if !slices.Equal(plan.Checks, want) {
		t.Fatalf("checks = %+v", plan.Checks)
	}
	if len(plan.Screens) != 1 || plan.Screens[0].Name != "Books" {
		t.Fatalf("screens = %+v", plan.Screens)
	}
	kinds := []string{}
	for _, part := range plan.Screens[0].Parts {
		kinds = append(kinds, part.Kind)
	}
	if strings.Join(kinds, ",") != "heading,field,button,list" {
		t.Fatalf("parts = %+v", plan.Screens[0].Parts)
	}
	if strings.Join(plan.Saved, "|") != "Books you added" || strings.Join(plan.NotNow, "|") != "Sharing the list" {
		t.Fatalf("saved %v not now %v", plan.Saved, plan.NotNow)
	}

	var long Plan
	long.Tasks = []PlanTask{{Title: "t"}}
	for i := 0; i < 20; i++ {
		long.Checks = append(long.Checks, PlanCheck{Label: strings.Repeat("x", 400), Command: "sh c" + string(rune('a'+i))})
		long.Saved = append(long.Saved, "item")
		long.Screens = append(long.Screens, PlanScreen{Name: "s", Parts: slices.Repeat([]ScreenPart{{Kind: "text"}}, 30)})
	}
	got, err := normalizePlan(long)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Checks) != maxBlueprintChecks || len(got.Saved) != maxBlueprintItems || len(got.Screens) != maxBlueprintScreens ||
		len(got.Screens[0].Parts) != maxScreenParts || len([]rune(got.Checks[0].Label)) != maxBlueprintLabel+1 {
		t.Fatalf("bounds: %d checks, %d saved, %d screens, %d parts, label %d runes",
			len(got.Checks), len(got.Saved), len(got.Screens), len(got.Screens[0].Parts), len([]rune(got.Checks[0].Label)))
	}
}

// A check is a verify command: fitPlan puts its command into verify once,
// next to the plan's own commands, and the repository's default verify
// still joins.
func TestBlueprintChecksBecomeVerifyCommands(t *testing.T) {
	p := webAppPipeline(t)
	p.RepoVerify = []string{"npm run lint"}
	p, act, err := Apply(p, Event{Kind: EventTurnCompleted, Turn: 1, Blocks: ParseBlocks(blueprintBlock)}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if act.Kind != ActionNone || p.OpenGate != GatePlan {
		t.Fatalf("act %v gate %q", act, p.OpenGate)
	}
	want := []string{"npm run build", "sh checks/add.sh", "sh checks/reload.sh", "npm run lint"}
	if !slices.Equal(p.Plan.Verify, want) {
		t.Fatalf("verify = %v, want %v", p.Plan.Verify, want)
	}
	if len(p.CheckResults) != 2 || p.CheckResults[0].State != CheckNotRun || p.CheckResults[1].Label != "The list stays after a reload" {
		t.Fatalf("check results = %+v", p.CheckResults)
	}
	gate := lastCardOfKind(p, CardGate)
	var shown Plan
	if err := json.Unmarshal(gate.Payload, &shown); err != nil || len(shown.Checks) != 2 || len(shown.Screens) != 1 {
		t.Fatalf("the plan gate's card lacks the blueprint: %s", gate.Payload)
	}
}

// A template without a blueprint drops the fields: its plan, verify list
// and pipeline are what they were.
func TestPlanWithoutBlueprintIsUnchanged(t *testing.T) {
	dev, _, err := Apply(New("s1", "goal", t0), Event{Kind: EventTurnCompleted, Turn: 1, Blocks: ParseBlocks(blueprintBlock)}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if dev.Plan.Checks != nil || dev.Plan.Screens != nil || dev.Plan.Saved != nil || dev.Plan.NotNow != nil || dev.CheckResults != nil {
		t.Fatalf("a dev plan kept the blueprint: %+v", dev.Plan)
	}
	if !slices.Equal(dev.Plan.Verify, []string{"npm run build", "sh checks/add.sh"}) {
		t.Fatalf("verify = %v", dev.Plan.Verify)
	}
	raw, _ := json.Marshal(dev)
	for _, key := range []string{`"checks"`, `"screens"`, `"blueprint"`, `"check_results"`, `"you_decide"`} {
		if strings.Contains(string(raw), key) {
			t.Errorf("pipeline JSON has %s", key)
		}
	}

	// A webapp plan without the fields has no check results either.
	p := webAppPipeline(t)
	p, _, err = Apply(p, Event{Kind: EventTurnCompleted, Turn: 1, Blocks: Blocks{Plan: testPlan()}}, t0)
	if err != nil || p.CheckResults != nil || p.Plan.Checks != nil {
		t.Fatalf("err %v results %+v", err, p.CheckResults)
	}
}

func lastCardOfKind(p Pipeline, kind string) Card {
	for i := len(p.Cards) - 1; i >= 0; i-- {
		if p.Cards[i].Kind == kind {
			return p.Cards[i]
		}
	}
	return Card{}
}

func approvedWebApp(t *testing.T) Pipeline {
	t.Helper()
	p, _, err := Apply(webAppPipeline(t), Event{Kind: EventTurnCompleted, Turn: 1, Blocks: ParseBlocks(blueprintBlock)}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if p, _, err = Apply(p, Event{Kind: EventGate, Gate: GatePlan, Action: GateApprove}, t0); err != nil {
		t.Fatal(err)
	}
	if p.Current != "make" {
		t.Fatalf("current %s", p.Current)
	}
	return p
}

// Each check shows its command's latest result: passed, failed with the
// check's own CHECK FAILED report, or not run when the run did not reach
// it. Transitions read only the exit codes, as for any verify command.
func TestBlueprintCheckResults(t *testing.T) {
	p := approvedWebApp(t)
	if g := Guidance(p); !strings.Contains(g, "Checks the person approved") || !strings.Contains(g, "- Adding a book shows it in the list — sh checks/add.sh") {
		t.Fatalf("build guidance lacks the checks:\n%s", g)
	}
	p, act := turn(p, t, &Report{Summary: "made", TasksDone: true})
	if act.Kind != ActionRunVerification {
		t.Fatalf("act %v", act)
	}
	report := "starting\nCHECK FAILED\nscreen: Books (http://127.0.0.1:5199/)\nexpected: Dune in the list\nseen: an empty list\n"
	p, act = verify(p, t, &Verification{Passed: false, Results: []VerificationResult{
		{Command: "npm run build", Passed: true},
		{Command: "sh checks/add.sh", ExitCode: 1, Excerpt: report},
	}})
	if act.Kind != ActionSendTurn || lastCardOfKind(p, CardFailure).Title != "Check failed on Books (http://127.0.0.1:5199/): expected Dune in the list, but saw an empty list" {
		t.Fatalf("act %v cards %+v", act, p.Cards)
	}
	add, reload := p.CheckResults[0], p.CheckResults[1]
	if add.State != CheckFailed || add.Check == nil || add.Check.Seen != "an empty list" || add.At == nil {
		t.Fatalf("add = %+v", add)
	}
	if reload.State != CheckNotRun || reload.At != nil {
		t.Fatalf("reload = %+v", reload)
	}

	p, _ = turn(p, t, &Report{Summary: "fixed", TasksDone: true})
	p, _ = verify(p, t, &Verification{Passed: true, Results: []VerificationResult{
		{Command: "npm run build", Passed: true}, {Command: "sh checks/add.sh", Passed: true}, {Command: "sh checks/reload.sh", Passed: true},
	}})
	for _, c := range p.CheckResults {
		if c.State != CheckPassed || c.Check != nil {
			t.Fatalf("after a passing run: %+v", c)
		}
	}
	if p.Current != "review" {
		t.Fatalf("current %s", p.Current)
	}
	raw, _ := json.Marshal(p)
	if !strings.Contains(string(raw), `"check_results":[{"label":"Adding a book shows it in the list","command":"sh checks/add.sh","state":"passed"`) {
		t.Fatalf("the pipeline JSON the console reads lacks the results: %s", raw)
	}
}

// The leader asks up to three multiple-choice questions before the plan, as
// decision cards that offer "you decide"; the turn needs no plan block, the
// answers go back as one turn, and the plan comes after.
func TestBlueprintQuestionsBeforePlan(t *testing.T) {
	p := webAppPipeline(t)
	if g := Guidance(p); !strings.Contains(g, "at most three questions") || !strings.Contains(g, "or exactly one plan block") || !strings.Contains(g, `"checks":[`) {
		t.Fatalf("plan guidance:\n%s", g)
	}
	ask := Blocks{Report: &Report{Summary: "a few questions", Decisions: []Decision{
		{ID: "d1", Question: "Who uses it?", Options: []string{"Only me", "My club", " ", "a", "b", "c"}, YouDecide: false},
		{ID: "d2", Question: "Show covers?", Options: []string{"Yes", "No"}},
		{ID: "d3", Question: "Sort by?", Options: []string{"Title", "Date"}},
		{ID: "d4", Question: "Fourth?", Options: []string{"x", "y"}},
	}}}
	p, act, err := Apply(p, Event{Kind: EventTurnCompleted, Turn: 1, Blocks: ask}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if act.Kind != ActionNone || p.OpenGate != GateNone || p.Plan != nil {
		t.Fatalf("act %v gate %q plan %v", act, p.OpenGate, p.Plan)
	}
	var questions []Card
	for _, c := range p.Cards {
		switch c.Kind {
		case CardNotice:
			t.Fatalf("a questions turn raised a notice: %+v", c)
		case CardDecision:
			questions = append(questions, c)
		}
	}
	if len(questions) != maxBlueprintQuestion {
		t.Fatalf("%d decision cards", len(questions))
	}
	var d Decision
	if err := json.Unmarshal(questions[0].Payload, &d); err != nil || !d.YouDecide || len(d.Options) != maxBlueprintAnswers || d.Options[2] != "a" {
		t.Fatalf("first question = %+v (%v)", d, err)
	}

	p, act = decide(t, p, questions[0].ID, "My club")
	p, act = decide(t, p, questions[1].ID, DecisionYouDecide)
	if act.Kind != ActionNone {
		t.Fatalf("answered before the last question: %v", act)
	}
	p, act = decide(t, p, questions[2].ID, "Title")
	if act.Kind != ActionSendTurn || !strings.Contains(act.Prompt, "Who uses it? → My club") || !strings.Contains(act.Prompt, "Show covers? → You decide") || len(act.Answers) != 3 {
		t.Fatalf("answers turn: %+v", act)
	}
	g := Guidance(p)
	if !strings.Contains(g, "write the plan now") || strings.Contains(g, "at most three questions") || strings.Contains(g, "or exactly one plan block") {
		t.Fatalf("guidance after the answers:\n%s", g)
	}

	// One round of questions: asking again is a turn without its plan.
	again, _, err := Apply(p, Event{Kind: EventTurnCompleted, Turn: 3, Blocks: ask}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if lastCardOfKind(again, CardNotice).Title != NoticeFormatMissing {
		t.Fatalf("a second round of questions passed: %+v", again.Cards)
	}

	p, _, err = Apply(p, Event{Kind: EventTurnCompleted, Turn: 3, Blocks: ParseBlocks(blueprintBlock)}, t0)
	if err != nil || p.OpenGate != GatePlan || len(p.Plan.Checks) != 2 {
		t.Fatalf("plan after the answers: gate %q err %v", p.OpenGate, err)
	}
}

// Elsewhere a decision is what it was: a dev plan turn that only asks still
// lacks its plan, and the model cannot mark a question itself.
func TestQuestionsOutsideTheBlueprintAreUnchanged(t *testing.T) {
	ask := ParseBlocks(`<focus-report>{"summary":"q","decisions":[{"id":"d1","question":"Which?","options":["a","b"],"you_decide":true}]}</focus-report>`)
	if ask.Report.Decisions[0].YouDecide {
		t.Fatal("the model set you_decide")
	}
	p, act, err := Apply(New("s1", "goal", t0), Event{Kind: EventTurnCompleted, Turn: 1, Blocks: ask}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if act.Kind != ActionSendTurn || lastCardOfKind(p, CardNotice).Title != NoticeFormatMissing {
		t.Fatalf("act %v cards %+v", act, p.Cards)
	}
	if strings.Contains(Guidance(New("s1", "goal", t0)), "questions") {
		t.Fatal("a dev plan is invited to ask questions")
	}
}

// A check is a verify command, and no edit removes it: in goal mode an
// agent's edit that drops one is refused when it changes anything else,
// and changes nothing when it does not; outside goal mode the check's
// command stays too. An edit has no field for the checks themselves.
func TestPlanEditCannotDropACheck(t *testing.T) {
	p := approvedWebApp(t)
	before := slices.Clone(p.Plan.Verify)

	on := StartGoal(p, 0, t0)
	if got, changed, err := EditPlanByAgent(on, PlanEdit{Verify: strs("npm run build")}, t0); err != nil || changed || !slices.Equal(got.Plan.Verify, before) {
		t.Fatalf("drop only: changed %v err %v verify %v", changed, err, got.Plan.Verify)
	}
	if _, _, err := EditPlanByAgent(on, PlanEdit{Verify: strs("npm run build", "true")}, t0); !errors.Is(err, ErrGoalModeChecks) {
		t.Fatalf("goal mode let an agent rewrite the checks: %v", err)
	}
	b := ParseBlocks(`<focus-plan-edit>{"goal":"new goal","checks":[{"label":"always","command":"true"}]}</focus-plan-edit>`)
	got, changed, err := EditPlanByAgent(on, *b.PlanEdit, t0)
	if err != nil || !changed || !slices.Equal(got.Plan.Checks, p.Plan.Checks) || !slices.Equal(got.Plan.Verify, before) {
		t.Fatalf("an edit reached the checks: %v %v %+v", changed, err, got.Plan)
	}

	got, _, err = EditPlan(p, PlanEdit{Verify: strs("npm test")}, t0)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range p.Plan.Checks {
		if !slices.Contains(got.Plan.Verify, c.Command) {
			t.Fatalf("the developer's edit dropped %q: %v", c.Command, got.Plan.Verify)
		}
	}
}
