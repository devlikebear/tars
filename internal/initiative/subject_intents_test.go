package initiative

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestDecideSubjectIntents(t *testing.T) {
	tests := []struct {
		name   string
		g      GoSignals
		t      TextSignals
		intent Intent
		reason string
	}{
		{"reminder is delivered", GoSignals{ReminderDue: true}, TextSignals{}, IntentRemind, "reminder_due"},
		{"reminder ignores a quiet request", GoSignals{ReminderDue: true}, TextSignals{QuietRequested: true}, IntentRemind, "reminder_due"},
		{"reminder ignores quiet hours, cooldown, cap and a stop", GoSignals{ReminderDue: true, QuietHours: true, CooldownActive: true, DailyCapReached: true, StoppedToday: true}, TextSignals{}, IntentRemind, "reminder_due"},
		{"reminder waits while typing", GoSignals{ReminderDue: true, Busy: true}, TextSignals{}, IntentNone, "busy"},
		{"reminder while typing on arrival still greets", GoSignals{ReminderDue: true, Busy: true, JustArrived: true, Reachable: true}, TextSignals{}, IntentGreet, "arrived"},
		{"pending memory is asked at the desk", GoSignals{MemoryPending: true, IdleAtDesk: true, Reachable: true}, TextSignals{}, IntentConfirmMemory, "memory_pending"},
		{"pending memory beats a body expression", GoSignals{MemoryPending: true, IdleAtDesk: true, BodyAvailable: true, Reachable: true}, TextSignals{}, IntentConfirmMemory, "memory_pending"},
		{"memory asked today falls back to body", GoSignals{MemoryPending: true, MemoryAskedToday: true, IdleAtDesk: true, BodyAvailable: true, Reachable: true}, TextSignals{}, IntentBodyOnly, "idle_at_desk"},
		{"memory is not asked away from the desk", GoSignals{MemoryPending: true, Reachable: true}, TextSignals{}, IntentNone, "nothing"},
		{"memory waits for a quiet request", GoSignals{MemoryPending: true, IdleAtDesk: true, Reachable: true}, TextSignals{QuietRequested: true}, IntentNone, "quiet_requested"},
		{"memory waits for a strained check-in", GoSignals{MemoryPending: true, IdleAtDesk: true, Reachable: true}, TextSignals{UserStrained: true}, IntentCheckIn, "strained"},
		{"memory waits for cooldown", GoSignals{MemoryPending: true, IdleAtDesk: true, Reachable: true, CooldownActive: true}, TextSignals{}, IntentNone, "cooldown"},
		{"memory waits for arrival greet", GoSignals{MemoryPending: true, IdleAtDesk: true, JustArrived: true, Reachable: true}, TextSignals{}, IntentGreet, "arrived"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Decide(tt.g, tt.t)
			if d.Intent != tt.intent || d.Reason != tt.reason {
				t.Fatalf("Decide = %+v, want %s/%s", d, tt.intent, tt.reason)
			}
		})
	}
}

// eachGoSignals calls fn with every combination of GoSignals' bool fields
// except the ones named in fixed, which keep the values given.
func eachGoSignals(fixed GoSignals, skip map[string]bool, fn func(GoSignals)) {
	typ := reflect.TypeOf(fixed)
	var free []int
	for i := 0; i < typ.NumField(); i++ {
		if typ.Field(i).Type.Kind() == reflect.Bool && !skip[typ.Field(i).Name] {
			free = append(free, i)
		}
	}
	for mask := 0; mask < 1<<len(free); mask++ {
		g := fixed
		v := reflect.ValueOf(&g).Elem()
		for bit, idx := range free {
			v.Field(idx).SetBool(mask&(1<<bit) != 0)
		}
		fn(g)
	}
}

func eachTextSignals(fn func(TextSignals)) {
	for mask := 0; mask < 8; mask++ {
		fn(TextSignals{QuietRequested: mask&1 != 0, UserStrained: mask&2 != 0, SpecialDay: mask&4 != 0})
	}
}

// TestRemindCaseKeepsTextSignalsReasoning shows the remind case's position
// keeps textSignalsMatter's call-saving reasoning: on every tick it matches,
// no text combination changes the outcome (so no read happens), and when
// Busy defers it, Decide reads exactly as if no reminder were due.
func TestRemindCaseKeepsTextSignalsReasoning(t *testing.T) {
	skip := map[string]bool{"ReminderDue": true, "Busy": true}
	eachGoSignals(GoSignals{ReminderDue: true}, skip, func(g GoSignals) {
		if textSignalsMatter(g) {
			t.Fatalf("textSignalsMatter(%+v) = true, want false for a due reminder", g)
		}
		if d := Decide(g, TextSignals{QuietRequested: true}); d.Intent != IntentRemind {
			t.Fatalf("Decide(%+v) = %+v, want remind", g, d)
		}
	})
	eachGoSignals(GoSignals{ReminderDue: true, Busy: true}, skip, func(g GoSignals) {
		without := g
		without.ReminderDue = false
		if textSignalsMatter(g) != textSignalsMatter(without) {
			t.Fatalf("busy reminder changed textSignalsMatter for %+v", g)
		}
		eachTextSignals(func(ts TextSignals) {
			if got, want := Decide(g, ts), Decide(without, ts); got != want {
				t.Fatalf("Decide(%+v, %+v) = %+v, want %+v as without a reminder", g, ts, got, want)
			}
		})
	})
}

// TestConfirmMemoryCaseOnlyTakesIdleOutcomes shows confirm_memory sits after
// every text-dependent case: turning MemoryPending on only ever replaces a
// body_only or "nothing" outcome, never one a text signal produced or a
// hard gate fixed.
func TestConfirmMemoryCaseOnlyTakesIdleOutcomes(t *testing.T) {
	skip := map[string]bool{"MemoryPending": true, "ReminderDue": true}
	eachGoSignals(GoSignals{}, skip, func(g GoSignals) {
		with := g
		with.MemoryPending = true
		eachTextSignals(func(ts TextSignals) {
			before, after := Decide(g, ts), Decide(with, ts)
			if before == after {
				return
			}
			if after.Intent != IntentConfirmMemory {
				t.Fatalf("MemoryPending turned %+v into %+v for %+v/%+v", before, after, g, ts)
			}
			if before.Intent != IntentBodyOnly && before.Reason != "nothing" {
				t.Fatalf("confirm_memory replaced %+v for %+v/%+v", before, g, ts)
			}
		})
	})
}

func TestTextSignalsMatterForConfirmMemory(t *testing.T) {
	g := GoSignals{MemoryPending: true, IdleAtDesk: true, Reachable: true}
	if !textSignalsMatter(g) {
		t.Fatal("a confirm_memory tick must read text first: quiet_requested can still hold it back")
	}
}

func subjectRuntime(t *testing.T, now *time.Time, cfg Config, obs *Observation, speaker Speaker, ledger *Ledger) *Runtime {
	t.Helper()
	if ledger == nil {
		ledger = OpenLedger(filepath.Join(t.TempDir(), "ledger.jsonl"), 0)
	}
	cfg.Enabled, cfg.Mode, cfg.Location = true, ModeLive, seoul
	return NewRuntime(cfg, Dependencies{
		Observer: observerFunc(func(now time.Time) Observation { o := *obs; o.Now = now; return o }),
		Ledger:   ledger, Logger: zerolog.Nop(), Now: func() time.Time { return *now }, Speaker: speaker,
	})
}

type observerFunc func(time.Time) Observation

func (f observerFunc) Observe(_ context.Context, now time.Time) (Observation, error) {
	return f(now), nil
}

func TestRemindIsDeliveredInQuietHoursWithoutReplyTracking(t *testing.T) {
	now := at(23, 30)
	ledgerPath := filepath.Join(t.TempDir(), "ledger.jsonl")
	ledger := OpenLedger(ledgerPath, 0)
	obs := Observation{
		ConsolePresentAt: now.Add(-time.Minute), ConsoleArrivedAt: now.Add(-time.Hour),
		LastUserAt: now.Add(-20 * time.Minute),
		Reminder:   &Subject{ID: "r1", Text: "약 먹기", At: now},
	}
	speaker := &fakeSpeaker{outcomes: []SpeakOutcome{{Delivered: true}}}
	r := subjectRuntime(t, &now, Config{Remind: true}, &obs, speaker, ledger)

	e := r.RunOnce(context.Background())
	if e.Intent != IntentRemind || e.Delivery != DeliveryDelivered {
		t.Fatalf("entry = %+v, want a delivered remind in quiet hours", e)
	}
	if got := speaker.calls[0].Subject; got == nil || got.ID != "r1" {
		t.Fatalf("speak subject = %+v, want the due reminder", got)
	}
	if len(r.hist.PendingReplies) != 0 {
		t.Fatalf("pending replies = %+v, a reminder must not open a reply window", r.hist.PendingReplies)
	}

	// A second reminder right after: cooldown does not hold it, and the
	// ledger keeps both even though reason and delivery repeat.
	now = now.Add(time.Minute)
	obs.Reminder = &Subject{ID: "r2", Text: "물 마시기", At: now}
	if e := r.RunOnce(context.Background()); e.Intent != IntentRemind || e.Delivery != DeliveryDelivered {
		t.Fatalf("second entry = %+v, want another delivered remind", e)
	}
	entries, err := ledger.Recent(10)
	if err != nil {
		t.Fatal(err)
	}
	if n := CountSpokenToday(entries, IntentRemind, now, seoul); n != 2 {
		t.Fatalf("ledger remind count = %d, want 2", n)
	}
	for _, e := range entries {
		if e.Kind == entryKindOutcome {
			t.Fatalf("unexpected outcome entry %+v for a reminder", e)
		}
	}
	if _, _, pending := ReplyState(entries); len(pending) != 0 {
		t.Fatalf("ReplyState pending = %+v, want none for reminders", pending)
	}
	raw := mustReadFile(t, ledgerPath)
	for _, word := range []string{"약 먹기", "r1", "r2"} {
		if strings.Contains(raw, word) {
			t.Fatalf("ledger holds %q: %s", word, raw)
		}
	}

	snap := r.Snapshot()
	if !snap.Remind.Enabled || !snap.Remind.Pending || snap.Remind.SpokenToday != 2 {
		t.Fatalf("snapshot remind = %+v", snap.Remind)
	}
	if snap.ConfirmMemory.Enabled {
		t.Fatalf("confirm_memory reported enabled: %+v", snap.ConfirmMemory)
	}
}

// TestFixedWordsRemindSkipsComposeGates: a reminder whose text may not go
// to the speak model needs no compose call, so an exhausted daily compose
// cap or a compose backoff does not hold it back; one that composes waits.
func TestFixedWordsRemindSkipsComposeGates(t *testing.T) {
	now := at(10, 0)
	obs := Observation{
		ConsolePresentAt: now.Add(-time.Minute), ConsoleArrivedAt: now.Add(-time.Hour),
		LastUserAt: now.Add(-20 * time.Minute),
		Reminder:   &Subject{ID: "r1", Text: "stretch", At: now},
	}
	speaker := &fakeSpeaker{outcomes: []SpeakOutcome{{Delivered: true}}}
	r := subjectRuntime(t, &now, Config{Remind: true}, &obs, speaker, nil)
	r.mu.Lock()
	r.loadHistoryLocked(now)
	r.hist.SpeakCallsToday, r.hist.LastSpeakCallAt = r.cfg.DailySpeakCalls, now
	r.hist.SpeakFailCount = 3
	r.mu.Unlock()
	if e := r.RunOnce(context.Background()); e.Delivery != DeliveryDelivered {
		t.Fatalf("fixed-words reminder = %+v, want delivered past the compose gates", e)
	}
	r.deps.SpeakSendsText = true
	now = now.Add(time.Minute)
	if e := r.RunOnce(context.Background()); e.Delivery != DeliverySkipped || e.DeliveryReason != deliveryReasonBackoff {
		t.Fatalf("composing reminder = %+v, want held by the compose backoff", e)
	}
}

func TestRemindOffIgnoresQueuedReminder(t *testing.T) {
	now := at(10, 0)
	obs := Observation{Reminder: &Subject{ID: "r1", Text: "x", At: now}}
	r := subjectRuntime(t, &now, Config{}, &obs, &fakeSpeaker{}, nil)
	if e := r.RunOnce(context.Background()); e.Intent == IntentRemind || e.Signals.ReminderDue {
		t.Fatalf("entry = %+v, remind must stay off by default", e)
	}
}

func TestConfirmMemoryAskOnceAndAnswer(t *testing.T) {
	now := at(14, 0)
	ledger := OpenLedger(filepath.Join(t.TempDir(), "ledger.jsonl"), 0)
	obs := Observation{
		ConsolePresentAt: now.Add(-time.Minute), ConsoleArrivedAt: now.Add(-3 * time.Hour),
		LastUserAt:      now.Add(-30 * time.Minute),
		MemoryCandidate: &Subject{ID: "cand-1", Text: "prefers short answers"},
	}
	speaker := &fakeSpeaker{outcomes: []SpeakOutcome{{Delivered: true, Composed: true}}}
	r := subjectRuntime(t, &now, Config{ConfirmMemory: true}, &obs, speaker, ledger)

	e := r.RunOnce(context.Background())
	if e.Intent != IntentConfirmMemory || e.Delivery != DeliveryDelivered || e.Channel != ChannelConsole {
		t.Fatalf("entry = %+v, want a delivered confirm_memory on the console", e)
	}
	if got := speaker.calls[0].Subject; got == nil || got.ID != "cand-1" {
		t.Fatalf("speak subject = %+v, want the candidate", got)
	}
	if len(r.hist.PendingReplies) != 1 {
		t.Fatalf("pending replies = %+v, want the question open", r.hist.PendingReplies)
	}
	if !r.RecordAnswer(e.ID) {
		t.Fatal("RecordAnswer did not find the pending question")
	}
	if r.RecordAnswer(e.ID) {
		t.Fatal("a second answer must not resolve twice")
	}
	if r.RecordAnswer("") || (*Runtime)(nil).RecordAnswer(e.ID) {
		t.Fatal("empty id or nil runtime must not resolve")
	}
	snap := r.Snapshot()
	if snap.Outcomes.Today.Replied != 1 || !snap.ConfirmMemory.Enabled || snap.ConfirmMemory.SpokenToday != 1 {
		t.Fatalf("snapshot = %+v / %+v", snap.Outcomes, snap.ConfirmMemory)
	}

	// Past the cooldown, still the same day: not asked again.
	now = now.Add(2 * time.Hour)
	obs.ConsolePresentAt = now.Add(-time.Minute)
	obs.LastUserAt = now.Add(-30 * time.Minute)
	if e := r.RunOnce(context.Background()); e.Intent == IntentConfirmMemory || !e.Signals.MemoryAskedToday {
		t.Fatalf("entry = %+v, want no second question today", e)
	}

	// A restart rebuilds the once-a-day rule from the ledger.
	restarted := subjectRuntime(t, &now, Config{ConfirmMemory: true}, &obs, speaker, ledger)
	if e := restarted.RunOnce(context.Background()); !e.Signals.MemoryAskedToday {
		t.Fatalf("restarted entry = %+v, want MemoryAskedToday from the ledger", e)
	}
}

func TestFixedSpeech(t *testing.T) {
	tests := []struct {
		intent  Intent
		subject string
		want    string
	}{
		{IntentRemind, "약 먹기", "알려 드릴 시간이에요: 약 먹기"},
		{IntentRemind, " stretch ", "Reminder: stretch"},
		{IntentConfirmMemory, "짧은 답을 좋아함", "이걸 기억해 둘까요? 짧은 답을 좋아함"},
		{IntentConfirmMemory, "likes tea", "Should I remember this? likes tea"},
		{IntentGreet, "hi", ""},
		{IntentRemind, "  ", ""},
	}
	for _, tt := range tests {
		if got := FixedSpeech(tt.intent, tt.subject); got != tt.want {
			t.Fatalf("FixedSpeech(%s, %q) = %q, want %q", tt.intent, tt.subject, got, tt.want)
		}
	}
}

func TestSpeechMessagesIncludeSubjectOnlyWhenSendsText(t *testing.T) {
	in := ComposeInput{Intent: IntentRemind, Now: at(9, 0), Location: seoul, Subject: "call mom"}
	if user := buildSpeechMessages(in)[1].Content; strings.Contains(user, "call mom") {
		t.Fatalf("subject leaked without SendsText: %s", user)
	}
	in.SendsText = true
	user := buildSpeechMessages(in)[1].Content
	if !strings.Contains(user, `subject: "call mom"`) || !strings.Contains(user, "reminder the user asked") {
		t.Fatalf("user message = %s", user)
	}
	in.Intent = IntentConfirmMemory
	if user := buildSpeechMessages(in)[1].Content; !strings.Contains(user, "whether to keep it in memory") {
		t.Fatalf("confirm_memory guidance missing: %s", user)
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
