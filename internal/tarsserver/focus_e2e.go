package tarsserver

import (
	"context"
	"strings"

	"github.com/devlikebear/tars/internal/computeruse"
	"github.com/devlikebear/tars/internal/focuspipeline"
)

// End-to-end verification (docs/decisions/focus-mode.md): the plan's e2e
// list is plain-language goals for TARS's own computer_use engine
// (internal/computeruse), not shell commands — the plan stage's guidance
// says so (guidance.go's planFormat and devPlanInstructions). A goal may
// start "@AppName " to name the app computer_use brings to front; the rest
// is the goal text.
//
// done passes; stuck, max_steps, needs_confirmation and error fail (a
// verification run never resumes a risky action unattended); unavailable
// (no backend or driver configured) is skipped — passed, so the stage is
// never blocked on a capability the environment does not have, with a
// setup hint carried in the excerpt instead.

// newFocusE2ERunner builds the driver's computer_use e2e step from the
// same engine the computer_use chat tool uses (helpers_computer_use.go):
// it never fails to construct, answering "unavailable" itself when no
// backend or driver is configured.
func newFocusE2ERunner(engine *computeruse.Engine) focusE2ERunner {
	return func(ctx context.Context, sessionID, goal string) (focuspipeline.VerificationResult, error) {
		app, text := splitE2EGoal(goal)
		if app == "" {
			// Without an app computer_use acts on the frontmost window: a
			// verification run on the developer's own desktop would read
			// and click whatever they happen to have open.
			return focuspipeline.VerificationResult{Command: goal, ExitCode: -1, E2E: true, Excerpt: e2eNoAppExcerpt}, nil
		}
		res := engine.Run(ctx, computeruse.Request{Goal: text, App: app})
		return e2eVerificationResult(goal, res), nil
	}
}

// e2eNoAppExcerpt is the failure of a goal that names no app.
const e2eNoAppExcerpt = "not run: the goal names no app. Start it with \"@AppName \" (the app e2e_setup opened) — " +
	"without one computer_use would act on whatever window is in front. This is a problem with the plan, not the code: " +
	"report it and ask the developer to edit the goal."

// splitE2EGoal splits a plan e2e item's leading "@AppName" from its goal
// text. Without one, app is "" (computer_use acts on the frontmost
// window).
func splitE2EGoal(item string) (app, goal string) {
	item = strings.TrimSpace(item)
	if !strings.HasPrefix(item, "@") {
		return "", item
	}
	rest := item[1:]
	sp := strings.IndexAny(rest, " \t\n")
	if sp <= 0 {
		return "", item
	}
	return rest[:sp], strings.TrimSpace(rest[sp:])
}

// e2eVerificationResult maps one computer_use run to the verification
// result the pipeline understands. command is the plan's original e2e item
// (its "@AppName" prefix included, if any) so a failure card names exactly
// what the plan asked for.
func e2eVerificationResult(command string, res computeruse.Result) focuspipeline.VerificationResult {
	r := focuspipeline.VerificationResult{Command: command, E2E: true, Excerpt: e2eExcerpt(res)}
	switch res.Status {
	case computeruse.StatusDone:
		r.Passed = true
	case computeruse.StatusUnavailable:
		r.Passed, r.Skipped = true, true
	default: // stuck, max_steps, needs_confirmation, error, cancelled
		r.Passed = false
		// A computer_use run has no process exit code; -1 (the same
		// sentinel a shell command that could not run at all gets, in
		// focus_driver.go) keeps a failed check from showing the console's
		// "→ 0" next to it, which reads as a successful shell exit.
		r.ExitCode = -1
	}
	return r
}

// e2eExcerpt is a computer_use result's status, reason, hint and last
// screen as one verification excerpt.
func e2eExcerpt(res computeruse.Result) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(string(res.Status)))
	if res.Reason != "" {
		b.WriteString(": ")
		b.WriteString(res.Reason)
	}
	if res.Hint != "" {
		b.WriteString("\nHint: ")
		b.WriteString(res.Hint)
	}
	if len(res.LastScreen) > 0 {
		b.WriteString("\nLast screen:\n")
		b.WriteString(strings.Join(res.LastScreen, "\n"))
	}
	return b.String()
}
