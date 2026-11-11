package initiative

import (
	"errors"
	"time"
)

// Check-in answers (case-companion phase 3): a delivered check_in's bubble
// carries three buttons, and the answer is the button the user pressed —
// never a reading of their words, so no model judges it. Each answer is a
// ledger line of its own (Kind "answer", Ref = the check-in's entry id,
// Answer = the button, Until for a snooze) and historyFrom/ReplyState
// replay them, so a restart keeps a snooze and a quiet-for-today.
const (
	// CheckInAnswerOK is "I'm fine": closes the bubble and counts as a
	// reply, so a check-in the user did answer never feeds the ignored
	// streak.
	CheckInAnswerOK = "ok"
	// CheckInAnswerSnooze is "again in 20 minutes": the check-in's own
	// spacing (normally checkInSpacing, three hours) ends at the snooze's
	// end instead, so a check-in may come back from then on — the cooldown
	// and every other silence rule still apply. It counts as a reply.
	CheckInAnswerSnooze = "snooze"
	// CheckInAnswerQuietToday is "quiet for today": the same stop a
	// "please stop" reply records (GoSignals.StoppedToday for the rest of
	// the local day), and the check-in resolves as stopped.
	CheckInAnswerQuietToday = "quiet_today"
)

// CheckInSnooze is how long "again in 20 minutes" holds check-ins.
const CheckInSnooze = 20 * time.Minute

// entryKindAnswer marks a ledger line as a button answer to a delivered
// speak rather than a tick entry.
const entryKindAnswer = "answer"

// ErrUnknownCheckInAnswer is returned for an action that is none of the
// three buttons.
var ErrUnknownCheckInAnswer = errors.New("initiative: unknown check-in answer")

// CheckInAnswerResult is what RecordCheckInAnswer did.
type CheckInAnswerResult struct {
	Action string `json:"action"`
	// Until is when a snooze ends; nil for the other answers.
	Until *time.Time `json:"until,omitempty"`
}

// RecordCheckInAnswer records the user's button answer to the check-in
// entryID names (the caller checks that it is one). The answer is written
// even when the check-in's reply window already closed (a typed reply, or
// the window elapsed): a quiet-for-today or a snooze is still what the user
// asked for. While the window is still open it also resolves it — replied
// for ok and snooze, stopped for quiet_today.
func (r *Runtime) RecordCheckInAnswer(entryID, action string) (CheckInAnswerResult, error) {
	switch action {
	case CheckInAnswerOK, CheckInAnswerSnooze, CheckInAnswerQuietToday:
	default:
		return CheckInAnswerResult{}, ErrUnknownCheckInAnswer
	}
	if r == nil || entryID == "" {
		return CheckInAnswerResult{}, errors.New("initiative: not running")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.deps.Now()
	r.loadHistoryLocked(now)

	answer := Entry{ID: nextEntryID(now), At: now, Mode: r.cfg.Mode, Kind: entryKindAnswer, Ref: entryID, Answer: action}
	result := CheckInAnswerResult{Action: action}
	if action == CheckInAnswerSnooze {
		until := now.Add(CheckInSnooze)
		answer.Until = &until
		result.Until = &until
	}
	applyAnswer(&r.hist, answer)
	if err := r.deps.Ledger.Append(answer); err != nil {
		r.deps.Logger.Warn().Err(err).Msg("initiative: ledger append failed")
	}

	outcome := OutcomeReplied
	if action == CheckInAnswerQuietToday {
		outcome = OutcomeStopped
	}
	for i, p := range r.hist.PendingReplies {
		if p.EntryID != entryID {
			continue
		}
		r.hist.PendingReplies = append(r.hist.PendingReplies[:i:i], r.hist.PendingReplies[i+1:]...)
		r.applyOutcomeLocked(Entry{ID: nextEntryID(now), At: now, Mode: r.cfg.Mode, Kind: entryKindOutcome, Ref: entryID, Outcome: outcome})
		break
	}

	r.snapMu.Lock()
	r.lastStoppedAt = r.hist.LastStoppedAt
	r.snoozedUntil = r.hist.CheckInSnoozedUntil
	r.recent = append(r.recent, answer)
	if len(r.recent) > recentKept {
		r.recent = r.recent[len(r.recent)-recentKept:]
	}
	r.snapMu.Unlock()
	return result, nil
}

// applyAnswer folds one answer line into pacing state — shared by the live
// path above and historyFrom's replay.
func applyAnswer(h *History, e Entry) {
	switch e.Answer {
	case CheckInAnswerSnooze:
		if e.Until != nil && !e.At.Before(h.CheckInSnoozedAt) {
			h.CheckInSnoozedAt = e.At
			h.CheckInSnoozedUntil = *e.Until
		}
	case CheckInAnswerQuietToday:
		if e.At.After(h.LastStoppedAt) {
			h.LastStoppedAt = e.At
		}
	}
}

// checkedInRecently is GoSignals.CheckedInRecently: a check-in counted as
// spoken within checkInSpacing — unless the user snoozed after the latest
// one, in which case the snooze's end replaces that spacing.
func checkedInRecently(h History, now time.Time) bool {
	if h.LastCheckInAt.IsZero() {
		return false
	}
	if !h.CheckInSnoozedUntil.IsZero() && !h.CheckInSnoozedAt.Before(h.LastCheckInAt) {
		return now.Before(h.CheckInSnoozedUntil)
	}
	return now.Sub(h.LastCheckInAt) < checkInSpacing
}
