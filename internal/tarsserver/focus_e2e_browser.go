package tarsserver

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/aside"
	"github.com/devlikebear/tars/internal/focuspipeline"
)

// Browser end-to-end goals: "@Browser <local URL> <goal>" runs in the
// developer's Aside browser through `aside exec` (internal/aside) instead
// of computer_use. Aside's agent looks at the rendered page, so it can
// judge what computer_use's accessibility tree cannot — an image, a chart,
// a layout. It runs on the model chosen in Aside's settings, not on a TARS
// tier.
//
// The browser is signed in to the developer's real accounts and nobody
// watches a verification run, so the start page must be the app under
// development (aside.LocalStartURL: loopback, *.localhost, *.test) and the
// prompt keeps the agent on that origin. A goal that breaks this is a plan
// problem and fails without opening anything, like a computer_use goal
// with no @App.
//
// No aside CLI is "unavailable" and skipped, as computer_use without a
// driver is: the stage is never blocked on a capability this machine does
// not have.

// browserE2EApp is the @App name that selects the Aside browser.
const browserE2EApp = "Browser"

// browserE2ETimeout bounds one Aside agent run.
const browserE2ETimeout = 10 * time.Minute

// browserE2EExcerptLines is how much of the agent's transcript a failure
// card keeps.
const browserE2EExcerptLines = 30

type browserCheckFunc func(ctx context.Context, check aside.Check) (aside.CheckResult, error)

// withBrowserE2E routes @Browser goals to verify and every other goal to
// next.
func withBrowserE2E(verify browserCheckFunc, next focusE2ERunner) focusE2ERunner {
	return func(ctx context.Context, sessionID, goal string) (focuspipeline.VerificationResult, error) {
		app, text := splitE2EGoal(goal)
		if !strings.EqualFold(app, browserE2EApp) {
			return next(ctx, sessionID, goal)
		}
		return runBrowserE2E(ctx, verify, goal, text), nil
	}
}

func runBrowserE2E(ctx context.Context, verify browserCheckFunc, goal, text string) focuspipeline.VerificationResult {
	failed := func(excerpt string) focuspipeline.VerificationResult {
		return focuspipeline.VerificationResult{Command: goal, ExitCode: -1, E2E: true, Excerpt: excerpt}
	}
	startURL, rest, _ := strings.Cut(strings.TrimSpace(text), " ")
	rest = strings.TrimSpace(rest)
	if _, err := aside.LocalStartURL(startURL); err != nil || rest == "" {
		return failed(browserE2EPlanExcerpt)
	}
	runCtx, cancel := context.WithTimeout(ctx, browserE2ETimeout)
	defer cancel()
	res, err := verify(runCtx, aside.Check{StartURL: startURL, Goal: rest})
	switch {
	case errors.Is(err, aside.ErrNotFound):
		return focuspipeline.VerificationResult{
			Command: goal, Passed: true, Skipped: true, E2E: true,
			Excerpt: "unavailable: " + err.Error() + "\nHint: install Aside's CLI (`aside login`) or set tools.aside.path.",
		}
	case err != nil:
		return failed("error: " + err.Error() + transcriptTail(res.Transcript))
	case res.Passed:
		return focuspipeline.VerificationResult{Command: goal, Passed: true, E2E: true, Excerpt: "done" + transcriptTail(res.Transcript)}
	default:
		return failed("failed: " + res.Reason + transcriptTail(res.Transcript))
	}
}

// browserE2EPlanExcerpt is the failure of a @Browser goal with no local
// start page or no goal.
const browserE2EPlanExcerpt = "not run: a @Browser goal is \"@Browser <URL> <goal>\" and the URL must be the app under development " +
	"(localhost, 127.0.0.1, [::1], *.localhost or *.test) — the browser is signed in to the developer's real accounts. " +
	"This is a problem with the plan, not the code: report it and ask the developer to edit the goal."

func transcriptTail(transcript string) string {
	transcript = strings.TrimSpace(transcript)
	if transcript == "" {
		return ""
	}
	lines := strings.Split(transcript, "\n")
	if len(lines) > browserE2EExcerptLines {
		lines = lines[len(lines)-browserE2EExcerptLines:]
	}
	return "\nLast output:\n" + strings.Join(lines, "\n")
}
