package focuspipeline

import (
	"strings"
	"time"
)

// SavedVersion is the version the server saved when a pipeline that asked
// for it (Pipeline.SaveVersion) finished: an easy-mode project has no pull
// request, so finishing is where its work becomes a version the person can
// go back to (docs/decisions/vibe-coder-identity.md, phase 2).
type SavedVersion struct {
	// Commit is the saved version's commit; when the work changed nothing
	// it is the commit that was already there.
	Commit string    `json:"commit,omitempty"`
	At     time.Time `json:"at"`
	// Error is a fixed code when saving failed ("save_failed"); the git
	// output goes to the server log only.
	Error string `json:"error,omitempty"`
}

// SaveFailed is SavedVersion.Error when the version could not be saved.
const SaveFailed = "save_failed"

// SaveDue reports whether p has just finished and still owes its version:
// it asked for one, ran to its end (never a stopped or blocked pipeline),
// and nothing was recorded yet.
func SaveDue(p Pipeline) bool {
	return p.SaveVersion && p.Saved == nil && Finished(p)
}

// RecordSaved stores how saving the finished pipeline's version went. It
// records once.
func RecordSaved(p Pipeline, saved SavedVersion, now time.Time) Pipeline {
	if p.Saved != nil {
		return p
	}
	next := p.clone()
	saved.At = now.UTC()
	next.Saved = &saved
	next.UpdatedAt = now.UTC()
	return next
}

// saveMessageRunes caps the saved version's message.
const saveMessageRunes = 72

// SaveMessage is the saved version's message: the approved plan's goal,
// else the pipeline's goal, first line, clipped.
func SaveMessage(p Pipeline) string {
	goal := p.Goal
	if p.Plan != nil && strings.TrimSpace(p.Plan.Goal) != "" {
		goal = p.Plan.Goal
	}
	line := strings.TrimSpace(strings.SplitN(strings.TrimSpace(goal), "\n", 2)[0])
	if runes := []rune(line); len(runes) > saveMessageRunes {
		line = strings.TrimSpace(string(runes[:saveMessageRunes-1])) + "…"
	}
	if line == "" {
		return "Save the finished work"
	}
	return line
}
