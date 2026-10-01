package focuspipeline

import (
	"slices"
	"time"
)

// Finished reports whether a pipeline ran to its end, which makes it part
// of the next release (ADR §9 P5): no stage is still active, blocked
// (stopped or over its limit), or pending, and its last stage is done —
// merge, or, for a plan that leaves merge out, the last planned stage in
// StageOrder.
func Finished(p Pipeline) bool {
	for _, s := range p.Stages {
		switch s.Status {
		case StatusActive, StatusBlocked, StatusPending:
			return false
		}
	}
	return lastStageDone(p)
}

func lastStageDone(p Pipeline) bool {
	if s, ok := p.Stage(StageMerge); ok && s.Status == StatusDone {
		return true
	}
	if p.Plan == nil {
		return false
	}
	for i := len(StageOrder) - 1; i >= 0; i-- {
		id := StageOrder[i]
		if !slices.Contains(p.Plan.Stages, id) {
			continue
		}
		s, ok := p.Stage(id)
		return ok && s.Status == StatusDone
	}
	return false
}

// stampFinished sets FinishedAt the first time p is Finished.
func stampFinished(p Pipeline, now time.Time) Pipeline {
	if p.FinishedAt != nil || !Finished(p) {
		return p
	}
	at := now.UTC()
	p.FinishedAt = &at
	return p
}

// ReleaseTime is when a pipeline's work became releasable: FinishedAt, or
// UpdatedAt for a pipeline finished before FinishedAt existed.
func ReleaseTime(p Pipeline) time.Time {
	if p.FinishedAt != nil {
		return *p.FinishedAt
	}
	return p.UpdatedAt
}
