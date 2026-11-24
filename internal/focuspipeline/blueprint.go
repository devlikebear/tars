package focuspipeline

import (
	"slices"
	"strings"
	"time"
)

// The blueprint (docs/plans/vibe-mode-redesign, unit 3): a plan written for
// someone who cannot read code. Besides its tasks it says, in plain words,
// how the work will be checked, what the app keeps, what is left out for
// now, and roughly what each screen holds; and before it the leader may ask
// a few multiple-choice questions.
//
// Nothing here is a new way to move between stages. A check is a label on a
// verify command: fitPlan puts every check's command into Plan.Verify, the
// review stage runs it like any other, and only its exit code counts. The
// label and the latest result are for the person's screen. The questions
// are ordinary decision cards, answered through the same path as any other
// (SetCardStateBy → AnswersPrompt).
//
// Only a pipeline whose template is marked Blueprint (the webapp template)
// keeps these fields: elsewhere fitPlan drops them, so a pipeline that does
// not opt in is what it was.

// Bounds on a blueprint. Longer lists and values are cut, not refused: a
// plan with one long label should not cost the person a whole turn.
const (
	maxBlueprintChecks   = 8
	maxBlueprintScreens  = 6
	maxScreenParts       = 12
	maxBlueprintItems    = 8
	maxBlueprintLabel    = 160
	maxBlueprintCommand  = 1000
	maxScreenPartText    = 80
	maxBlueprintAnswers  = 4
	maxBlueprintQuestion = 3
)

// PlanCheck is one check of a blueprint: what it proves, in the person's
// words, and the verify command that proves it.
type PlanCheck struct {
	Label   string `json:"label"`
	Command string `json:"command"`
}

// PlanScreen is a rough sketch of one screen: its name, one line about it,
// and its parts top to bottom.
type PlanScreen struct {
	Name  string       `json:"name"`
	Line  string       `json:"line,omitempty"`
	Parts []ScreenPart `json:"parts,omitempty"`
}

// ScreenPart is one simple part of a sketched screen. Kind is one of
// ScreenPartKinds; Text is what it says, if anything.
type ScreenPart struct {
	Kind string `json:"kind"`
	Text string `json:"text,omitempty"`
}

// ScreenPartKinds is the fixed vocabulary of a screen sketch: the console
// draws each kind as a simple shape. A part of any other kind is dropped.
var ScreenPartKinds = []string{"heading", "text", "field", "button", "list", "image", "link"}

// Check result states (CheckResult.State).
const (
	CheckNotRun = "not_run"
	CheckPassed = "passed"
	CheckFailed = "failed"
)

// CheckResult is a check of the approved blueprint with its latest result:
// the VerificationResult of its command in the latest verification run.
// Check is the failure in the check's own words when the command printed a
// CHECK FAILED block (ParseCheckReport).
type CheckResult struct {
	Label    string       `json:"label"`
	Command  string       `json:"command"`
	State    string       `json:"state"`
	TimedOut bool         `json:"timed_out,omitempty"`
	Check    *CheckReport `json:"check,omitempty"`
	At       *time.Time   `json:"at,omitempty"`
}

// DecisionYouDecide is the answer of a blueprint question's "you decide"
// choice. The server marks the question (Decision.YouDecide) and the
// screen offers the choice in the person's language; the answer the leader
// reads is this text.
const DecisionYouDecide = "You decide"

// normalizeBlueprint trims and bounds a plan's blueprint fields.
func normalizeBlueprint(plan Plan) Plan {
	var checks []PlanCheck
	for _, c := range plan.Checks {
		c.Label = clipRunes(strings.TrimSpace(c.Label), maxBlueprintLabel)
		c.Command = strings.TrimSpace(c.Command)
		if c.Label == "" || c.Command == "" || len(c.Command) > maxBlueprintCommand {
			continue
		}
		if slices.ContainsFunc(checks, func(o PlanCheck) bool { return o.Command == c.Command }) {
			continue // one command, one check: its result has one label
		}
		checks = append(checks, c)
		if len(checks) == maxBlueprintChecks {
			break
		}
	}
	plan.Checks = checks
	var screens []PlanScreen
	for _, s := range plan.Screens {
		s.Name = clipRunes(strings.TrimSpace(s.Name), maxBlueprintLabel)
		s.Line = clipRunes(strings.TrimSpace(s.Line), maxBlueprintLabel)
		if s.Name == "" {
			continue
		}
		var parts []ScreenPart
		for _, part := range s.Parts {
			part.Kind = strings.ToLower(strings.TrimSpace(part.Kind))
			part.Text = clipRunes(strings.TrimSpace(part.Text), maxScreenPartText)
			if !slices.Contains(ScreenPartKinds, part.Kind) {
				continue
			}
			parts = append(parts, part)
			if len(parts) == maxScreenParts {
				break
			}
		}
		s.Parts = parts
		screens = append(screens, s)
		if len(screens) == maxBlueprintScreens {
			break
		}
	}
	plan.Screens = screens
	plan.Saved = boundItems(plan.Saved)
	plan.NotNow = boundItems(plan.NotNow)
	return plan
}

func boundItems(items []string) []string {
	items = cleanStrings(items)
	if len(items) > maxBlueprintItems {
		items = items[:maxBlueprintItems]
	}
	for i := range items {
		items[i] = clipRunes(items[i], maxBlueprintLabel)
	}
	return items
}

// fitBlueprint keeps a plan's blueprint only in a pipeline that has one,
// and puts every check's command into the plan's verify list. A command
// the plan also lists in verify on its own is the same command run once:
// the check gives that one verify entry its label. Check commands the plan
// did not list are added after its own, in check order.
func (p Pipeline) fitBlueprint(plan Plan) Plan {
	if !p.Blueprint {
		plan.Checks, plan.Screens, plan.Saved, plan.NotNow = nil, nil, nil, nil
		return plan
	}
	plan.Verify = withCheckCommands(plan.Verify, plan.Checks)
	return plan
}

// withCheckCommands is list with every check's command in it.
func withCheckCommands(list []string, checks []PlanCheck) []string {
	commands := make([]string, 0, len(checks))
	for _, c := range checks {
		commands = append(commands, c.Command)
	}
	return MergeRepoVerify(list, commands)
}

// editedList is what an edit of the approved plan's list named name
// becomes: pinList's commands, and for verify the commands of the plan's
// checks, which no edit removes — an edit has no field for the checks, and
// dropping a check's command from verify would leave the person's check
// unrun while their screen still lists it.
func (p Pipeline) editedList(name string, list []string) []string {
	list = p.pinList(name, list)
	if name == "verify" && p.Plan != nil {
		list = withCheckCommands(list, p.Plan.Checks)
	}
	return list
}

// resetCheckResults sets every check of the plan as not run yet; nil when
// the plan has no checks.
func (p *Pipeline) resetCheckResults() {
	p.CheckResults = nil
	if p.Plan == nil {
		return
	}
	for _, c := range p.Plan.Checks {
		p.CheckResults = append(p.CheckResults, CheckResult{Label: c.Label, Command: c.Command, State: CheckNotRun})
	}
}

// recordCheckResults gives each check the result of its command in v. A
// check whose command v did not run is not run.
func (p *Pipeline) recordCheckResults(v Verification, now time.Time) {
	if p.Plan == nil || len(p.Plan.Checks) == 0 {
		return
	}
	p.resetCheckResults()
	at := now.UTC()
	for i := range p.CheckResults {
		c := &p.CheckResults[i]
		idx := slices.IndexFunc(v.Results, func(r VerificationResult) bool { return !r.E2E && r.Command == c.Command })
		if idx < 0 {
			continue
		}
		r := v.Results[idx]
		c.At = &at
		if r.Passed {
			c.State = CheckPassed
			continue
		}
		c.State, c.TimedOut = CheckFailed, r.TimedOut
		if !r.TimedOut {
			c.Check = ParseCheckReport(r.Excerpt)
		}
	}
}

// blueprintQuestions are the questions a plan-stage turn of a blueprint
// pipeline asks: at most maxBlueprintQuestion, each with at most
// maxBlueprintAnswers choices, and each marked to offer "you decide".
func blueprintQuestions(decisions []Decision) []Decision {
	out := make([]Decision, 0, min(len(decisions), maxBlueprintQuestion))
	for _, d := range decisions {
		if len(out) == maxBlueprintQuestion {
			break
		}
		d.Options = cleanStrings(d.Options)
		if len(d.Options) > maxBlueprintAnswers {
			d.Options = d.Options[:maxBlueprintAnswers]
		}
		d.YouDecide = true
		out = append(out, d)
	}
	return out
}

// planDecisions are the decision cards a turn's report raises: in the plan
// stage of a blueprint pipeline, the leader's bounded questions.
func (p Pipeline) planDecisions(r Report) []Decision {
	if p.Blueprint && p.CurrentKind() == StagePlan {
		return blueprintQuestions(r.Decisions)
	}
	return r.Decisions
}

// asksBeforePlan reports whether b is the leader's questions before the
// plan: a plan-stage turn of a blueprint pipeline that asked nothing yet,
// with a report that asks and no plan. Such a turn needs no plan block;
// the answers (AnswersPrompt) start the turn that writes the plan.
func (p Pipeline) asksBeforePlan(b Blocks) bool {
	return p.Blueprint && p.CurrentKind() == StagePlan && b.Plan == nil &&
		b.Report != nil && len(b.Report.Decisions) > 0 && !p.planQuestionsAsked()
}

// planQuestionsAsked reports whether the plan stage already asked its
// questions: one round only, so the leader cannot keep the person
// answering instead of planning.
func (p Pipeline) planQuestionsAsked() bool {
	return slices.ContainsFunc(p.Cards, func(c Card) bool { return c.Kind == CardDecision && c.Stage == StagePlan })
}

// blueprintPlanFields are the plan block's blueprint fields, shown only to
// a blueprint pipeline.
const blueprintPlanFields = `"checks":[{"label":"…","command":"…"}],"screens":[{"name":"…","line":"…","parts":[{"kind":"heading","text":"…"},{"kind":"field","text":"…"},{"kind":"button","text":"…"}]}],"saved":["…"],"not_now":["…"],`

// blueprintQuestionsInstructions invite the questions before the plan.
const blueprintQuestionsInstructions = " Before you write the plan you may ask the person at most three questions whose answers change " +
	"what you make, each with two to four short choices in plain words. Ask them in a report block instead of the plan block, " +
	"with no plan block in that reply, and wait: the answers come back as the next message. Do not add a \"you decide\" choice: " +
	"the screen adds one, and its answer reads \"" + DecisionYouDecide + "\", which leaves that choice to you. Ask only when the " +
	"idea leaves a real choice open; otherwise write the plan straight away. You get one round of questions."

// blueprintAnsweredInstructions follow the answered questions.
const blueprintAnsweredInstructions = " You already asked your questions and the conversation has the answers (\"" + DecisionYouDecide +
	"\" leaves that choice to you): write the plan now, and do not ask again."

// blueprintPlanInstructions explain the blueprint fields of the plan block.
const blueprintPlanInstructions = " The plan is shown to the person as a blueprint, so also fill these fields in plain words: " +
	"\"checks\" — two to five checks, each a short sentence of what the person will be able to see or do (\"Adding a book shows it " +
	"in the list\") and the verify command that proves it; the server adds every check's command to \"verify\" and runs it after the " +
	"work, so a check's command is a script one of the tasks writes. \"screens\" — each screen's name, one line about it, and its parts " +
	"top to bottom, each part one of " + `"heading", "text", "field", "button", "list", "image", "link"` + " with the words it shows. " +
	"\"saved\" — what the app keeps after the page is closed (\"Books you added\"); empty when it keeps nothing. \"not_now\" — what " +
	"this first version leaves out, so the person is not surprised."

// blueprintStageInstructions is the blueprint paragraph of the plan
// stage's instructions.
func (p Pipeline) blueprintStageInstructions() string {
	if !p.Blueprint {
		return ""
	}
	questions := blueprintQuestionsInstructions
	if p.planQuestionsAsked() {
		questions = blueprintAnsweredInstructions
	}
	return questions + blueprintPlanInstructions
}

// blueprintPlanBlockRequest is the plan stage's block request of a
// blueprint pipeline that may still ask its questions.
func (p Pipeline) blueprintPlanBlockRequest() string {
	return "End your reply with either a report block that only asks your questions (no plan block in that reply):\n" +
		reportFormat + "\nor exactly one plan block in this format:\n" + p.planBlockFormat() + "\n" + blockTail
}

// writeChecks lists the approved checks for the stages after the plan.
func writeChecks(b *strings.Builder, checks []PlanCheck) {
	if len(checks) == 0 {
		return
	}
	b.WriteString("Checks the person approved (each runs as the verification command named; make a failing one pass by fixing " +
		"the app, never by changing the check):\n")
	for _, c := range checks {
		b.WriteString("- " + c.Label + " — " + c.Command + "\n")
	}
}
