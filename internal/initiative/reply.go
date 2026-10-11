package initiative

import "time"

// Outcome values for a reply-tracking outcome Entry (Kind ==
// entryKindOutcome, tars#1221). Judged purely from timestamps and the
// already-read quiet_requested text signal — never a new keyword match and
// never a new LLM call made just to classify the outcome (tars#998's rule
// against asking a model a combined judgment applies here too).
const (
	OutcomeReplied = "replied"
	OutcomeIgnored = "ignored"
	OutcomeStopped = "stopped"
)

// entryKindOutcome marks a ledger line as a reply-tracking outcome rather
// than a normal tick entry.
const entryKindOutcome = "outcome"

// replyCooldownCapMultiplier bounds how far a run of consecutive "ignored"
// outcomes can stretch the cooldown (tars#1221): doubled per consecutive
// ignored reply, capped at 8x the configured base.
const replyCooldownCapMultiplier = 8

// PendingReply is one delivered speak awaiting a reply-tracking outcome:
// its entry ID (so the eventual outcome Entry can reference it via Ref)
// and the moment it was delivered (the reply window's start).
type PendingReply struct {
	EntryID string
	At      time.Time
}

// ReplyResolution is one pending reply's outcome, decided by
// ResolveReplies for this tick.
type ReplyResolution struct {
	EntryID string
	Outcome string
}

// TextReadStatus classifies what this tick's text-signal read actually
// achieved, so ResolveReplies can tell "we don't know yet" from "we will
// never know" when a reply arrives within the window but quiet_requested
// could not be read this very tick (tars#1221 review): a transient read
// failure or backoff must not lock in the wrong outcome (or lose a real
// "please stop" forever), but a structurally unreadable backend must not
// leave the reply pending forever either.
type TextReadStatus int

const (
	// TextReadOK means a text-signal read actually happened this tick
	// (fresh or a cache hit) and succeeded — quietRequested reflects its
	// real answer, so a reply within the window can be finalized now.
	TextReadOK TextReadStatus = iota
	// TextReadTemporarilyUnavailable means this tick could not read
	// (backend error, or skipped under the existing read-failure backoff)
	// but a later tick might succeed — a reply within the window stays
	// pending until either a read succeeds or the window itself elapses
	// with no successful read (at which point it finalizes as replied,
	// never ignored: a reply did arrive).
	TextReadTemporarilyUnavailable
	// TextReadUnavailable means the text-signal backend cannot be asked
	// about this reply at all (user text is not sent to it this tick —
	// not sendsText, or the daily call cap is exhausted) — waiting would
	// never produce an answer, so a reply within the window resolves to
	// "replied" immediately.
	TextReadUnavailable
)

// ResolveReplies decides, for each pending delivered speak (oldest first),
// whether this tick resolves it. A main-session user message newer than
// the entry, within replyWindow, is a reply: when textStatus is
// TextReadOK, it resolves to "replied" (or "stopped" when quietRequested —
// this tick's already-read text signal — is true); when
// TextReadUnavailable, it resolves to "replied" right away (waiting would
// never read quiet_requested); when TextReadTemporarilyUnavailable, it
// stays pending so a later tick's successful read can still decide
// replied vs. stopped — unless replyWindow has itself elapsed by now, in
// which case it finalizes as "replied" (a reply did arrive; the window
// contract only ever turns an unanswered tick into "ignored"). No reply
// within replyWindow at all, once replyWindow elapses, resolves to
// "ignored". Anything not yet resolved is returned unchanged in
// remaining, in the same order. Pure — no I/O, no randomness, so every
// branch is table-tested.
//
// A reply that arrives after replyWindow has already elapsed is judged
// "ignored", not a late "replied": the reply-tracking contract is "within
// the window".
func ResolveReplies(pending []PendingReply, mainUserAt time.Time, textStatus TextReadStatus, quietRequested bool, now time.Time, replyWindow time.Duration) (resolved []ReplyResolution, remaining []PendingReply) {
	for _, p := range pending {
		withinWindow := !mainUserAt.IsZero() && mainUserAt.After(p.At) && mainUserAt.Sub(p.At) <= replyWindow
		windowElapsed := now.Sub(p.At) >= replyWindow
		switch {
		case withinWindow && textStatus == TextReadOK:
			outcome := OutcomeReplied
			if quietRequested {
				outcome = OutcomeStopped
			}
			resolved = append(resolved, ReplyResolution{EntryID: p.EntryID, Outcome: outcome})
		case withinWindow && textStatus == TextReadUnavailable:
			resolved = append(resolved, ReplyResolution{EntryID: p.EntryID, Outcome: OutcomeReplied})
		case withinWindow && windowElapsed:
			// textStatus == TextReadTemporarilyUnavailable here (the two
			// cases above already matched otherwise): never read
			// quiet_requested in time, but a reply did arrive — replied,
			// not ignored.
			resolved = append(resolved, ReplyResolution{EntryID: p.EntryID, Outcome: OutcomeReplied})
		case withinWindow:
			// textStatus == TextReadTemporarilyUnavailable, window still
			// open: wait for a later tick's read.
			remaining = append(remaining, p)
		case windowElapsed:
			resolved = append(resolved, ReplyResolution{EntryID: p.EntryID, Outcome: OutcomeIgnored})
		default:
			remaining = append(remaining, p)
		}
	}
	return resolved, remaining
}

// pendingReplyNeedsText reports whether some pending reply would resolve
// to replied/stopped this tick (a main-session message arrived within its
// window) — the one extra reason (on top of textSignalsMatter) a tick
// calls the text-signal backend: the quiet_requested reading of that very
// reply is what tells replied from stopped apart (tars#1221). It does not
// duplicate the "ignored" branch of ResolveReplies, which needs no text
// signal at all.
func pendingReplyNeedsText(pending []PendingReply, mainUserAt time.Time, replyWindow time.Duration) bool {
	if mainUserAt.IsZero() {
		return false
	}
	for _, p := range pending {
		if mainUserAt.After(p.At) && mainUserAt.Sub(p.At) <= replyWindow {
			return true
		}
	}
	return false
}

// EffectiveCooldown returns the cooldown actually in force given a base
// cooldown and the current consecutive-ignored streak (tars#1221): doubled
// per consecutive ignored reply, capped at replyCooldownCapMultiplier
// times base. A streak of zero (never ignored in a row, or reset by a
// replied/stopped outcome) returns base unchanged.
func EffectiveCooldown(base time.Duration, consecutiveIgnored int) time.Duration {
	if consecutiveIgnored <= 0 || base <= 0 {
		return base
	}
	mult := 1
	for i := 0; i < consecutiveIgnored && mult < replyCooldownCapMultiplier; i++ {
		mult *= 2
	}
	return base * time.Duration(mult)
}

// ReplyState replays the reply-tracking portion of history from a list of
// entries (oldest first): the consecutive-ignored streak, the most recent
// "stopped" outcome's timestamp, and every delivered speak not yet
// resolved to an outcome, in the order given. Shared by historyFrom (a
// live runtime restart) and any standalone reader of the same ledger that
// needs the same pacing facts without the live runtime itself — `tars
// doctor` (tars#1221).
func ReplyState(entries []Entry) (consecutiveIgnored int, lastStoppedAt time.Time, pending []PendingReply) {
	open := map[string]PendingReply{}
	var order []string
	for _, e := range entries {
		switch {
		case e.Kind == entryKindOutcome:
			if _, ok := open[e.Ref]; !ok {
				continue
			}
			delete(open, e.Ref)
			switch e.Outcome {
			case OutcomeReplied, OutcomeStopped:
				consecutiveIgnored = 0
			case OutcomeIgnored:
				consecutiveIgnored++
			}
			if e.Outcome == OutcomeStopped && e.At.After(lastStoppedAt) {
				lastStoppedAt = e.At
			}
		case e.Delivery == DeliveryDelivered:
			open[e.ID] = PendingReply{EntryID: e.ID, At: e.At}
			order = append(order, e.ID)
		}
	}
	for _, id := range order {
		if p, ok := open[id]; ok {
			pending = append(pending, p)
		}
	}
	return consecutiveIgnored, lastStoppedAt, pending
}

// OutcomeCounts tallies how many delivered speaks resolved to each result.
type OutcomeCounts struct {
	Replied int `json:"replied"`
	Ignored int `json:"ignored"`
	Stopped int `json:"stopped"`
}

func (c *OutcomeCounts) add(outcome string) {
	switch outcome {
	case OutcomeReplied:
		c.Replied++
	case OutcomeIgnored:
		c.Ignored++
	case OutcomeStopped:
		c.Stopped++
	}
}

// OutcomeSummary is the reply-tracking aggregate GET /v1/initiative/status
// and `tars doctor` both show — only counts, the currently-applied
// cooldown, and a boolean, never the composed words themselves (tars#1003
// applies here too).
type OutcomeSummary struct {
	Today  OutcomeCounts `json:"today"`
	Recent OutcomeCounts `json:"recent"`
	// ConsecutiveIgnored and EffectiveCooldownSeconds are the authoritative
	// pacing state's current values (not re-derived from entries) when the
	// caller has them (the live runtime); otherwise a fresh ReplyState
	// replay of the same entries (doctor).
	ConsecutiveIgnored       int     `json:"consecutive_ignored"`
	EffectiveCooldownSeconds float64 `json:"effective_cooldown_seconds"`
	StoppedToday             bool    `json:"stopped_today"`
}

// SummarizeOutcomes tallies outcome entries (Kind == entryKindOutcome) from
// entries into Today/Recent counts, and reports the cooldown multiplier
// currently in force given consecutiveIgnored and whether lastStoppedAt
// falls on the same local day as now. entries need not be exhaustive —
// the live runtime passes its bounded in-memory recent window, doctor
// passes a fresh ledger read — but consecutiveIgnored/lastStoppedAt should
// be the authoritative pacing values when the caller has them. One
// function, shared by both (tars#1221).
func SummarizeOutcomes(entries []Entry, now time.Time, loc *time.Location, baseCooldown time.Duration, consecutiveIgnored int, lastStoppedAt time.Time) OutcomeSummary {
	if loc == nil {
		loc = time.UTC
	}
	var sum OutcomeSummary
	sum.ConsecutiveIgnored = consecutiveIgnored
	sum.EffectiveCooldownSeconds = EffectiveCooldown(baseCooldown, consecutiveIgnored).Seconds()
	sum.StoppedToday = !lastStoppedAt.IsZero() && sameLocalDay(lastStoppedAt, now, loc)
	for _, e := range entries {
		if e.Kind != entryKindOutcome {
			continue
		}
		sum.Recent.add(e.Outcome)
		if sameLocalDay(e.At, now, loc) {
			sum.Today.add(e.Outcome)
		}
	}
	return sum
}

func sameLocalDay(a, b time.Time, loc *time.Location) bool {
	if loc == nil {
		loc = time.UTC
	}
	return a.In(loc).Format("2006-01-02") == b.In(loc).Format("2006-01-02")
}
