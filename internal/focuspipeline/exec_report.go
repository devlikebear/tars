package focuspipeline

import (
	"slices"
	"time"
)

// The execution tier measurement (#1293, docs/decisions/focus-mode.md
// §5.3): the configurations compared are
//
//	A  heavy execution tier,            no harness library
//	B  standard or light execution tier, no harness library
//	C  standard or light execution tier, a harness library applied
//
// and every number is a fact the server already holds — the pipeline's
// cost from the usage records, its fix rounds, failed verifications, goal
// mode pushes and how it ended. ExecFactsOf reads them from one pipeline
// (the server adds the cost) and ExecReport groups them by configuration.
// No model is asked anything.

// Library labels of a report row.
const (
	ExecLibraryNone       = "none"
	ExecLibraryApplied    = "applied"
	ExecLibraryUnrecorded = "unrecorded"
)

// ExecTierSession is the tier label of a pipeline that did not opt in: all
// of its turns ran on the session's tier, which the pipeline never
// recorded.
const ExecTierSession = "session"

// Pipeline outcomes in the report.
const (
	ExecOutcomeFinished = "finished"
	ExecOutcomeStopped  = "stopped"
	ExecOutcomeRunning  = "running"
)

// ExecMinSamples is how many ended pipelines each of A and C needs before
// the report says whether C is the configuration to recommend.
const ExecMinSamples = 3

// ExecFacts are one pipeline's measured facts.
type ExecFacts struct {
	SessionID string `json:"session_id"`
	// Tier is the execution tier, ExecTierSession without the opt-in.
	Tier string `json:"tier"`
	// Library is ExecLibraryApplied when the repository's harness library
	// held adopted lessons at creation, ExecLibraryNone when it held none,
	// ExecLibraryUnrecorded without the opt-in.
	Library string `json:"library"`
	Repo    string `json:"repo,omitempty"`
	Outcome string `json:"outcome"`
	// Start is when the pipeline was created (opt-in) or its goal mode
	// started; zero when neither is known.
	Start time.Time `json:"start,omitzero"`
	// FixRounds counts the review and pr_review rounds after the first:
	// each one exists because a fix turn ran.
	FixRounds int `json:"fix_rounds"`
	// FailedVerifies counts the verification failures (failure cards).
	FailedVerifies int `json:"failed_verifies"`
	// Goal says the pipeline ran in goal mode; GoalEnded that goal mode
	// ended, GoalFinished that it ended because the pipeline finished, and
	// GoalPushes the retries and pushes it spent.
	Goal         bool `json:"goal"`
	GoalEnded    bool `json:"goal_ended"`
	GoalFinished bool `json:"goal_finished"`
	GoalPushes   int  `json:"goal_pushes"`
	// CostUSD is the pipeline session's recorded cost since Start, and
	// UnpricedCalls the calls no price was known for (missing from it).
	CostUSD       float64 `json:"cost_usd"`
	UnpricedCalls int     `json:"unpriced_calls"`
}

// ExecFactsOf reads a pipeline's facts; the cost is the caller's to add.
func ExecFactsOf(p Pipeline) ExecFacts {
	f := ExecFacts{SessionID: p.SessionID, Tier: ExecTierSession, Library: ExecLibraryUnrecorded}
	if et := p.ExecTier; et != nil && et.Tier != "" {
		f.Tier, f.Repo, f.Start = et.Tier, et.Repo, et.At
		f.Library = ExecLibraryNone
		if et.Lessons > 0 {
			f.Library = ExecLibraryApplied
		}
	}
	switch {
	case Finished(p):
		f.Outcome = ExecOutcomeFinished
	case p.Active():
		f.Outcome = ExecOutcomeRunning
	default:
		f.Outcome = ExecOutcomeStopped
	}
	for _, s := range p.Stages {
		switch s.KindOf() {
		case StageReview, StagePRReview:
			f.FixRounds += max(0, s.Iteration-1)
		}
	}
	for _, c := range p.Cards {
		if c.Kind == CardFailure {
			f.FailedVerifies++
		}
	}
	if g := p.GoalMode; g != nil {
		f.Goal = true
		f.GoalPushes = g.Pushes
		f.GoalEnded = g.EndedAt != nil
		f.GoalFinished = g.EndReason == GoalEndFinished
		if f.Start.IsZero() {
			f.Start = g.StartedAt
		}
	}
	return f
}

// ExecReportRow is one configuration's numbers. The rates and means count
// ended pipelines only (finished or stopped): a running one has not spent
// what it will.
type ExecReportRow struct {
	Tier      string `json:"tier"`
	Library   string `json:"library"`
	Pipelines int    `json:"pipelines"`
	Finished  int    `json:"finished"`
	Stopped   int    `json:"stopped"`
	Running   int    `json:"running"`
	// CompletionRate is finished / (finished + stopped).
	CompletionRate float64 `json:"completion_rate"`
	// GoalRuns counts the ended pipelines whose goal mode ended, and
	// GoalCompletionRate the share of those it ended by finishing.
	GoalRuns           int     `json:"goal_runs"`
	GoalFinished       int     `json:"goal_finished"`
	GoalCompletionRate float64 `json:"goal_completion_rate"`
	CostUSD            float64 `json:"cost_usd"`
	MeanCostUSD        float64 `json:"mean_cost_usd"`
	UnpricedCalls      int     `json:"unpriced_calls"`
	MeanFixRounds      float64 `json:"mean_fix_rounds"`
	MeanFailedVerifies float64 `json:"mean_failed_verifies"`
	MeanGoalPushes     float64 `json:"mean_goal_pushes"`
}

// ExecComparison is configuration C (a lower tier with a library) against
// A (heavy, no library). Recommend holds when C is cheaper per ended
// pipeline and completes at least as often — the goal mode completion
// rate when both ran goal mode, else the completion rate — with
// ExecMinSamples ended pipelines on each side.
type ExecComparison struct {
	Tier         string `json:"tier"`
	Enough       bool   `json:"enough_samples"`
	Cheaper      bool   `json:"cheaper"`
	CompletionOK bool   `json:"completion_at_least_a"`
	// Rate names the rate compared: "goal_completion" or "completion".
	Rate      string `json:"rate"`
	Recommend bool   `json:"recommend"`
}

// ExecReportResult is the report: one row per configuration and C's
// comparison with A for each lower tier that has both.
type ExecReportResult struct {
	Rows        []ExecReportRow  `json:"rows"`
	Comparisons []ExecComparison `json:"comparisons"`
}

// ExecReport groups facts by (tier, library), in the order heavy,
// standard, light, session and none, applied, unrecorded.
func ExecReport(facts []ExecFacts) ExecReportResult {
	type acc struct {
		row                            ExecReportRow
		fix, failed, pushes, goalEnded int
	}
	groups := map[[2]string]*acc{}
	for _, f := range facts {
		key := [2]string{f.Tier, f.Library}
		a := groups[key]
		if a == nil {
			a = &acc{row: ExecReportRow{Tier: f.Tier, Library: f.Library}}
			groups[key] = a
		}
		a.row.Pipelines++
		switch f.Outcome {
		case ExecOutcomeRunning:
			a.row.Running++
			continue
		case ExecOutcomeFinished:
			a.row.Finished++
		default:
			a.row.Stopped++
		}
		a.row.CostUSD += f.CostUSD
		a.row.UnpricedCalls += f.UnpricedCalls
		a.fix += f.FixRounds
		a.failed += f.FailedVerifies
		a.pushes += f.GoalPushes
		if f.Goal && f.GoalEnded {
			a.row.GoalRuns++
			if f.GoalFinished {
				a.row.GoalFinished++
			}
		}
	}
	out := ExecReportResult{Rows: []ExecReportRow{}, Comparisons: []ExecComparison{}}
	for _, a := range groups {
		r := a.row
		if ended := r.Finished + r.Stopped; ended > 0 {
			n := float64(ended)
			r.CompletionRate = float64(r.Finished) / n
			r.MeanCostUSD = r.CostUSD / n
			r.MeanFixRounds = float64(a.fix) / n
			r.MeanFailedVerifies = float64(a.failed) / n
			r.MeanGoalPushes = float64(a.pushes) / n
		}
		if r.GoalRuns > 0 {
			r.GoalCompletionRate = float64(r.GoalFinished) / float64(r.GoalRuns)
		}
		out.Rows = append(out.Rows, r)
	}
	slices.SortFunc(out.Rows, func(x, y ExecReportRow) int {
		if d := execTierRank(x.Tier) - execTierRank(y.Tier); d != 0 {
			return d
		}
		return execLibraryRank(x.Library) - execLibraryRank(y.Library)
	})
	i := slices.IndexFunc(out.Rows, func(r ExecReportRow) bool { return r.Tier == "heavy" && r.Library == ExecLibraryNone })
	if i < 0 {
		return out
	}
	for _, r := range out.Rows {
		if r.Library != ExecLibraryApplied || r.Tier == "heavy" || r.Tier == ExecTierSession {
			continue
		}
		out.Comparisons = append(out.Comparisons, compareExec(out.Rows[i], r))
	}
	return out
}

// compareExec compares configuration C (c) with A (a).
func compareExec(a, c ExecReportRow) ExecComparison {
	aEnded, cEnded := a.Finished+a.Stopped, c.Finished+c.Stopped
	cmp := ExecComparison{Tier: c.Tier, Rate: "completion", Enough: aEnded >= ExecMinSamples && cEnded >= ExecMinSamples}
	if aEnded == 0 || cEnded == 0 {
		return cmp
	}
	aRate, cRate := a.CompletionRate, c.CompletionRate
	if a.GoalRuns > 0 && c.GoalRuns > 0 {
		cmp.Rate = "goal_completion"
		aRate, cRate = a.GoalCompletionRate, c.GoalCompletionRate
	}
	cmp.Cheaper = c.MeanCostUSD < a.MeanCostUSD
	cmp.CompletionOK = cRate >= aRate
	cmp.Recommend = cmp.Enough && cmp.Cheaper && cmp.CompletionOK
	return cmp
}

func execTierRank(tier string) int {
	switch tier {
	case "heavy":
		return 0
	case "standard":
		return 1
	case "light":
		return 2
	case ExecTierSession:
		return 4
	}
	return 3
}

func execLibraryRank(library string) int {
	switch library {
	case ExecLibraryNone:
		return 0
	case ExecLibraryApplied:
		return 1
	}
	return 2
}
