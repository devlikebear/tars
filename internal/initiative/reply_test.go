package initiative

import (
	"testing"
	"time"
)

func TestResolveReplies(t *testing.T) {
	delivered := at(9, 0)
	tests := []struct {
		name           string
		pending        []PendingReply
		mainUserAt     time.Time
		textStatus     TextReadStatus // zero value is TextReadOK
		quietRequested bool
		now            time.Time
		replyWindow    time.Duration
		wantResolved   []ReplyResolution
		wantRemaining  int
	}{
		{
			name:          "no reply yet, window still open",
			pending:       []PendingReply{{EntryID: "e1", At: delivered}},
			now:           delivered.Add(10 * time.Minute),
			replyWindow:   30 * time.Minute,
			wantRemaining: 1,
		},
		{
			name:         "reply within window, no quiet request: replied",
			pending:      []PendingReply{{EntryID: "e1", At: delivered}},
			mainUserAt:   delivered.Add(5 * time.Minute),
			now:          delivered.Add(5 * time.Minute),
			replyWindow:  30 * time.Minute,
			wantResolved: []ReplyResolution{{EntryID: "e1", Outcome: OutcomeReplied}},
		},
		{
			name:           "reply within window asking for quiet: stopped overrides replied",
			pending:        []PendingReply{{EntryID: "e1", At: delivered}},
			mainUserAt:     delivered.Add(5 * time.Minute),
			quietRequested: true,
			now:            delivered.Add(5 * time.Minute),
			replyWindow:    30 * time.Minute,
			wantResolved:   []ReplyResolution{{EntryID: "e1", Outcome: OutcomeStopped}},
		},
		{
			name:          "window elapses with no reply: ignored",
			pending:       []PendingReply{{EntryID: "e1", At: delivered}},
			now:           delivered.Add(30 * time.Minute),
			replyWindow:   30 * time.Minute,
			wantResolved:  []ReplyResolution{{EntryID: "e1", Outcome: OutcomeIgnored}},
			wantRemaining: 0,
		},
		{
			name:          "a reply arriving after the window already elapsed is ignored, not a late reply",
			pending:       []PendingReply{{EntryID: "e1", At: delivered}},
			mainUserAt:    delivered.Add(45 * time.Minute),
			now:           delivered.Add(45 * time.Minute),
			replyWindow:   30 * time.Minute,
			wantResolved:  []ReplyResolution{{EntryID: "e1", Outcome: OutcomeIgnored}},
			wantRemaining: 0,
		},
		{
			name:          "a main-session message from before delivery is not a reply",
			pending:       []PendingReply{{EntryID: "e1", At: delivered}},
			mainUserAt:    delivered.Add(-5 * time.Minute),
			now:           delivered.Add(10 * time.Minute),
			replyWindow:   30 * time.Minute,
			wantRemaining: 1,
		},
		{
			name: "two pending entries resolve independently",
			pending: []PendingReply{
				{EntryID: "old", At: delivered},
				{EntryID: "new", At: delivered.Add(40 * time.Minute)},
			},
			now:           delivered.Add(45 * time.Minute),
			replyWindow:   30 * time.Minute,
			wantResolved:  []ReplyResolution{{EntryID: "old", Outcome: OutcomeIgnored}},
			wantRemaining: 1,
		},
		// Review follow-up (tars#1221): a transient read failure or
		// backoff must not lock in the wrong outcome (or silently drop a
		// real "please stop") just because the LLM happened to be
		// unreachable the one tick a reply arrived.
		{
			name:          "reply within window but this tick's text read errored: stays pending",
			pending:       []PendingReply{{EntryID: "e1", At: delivered}},
			mainUserAt:    delivered.Add(5 * time.Minute),
			textStatus:    TextReadTemporarilyUnavailable,
			now:           delivered.Add(5 * time.Minute),
			replyWindow:   30 * time.Minute,
			wantRemaining: 1,
		},
		{
			name:          "reply within window, read kept failing until the window itself elapsed: replied, not ignored",
			pending:       []PendingReply{{EntryID: "e1", At: delivered}},
			mainUserAt:    delivered.Add(5 * time.Minute),
			textStatus:    TextReadTemporarilyUnavailable,
			now:           delivered.Add(35 * time.Minute),
			replyWindow:   30 * time.Minute,
			wantResolved:  []ReplyResolution{{EntryID: "e1", Outcome: OutcomeReplied}},
			wantRemaining: 0,
		},
		{
			name:          "reply within window but text cannot structurally be read: replied immediately",
			pending:       []PendingReply{{EntryID: "e1", At: delivered}},
			mainUserAt:    delivered.Add(time.Minute),
			textStatus:    TextReadUnavailable,
			now:           delivered.Add(time.Minute),
			replyWindow:   30 * time.Minute,
			wantResolved:  []ReplyResolution{{EntryID: "e1", Outcome: OutcomeReplied}},
			wantRemaining: 0,
		},
		{
			name:           "structurally unreadable never resolves to stopped, even with a stray true",
			pending:        []PendingReply{{EntryID: "e1", At: delivered}},
			mainUserAt:     delivered.Add(time.Minute),
			textStatus:     TextReadUnavailable,
			quietRequested: true,
			now:            delivered.Add(time.Minute),
			replyWindow:    30 * time.Minute,
			wantResolved:   []ReplyResolution{{EntryID: "e1", Outcome: OutcomeReplied}},
			wantRemaining:  0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, remaining := ResolveReplies(tt.pending, tt.mainUserAt, tt.textStatus, tt.quietRequested, tt.now, tt.replyWindow)
			if len(resolved) != len(tt.wantResolved) {
				t.Fatalf("resolved = %+v, want %+v", resolved, tt.wantResolved)
			}
			for i, r := range resolved {
				if r != tt.wantResolved[i] {
					t.Fatalf("resolved[%d] = %+v, want %+v", i, r, tt.wantResolved[i])
				}
			}
			if len(remaining) != tt.wantRemaining {
				t.Fatalf("remaining = %+v, want %d entries", remaining, tt.wantRemaining)
			}
		})
	}
}

// TestResolveRepliesWaitsForSuccessfulReadBeforeFinalizing is the sequence
// version of the table above (tars#1221 review): a reply arrives while
// the text backend is down, stays pending across ticks that keep
// failing, and only resolves once a tick's read actually succeeds — as
// either "stopped" or "replied" depending on that successful read's own
// quiet_requested answer, never on a guess made while the read was down.
func TestResolveRepliesWaitsForSuccessfulReadBeforeFinalizing(t *testing.T) {
	delivered := at(9, 0)
	replyAt := delivered.Add(5 * time.Minute)

	t.Run("eventually succeeds and quiet_requested is true: stopped", func(t *testing.T) {
		pending := []PendingReply{{EntryID: "e1", At: delivered}}
		resolved, remaining := ResolveReplies(pending, replyAt, TextReadTemporarilyUnavailable, false, delivered.Add(6*time.Minute), 30*time.Minute)
		if len(resolved) != 0 || len(remaining) != 1 {
			t.Fatalf("tick with a failed read = resolved:%+v remaining:%+v, want still pending", resolved, remaining)
		}
		resolved, remaining = ResolveReplies(remaining, replyAt, TextReadOK, true, delivered.Add(11*time.Minute), 30*time.Minute)
		if len(resolved) != 1 || resolved[0].Outcome != OutcomeStopped || len(remaining) != 0 {
			t.Fatalf("tick with a successful read = resolved:%+v remaining:%+v, want stopped", resolved, remaining)
		}
	})

	t.Run("eventually succeeds and quiet_requested is false: replied", func(t *testing.T) {
		pending := []PendingReply{{EntryID: "e1", At: delivered}}
		resolved, remaining := ResolveReplies(pending, replyAt, TextReadTemporarilyUnavailable, false, delivered.Add(6*time.Minute), 30*time.Minute)
		if len(resolved) != 0 || len(remaining) != 1 {
			t.Fatalf("tick with a failed read = resolved:%+v remaining:%+v, want still pending", resolved, remaining)
		}
		resolved, remaining = ResolveReplies(remaining, replyAt, TextReadOK, false, delivered.Add(11*time.Minute), 30*time.Minute)
		if len(resolved) != 1 || resolved[0].Outcome != OutcomeReplied || len(remaining) != 0 {
			t.Fatalf("tick with a successful read = resolved:%+v remaining:%+v, want replied", resolved, remaining)
		}
	})
}

// TestResolveRepliesOutcomeNeverReopened guards tars#1221's "once decided,
// never changes" rule at the point where it actually matters: once an
// entry resolves (here, as "replied" because the text backend cannot
// structurally be asked this tick, so quietRequested defaults to false
// and is never consulted), it is gone from the pending list ResolveReplies
// is given next tick — a later tick reading quiet_requested=true from the
// same or a newer message can never turn that already-recorded outcome
// into "stopped" after the fact.
func TestResolveRepliesOutcomeNeverReopened(t *testing.T) {
	delivered := at(9, 0)
	pending := []PendingReply{{EntryID: "e1", At: delivered}}
	resolved, remaining := ResolveReplies(pending, delivered.Add(time.Minute), TextReadUnavailable, false, delivered.Add(time.Minute), 30*time.Minute)
	if len(resolved) != 1 || resolved[0].Outcome != OutcomeReplied || len(remaining) != 0 {
		t.Fatalf("first resolution = %+v / %+v, want replied and nothing remaining", resolved, remaining)
	}
	// Next tick: the entry is no longer in pending (the runtime removed
	// it), so passing an empty pending list (as the runtime would) proves
	// there is nothing left to flip even though quiet_requested now reads
	// true.
	resolved2, remaining2 := ResolveReplies(nil, delivered.Add(2*time.Minute), TextReadOK, true, delivered.Add(2*time.Minute), 30*time.Minute)
	if len(resolved2) != 0 || len(remaining2) != 0 {
		t.Fatalf("second tick should resolve nothing: resolved=%+v remaining=%+v", resolved2, remaining2)
	}
}

func TestPendingReplyNeedsText(t *testing.T) {
	delivered := at(9, 0)
	pending := []PendingReply{{EntryID: "e1", At: delivered}}
	if pendingReplyNeedsText(pending, time.Time{}, 30*time.Minute) {
		t.Fatal("no main-session message yet: should not need a read")
	}
	if pendingReplyNeedsText(pending, delivered.Add(-time.Minute), 30*time.Minute) {
		t.Fatal("a message from before delivery is not a reply: should not need a read")
	}
	if !pendingReplyNeedsText(pending, delivered.Add(5*time.Minute), 30*time.Minute) {
		t.Fatal("a reply within the window should need a read to tell replied from stopped")
	}
	if pendingReplyNeedsText(pending, delivered.Add(45*time.Minute), 30*time.Minute) {
		t.Fatal("a reply past the window resolves to ignored by elapsed time alone: should not need a read")
	}
}

func TestEffectiveCooldown(t *testing.T) {
	base := 45 * time.Minute
	tests := []struct {
		consecutiveIgnored int
		want               time.Duration
	}{
		{0, 45 * time.Minute},
		{1, 90 * time.Minute},
		{2, 180 * time.Minute},
		{3, 360 * time.Minute},
		{4, 360 * time.Minute}, // capped at 8x base
		{10, 360 * time.Minute},
	}
	for _, tt := range tests {
		if got := EffectiveCooldown(base, tt.consecutiveIgnored); got != tt.want {
			t.Fatalf("EffectiveCooldown(%v, %d) = %v, want %v", base, tt.consecutiveIgnored, got, tt.want)
		}
	}
}

// TestReplyStateReplaysStreakAndPending covers historyFrom's restart path
// (tars#1221): a restart must recompute the exact same consecutive-ignored
// streak, last-stopped timestamp, and pending list live ticks would have
// produced.
func TestReplyStateReplaysStreakAndPending(t *testing.T) {
	entries := []Entry{
		{ID: "e1", At: at(9, 0), Delivery: DeliveryDelivered},
		{At: at(9, 10), Kind: entryKindOutcome, Ref: "e1", Outcome: OutcomeIgnored},
		{ID: "e2", At: at(10, 0), Delivery: DeliveryDelivered},
		{At: at(10, 10), Kind: entryKindOutcome, Ref: "e2", Outcome: OutcomeIgnored},
		{ID: "e3", At: at(11, 0), Delivery: DeliveryDelivered},
		{At: at(11, 5), Kind: entryKindOutcome, Ref: "e3", Outcome: OutcomeReplied}, // resets the streak
		{ID: "e4", At: at(12, 0), Delivery: DeliveryDelivered},
		{At: at(12, 10), Kind: entryKindOutcome, Ref: "e4", Outcome: OutcomeIgnored},
		{ID: "e5", At: at(13, 0), Delivery: DeliveryDelivered}, // never resolved: still pending
	}
	consecutiveIgnored, lastStoppedAt, pending := ReplyState(entries)
	if consecutiveIgnored != 1 {
		t.Fatalf("consecutiveIgnored = %d, want 1 (reset by e3's replied, then one more ignored)", consecutiveIgnored)
	}
	if !lastStoppedAt.IsZero() {
		t.Fatalf("lastStoppedAt = %v, want zero (no stopped outcome recorded)", lastStoppedAt)
	}
	if len(pending) != 1 || pending[0].EntryID != "e5" {
		t.Fatalf("pending = %+v, want only e5", pending)
	}
}

func TestReplyStateStoppedResetsStreakAndTracksLatest(t *testing.T) {
	entries := []Entry{
		{ID: "e1", At: at(9, 0), Delivery: DeliveryDelivered},
		{At: at(9, 10), Kind: entryKindOutcome, Ref: "e1", Outcome: OutcomeIgnored},
		{ID: "e2", At: at(10, 0), Delivery: DeliveryDelivered},
		{At: at(10, 10), Kind: entryKindOutcome, Ref: "e2", Outcome: OutcomeStopped},
	}
	consecutiveIgnored, lastStoppedAt, pending := ReplyState(entries)
	if consecutiveIgnored != 0 {
		t.Fatalf("consecutiveIgnored = %d, want 0 (stopped resets the streak)", consecutiveIgnored)
	}
	if !lastStoppedAt.Equal(at(10, 10)) {
		t.Fatalf("lastStoppedAt = %v, want %v", lastStoppedAt, at(10, 10))
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %+v, want none", pending)
	}
}

// TestHistoryFromRestoresReplyState is historyFrom's own integration check
// (as opposed to ReplyState's direct unit tests above): a restart must
// expose the same fields through History.
func TestHistoryFromRestoresReplyState(t *testing.T) {
	entries := []Entry{
		{ID: "e1", At: at(9, 0), Delivery: DeliveryDelivered},
		{At: at(9, 10), Kind: entryKindOutcome, Ref: "e1", Outcome: OutcomeIgnored},
		{ID: "e2", At: at(10, 0), Delivery: DeliveryDelivered},
	}
	h := historyFrom(entries, at(15, 0), seoul)
	if h.ConsecutiveIgnored != 1 {
		t.Fatalf("ConsecutiveIgnored = %d, want 1", h.ConsecutiveIgnored)
	}
	if len(h.PendingReplies) != 1 || h.PendingReplies[0].EntryID != "e2" {
		t.Fatalf("PendingReplies = %+v, want only e2", h.PendingReplies)
	}
}

func TestSummarizeOutcomes(t *testing.T) {
	now := at(15, 0)
	entries := []Entry{
		{At: now.Add(-20 * time.Hour), Kind: entryKindOutcome, Outcome: OutcomeIgnored}, // yesterday: Recent only
		{At: at(9, 0), Kind: entryKindOutcome, Outcome: OutcomeReplied},
		{At: at(10, 0), Kind: entryKindOutcome, Outcome: OutcomeIgnored},
		{At: at(11, 0), Kind: entryKindOutcome, Outcome: OutcomeIgnored},
		{At: at(12, 0), Kind: entryKindOutcome, Outcome: OutcomeStopped},
		{At: at(13, 0), Decision: Decision{Intent: IntentGreet, Speak: true}}, // a normal tick entry, not an outcome
	}
	sum := SummarizeOutcomes(entries, now, seoul, 45*time.Minute, 2, at(12, 0))
	if sum.Today.Replied != 1 || sum.Today.Ignored != 2 || sum.Today.Stopped != 1 {
		t.Fatalf("Today = %+v", sum.Today)
	}
	if sum.Recent.Ignored != 3 || sum.Recent.Replied != 1 || sum.Recent.Stopped != 1 {
		t.Fatalf("Recent = %+v", sum.Recent)
	}
	if sum.ConsecutiveIgnored != 2 {
		t.Fatalf("ConsecutiveIgnored = %d, want 2", sum.ConsecutiveIgnored)
	}
	if want := (180 * time.Minute).Seconds(); sum.EffectiveCooldownSeconds != want {
		t.Fatalf("EffectiveCooldownSeconds = %v, want %v", sum.EffectiveCooldownSeconds, want)
	}
	if !sum.StoppedToday {
		t.Fatal("StoppedToday = false, want true (lastStoppedAt is today)")
	}
}

func TestSummarizeOutcomesStoppedYesterdayDoesNotBlockToday(t *testing.T) {
	now := at(15, 0)
	sum := SummarizeOutcomes(nil, now, seoul, 45*time.Minute, 0, now.Add(-20*time.Hour))
	if sum.StoppedToday {
		t.Fatal("StoppedToday = true, want false (the stopped outcome was yesterday)")
	}
}
