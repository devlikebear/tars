package focuspipeline

import (
	"testing"
	"time"
)

// withStatuses builds a pipeline whose stages have the given statuses, in
// StageOrder, and an optional plan.
func withStatuses(plan []StageID, statuses ...StageStatus) Pipeline {
	p := New("s1", "g", time.Unix(0, 0))
	for i, st := range statuses {
		p.Stages[i].Status = st
	}
	if plan != nil {
		p.Plan = &Plan{Stages: plan}
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
