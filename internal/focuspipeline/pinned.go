package focuspipeline

import (
	"fmt"
	"slices"
	"strings"
)

// Pinned checks. A check the developer wrote into the goal ("use exactly
// these verify and e2e commands") was only a request to the plan stage: the
// agent rewrote it into checks it could pass, and in goal mode nobody was at
// G1 to notice (dogfooding, 2026-11-15: an end-to-end goal meant to fail
// came back as three goals the page already met, and the pipeline finished).
// A creator who means a check passes it as POST /v1/focus/pipelines
// `checks`: every plan carries it, and no edit — the agent's or the
// developer's — leaves it out.

// PinnedChecks are the checks a pipeline's creator fixed. Verify and E2E are
// added to whatever the plan lists (the plan may add more); E2ESetup and
// E2ETeardown, when given, replace the plan's — the pinned goals name what
// the pinned setup serves, and a second setup would start a second server.
type PinnedChecks struct {
	Verify      []string `json:"verify,omitempty"`
	E2E         []string `json:"e2e,omitempty"`
	E2ESetup    []string `json:"e2e_setup,omitempty"`
	E2ETeardown []string `json:"e2e_teardown,omitempty"`
}

// MaxPinnedChecks caps each pinned list.
const MaxPinnedChecks = 20

// CleanPinnedChecks trims c's lists and checks them against the pipeline's
// opt-ins: end-to-end lists need e2e on. It returns nil when nothing is
// pinned.
func CleanPinnedChecks(c *PinnedChecks, e2e bool) (*PinnedChecks, error) {
	if c == nil {
		return nil, nil
	}
	out := PinnedChecks{
		Verify: cleanStrings(c.Verify), E2E: cleanStrings(c.E2E),
		E2ESetup: cleanStrings(c.E2ESetup), E2ETeardown: cleanStrings(c.E2ETeardown),
	}
	for name, list := range map[string][]string{"verify": out.Verify, "e2e": out.E2E, "e2e_setup": out.E2ESetup, "e2e_teardown": out.E2ETeardown} {
		if len(list) > MaxPinnedChecks {
			return nil, fmt.Errorf("checks.%s: at most %d entries", name, MaxPinnedChecks)
		}
	}
	if !e2e && (len(out.E2E) > 0 || len(out.E2ESetup) > 0 || len(out.E2ETeardown) > 0) {
		return nil, fmt.Errorf("checks: end-to-end checks need e2e on")
	}
	if len(out.Verify)+len(out.E2E)+len(out.E2ESetup)+len(out.E2ETeardown) == 0 {
		return nil, nil
	}
	return &out, nil
}

// pinList is a plan's check list named name with what the pipeline fixes:
// the repository's default verify and the pinned verify added to verify,
// the pinned goals added to e2e, and a pinned setup or teardown in place of
// the plan's.
func (p Pipeline) pinList(name string, list []string) []string {
	c := p.Checks
	switch name {
	case "verify":
		list = MergeRepoVerify(list, p.RepoVerify)
		if c != nil {
			list = MergeRepoVerify(list, c.Verify)
		}
	case "e2e":
		if c != nil && p.E2E {
			list = MergeRepoVerify(list, c.E2E)
		}
	case "e2e_setup":
		if c != nil && p.E2E && len(c.E2ESetup) > 0 {
			list = slices.Clone(c.E2ESetup)
		}
	case "e2e_teardown":
		if c != nil && p.E2E && len(c.E2ETeardown) > 0 {
			list = slices.Clone(c.E2ETeardown)
		}
	}
	return list
}

// pinPlan applies pinList to every check list of plan.
func (p Pipeline) pinPlan(plan Plan) Plan {
	plan.Verify = p.pinList("verify", plan.Verify)
	plan.E2E = p.pinList("e2e", plan.E2E)
	plan.E2ESetup = p.pinList("e2e_setup", plan.E2ESetup)
	plan.E2ETeardown = p.pinList("e2e_teardown", plan.E2ETeardown)
	return plan
}

// pinnedPlanInstructions tell the plan stage which checks are fixed, so it
// plans the work to pass them instead of rewriting or questioning them.
func (p Pipeline) pinnedPlanInstructions() string {
	c := p.Checks
	if c == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(" The developer pinned these checks when starting this task. The server adds them to your plan whatever " +
		"the plan block lists, and no edit removes them: plan the work so they pass, do not reword or drop them, and do not " +
		"ask whether to keep them.")
	write := func(label string, list []string) {
		if len(list) == 0 {
			return
		}
		fmt.Fprintf(&b, " Pinned %s: %s.", label, strings.Join(quoteAll(list), "; "))
	}
	write("verify", c.Verify)
	if p.E2E {
		write("e2e", c.E2E)
		write("e2e_setup (replaces any you list)", c.E2ESetup)
		write("e2e_teardown (replaces any you list)", c.E2ETeardown)
	}
	return b.String()
}

func quoteAll(list []string) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = fmt.Sprintf("%q", s)
	}
	return out
}
