package initiative

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// longSessionObservation is a tick inside a three-hour run of messages that
// ended five minutes ago, at a console the user arrived at long ago: Decide
// says check_in (long_session) unless a check-in was spoken recently.
func longSessionObservation(now time.Time) Observation {
	var msgs []UserMessage
	for at := now.Add(-3*time.Hour - 10*time.Minute); !at.After(now.Add(-5 * time.Minute)); at = at.Add(20 * time.Minute) {
		msgs = append(msgs, UserMessage{At: at, Text: "x"})
	}
	last := msgs[len(msgs)-1].At
	return Observation{ConsolePresentAt: now, ConsoleArrivedAt: now.Add(-4 * time.Hour), RecentUser: msgs, LastUserAt: last}
}

func checkInRuntime(t *testing.T, ledgerPath string, now *time.Time) (*Runtime, *fakeObserver) {
	t.Helper()
	observer := &fakeObserver{}
	r := NewRuntime(Config{Enabled: true, Mode: ModeLive, Location: seoul, DailySpeakCalls: 12, DailyTextCalls: 60}, Dependencies{
		Observer: observer,
		Ledger:   OpenLedger(ledgerPath, 0),
		Logger:   zerolog.Nop(),
		Now:      func() time.Time { return *now },
		Speaker:  &fakeSpeaker{outcomes: []SpeakOutcome{{Delivered: true, Composed: true}}},
	})
	return r, observer
}

func deliveredCheckIn(t *testing.T, r *Runtime, observer *fakeObserver, now time.Time) Entry {
	t.Helper()
	observer.obs = longSessionObservation(now)
	e := r.RunOnceAt(context.Background(), now)
	if e.Intent != IntentCheckIn || e.Delivery != DeliveryDelivered {
		t.Fatalf("tick = %+v, want a delivered check_in", e)
	}
	return e
}

// TestCheckInSnoozeBringsCheckInBackAfterItEnds: "again in 20 minutes"
// replaces the three-hour check-in spacing with the snooze's end — no
// check-in until then, one again once the cooldown has also passed — and
// a restart replays it from the ledger.
func TestCheckInSnoozeBringsCheckInBackAfterItEnds(t *testing.T) {
	now := at(10, 0)
	path := filepath.Join(t.TempDir(), "ledger.jsonl")
	r, observer := checkInRuntime(t, path, &now)
	first := deliveredCheckIn(t, r, observer, now)

	now = now.Add(time.Minute)
	res, err := r.RecordCheckInAnswer(first.ID, CheckInAnswerSnooze)
	if err != nil || res.Until == nil || !res.Until.Equal(now.Add(CheckInSnooze)) {
		t.Fatalf("answer = %+v, %v", res, err)
	}
	snap := r.Snapshot()
	if snap.CheckInSnoozedUntil == nil || !snap.CheckInSnoozedUntil.Equal(*res.Until) {
		t.Fatalf("snapshot snoozed until = %v, want %v", snap.CheckInSnoozedUntil, res.Until)
	}
	if snap.Outcomes.Today.Replied != 1 {
		t.Fatalf("outcomes = %+v, want the snooze counted as a reply", snap.Outcomes)
	}

	// Inside the snooze: the check-in signal holds.
	if g := deriveGoSignals(r.cfg, longSessionObservation(now.Add(10*time.Minute)), r.hist); !g.CheckedInRecently {
		t.Fatalf("signals during snooze = %+v, want checked_in_recently", g)
	}

	// After the snooze and the 45m cooldown: a check-in again, well before
	// the usual three hours.
	now = first.At.Add(50 * time.Minute)
	observer.obs = longSessionObservation(now)
	if e := r.RunOnceAt(context.Background(), now); e.Intent != IntentCheckIn || e.Delivery != DeliveryDelivered {
		t.Fatalf("tick after snooze = %+v, want a check_in", e)
	}

	// A restart replays the snooze too.
	restarted, _ := checkInRuntime(t, path, &now)
	restarted.mu.Lock()
	restarted.loadHistoryLocked(now)
	h := restarted.hist
	restarted.mu.Unlock()
	if !h.CheckInSnoozedUntil.Equal(*res.Until) {
		t.Fatalf("replayed snooze until = %v, want %v", h.CheckInSnoozedUntil, res.Until)
	}
}

// TestCheckInWithoutSnoozeKeepsThreeHourSpacing is the control: no answer,
// the same tick 50 minutes later stays quiet.
func TestCheckInWithoutSnoozeKeepsThreeHourSpacing(t *testing.T) {
	now := at(10, 0)
	r, observer := checkInRuntime(t, filepath.Join(t.TempDir(), "ledger.jsonl"), &now)
	first := deliveredCheckIn(t, r, observer, now)
	now = first.At.Add(50 * time.Minute)
	observer.obs = longSessionObservation(now)
	if e := r.RunOnceAt(context.Background(), now); e.Speak {
		t.Fatalf("tick = %+v, want no speak inside the three-hour spacing", e)
	}
}

// TestCheckInQuietTodayStopsTheDay: the button records the same stop a
// "please stop" reply does, the check-in resolves as stopped, and a restart
// keeps it — also when the reply window had already closed.
func TestCheckInQuietTodayStopsTheDay(t *testing.T) {
	now := at(10, 0)
	path := filepath.Join(t.TempDir(), "ledger.jsonl")
	r, observer := checkInRuntime(t, path, &now)
	first := deliveredCheckIn(t, r, observer, now)

	// Let the reply window close first (ignored), then answer.
	now = now.Add(31 * time.Minute)
	observer.obs = Observation{ConsolePresentAt: now, ConsoleArrivedAt: now.Add(-5 * time.Hour)}
	r.RunOnceAt(context.Background(), now)
	if _, err := r.RecordCheckInAnswer(first.ID, CheckInAnswerQuietToday); err != nil {
		t.Fatal(err)
	}
	if snap := r.Snapshot(); !snap.Outcomes.StoppedToday {
		t.Fatalf("outcomes = %+v, want stopped today", snap.Outcomes)
	}

	now = now.Add(4 * time.Hour)
	restarted, obs2 := checkInRuntime(t, path, &now)
	obs2.obs = Observation{ConsolePresentAt: now, ConsoleArrivedAt: now, LastUserAt: now.Add(-5 * time.Hour)}
	if e := restarted.RunOnceAt(context.Background(), now); e.Reason != "stopped_today" {
		t.Fatalf("tick after restart = %+v, want stopped_today", e)
	}
}

// TestCheckInQuietTodayInsideWindowResolvesStopped and OK resolves replied.
func TestCheckInAnswersResolveTheReplyWindow(t *testing.T) {
	for _, tc := range []struct {
		action string
		want   OutcomeCounts
	}{
		{CheckInAnswerOK, OutcomeCounts{Replied: 1}},
		{CheckInAnswerQuietToday, OutcomeCounts{Stopped: 1}},
	} {
		t.Run(tc.action, func(t *testing.T) {
			now := at(10, 0)
			r, observer := checkInRuntime(t, filepath.Join(t.TempDir(), "ledger.jsonl"), &now)
			first := deliveredCheckIn(t, r, observer, now)
			now = now.Add(2 * time.Minute)
			res, err := r.RecordCheckInAnswer(first.ID, tc.action)
			if err != nil || res.Until != nil {
				t.Fatalf("answer = %+v, %v", res, err)
			}
			snap := r.Snapshot()
			if snap.Outcomes.Today != tc.want || snap.CheckInSnoozedUntil != nil {
				t.Fatalf("snapshot = %+v, want outcomes %+v and no snooze", snap.Outcomes, tc.want)
			}
		})
	}
}

func TestCheckInAnswerRejectsUnknownAction(t *testing.T) {
	now := at(10, 0)
	r, _ := checkInRuntime(t, filepath.Join(t.TempDir(), "ledger.jsonl"), &now)
	if _, err := r.RecordCheckInAnswer("init_1", "later"); !errors.Is(err, ErrUnknownCheckInAnswer) {
		t.Fatalf("err = %v, want ErrUnknownCheckInAnswer", err)
	}
	var nilRuntime *Runtime
	if _, err := nilRuntime.RecordCheckInAnswer("init_1", CheckInAnswerOK); err == nil {
		t.Fatal("nil runtime accepted an answer")
	}
}

func TestCheckedInRecently(t *testing.T) {
	base := at(10, 0)
	for _, tc := range []struct {
		name string
		h    History
		now  time.Time
		want bool
	}{
		{"never", History{}, base, false},
		{"inside spacing", History{LastCheckInAt: base}, base.Add(2 * time.Hour), true},
		{"past spacing", History{LastCheckInAt: base}, base.Add(3 * time.Hour), false},
		{"snoozed, inside", History{LastCheckInAt: base, CheckInSnoozedAt: base.Add(time.Minute), CheckInSnoozedUntil: base.Add(21 * time.Minute)}, base.Add(20 * time.Minute), true},
		{"snoozed, ended", History{LastCheckInAt: base, CheckInSnoozedAt: base.Add(time.Minute), CheckInSnoozedUntil: base.Add(21 * time.Minute)}, base.Add(22 * time.Minute), false},
		{"newer check-in than the snooze", History{LastCheckInAt: base.Add(time.Hour), CheckInSnoozedAt: base, CheckInSnoozedUntil: base.Add(20 * time.Minute)}, base.Add(90 * time.Minute), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := checkedInRecently(tc.h, tc.now); got != tc.want {
				t.Fatalf("checkedInRecently = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestAnswerLinesInDatasetAndReplay: an answer joins its check-in's record
// in the export and is not a record of its own.
func TestAnswerLinesInDatasetAndReplay(t *testing.T) {
	t0 := at(10, 0)
	until := t0.Add(21 * time.Minute)
	entries := []Entry{
		{ID: "a", At: t0, Mode: ModeLive, Decision: Decision{Intent: IntentCheckIn, Speak: true}, Delivery: DeliveryDelivered},
		{ID: "b", At: t0.Add(time.Minute), Kind: entryKindAnswer, Ref: "a", Answer: CheckInAnswerSnooze, Until: &until},
	}
	recs := BuildDataset(entries, time.Time{}, nil)
	if len(recs) != 1 || recs[0].Answer != CheckInAnswerSnooze {
		t.Fatalf("records = %+v", recs)
	}
	h := historyFrom(entries, t0.Add(2*time.Minute), seoul)
	if !h.CheckInSnoozedUntil.Equal(until) || !h.LastCheckInAt.Equal(t0) {
		t.Fatalf("history = %+v", h)
	}
}
