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
	StagePRReview: "Check the pull request's CI results and review comments. " +
		"Report each failure or comment that needs a change as a finding; do not edit files in this turn.",
	StageMerge: "Summarize what will be merged and anything left open. " +
		"Do not merge until the developer approves the merge gate.",
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
	instructions, blocks := stageInstructions[stage.ID], requiredBlocks(stage.ID)
	if stage.ID == StageReview {
		instructions, blocks = reviewGuidance(p)
	}
	b.WriteString(instructions)
	b.WriteString("\n")
	if p.Plan != nil && stage.ID != StagePlan {
		writePlan(&b, *p.Plan)
	}
	b.WriteString("\n")
	b.WriteString(blocks)
	return strings.TrimRight(b.String(), "\n")
}

func writePlan(b *strings.Builder, plan Plan) {
	b.WriteString("\nApproved tasks and their done criteria:\n")
	for i, task := range plan.Tasks {
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

func requiredBlocks(stage StageID) string {
	const tail = blockTail
	switch stage {
	case StagePlan:
		return "End your reply with exactly one plan block in this format:\n" + planFormat + "\n" + tail
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
