package focuspipeline

import (
	"fmt"
	"slices"
	"strings"
)

// Waiting is what a pipeline waits on its developer for, as the phone's
// alert reads it (docs/decisions/mobile-remote.md §7): the open gate and the
// decision and finding cards nobody has decided. Only Go facts — the alert
// never asks a model whether something is worth a ping.
type Waiting struct {
	Gate string
	// Cards are the ids of the undecided decision and finding cards, in deck
	// order. Gate cards are left out: the open gate already says it.
	Cards []string
	// titles maps a card id to its title, for the message.
	titles map[string]string
}

// WaitingOf reads what p waits on its developer for. A pipeline in goal mode
// waits on nobody — its watcher answers gates and cards by itself within
// seconds — and a finished pipeline waits on nothing.
func WaitingOf(p Pipeline) Waiting {
	w := Waiting{titles: map[string]string{}}
	if p.GoalActive() || p.FinishedAt != nil {
		return w
	}
	w.Gate = p.OpenGate
	for _, c := range p.Cards {
		if c.State == CardDecided || (c.Kind != CardDecision && c.Kind != CardFinding) {
			continue
		}
		if s, ok := p.Stage(c.Stage); ok && (s.Status == StatusDone || s.Status == StatusSkipped) {
			continue
		}
		w.Cards = append(w.Cards, c.ID)
		w.titles[c.ID] = strings.TrimSpace(c.Title)
	}
	return w
}

// Empty reports whether nothing waits.
func (w Waiting) Empty() bool {
	return w.Gate == GateNone && len(w.Cards) == 0
}

// NewSince lists what waits in w that did not wait in prev, as short lines
// for a message: a gate that opened (or changed), then each new card.
// Something that waited in prev and still does is never listed again, so a
// pipeline left waiting alerts once, not every tick.
func (w Waiting) NewSince(prev Waiting) []string {
	var out []string
	if w.Gate != GateNone && w.Gate != prev.Gate {
		out = append(out, gateLine(w.Gate))
	}
	for _, id := range w.Cards {
		if slices.Contains(prev.Cards, id) {
			continue
		}
		title := w.titles[id]
		if title == "" {
			title = "A card waits for a decision"
		}
		out = append(out, title)
	}
	return out
}

// gateLine names an open gate for the developer.
func gateLine(gate string) string {
	switch gate {
	case GatePlan:
		return "Plan ready for approval (G1)"
	case GateTriage:
		return "Findings to triage"
	case GatePR:
		return "Ready to open a pull request (G3)"
	case GateMerge:
		return "Ready to merge (G4)"
	case GateBlocked:
		return "Blocked: needs a decision"
	}
	return fmt.Sprintf("Gate %q is open", gate)
}
