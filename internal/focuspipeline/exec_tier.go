package focuspipeline

import (
	"slices"
	"time"
)

// The execution tier (docs/decisions/focus-mode.md §5.3, #1293). A
// pipeline created with exec_tier sends its execution turns — every turn of
// a build-kind stage and the fix turns of review and pr_review — on that
// tier, and keeps plan, review, PR and merge turns on the session's own
// tier. Writing the work is what a cheaper model may do: verify and CI
// still decide every transition, so a weaker executor costs fix rounds,
// never a wrong transition. Planning and reviewing are what those rounds
// are measured against, so they stay where the developer put them.
//
// The opt-in is also the measurement label: the tier it ran on and how many
// adopted harness lessons the repository's library held when it started
// (harness_library.go), so ExecReport can compare the configurations of
// #1293 (A heavy, B lower tier, C lower tier with a library). A pipeline
// that did not opt in carries none of it and stays byte for byte what it
// was (testdata/no_opt_in.golden).

// NoticeExecTierFallback is the notice an execution turn leaves when it
// stayed on the session's tier: switching would have broken a resumed CLI
// provider session. Left once per pipeline.
const NoticeExecTierFallback = "execution tier not used"

// ExecTierFallbackResume is the notice's reason: the session's provider and
// the execution tier's are different kinds and one of them resumes an
// upstream session instead of replaying the transcript.
const ExecTierFallbackResume = "resume_provider"

// ExecTierFallbackUnresolved is the notice's reason when the execution
// tier no longer resolves in the configuration.
const ExecTierFallbackUnresolved = "unresolved"

// ExecTier opts a pipeline in to running its execution turns on Tier.
type ExecTier struct {
	// Tier is heavy, standard or light (llm.ParseTier).
	Tier string `json:"tier"`
	// Repo is the main checkout of the pipeline's repository when it was
	// created ("" outside a git repository): the report groups by it.
	Repo string `json:"repo,omitempty"`
	// Lessons is how many adopted lessons the repository's harness library
	// held when the pipeline was created: more than zero is configuration
	// C's "library applied".
	Lessons int `json:"lessons,omitempty"`
	// At is when the pipeline was created; the report reads its cost from
	// here on.
	At time.Time `json:"at"`
}

// ExecTurn reports whether the turn the pipeline owes now is execution
// work: a build-kind stage's turn, or a fix turn of review (Review.Fixing)
// or pr_review (PRWait fix). Only a pipeline that opted in has any.
func (p Pipeline) ExecTurn() bool {
	if p.ExecTier == nil || p.ExecTier.Tier == "" || !p.Active() {
		return false
	}
	switch p.CurrentKind() {
	case StageBuild:
		return true
	case StageReview:
		return p.Review.Fixing
	case StagePRReview:
		return p.PRWait == PRWaitFix
	}
	return false
}

// ExecTurnTier is the tier the owed turn runs on: the opted-in tier for an
// execution turn, "" (the session's own tier) for any other.
func (p Pipeline) ExecTurnTier() string {
	if !p.ExecTurn() {
		return ""
	}
	return p.ExecTier.Tier
}

// AddExecTierFallback leaves the fallback notice the first time an
// execution turn stays on the session's tier; ok is false when the pipeline
// already has it (it would only repeat every turn).
func AddExecTierFallback(p Pipeline, reason, sessionProvider, execProvider string, now time.Time) (Pipeline, bool) {
	if slices.ContainsFunc(p.Cards, func(c Card) bool { return c.Kind == CardNotice && c.Title == NoticeExecTierFallback }) {
		return p, false
	}
	tier := ""
	if p.ExecTier != nil {
		tier = p.ExecTier.Tier
	}
	return AddNotice(p, NoticeExecTierFallback, map[string]any{
		"reason": reason, "tier": tier, "session_provider": sessionProvider, "exec_provider": execProvider,
	}, now), true
}

// ExecTierSwitchSafe reports whether a session on sessionKind may run one
// turn on execKind and come back. The same kind always may (a CLI provider
// resumes its upstream session across models). Different kinds may only
// when neither resumes an upstream session: a resuming provider never sees
// the turns another provider ran in between, and hands the other kind an
// upstream id it cannot use. resumes says which kinds resume
// (claude-code-cli, antigravity-cli).
func ExecTierSwitchSafe(sessionKind, execKind string, resumes func(kind string) bool) bool {
	if sessionKind == execKind {
		return true
	}
	return !resumes(sessionKind) && !resumes(execKind)
}
