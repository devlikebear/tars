package easyproject

import (
	"encoding/json"
	"time"

	"github.com/devlikebear/tars/internal/focuspipeline"
)

// State is where a project stands on the project board
// (docs/plans/vibe-mode-redesign, unit 1): one word per project, read from
// the project record and its pipeline's facts. No model is asked.
type State string

const (
	// StateStarting: the folder and the stack are being set up.
	StateStarting State = "starting"
	// StateFailed: setting the project up failed (Project.ErrorCode).
	StateFailed State = "failed"
	// StateYourTurn: the team waits on the person (the plan to approve, or
	// a question asked before goal mode started).
	StateYourTurn State = "your_turn"
	// StateWorking: the team works by itself.
	StateWorking State = "working"
	// StatePaused: the AI's usage limit stopped the team; it goes on when
	// the limit lifts and the person says so.
	StatePaused State = "paused"
	// StateStuck: the team stopped and cannot go on by itself.
	StateStuck State = "stuck"
	// StateDone: finished, and its version saved (or saving it failed,
	// which Board.SaveFailed says).
	StateDone State = "done"
	// StatePublished is reserved for publishing (unit 9); BoardOf never
	// gives it yet.
	StatePublished State = "published"
)

// States are the board's states in the order the board shows them.
var States = []State{StateYourTurn, StateWorking, StatePaused, StateStuck, StateDone, StatePublished, StateStarting, StateFailed}

// Waiting kinds a your_turn project waits on.
const (
	WaitingPlan     = "plan"
	WaitingQuestion = "question"
)

// Stuck reasons.
const (
	// StuckStopped: the person stopped the team.
	StuckStopped = "stopped"
	// StuckGoalEnded: the team gave up going on by itself (goal mode ended
	// on something other than finishing).
	StuckGoalEnded = "goal_ended"
	// StuckBlocked: the team stopped at a problem it cannot pass.
	StuckBlocked = "blocked"
)

// Board is a project's state and the fixed facts its card shows.
type Board struct {
	State State `json:"state"`
	// Waiting is what a your_turn project waits on (WaitingPlan,
	// WaitingQuestion).
	Waiting string `json:"waiting,omitempty"`
	// Resets and ResetsAt are when a paused project's usage limit lifts:
	// the provider's own words, or a time when it gave one.
	Resets   string     `json:"resets,omitempty"`
	ResetsAt *time.Time `json:"resets_at,omitempty"`
	// Stuck says why a stuck project stopped (StuckStopped, …).
	Stuck string `json:"stuck,omitempty"`
	// Check is the last failed check, in the check's own words.
	Check *focuspipeline.CheckReport `json:"check,omitempty"`
	// SaveFailed marks a done project whose version could not be saved.
	SaveFailed bool `json:"save_failed,omitempty"`
	// ChangedAt is the latest change to the project or its pipeline: the
	// board's order, newest first.
	ChangedAt time.Time `json:"changed_at"`
}

// BoardOf gives a project its board state from its record and its current
// pipeline (nil when it has none). It follows the console's own reading of
// the same facts (frontend/console/src/lib/easy.ts easyView): finished
// before a plan to approve, a plan to approve before a stop, and a stop on
// the usage limit is a pause, not a problem.
//
// A ready project whose pipeline is gone (its session was deleted) is done:
// its folder still holds the saved versions, and nothing works in it.
func BoardOf(p Project, pipe *focuspipeline.Pipeline) Board {
	b := Board{ChangedAt: p.UpdatedAt}
	if p.CreatedAt.After(b.ChangedAt) {
		b.ChangedAt = p.CreatedAt
	}
	switch p.Status {
	case StatusFailed:
		b.State = StateFailed
		return b
	case StatusCreating:
		b.State = StateStarting
		return b
	}
	if p.SessionID == "" {
		b.State = StateStarting
		return b
	}
	if pipe == nil {
		b.State = StateDone
		return b
	}
	if pipe.UpdatedAt.After(b.ChangedAt) {
		b.ChangedAt = pipe.UpdatedAt
	}
	b.Check = lastCheckFailure(*pipe)

	if pipe.FinishedAt != nil || focuspipeline.Finished(*pipe) {
		// The server saves the version a moment after the pipeline
		// finishes; until then the team is still at it.
		if pipe.SaveVersion && pipe.Saved == nil {
			b.State = StateWorking
			return b
		}
		b.State = StateDone
		b.SaveFailed = pipe.Saved != nil && (pipe.Saved.Error != "" || pipe.Saved.Commit == "")
		b.Check = nil
		return b
	}
	waiting := focuspipeline.WaitingOf(*pipe)
	if waiting.Gate == focuspipeline.GatePlan && pipe.Plan != nil {
		b.State, b.Waiting = StateYourTurn, WaitingPlan
		return b
	}
	if limit := pipe.UsageLimitStop(); limit != nil {
		b.State, b.Resets, b.ResetsAt = StatePaused, limit.Resets, limit.ResetsAt
		return b
	}
	if stuck := stuckReason(*pipe); stuck != "" {
		b.State, b.Stuck = StateStuck, stuck
		return b
	}
	// A question asked before goal mode started waits on the person;
	// once goal mode runs, its watcher answers cards by itself.
	if pipe.GoalMode == nil && len(waiting.Cards) > 0 {
		b.State, b.Waiting = StateYourTurn, WaitingQuestion
		return b
	}
	b.State = StateWorking
	return b
}

// stuckReason says why the team cannot go on by itself, or "" when it can:
// the person stopped it, its current stage is blocked at a gate, or goal
// mode ended without finishing. While goal mode is still on, a blocked
// stage or gate is one it retries by itself in a moment (a failed turn
// waits out its backoff at a blocked gate), so the team is still at work:
// the console's easyView reads the same, and
// testdata/board_states.json holds both to it.
func stuckReason(p focuspipeline.Pipeline) string {
	if p.GoalMode != nil && p.GoalMode.Enabled {
		return ""
	}
	if p.Stopped() {
		return StuckStopped
	}
	if p.OpenGate == focuspipeline.GateBlocked {
		return StuckBlocked
	}
	for _, s := range p.Stages {
		if s.Status == focuspipeline.StatusBlocked {
			return StuckBlocked
		}
	}
	if g := p.GoalMode; g != nil && !g.Enabled && g.EndReason != focuspipeline.GoalEndFinished {
		return StuckGoalEnded
	}
	return ""
}

// lastCheckFailure is the last failure card's check report, when that card
// carries one (easyView reads the same).
func lastCheckFailure(p focuspipeline.Pipeline) *focuspipeline.CheckReport {
	for i := len(p.Cards) - 1; i >= 0; i-- {
		c := p.Cards[i]
		if c.Kind != focuspipeline.CardFailure {
			continue
		}
		var fact focuspipeline.FailureFact
		ok := len(c.Payload) > 0 && json.Unmarshal(c.Payload, &fact) == nil
		if !ok || fact.Check == nil || fact.Check.Expected == "" || fact.Check.Seen == "" {
			return nil
		}
		check := *fact.Check
		return &check
	}
	return nil
}
