package initiative

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestNewCoworkerCasesKeepTextSignalsReasoning checks, over every GoSignals
// combination, that the two tars#1002 cases only ever replace the outcomes
// their position says they may, and that textSignalsMatter's call-saving
// reasoning still holds around them: a tick they take always reads text
// first (quiet_requested can still hold them back), and a tick they do not
// take reads exactly as if the signal were off.
func TestNewCoworkerCasesKeepTextSignalsReasoning(t *testing.T) {
	cases := []struct {
		field    string
		set      func(*GoSignals)
		intent   Intent
		replaces map[string]bool // reasons the new case may take over
	}{
		{"FollowUpPending", func(g *GoSignals) { g.FollowUpPending = true }, IntentFollowUp,
			map[string]bool{"arrived": true, "special_day": true}},
		{"ResultReady", func(g *GoSignals) { g.ResultReady = true }, IntentShareResult,
			map[string]bool{"end_of_day": true, "long_session": true, "strained": true, "long_absence": true, "unreachable": true, "memory_pending": true, "idle_at_desk": true, "nothing": true}},
	}
	for _, c := range cases {
		t.Run(c.field, func(t *testing.T) {
			checkCaseKeepsTextSignals(t, c.field, c.set, c.intent, c.replaces)
		})
	}
}

// checkCaseKeepsTextSignals is the exhaustive check shared by the cases
// that read Go signals only: turning field on may only replace the outcomes
// named in replaces, with intent; a tick it takes reads text first and
// quiet_requested still holds it back; a tick it does not take reads as if
// the signal were off.
func checkCaseKeepsTextSignals(t *testing.T, field string, set func(*GoSignals), intent Intent, replaces map[string]bool) {
	t.Helper()
	eachGoSignals(GoSignals{}, map[string]bool{field: true}, func(g GoSignals) {
		with := g
		set(&with)
		eachTextSignals(func(ts TextSignals) {
			before, after := Decide(g, ts), Decide(with, ts)
			if before == after {
				return
			}
			if after.Intent != intent || !replaces[before.Reason] {
				t.Fatalf("%s turned %+v into %+v for %+v/%+v", field, before, after, g, ts)
			}
		})
		if Decide(with, TextSignals{}).Intent == intent {
			if !textSignalsMatter(with) {
				t.Fatalf("a %s tick must read text first: %+v", intent, with)
			}
			if d := Decide(with, TextSignals{QuietRequested: true}); d.Speak {
				t.Fatalf("quiet_requested did not hold back %s: %+v", intent, d)
			}
			return
		}
		if textSignalsMatter(with) != textSignalsMatter(g) {
			t.Fatalf("%s changed textSignalsMatter on a tick it does not take: %+v", field, with)
		}
	})
}

func TestDecideCoworkerIntents(t *testing.T) {
	tests := []struct {
		name   string
		g      GoSignals
		t      TextSignals
		intent Intent
		reason string
	}{
		{"follow-up replaces the first greet", GoSignals{JustArrived: true, Reachable: true, FollowUpPending: true}, TextSignals{}, IntentFollowUp, "unfinished_work"},
		{"no follow-up once greeted today", GoSignals{JustArrived: true, GreetedToday: true, Reachable: true, FollowUpPending: true}, TextSignals{}, IntentNone, "nothing"},
		{"follow-up without arrival says nothing", GoSignals{Reachable: true, FollowUpPending: true}, TextSignals{}, IntentNone, "nothing"},
		{"result is reported", GoSignals{ResultReady: true, Reachable: true}, TextSignals{}, IntentShareResult, "result_ready"},
		{"result waits for typing", GoSignals{ResultReady: true, Reachable: true, Busy: true}, TextSignals{}, IntentNone, "busy"},
		{"result waits for the arrival greet", GoSignals{ResultReady: true, Reachable: true, JustArrived: true}, TextSignals{}, IntentGreet, "arrived"},
		{"result before a long-session check-in", GoSignals{ResultReady: true, Reachable: true, LongSession: true}, TextSignals{}, IntentShareResult, "result_ready"},
		{"reminder before a result", GoSignals{ResultReady: true, Reachable: true, ReminderDue: true}, TextSignals{}, IntentRemind, "reminder_due"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if d := Decide(tt.g, tt.t); d.Intent != tt.intent || d.Reason != tt.reason {
				t.Fatalf("Decide = %+v, want %s/%s", d, tt.intent, tt.reason)
			}
		})
	}
}

func TestFollowUpTakesTheDaysGreet(t *testing.T) {
	now := at(9, 0)
	ledgerPath := filepath.Join(t.TempDir(), "ledger.jsonl")
	ledger := OpenLedger(ledgerPath, 0)
	obs := Observation{
		ConsolePresentAt: now, ConsoleArrivedAt: now.Add(-time.Minute),
		LastUserAt: now.Add(-14 * time.Hour),
		FollowUp:   &Subject{ID: "", Text: `focus "결제 리팩터링": pull request #12 waiting`, At: now.Add(-15 * time.Hour)},
	}
	speaker := &fakeSpeaker{outcomes: []SpeakOutcome{{Delivered: true, Composed: true}}}
	r := subjectRuntime(t, &now, Config{FollowUp: true}, &obs, speaker, ledger)

	e := r.RunOnce(context.Background())
	if e.Intent != IntentFollowUp || e.Delivery != DeliveryDelivered {
		t.Fatalf("entry = %+v, want a delivered follow_up", e)
	}
	if got := speaker.calls[0].Subject; got == nil || !strings.Contains(got.Text, "결제") {
		t.Fatalf("speak subject = %+v, want the unfinished work", got)
	}
	if len(r.hist.PendingReplies) != 1 {
		t.Fatalf("pending replies = %+v, a follow-up question opens a reply window", r.hist.PendingReplies)
	}

	// Later the same day, a new arrival is neither greeted nor asked again.
	now = now.Add(6 * time.Hour)
	obs.ConsolePresentAt, obs.ConsoleArrivedAt = now, now.Add(-time.Minute)
	obs.LastUserAt = now.Add(-5 * time.Hour)
	if e := r.RunOnce(context.Background()); !e.Signals.GreetedToday || e.Intent == IntentFollowUp || e.Intent == IntentGreet {
		t.Fatalf("second arrival = %+v, want no greet or follow-up", e)
	}
	restarted := subjectRuntime(t, &now, Config{FollowUp: true}, &obs, speaker, ledger)
	if e := restarted.RunOnce(context.Background()); !e.Signals.GreetedToday {
		t.Fatalf("restarted entry = %+v, want GreetedToday rebuilt from the follow_up", e)
	}
	if raw := mustReadFile(t, ledgerPath); strings.Contains(raw, "결제") || strings.Contains(raw, "#12") {
		t.Fatalf("ledger holds the follow-up subject: %s", raw)
	}
	snap := restarted.Snapshot()
	if !snap.FollowUp.Enabled || !snap.FollowUp.Pending || snap.ShareResult.Enabled {
		t.Fatalf("snapshot follow_up = %+v share_result = %+v", snap.FollowUp, snap.ShareResult)
	}
}

func TestShareResultIsReportedWithoutReplyTracking(t *testing.T) {
	now := at(15, 0)
	ledgerPath := filepath.Join(t.TempDir(), "ledger.jsonl")
	ledger := OpenLedger(ledgerPath, 0)
	obs := Observation{
		ConsolePresentAt: now, ConsoleArrivedAt: now.Add(-2 * time.Hour),
		LastUserAt: now.Add(-40 * time.Minute),
		Result:     &Subject{ID: "res_1,res_2", Text: `agent run "nightly-digest" failed: rate limited`, At: now.Add(-5 * time.Minute)},
	}
	speaker := &fakeSpeaker{outcomes: []SpeakOutcome{{Delivered: true}}}
	r := subjectRuntime(t, &now, Config{ShareResult: true}, &obs, speaker, ledger)

	e := r.RunOnce(context.Background())
	if e.Intent != IntentShareResult || e.Delivery != DeliveryDelivered {
		t.Fatalf("entry = %+v, want a delivered share_result", e)
	}
	if got := speaker.calls[0].Subject; got == nil || got.ID != "res_1,res_2" {
		t.Fatalf("speak subject = %+v, want the queued results", got)
	}
	if len(r.hist.PendingReplies) != 0 {
		t.Fatalf("pending replies = %+v, a report must not open a reply window", r.hist.PendingReplies)
	}
	raw := mustReadFile(t, ledgerPath)
	for _, word := range []string{"nightly-digest", "res_1", "rate limited"} {
		if strings.Contains(raw, word) {
			t.Fatalf("ledger holds %q: %s", word, raw)
		}
	}
	snap := r.Snapshot()
	if !snap.ShareResult.Enabled || !snap.ShareResult.Pending || snap.ShareResult.SpokenToday != 1 {
		t.Fatalf("snapshot share_result = %+v", snap.ShareResult)
	}
}

func TestCoworkerIntentsOffByDefault(t *testing.T) {
	now := at(9, 0)
	obs := Observation{
		ConsolePresentAt: now, ConsoleArrivedAt: now.Add(-time.Minute), LastUserAt: now.Add(-14 * time.Hour),
		Result:   &Subject{ID: "r", Text: "x"},
		FollowUp: &Subject{Text: "y"},
	}
	r := subjectRuntime(t, &now, Config{}, &obs, &fakeSpeaker{outcomes: []SpeakOutcome{{Delivered: true, Composed: true}}}, nil)
	e := r.RunOnce(context.Background())
	if e.Signals.ResultReady || e.Signals.FollowUpPending || e.Intent != IntentGreet {
		t.Fatalf("entry = %+v, want a plain greet with both options off", e)
	}
}

func TestCoworkerFixedSpeechAndGuidance(t *testing.T) {
	if got := FixedSpeech(IntentShareResult, "cron job \"backup\" failed: disk full"); got != "Background work finished.\ncron job \"backup\" failed: disk full" {
		t.Fatalf("share_result fixed speech = %q", got)
	}
	if got := FixedSpeech(IntentShareResult, "백업 실패"); !strings.HasPrefix(got, "백그라운드 작업 결과") {
		t.Fatalf("korean share_result fixed speech = %q", got)
	}
	if got := FixedSpeech(IntentFollowUp, "session \"docs\": finish the guide"); !strings.HasSuffix(got, "Want to pick it up?") {
		t.Fatalf("follow_up fixed speech = %q", got)
	}
	if got := FixedSpeech(IntentFollowUp, "문서 마무리"); !strings.HasSuffix(got, "이어서 할까요?") {
		t.Fatalf("korean follow_up fixed speech = %q", got)
	}
	for intent, phrase := range map[Intent]string{IntentShareResult: "ran in the background", IntentFollowUp: "left unfinished on an earlier day"} {
		in := ComposeInput{Intent: intent, Now: at(9, 0), Location: seoul, Subject: "s", SendsText: true}
		if user := buildSpeechMessages(in)[1].Content; !strings.Contains(user, phrase) {
			t.Fatalf("%s guidance missing: %s", intent, user)
		}
	}
}
