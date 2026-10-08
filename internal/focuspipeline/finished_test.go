package focuspipeline

import (
	"testing"
	"time"
)

// withStatuses builds a pipeline whose stages have the given statuses, in
// StageOrder, and an optional plan. The dev template's release stage, which
// StageOrder does not list, is left at New's default (pending) when plan is
// nil, and skipped otherwise, same as a real plan that left it out
// (skipUnplannedStages): these statuses only ever name StageOrder's six.
func withStatuses(plan []StageID, statuses ...StageStatus) Pipeline {
	p := New("s1", "g", time.Unix(0, 0))
	for i, st := range statuses {
		p.Stages[i].Status = st
	}
	if plan != nil {
		p.Plan = &Plan{Stages: plan}
		keep := map[StageID]bool{}
		for _, id := range plan {
			keep[id] = true
		}
		for i := len(statuses); i < len(p.Stages); i++ {
			if !keep[p.Stages[i].ID] {
				p.Stages[i].Status = StatusSkipped
			}
		}
	}
	return p
}

func TestFinished(t *testing.T) {
	all := []StageID{StagePlan, StageBuild, StageReview, StagePR, StagePRReview, StageMerge}
	local := []StageID{StagePlan, StageBuild, StageReview}
	tests := []struct {
		name string
		p    Pipeline
		want bool
	}{
		{"new pipeline", New("s1", "g", time.Unix(0, 0)), false},
		{"merged", withStatuses(all, StatusDone, StatusDone, StatusDone, StatusDone, StatusDone, StatusDone), true},
		{"merged with skipped review", withStatuses(all, StatusDone, StatusDone, StatusSkipped, StatusDone, StatusSkipped, StatusDone), true},
		{"merge active", withStatuses(all, StatusDone, StatusDone, StatusDone, StatusDone, StatusDone, StatusActive), false},
		{"merge blocked", withStatuses(all, StatusDone, StatusDone, StatusDone, StatusDone, StatusDone, StatusBlocked), false},
		{"merge pending", withStatuses(all, StatusDone, StatusDone, StatusDone, StatusDone, StatusDone, StatusPending), false},
		{"merge skipped, last planned done", withStatuses(local, StatusDone, StatusDone, StatusDone, StatusSkipped, StatusSkipped, StatusSkipped), true},
		{"merge skipped, last planned skipped", withStatuses(local, StatusDone, StatusDone, StatusSkipped, StatusSkipped, StatusSkipped, StatusSkipped), false},
		{"merge skipped, no plan", withStatuses(nil, StatusDone, StatusDone, StatusSkipped, StatusSkipped, StatusSkipped, StatusSkipped), false},
		{"stopped mid build", withStatuses(local, StatusDone, StatusSkipped, StatusSkipped, StatusSkipped, StatusSkipped, StatusSkipped), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Finished(tt.p); got != tt.want {
				t.Fatalf("Finished = %v, want %v (stages %+v)", got, tt.want, tt.p.Stages)
			}
		})
	}
}

func TestApplyStampsFinishedAt(t *testing.T) {
	t0 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	p := New("s1", "g", t0)
	p.Plan = &Plan{Stages: []StageID{StagePlan, StageBuild}}
	p.skipUnplannedStages()
	p.advance() // build is active, the last planned stage
	if p.FinishedAt != nil {
		t.Fatal("an active pipeline has no FinishedAt")
	}

	finishAt := t0.Add(time.Hour)
	done, _, err := Apply(p, Event{Kind: EventAdvance, Stage: StageBuild}, finishAt)
	if err != nil {
		t.Fatal(err)
	}
	if !Finished(done) || done.FinishedAt == nil || !done.FinishedAt.Equal(finishAt) {
		t.Fatalf("finishing stamps FinishedAt: finished=%v at=%v", Finished(done), done.FinishedAt)
	}
	if p.FinishedAt != nil {
		t.Fatal("Apply never modifies its input")
	}

	// Later changes (acknowledging a card) move UpdatedAt, never FinishedAt.
	done.Cards = append(done.Cards, Card{ID: "c1", Kind: CardReport, State: CardUnseen})
	later := finishAt.Add(24 * time.Hour)
	acked, _, err := SetCardState(done, "c1", CardSeen, "", later)
	if err != nil {
		t.Fatal(err)
	}
	if !acked.UpdatedAt.Equal(later) || !acked.FinishedAt.Equal(finishAt) {
		t.Fatalf("updated=%v finished=%v", acked.UpdatedAt, acked.FinishedAt)
	}
	// A stamped pipeline keeps its first finish time through another Apply.
	again, _, _ := Apply(acked, Event{Kind: EventTurnCompleted, Turn: 9}, later)
	if !again.FinishedAt.Equal(finishAt) {
		t.Fatalf("restamped: %v", again.FinishedAt)
	}
}

func TestReleaseTime(t *testing.T) {
	updated := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	p := New("s1", "g", updated)
	if got := ReleaseTime(p); !got.Equal(updated) {
		t.Fatalf("no FinishedAt falls back to UpdatedAt: %v", got)
	}
	finished := updated.Add(-time.Hour)
	p.FinishedAt = &finished
	if got := ReleaseTime(p); !got.Equal(finished) {
		t.Fatalf("FinishedAt wins: %v", got)
	}
}

// legacyFinished is a pipeline that finished before FinishedAt existed.
func legacyFinished(updated time.Time) Pipeline {
	p := withStatuses([]StageID{StagePlan, StageBuild, StageMerge}, StatusDone, StatusDone, StatusSkipped, StatusSkipped, StatusSkipped, StatusDone)
	p.Current = StageMerge
	p.UpdatedAt = updated
	p.Cards = []Card{{ID: "c1", Kind: CardReport, State: CardUnseen}}
	return p
}

// Item 4: the first mutation of a legacy finished pipeline stamps
// FinishedAt from the time it last changed, before the mutation moves it.
func TestStoreUpdateBackfillsFinishedAt(t *testing.T) {
	store := NewStore(t.TempDir())
	last := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	if err := store.Save(legacyFinished(last)); err != nil {
		t.Fatal(err)
	}
	later := last.Add(30 * 24 * time.Hour)
	got, _, err := store.Update("s1", func(p Pipeline) (Pipeline, error) {
		next, _, err := SetCardState(p, "c1", CardSeen, "", later)
		return next, err
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.FinishedAt == nil || !got.FinishedAt.Equal(last) || !got.UpdatedAt.Equal(later) {
		t.Fatalf("finished=%v updated=%v", got.FinishedAt, got.UpdatedAt)
	}
	saved, _, _ := store.Get("s1")
	if saved.FinishedAt == nil || !saved.FinishedAt.Equal(last) {
		t.Fatalf("saved finished=%v", saved.FinishedAt)
	}
}

func TestApplyBackfillsLegacyFinishedAt(t *testing.T) {
	last := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	p := legacyFinished(last)
	got, _, _ := Apply(p, Event{Kind: EventTurnCompleted, Turn: 3}, last.Add(time.Hour))
	if got.FinishedAt == nil || !got.FinishedAt.Equal(last) {
		t.Fatalf("finished=%v, want the pre-mutation UpdatedAt", got.FinishedAt)
	}
	if p.FinishedAt != nil {
		t.Fatal("Apply never modifies its input")
	}
}

// Item 5: only merged work is releasable.
func TestReleasable(t *testing.T) {
	all := []StageID{StagePlan, StageBuild, StageReview, StagePR, StagePRReview, StageMerge}
	local := []StageID{StagePlan, StageBuild, StageReview}
	if !Releasable(withStatuses(all, StatusDone, StatusDone, StatusDone, StatusDone, StatusDone, StatusDone)) {
		t.Fatal("merged is releasable")
	}
	noMerge := withStatuses(local, StatusDone, StatusDone, StatusDone, StatusSkipped, StatusSkipped, StatusSkipped)
	if !Finished(noMerge) || Releasable(noMerge) {
		t.Fatal("a plan that skipped merge finishes but is not releasable")
	}
	if Releasable(New("s1", "g", time.Unix(0, 0))) {
		t.Fatal("a running pipeline is not releasable")
	}
}
