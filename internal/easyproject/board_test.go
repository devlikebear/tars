package easyproject

import (
	"encoding/json"
	"testing"
	"time"

	fp "github.com/devlikebear/tars/internal/focuspipeline"
)

// boardPipeline is a webapp pipeline (plan → make → check) with the
// statuses given, the current stage the first one not done.
func boardPipeline(statuses ...fp.StageStatus) fp.Pipeline {
	ids := []fp.StageID{fp.StagePlan, fp.StageBuild, fp.StageReview}
	p := fp.Pipeline{SessionID: "s1", SaveVersion: true, Plan: &fp.Plan{Goal: "A page", Stages: ids}}
	for i, id := range ids {
		p.Stages = append(p.Stages, fp.Stage{ID: id, Kind: id, Status: statuses[i]})
		if p.Current == "" && statuses[i] != fp.StatusDone {
			p.Current = id
		}
	}
	if p.Current == "" {
		p.Current = fp.StageReview
	}
	return p
}

func card(t *testing.T, kind, state string, stage fp.StageID, payload any) fp.Card {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return fp.Card{ID: kind + "-1", Kind: kind, Stage: stage, State: state, Payload: raw}
}

func TestBoardOf(t *testing.T) {
	t0 := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	later := t0.Add(time.Hour)
	resetsAt := t0.Add(5 * time.Hour)
	ready := Project{ID: "0123456789abcdef", Status: StatusReady, SessionID: "s1", CreatedAt: t0, UpdatedAt: t0}
	A, D, B, P := fp.StatusActive, fp.StatusDone, fp.StatusBlocked, fp.StatusPending

	planGate := boardPipeline(B, P, P)
	planGate.OpenGate = fp.GatePlan
	planGate.Cards = []fp.Card{{ID: "g", Kind: fp.CardGate, Stage: fp.StagePlan, State: fp.CardUnseen}}

	planning := boardPipeline(A, P, P)
	planning.Plan = nil

	question := boardPipeline(A, P, P)
	question.Plan = nil
	question.Cards = []fp.Card{{ID: "q", Kind: fp.CardDecision, Stage: fp.StagePlan, State: fp.CardUnseen}}

	building := boardPipeline(D, A, P)
	building.GoalMode = &fp.GoalMode{Enabled: true}
	building.UpdatedAt = later
	// Goal mode answers its own cards: an undecided one is not the person's.
	building.Cards = []fp.Card{{ID: "q", Kind: fp.CardDecision, Stage: fp.StageBuild, State: fp.CardUnseen}}

	limit := boardPipeline(D, B, P)
	limit.OpenGate = fp.GateBlocked
	limit.GoalMode = &fp.GoalMode{EndReason: fp.GoalEndUsageLimit}
	limit.Cards = []fp.Card{card(t, fp.CardGate, fp.CardUnseen, fp.StageBuild, fp.BlockedFact{
		Reason: fp.BlockedTurnFailed, UsageLimit: true, Resets: "2:40pm (Asia/Seoul)", ResetsAt: &resetsAt})}

	oldLimit := boardPipeline(D, B, P)
	oldLimit.OpenGate = fp.GateBlocked
	oldLimit.Cards = []fp.Card{card(t, fp.CardGate, fp.CardUnseen, fp.StageBuild, fp.BlockedFact{
		Reason: fp.BlockedTurnFailed, Error: "Claude AI usage limit reached, resets 2:40pm (Asia/Seoul)"})}

	failedTurn := boardPipeline(D, B, P)
	failedTurn.OpenGate = fp.GateBlocked
	failedTurn.Cards = []fp.Card{
		card(t, fp.CardFailure, fp.CardUnseen, fp.StageReview, fp.FailureFact{Check: &fp.CheckReport{Screen: "home", Expected: "a button", Seen: "nothing"}}),
		card(t, fp.CardGate, fp.CardUnseen, fp.StageBuild, fp.BlockedFact{Reason: fp.BlockedTurnFailed, Error: "boom"}),
	}

	stopped := boardPipeline(D, B, P)

	goalEnded := boardPipeline(D, D, A)
	goalEnded.GoalMode = &fp.GoalMode{EndReason: fp.GoalEndExhausted}

	finished := boardPipeline(D, D, D)
	finished.FinishedAt = &later
	finished.Saved = &fp.SavedVersion{Commit: "abc", At: later}
	finished.Cards = []fp.Card{card(t, fp.CardFailure, fp.CardDecided, fp.StageReview, fp.FailureFact{Check: &fp.CheckReport{Expected: "x", Seen: "y"}})}

	saving := boardPipeline(D, D, D)
	saving.FinishedAt = &later

	saveFailed := boardPipeline(D, D, D)
	saveFailed.FinishedAt = &later
	saveFailed.Saved = &fp.SavedVersion{Error: fp.SaveFailed}

	tests := []struct {
		name    string
		project Project
		pipe    *fp.Pipeline
		want    Board
	}{
		{"creating", Project{Status: StatusCreating, CreatedAt: t0}, nil, Board{State: StateStarting}},
		{"failed", Project{Status: StatusFailed, ErrorCode: ErrorScaffoldFailed, CreatedAt: t0}, nil, Board{State: StateFailed}},
		{"ready without a session", Project{Status: StatusReady, CreatedAt: t0}, nil, Board{State: StateStarting}},
		{"pipeline gone (session deleted)", ready, nil, Board{State: StateDone}},
		{"leader writing the plan", ready, &planning, Board{State: StateWorking}},
		{"plan to approve", ready, &planGate, Board{State: StateYourTurn, Waiting: WaitingPlan}},
		{"question before the plan", ready, &question, Board{State: StateYourTurn, Waiting: WaitingQuestion}},
		{"team building in goal mode", ready, &building, Board{State: StateWorking}},
		{"usage limit", ready, &limit, Board{State: StatePaused, Resets: "2:40pm (Asia/Seoul)", ResetsAt: &resetsAt}},
		{"usage limit read from an old error", ready, &oldLimit, Board{State: StatePaused, Resets: "2:40pm (Asia/Seoul)"}},
		{"failed turn", ready, &failedTurn, Board{State: StateStuck, Stuck: StuckBlocked, Check: &fp.CheckReport{Screen: "home", Expected: "a button", Seen: "nothing"}}},
		{"stopped by the person", ready, &stopped, Board{State: StateStuck, Stuck: StuckStopped}},
		{"goal mode gave up", ready, &goalEnded, Board{State: StateStuck, Stuck: StuckGoalEnded}},
		{"finished and saved", ready, &finished, Board{State: StateDone}},
		{"finished, still saving", ready, &saving, Board{State: StateWorking}},
		{"finished, saving failed", ready, &saveFailed, Board{State: StateDone, SaveFailed: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BoardOf(tt.project, tt.pipe)
			if got.State != tt.want.State || got.Waiting != tt.want.Waiting || got.Stuck != tt.want.Stuck ||
				got.Resets != tt.want.Resets || got.SaveFailed != tt.want.SaveFailed {
				t.Fatalf("BoardOf = %+v, want %+v", got, tt.want)
			}
			if (got.ResetsAt == nil) != (tt.want.ResetsAt == nil) || (got.ResetsAt != nil && !got.ResetsAt.Equal(*tt.want.ResetsAt)) {
				t.Fatalf("resets_at = %v, want %v", got.ResetsAt, tt.want.ResetsAt)
			}
			if (got.Check == nil) != (tt.want.Check == nil) || (got.Check != nil && *got.Check != *tt.want.Check) {
				t.Fatalf("check = %+v, want %+v", got.Check, tt.want.Check)
			}
			if got.State == StatePublished {
				t.Fatal("published is reserved")
			}
		})
	}
}

func TestBoardChangedAtIsTheLatestChange(t *testing.T) {
	t0 := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	p := Project{Status: StatusReady, SessionID: "s1", CreatedAt: t0, UpdatedAt: t0.Add(time.Minute)}
	if got := BoardOf(p, nil).ChangedAt; !got.Equal(t0.Add(time.Minute)) {
		t.Fatalf("record only = %v", got)
	}
	pipe := boardPipeline(fp.StatusDone, fp.StatusActive, fp.StatusPending)
	pipe.UpdatedAt = t0.Add(time.Hour)
	if got := BoardOf(p, &pipe).ChangedAt; !got.Equal(t0.Add(time.Hour)) {
		t.Fatalf("with pipeline = %v", got)
	}
	// A project a month old with its pipeline finished is still done.
	pipe = boardPipeline(fp.StatusDone, fp.StatusDone, fp.StatusDone)
	pipe.FinishedAt, pipe.Saved = &t0, &fp.SavedVersion{Commit: "abc"}
	if got := BoardOf(p, &pipe); got.State != StateDone {
		t.Fatalf("old finished project = %+v", got)
	}
}
