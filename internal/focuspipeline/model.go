// Package focuspipeline is the server side of Focus mode
// (docs/decisions/focus-mode.md): a development pipeline attached to one
// ordinary chat session.
//
// The package is deliberately small and mostly pure:
//
//   - model.go   the persisted pipeline (stages, gates, cards, plan)
//   - blocks.go  the parser for the <focus-*> blocks a focus turn ends with
//   - machine.go Apply, the pure state machine fed by turn completions and
//     gate actions
//   - guidance.go the stage instructions appended to each focus turn
//   - store.go   <workspace>/sessions/<id>.pipeline.json
//   - template.go the stage lists a pipeline can start from (development,
//     writing, research, and the workspace's own)
//   - goal.go    goal mode: the fixed policy that decides the gates when
//     nobody is at them
//
// Stage transitions are decided by facts (blocks, gate actions, and from P2
// on verification exit codes) and never by asking the model whether a stage
// is done.
package focuspipeline

import (
	"encoding/json"
	"maps"
	"strings"
	"time"
)

// Version is the pipeline file format version.
const Version = 1

// StageID names one stage of the development loop.
type StageID string

// The stages, in pipeline order.
const (
	StagePlan     StageID = "plan"
	StageBuild    StageID = "build"
	StageReview   StageID = "review"
	StagePR       StageID = "pr"
	StagePRReview StageID = "pr_review"
	StageMerge    StageID = "merge"
)

// StageOrder is the order of the development template's stages, the
// default every pipeline had before templates (template.go). A stage of
// another template keeps one of these as its Kind.
var StageOrder = []StageID{StagePlan, StageBuild, StageReview, StagePR, StagePRReview, StageMerge}

// StageStatus is where a stage stands.
type StageStatus string

// Stage statuses.
const (
	StatusPending StageStatus = "pending"
	StatusActive  StageStatus = "active"
	StatusDone    StageStatus = "done"
	StatusSkipped StageStatus = "skipped"
	StatusBlocked StageStatus = "blocked"
)

// Gates the developer acts on. GateNone means no gate is open.
const (
	GateNone    = ""
	GatePlan    = "plan"
	GateTriage  = "triage"
	GatePR      = "pr"
	GateMerge   = "merge"
	GateBlocked = "blocked"
)

// Card kinds, in deck priority order.
const (
	CardGate     = "gate"
	CardDecision = "decision"
	CardFinding  = "finding"
	CardFailure  = "failure"
	CardReport   = "report"
	CardChange   = "change"
	CardNotice   = "notice"
)

// Card states.
const (
	CardUnseen  = "unseen"
	CardSeen    = "seen"
	CardDecided = "decided"
)

// DefaultLimits are the loop limits a plan does not override.
var DefaultLimits = map[StageID]int{StageBuild: 3, StageReview: 2, StagePR: 3}

// Stage is one step of the pipeline.
type Stage struct {
	ID        StageID     `json:"id"`
	Status    StageStatus `json:"status"`
	Iteration int         `json:"iteration"`
	Limit     int         `json:"limit,omitempty"`
	// Turns counts the focus turns completed in the stage (build's
	// no-progress cap).
	Turns int `json:"turns,omitempty"`
	// Kind is the behaviour the stage runs with (template.go): one of the
	// development stages. Empty means the stage's own id, which is every
	// pipeline from before templates.
	Kind StageID `json:"kind,omitempty"`
	// Label is the template's name for the stage, shown as written.
	Label string `json:"label,omitempty"`
	// Instructions replace the kind's default stage instructions, and
	// FixInstructions a review-kind stage's fix-turn instructions.
	Instructions    string `json:"instructions,omitempty"`
	FixInstructions string `json:"fix_instructions,omitempty"`
}

// KindOf is the behaviour the stage runs with: its Kind, or its id.
func (s Stage) KindOf() StageID {
	if s.Kind != "" {
		return s.Kind
	}
	return s.ID
}

// Card is one item of the focus deck.
type Card struct {
	ID    string  `json:"id"`
	Kind  string  `json:"kind"`
	Stage StageID `json:"stage"`
	// Turn is the transcript turn (1-based count of user messages) the card
	// came from; 0 when it came from no turn.
	Turn int `json:"turn"`
	// Iteration is the stage's round the card came from (0 on cards from
	// before it was recorded).
	Iteration int             `json:"iteration,omitempty"`
	Title     string          `json:"title"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	State     string          `json:"state"`
	Decision  string          `json:"decision,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

// Plan is what the plan stage proposes and G1 approves.
type Plan struct {
	Goal   string         `json:"goal"`
	Tasks  []PlanTask     `json:"tasks"`
	Stages []StageID      `json:"stages"`
	Verify []string       `json:"verify"`
	E2E    []string       `json:"e2e,omitempty"`
	Limits map[string]int `json:"limits,omitempty"`
}

// PlanTask is one task of a plan and what "done" means for it.
type PlanTask struct {
	Title string `json:"title"`
	Done  string `json:"done"`
	// Stage is the work stage the task belongs to, for templates with more
	// than one; empty means every work stage.
	Stage StageID `json:"stage,omitempty"`
}

// PRInfo is the pull request a pipeline opened (P4).
type PRInfo struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
	State  string `json:"state"`
	// MergeState, Checks and the head are the latest probe's (P4).
	MergeState string    `json:"merge_state,omitempty"`
	Checks     []PRCheck `json:"checks,omitempty"`
	HeadOID    string    `json:"head_oid,omitempty"`
	HeadRef    string    `json:"head_ref,omitempty"`
	// MergeOID is the commit the PR merged as (the squash or merge commit
	// on the base branch); "" until it merges. The release train reads it.
	MergeOID string `json:"merge_oid,omitempty"`
	// HeadSince is when HeadOID was first seen (the last push); NoCI holds
	// while no probe of the PR has reported a check.
	HeadSince *time.Time `json:"head_since,omitempty"`
	NoCI      bool       `json:"no_ci,omitempty"`
}

// Pipeline is the persisted focus state of one session.
type Pipeline struct {
	Version   int     `json:"version"`
	SessionID string  `json:"session_id"`
	Goal      string  `json:"goal"`
	Stages    []Stage `json:"stages"`
	Current   StageID `json:"current"`
	Plan      *Plan   `json:"plan,omitempty"`
	// OpenGate is the gate waiting for the developer, GateNone when none.
	OpenGate string  `json:"open_gate,omitempty"`
	Cards    []Card  `json:"cards"`
	PR       *PRInfo `json:"pr,omitempty"`
	// TasksDone is the latest build report's claim that every task is
	// complete; build exits when it holds and verification passes.
	TasksDone bool `json:"tasks_done,omitempty"`
	// AwaitingVerification is set when a build turn asked for verification
	// and cleared by its result, so a stale result changes nothing.
	AwaitingVerification bool `json:"awaiting_verification,omitempty"`
	// PendingTurn is the prompt of the turn the server owes the pipeline:
	// set with every send_turn action, cleared when a turn completes or the
	// pipeline stops. A server restart that finds it set (or verification
	// awaited) raises the interrupted gate instead of resuming silently.
	PendingTurn string `json:"pending_turn,omitempty"`
	// LastFailure is the build's latest verification failure, for repeat
	// detection and the blocked gate's retry.
	LastFailure *FailureFact `json:"last_failure,omitempty"`
	// QASessionID is the hidden Q&A session of the pipeline (ADR §8), and
	// QATurns maps a card id to the Q&A turns asked about it. Q&A changes
	// nothing else.
	QASessionID string           `json:"qa_session_id,omitempty"`
	QATurns     map[string][]int `json:"qa_turns,omitempty"`
	UpdatedAt   time.Time        `json:"updated_at"`
	// FinishedAt is when the pipeline ran to its end (Finished), stamped once
	// by Apply; nil while it runs and for pipelines finished before P5. The
	// release train compares it, not UpdatedAt, with the latest tag, since
	// acknowledging a card later moves UpdatedAt.
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	// Kind is KindRelease for a pipeline started from the release train;
	// empty for feature work.
	Kind string `json:"kind,omitempty"`
	// Kickoff is the first turn's text when it differs from Goal (a release
	// lists every merged change there); stage guidance repeats only Goal.
	Kickoff string `json:"kickoff,omitempty"`
	// ReleaseItems are the sessions a release pipeline ships, and
	// ReleaseSince the cut-off its list started from (the latest tag then);
	// once the release finishes, the release train treats both as released.
	ReleaseItems []string   `json:"release_items,omitempty"`
	ReleaseSince *time.Time `json:"release_since,omitempty"`
	// BaseCommit is HEAD of the session's folder when the pipeline's first
	// turn ran, before any change: the review diff starts there (P3).
	BaseCommit string `json:"base_commit,omitempty"`
	// Review is the review loop's position in its round (P3).
	Review ReviewState `json:"review,omitzero"`
	// PRDraft is the draft G3 approved (P4, pr.go). PRWait is what the
	// pipeline waits for after a write turn (PRWaitOpen, PRWaitFix,
	// PRWaitMerge) and PRProbes how many probes found nothing meanwhile.
	// PRFixCursor is the card count at the last fix turn: findings from
	// there on are the next round's.
	PRDraft     *PRDraft `json:"pr_draft,omitempty"`
	PRWait      string   `json:"pr_wait,omitempty"`
	PRProbes    int      `json:"pr_probes,omitempty"`
	PRFixCursor int      `json:"pr_fix_cursor,omitempty"`
	// PRUnavailable is the latest probe's error while gh cannot run; a
	// probe that runs clears it.
	PRUnavailable string `json:"pr_unavailable,omitempty"`
	// WorktreeEnd is how the session worktree ended once the pipeline
	// finished (server-recorded).
	WorktreeEnd *WorktreeEnd `json:"worktree_end,omitempty"`
	// Template is the id of the template the stages came from (template.go);
	// empty is the development template.
	Template string `json:"template,omitempty"`
	// GoalMode is the pipeline's goal mode (goal.go): the server decides
	// every gate itself and pushes the pipeline to its end.
	GoalMode *GoalMode `json:"goal_mode,omitempty"`
}

// KindRelease marks a release pipeline, which the release train never lists.
const KindRelease = "release"

// New starts a pipeline for a session: every stage pending but plan, which
// is active.
func New(sessionID, goal string, now time.Time) Pipeline {
	return NewFromTemplate(sessionID, goal, DevTemplate(), now)
}

// NewFromTemplate starts a pipeline with a template's stages. The stages
// are copied into the pipeline, so editing the template later never changes
// a pipeline that is running. tpl must be valid (Template.Validate).
func NewFromTemplate(sessionID, goal string, tpl Template, now time.Time) Pipeline {
	stages := make([]Stage, 0, len(tpl.Stages))
	for _, ts := range tpl.Stages {
		stage := Stage{
			ID: ts.ID, Status: StatusPending, Label: strings.TrimSpace(ts.Label),
			Instructions: strings.TrimSpace(ts.Instructions), FixInstructions: strings.TrimSpace(ts.FixInstructions),
		}
		if ts.Kind != "" && ts.Kind != ts.ID {
			stage.Kind = ts.Kind
		}
		stages = append(stages, stage)
	}
	stages[0].Status = StatusActive
	stages[0].Iteration = 1
	p := Pipeline{
		Version:   Version,
		SessionID: sessionID,
		Goal:      goal,
		Stages:    stages,
		Current:   StagePlan,
		Cards:     []Card{},
		UpdatedAt: now.UTC(),
	}
	if tpl.ID != DevTemplateID {
		p.Template = tpl.ID
	}
	return p
}

// CurrentKind is the behaviour of the current stage.
func (p Pipeline) CurrentKind() StageID {
	s, _ := p.Stage(p.Current)
	return s.KindOf()
}

// workStages counts the pipeline's build-kind stages.
func (p Pipeline) workStages() int {
	n := 0
	for _, s := range p.Stages {
		if s.KindOf() == StageBuild {
			n++
		}
	}
	return n
}

// stageTasks are the plan's tasks of a work stage: those naming it and
// those naming none.
func (p Pipeline) stageTasks(id StageID) []PlanTask {
	if p.Plan == nil {
		return nil
	}
	var out []PlanTask
	for _, t := range p.Plan.Tasks {
		if t.Stage == "" || t.Stage == id {
			out = append(out, t)
		}
	}
	return out
}

// Stage returns the stage with id, or false.
func (p Pipeline) Stage(id StageID) (Stage, bool) {
	for _, s := range p.Stages {
		if s.ID == id {
			return s, true
		}
	}
	return Stage{}, false
}

// Active reports whether the pipeline still has a stage to work on: its
// current stage is active (not blocked, and not past the last stage).
func (p Pipeline) Active() bool {
	s, ok := p.Stage(p.Current)
	return ok && s.Status == StatusActive
}

// NeedsInput counts the cards waiting for the developer: unseen gates,
// decisions and findings.
func (p Pipeline) NeedsInput() int {
	n := 0
	for _, c := range p.Cards {
		if c.State != CardUnseen {
			continue
		}
		switch c.Kind {
		case CardGate, CardDecision, CardFinding:
			n++
		}
	}
	return n
}

// clone copies the slices and pointers Apply mutates so the caller's
// pipeline is never changed.
func (p Pipeline) clone() Pipeline {
	out := p
	out.Stages = append([]Stage(nil), p.Stages...)
	out.Cards = append([]Card{}, p.Cards...)
	if p.Plan != nil {
		plan := clonePlan(*p.Plan)
		out.Plan = &plan
	}
	if p.PR != nil {
		pr := *p.PR
		pr.Checks = append([]PRCheck(nil), p.PR.Checks...)
		if p.PR.HeadSince != nil {
			at := *p.PR.HeadSince
			pr.HeadSince = &at
		}
		out.PR = &pr
	}
	if p.WorktreeEnd != nil {
		end := *p.WorktreeEnd
		out.WorktreeEnd = &end
	}
	if p.PRDraft != nil {
		d := *p.PRDraft
		out.PRDraft = &d
	}
	if p.FinishedAt != nil {
		at := *p.FinishedAt
		out.FinishedAt = &at
	}
	out.ReleaseItems = append([]string(nil), p.ReleaseItems...)
	if p.ReleaseSince != nil {
		since := *p.ReleaseSince
		out.ReleaseSince = &since
	}
	if p.LastFailure != nil {
		f := *p.LastFailure
		f.Results = append([]VerificationResult(nil), f.Results...)
		out.LastFailure = &f
	}
	out.Review = p.Review.clone()
	if p.GoalMode != nil {
		g := *p.GoalMode
		if g.EndedAt != nil {
			at := *g.EndedAt
			g.EndedAt = &at
		}
		out.GoalMode = &g
	}
	if p.QATurns != nil {
		out.QATurns = make(map[string][]int, len(p.QATurns))
		for k, v := range p.QATurns {
			out.QATurns[k] = append([]int(nil), v...)
		}
	}
	return out
}

func clonePlan(p Plan) Plan {
	out := p
	out.Tasks = append([]PlanTask(nil), p.Tasks...)
	out.Stages = append([]StageID(nil), p.Stages...)
	out.Verify = append([]string(nil), p.Verify...)
	out.E2E = append([]string(nil), p.E2E...)
	out.Limits = maps.Clone(p.Limits)
	return out
}
