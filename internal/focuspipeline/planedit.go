package focuspipeline

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Editing an approved plan. G1 is the only place a plan could change, so a
// goal or check that turned out wrong after approval (an end-to-end goal
// the environment cannot run, say) kept the loop going until its limits
// ran out. EditPlan changes the plan's goal and its check lists in place:
// stages, tasks and limits stay as approved, and no stage transition
// happens — the next verification simply reads the edited lists.

// TagPlanEdit is the block a turn edits the approved plan with.
const TagPlanEdit = "focus-plan-edit"

// NoticePlanEdited is the title of the card an edit leaves, with what
// changed in its payload ("changes").
const NoticePlanEdited = "plan edited"

// ErrNoApprovedPlan is an edit of a pipeline whose plan is not approved
// yet (the plan stage edits it at G1) or that already ran to its end.
var ErrNoApprovedPlan = errors.New("no approved plan to edit")

// ErrGoalModeChecks is an edit the agent made in goal mode that would
// change the plan's checks. In goal mode no one is at the gates and the
// checks' exit codes are what lets review pass, so the agent must not be
// able to rewrite them (#1290). The developer still can, from the console.
var ErrGoalModeChecks = errors.New("goal mode is on: the plan's checks (verify, e2e, e2e_setup, e2e_teardown) are fixed; fix the code, not the plan")

// PlanEdit is the fields of an approved plan to replace. A nil field stays
// as it is; an empty list clears that list.
type PlanEdit struct {
	Goal        *string   `json:"goal,omitempty"`
	Verify      *[]string `json:"verify,omitempty"`
	E2E         *[]string `json:"e2e,omitempty"`
	E2ESetup    *[]string `json:"e2e_setup,omitempty"`
	E2ETeardown *[]string `json:"e2e_teardown,omitempty"`
}

// Empty reports whether the edit names no field.
func (e PlanEdit) Empty() bool {
	return e.Goal == nil && e.Verify == nil && e.E2E == nil && e.E2ESetup == nil && e.E2ETeardown == nil
}

// GoalModeLockedFields names the checks an edit from the agent (the
// focus_plan_edit tool or a <focus-plan-edit> block) would change while
// the pipeline is in goal mode, in plan order; nil when the edit may go
// ahead. A list given exactly as it is changes nothing and is not named,
// so an edit that restates the checks next to a new goal still passes.
// The goal's wording is not locked: no transition reads it.
func GoalModeLockedFields(p Pipeline, edit PlanEdit) []string {
	if !p.GoalActive() || p.Plan == nil {
		return nil
	}
	var locked []string
	for _, l := range []struct {
		name string
		edit *[]string
		list []string
	}{
		{"verify", edit.Verify, p.Plan.Verify},
		{"e2e", edit.E2E, p.Plan.E2E},
		{"e2e_setup", edit.E2ESetup, p.Plan.E2ESetup},
		{"e2e_teardown", edit.E2ETeardown, p.Plan.E2ETeardown},
	} {
		if l.edit != nil && !slices.Equal(l.list, cleanStrings(*l.edit)) {
			locked = append(locked, l.name)
		}
	}
	return locked
}

// EditPlanByAgent is EditPlan for an edit the agent made: in goal mode an
// edit that would change a check is refused with ErrGoalModeChecks and
// changes nothing.
func EditPlanByAgent(p Pipeline, edit PlanEdit, now time.Time) (Pipeline, bool, error) {
	if locked := GoalModeLockedFields(p, edit); len(locked) > 0 {
		return p, false, fmt.Errorf("%w (asked to change %s)", ErrGoalModeChecks, strings.Join(locked, ", "))
	}
	return EditPlan(p, edit, now)
}

// EditPlan applies edit to the pipeline's approved plan and leaves a notice
// card listing what changed. changed is false, and p is returned as it
// was, when the edit changes nothing. On error p is returned unchanged.
func EditPlan(p Pipeline, edit PlanEdit, now time.Time) (next Pipeline, changed bool, err error) {
	if p.Plan == nil || p.Current == StagePlan || Finished(p) {
		return p, false, ErrNoApprovedPlan
	}
	if !p.E2E && (edit.E2E != nil || edit.E2ESetup != nil || edit.E2ETeardown != nil) {
		return p, false, fmt.Errorf("%w: end-to-end goals are off for this pipeline; put the check in verify as a shell command", ErrInvalidEdits)
	}
	if edit.Goal != nil && strings.TrimSpace(*edit.Goal) == "" {
		return p, false, fmt.Errorf("%w: goal must not be empty", ErrInvalidEdits)
	}
	next = p.clone()
	var changes []string
	if edit.Goal != nil {
		if goal := strings.TrimSpace(*edit.Goal); goal != next.Plan.Goal || goal != next.Goal {
			changes = append(changes, fmt.Sprintf("goal: %s → %s", orDash(next.Plan.Goal), goal))
			next.Plan.Goal, next.Goal = goal, goal
		}
	}
	lists := []struct {
		name string
		edit *[]string
		list *[]string
	}{
		{"verify", edit.Verify, &next.Plan.Verify},
		{"e2e", edit.E2E, &next.Plan.E2E},
		{"e2e_setup", edit.E2ESetup, &next.Plan.E2ESetup},
		{"e2e_teardown", edit.E2ETeardown, &next.Plan.E2ETeardown},
	}
	for _, l := range lists {
		if l.edit == nil {
			continue
		}
		after := cleanStrings(*l.edit)
		if slices.Equal(*l.list, after) {
			continue
		}
		changes = append(changes, listChanges(l.name, *l.list, after)...)
		*l.list = after
	}
	if len(changes) == 0 {
		return p, false, nil
	}
	// A failure of a check the plan no longer has is not "the same failure
	// twice" for the checks it has now.
	if f := next.LastFailure; f != nil && !next.Plan.hasCheck(f.Command) {
		next.LastFailure = nil
	}
	next.addCard(CardNotice, 0, NoticePlanEdited, map[string]any{"changes": changes}, now.UTC())
	next.UpdatedAt = now.UTC()
	return next, true, nil
}

// hasCheck reports whether command is one of the plan's checks.
func (p Plan) hasCheck(command string) bool {
	command = strings.TrimSpace(command)
	for _, list := range [][]string{p.Verify, p.E2ESetup, p.E2E} {
		if slices.Contains(list, command) {
			return true
		}
	}
	return false
}

// listChanges names what an edit removed from and added to a list.
func listChanges(name string, before, after []string) []string {
	var out []string
	for _, v := range before {
		if !slices.Contains(after, v) {
			out = append(out, fmt.Sprintf("%s − %s", name, v))
		}
	}
	for _, v := range after {
		if !slices.Contains(before, v) {
			out = append(out, fmt.Sprintf("%s + %s", name, v))
		}
	}
	if len(out) == 0 {
		out = append(out, name+": reordered")
	}
	return out
}

// planEditGuidance tells a turn after G1 how the approved plan changes.
// The end-to-end fields are named only to a pipeline that has them. In goal
// mode the checks are locked (GoalModeLockedFields), so the guidance offers
// only the goal and says the checks are fixed rather than invite an edit
// the server refuses.
func (p Pipeline) planEditGuidance() string {
	if p.GoalActive() {
		checks := "verification commands"
		if p.E2E {
			checks = "verification commands and end-to-end goals"
		}
		return "Goal mode is on: the plan's " + checks + " cannot be changed in this run, and an edit to them is refused. " +
			"Only the goal's wording changes, and only when the developer asks for it in this conversation: then call the focus_plan_edit tool if you have it, " +
			"or else add this block before the others:\n" +
			`<focus-plan-edit>{"goal":"…"}</focus-plan-edit>` + "\n" +
			"A failing check is fixed in the code, never by editing the plan."
	}
	what, fields := "a verification command", ""
	if p.E2E {
		what, fields = "a verification command or end-to-end goal", `,"e2e":["…"],"e2e_setup":["…"],"e2e_teardown":["…"]`
	}
	return "The approved plan changes only when the developer asks for it in this conversation " +
		"(a new goal, or " + what + " to add, drop or reword): then call the focus_plan_edit tool if you have it, " +
		"or else add this block before the others, with only the fields that change (a list replaces the whole list; [] clears it):\n" +
		`<focus-plan-edit>{"goal":"…","verify":["…"]` + fields + `}</focus-plan-edit>` + "\n" +
		"Never edit the plan on your own to get past a failing check."
}
