package focuspipeline

import (
	"fmt"
	"strings"
)

// Block formats the agent must follow, quoted verbatim in the guidance.
const (
	planFormat     = `<focus-plan>{"goal":"…","tasks":[{"title":"…","done":"…"}],"stages":["plan","build","review","pr","pr_review","merge"],"verify":["make test"],"e2e":["…"],"limits":{"build":3,"review":2,"pr":3}}</focus-plan>`
	reportFormat   = `<focus-report>{"summary":"…","decisions":[{"id":"d1","question":"…","options":["…","…"]}],"risks":["…"]}</focus-report>`
	findingsFormat = `<focus-findings>[{"id":"f1","severity":"high|medium|low","file":"…","line":42,"title":"…","scenario":"…"}]</focus-findings>`
	prFormat       = `<focus-pr>{"title":"…","body":"…"}</focus-pr>`
	// buildReportFormat adds the build loop's tasks_done claim.
	buildReportFormat = `<focus-report>{"summary":"…","tasks_done":false,"decisions":[{"id":"d1","question":"…","options":["…","…"]}],"risks":["…"]}</focus-report>`
	buildTasksDone    = `Set "tasks_done" to true only when every approved task is complete. ` +
		"After your reply the server runs the verification commands: a failure starts the next turn with its output, " +
		"and the build ends when tasks_done is true and every command passes."
)

// stageInstructions are the specific instructions of each stage.
var stageInstructions = map[StageID]string{
	StagePlan: "Do not edit files. Read the code you need, then propose a plan: small tasks in order, " +
		"each with what \"done\" means; which stages apply (a small fix may skip review or pr_review); " +
		"the verification commands that prove the work (and end-to-end commands, if any); loop limits. " +
		"The developer approves or edits the plan before any change is made.",
	StageBuild: "Implement the approved tasks in order, keeping changes focused. " +
		"Run the verification commands yourself before you finish. " +
		"If you need a decision from the developer, ask it as a decision in the report instead of guessing.",
	StageReview: "Review the changes made for this goal. Report findings; do not edit files in this turn. " +
		"Each finding needs a concrete failure scenario (inputs or state → wrong output or crash).",
	StagePR: "Draft the pull request title and body for the approved work. " +
		"Do not open, push or merge anything until the developer approves the PR gate.",
	// Passed into by hand (gh could not run on the server): the agent reads
	// CI itself. Facts from the server's probe otherwise.
	StagePRReview: "Check the pull request's CI results (`gh pr checks`) and review comments. " +
		"Report each failure or comment that needs a change as a finding; do not edit files in this turn.",
	StageMerge: "Summarize what will be merged and anything left open. " +
		"Do not merge until the developer approves the merge gate.",
}

// prWaitInstructions replace a PR stage's instructions while the pipeline
// waits on a write turn the developer approved (pr.go): the turn carries
// out that write; the server's gh probe, not the reply, decides what
// happens next.
var prWaitInstructions = map[string]string{
	PRWaitOpen: "The developer approved the PR gate. Push the branch and open the pull request with `gh pr create` " +
		"using exactly the approved title and body. Do not merge. Report the PR's URL.",
	PRWaitFix: "Fix the accepted pull request findings listed in the message, run the verification commands yourself, " +
		"commit, and push the branch so CI runs again. Do not merge.",
	PRWaitMerge: "The developer approved the merge gate. Merge the pull request with `gh pr merge --squash` and report the result.",
}

func stageInstruction(p Pipeline) string {
	if text, ok := prWaitInstructions[p.PRWait]; ok {
		return text
	}
	stage, _ := p.Stage(p.Current)
	if stage.Instructions != "" {
		return stage.Instructions
	}
	return stageInstructions[stage.KindOf()]
}

// QuestionGuidance is the guidance of a turn that is the developer's
// question at gate: the gate's guidance even when the gate was decided
// before the turn started, so a question asked at G3 or G4 is never told
// to carry out the write step the approval started. With no question gate
// it is the stage's Guidance.
func QuestionGuidance(p Pipeline, gate string) string {
	if !questionGates[gate] || p.OpenGate == gate {
		return Guidance(p)
	}
	q := p.clone()
	q.OpenGate = gate
	if gate == GatePR || gate == GateMerge {
		q.PRWait = ""
	}
	return Guidance(q)
}

// Guidance is the hidden instruction appended to a focus turn's user
// message: the current stage, its instructions, the plan's done criteria and
// verification commands, and the exact block the reply must end with. It is
// empty when the pipeline has no active stage (complete or stopped).
func Guidance(p Pipeline) string {
	if !p.Active() {
		return ""
	}
	stage, _ := p.Stage(p.Current)
	var b strings.Builder
	fmt.Fprintf(&b, "Focus mode — current stage: %s", stage.ID)
	if stage.Limit > 0 {
		fmt.Fprintf(&b, " (iteration %d of %d)", stage.Iteration, stage.Limit)
	}
	b.WriteString(".\n")
	if goal := strings.TrimSpace(p.Goal); goal != "" {
		fmt.Fprintf(&b, "Goal: %s\n", goal)
	}
	instructions, blocks := stageInstruction(p), p.requiredBlocks(stage.KindOf())
	if stage.KindOf() == StageReview {
		instructions, blocks = reviewGuidance(p)
	}
	if p.PRWait != "" {
		// The write turn reports; a new draft or findings would be noise.
		blocks = p.requiredBlocks(StageMerge)
	}
	b.WriteString(instructions)
	b.WriteString("\n")
	if p.GoalActive() && !questionGates[p.OpenGate] {
		b.WriteString(goalGuidance)
		b.WriteString("\n")
	}
	if p.Plan != nil && stage.ID != StagePlan {
		p.writePlan(&b, stage)
	}
	b.WriteString("\n")
	b.WriteString(blocks)
	return strings.TrimRight(b.String(), "\n")
}

// writePlan lists the approved tasks and commands. A work stage of a
// template with several lists only its own tasks.
func (p Pipeline) writePlan(b *strings.Builder, stage Stage) {
	plan := *p.Plan
	tasks := plan.Tasks
	if stage.KindOf() == StageBuild && p.workStages() > 1 {
		tasks = p.stageTasks(stage.ID)
		b.WriteString("\nApproved tasks of this stage and their done criteria:\n")
	} else {
		b.WriteString("\nApproved tasks and their done criteria:\n")
	}
	for i, task := range tasks {
		fmt.Fprintf(b, "%d. %s — done when: %s\n", i+1, task.Title, orDash(task.Done))
	}
	if len(plan.Verify) > 0 {
		b.WriteString("Verification commands:\n")
		for _, cmd := range plan.Verify {
			fmt.Fprintf(b, "- %s\n", cmd)
		}
	}
	if len(plan.E2E) > 0 {
		b.WriteString("End-to-end commands:\n")
		for _, cmd := range plan.E2E {
			fmt.Fprintf(b, "- %s\n", cmd)
		}
	}
}

// blockTail closes every block request.
const blockTail = "Put the block at the very end of your reply, outside any code fence, with valid JSON between the tags. " +
	"If you include the block more than once, only the last one counts."

// planBlockFormat is the plan block of the pipeline's template: its stage
// ids, a limit for each loop, and, with several work stages, the stage a
// task belongs to.
func (p Pipeline) planBlockFormat() string {
	if p.Template == "" {
		return planFormat
	}
	ids := make([]string, 0, len(p.Stages))
	var limits []string
	work := ""
	for _, s := range p.Stages {
		ids = append(ids, fmt.Sprintf("%q", s.ID))
		switch kind := s.KindOf(); kind {
		case StageBuild, StageReview, StagePR:
			limits = append(limits, fmt.Sprintf("%q:%d", s.ID, DefaultLimits[kind]))
			if kind == StageBuild && work == "" {
				work = string(s.ID)
			}
		}
	}
	task := `{"title":"…","done":"…"}`
	if p.workStages() > 1 {
		task = fmt.Sprintf(`{"title":"…","done":"…","stage":%q}`, work)
	}
	return fmt.Sprintf(`<focus-plan>{"goal":"…","tasks":[%s],"stages":[%s],"verify":[],"limits":{%s}}</focus-plan>`,
		task, strings.Join(ids, ","), strings.Join(limits, ","))
}

// requiredBlocks is the block request of a stage kind.
func (p Pipeline) requiredBlocks(kind StageID) string {
	const tail = blockTail
	switch kind {
	case StagePlan:
		return "End your reply with exactly one plan block in this format:\n" + p.planBlockFormat() + "\n" + tail
	case StageReview, StagePRReview:
		return "End your reply with a findings block (an empty array when there are none) and a report block:\n" +
			findingsFormat + "\n" + reportFormat + "\n" + tail
	case StagePR:
		return "End your reply with a PR draft block and a report block:\n" + prFormat + "\n" + reportFormat + "\n" + tail
	case StageBuild:
		return "End your reply with exactly one report block in this format (decisions and risks may be empty):\n" +
			buildReportFormat + "\n" + buildTasksDone + "\n" + tail
	default:
		return reportBlock()
	}
}

// reportBlock asks for the plain report block.
func reportBlock() string {
	return "End your reply with exactly one report block in this format (decisions and risks may be empty):\n" +
		reportFormat + "\n" + blockTail
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}
