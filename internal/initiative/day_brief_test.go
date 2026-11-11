package initiative

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestEveningRecapKeepsTextSignalsReasoning: the evening recap only takes
// the outcomes after it (the care check-ins, confirm_memory, body_only,
// nothing), a tick it takes reads text first, and a tick it does not take
// reads as if it were off.
func TestEveningRecapKeepsTextSignalsReasoning(t *testing.T) {
	checkCaseKeepsTextSignals(t, "RecapDue", func(g *GoSignals) { g.RecapDue = true }, IntentEveningRecap,
		map[string]bool{"long_session": true, "strained": true, "long_absence": true, "unreachable": true, "memory_pending": true, "idle_at_desk": true, "nothing": true})
}

func TestDecideEveningRecap(t *testing.T) {
	tests := []struct {
		name   string
		g      GoSignals
		t      TextSignals
		intent Intent
		reason string
	}{
		{"recap goes out", GoSignals{RecapDue: true, Reachable: true}, TextSignals{}, IntentEveningRecap, "end_of_day"},
		{"nobody to tell", GoSignals{RecapDue: true}, TextSignals{}, IntentNone, "nothing"},
		{"waits for typing", GoSignals{RecapDue: true, Reachable: true, Busy: true}, TextSignals{}, IntentNone, "busy"},
		{"quiet hours win", GoSignals{RecapDue: true, Reachable: true, QuietHours: true}, TextSignals{}, IntentNone, "quiet_hours"},
		{"quiet request wins", GoSignals{RecapDue: true, Reachable: true}, TextSignals{QuietRequested: true}, IntentNone, "quiet_requested"},
		{"a result is reported first", GoSignals{RecapDue: true, ResultReady: true, Reachable: true}, TextSignals{}, IntentShareResult, "result_ready"},
		{"recap before a strained check-in", GoSignals{RecapDue: true, Reachable: true}, TextSignals{UserStrained: true}, IntentEveningRecap, "end_of_day"},
		{"arrival greet first", GoSignals{RecapDue: true, Reachable: true, JustArrived: true}, TextSignals{}, IntentGreet, "arrived"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if d := Decide(tt.g, tt.t); d.Intent != tt.intent || d.Reason != tt.reason {
				t.Fatalf("Decide = %+v, want %s/%s", d, tt.intent, tt.reason)
			}
		})
	}
}

func TestRecapDueSignal(t *testing.T) {
	recap := &RecapFacts{Open: []WorkItem{{Title: "docs"}}}
	cfg := Config{EveningRecap: true, Location: seoul}.WithDefaults()
	if cfg.RecapWindow != "22:00-23:00" {
		t.Fatalf("default window = %q", cfg.RecapWindow)
	}
	in := Observation{Now: at(22, 30), Recap: recap}
	tests := []struct {
		name string
		cfg  Config
		obs  Observation
		hist History
		want bool
	}{
		{"due in the window", cfg, in, History{}, true},
		{"off", Config{Location: seoul}.WithDefaults(), in, History{}, false},
		{"before the window", cfg, Observation{Now: at(21, 59), Recap: recap}, History{}, false},
		{"nothing to say", cfg, Observation{Now: at(22, 30), Recap: &RecapFacts{}}, History{}, false},
		{"no facts", cfg, Observation{Now: at(22, 30)}, History{}, false},
		{"already said today", cfg, in, History{LastRecapAt: at(22, 1)}, false},
		{"said yesterday", cfg, in, History{LastRecapAt: at(22, 1).Add(-24 * time.Hour)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveGoSignals(tt.cfg, tt.obs, tt.hist).RecapDue; got != tt.want {
				t.Fatalf("RecapDue = %v, want %v", got, tt.want)
			}
		})
	}
	custom := Config{EveningRecap: true, Location: seoul, RecapWindow: "18:00-19:00"}.WithDefaults()
	if !custom.RecapWindowOpen(at(18, 15)) || custom.RecapWindowOpen(at(22, 15)) {
		t.Fatal("a configured window must replace the default")
	}
}

func TestDefaultRecapWindow(t *testing.T) {
	for quiet, want := range map[string]string{
		"23:00-07:00": "22:00-23:00",
		"00:30-06:00": "23:30-00:30",
		"bogus":       "21:00-22:00",
		"":            "21:00-22:00",
	} {
		if got := DefaultRecapWindow(quiet); got != want {
			t.Fatalf("DefaultRecapWindow(%q) = %q, want %q", quiet, got, want)
		}
	}
}

func TestMorningBrief(t *testing.T) {
	facts := MorningFacts{
		Cron: []CronRun{
			{Name: "backup", At: at(9, 0)},
			{Name: "poll", Every: 30 * time.Minute},
			{Name: "digest", At: at(18, 0)},
			{Name: "late", At: at(23, 0)},
		},
		Yesterday: DayLog{ChatTurns: 14, Sessions: 3, CronRuns: 2},
		Open:      []WorkItem{{Title: "Fix login", Stage: "review", Href: "/console/focus/s1"}},
	}
	en := MorningBrief(facts, seoul, "en")
	ko := MorningBrief(facts, seoul, "ko")
	want := []string{
		`Cron today: "backup" at 09:00, "poll" every 30m, "digest" at 18:00 and 1 more`,
		`Yesterday: 14 chat turns in 3 sessions, 2 cron runs`,
		`In progress: "Fix login" (review)`,
	}
	wantKo := []string{
		`오늘 cron: "backup" 09:00, "poll" 30m마다, "digest" 18:00 외 1건`,
		`어제: 세션 3곳에서 대화 14번, cron 실행 2번`,
		`진행 중: "Fix login" (review)`,
	}
	if len(en) != 3 || len(ko) != 3 {
		t.Fatalf("lines = %+v / %+v", en, ko)
	}
	for i := range want {
		if en[i].Text != want[i] || ko[i].Text != wantKo[i] {
			t.Fatalf("line %d = %q / %q", i, en[i].Text, ko[i].Text)
		}
		if en[i].Kind != ko[i].Kind || en[i].Href != ko[i].Href || en[i].Severity != BriefInfo {
			t.Fatalf("line %d differs between languages: %+v / %+v", i, en[i], ko[i])
		}
	}
	if en[0].Href != "/console/cron" || en[2].Href != "/console/focus/s1" {
		t.Fatalf("hrefs = %q %q", en[0].Href, en[2].Href)
	}
	if got := MorningBrief(MorningFacts{}, nil, "en"); got != nil {
		t.Fatalf("empty facts = %+v, want nil", got)
	}
	one := MorningBrief(MorningFacts{Yesterday: DayLog{ChatTurns: 1}}, nil, "en")
	if len(one) != 1 || one[0].Text != "Yesterday: 1 chat turn" {
		t.Fatalf("one turn = %+v", one)
	}
	if got := MorningBrief(MorningFacts{Yesterday: DayLog{ChatTurns: 2}}, nil, "ko"); got[0].Text != "어제: 대화 2번" {
		t.Fatalf("ko turns without sessions = %q", got[0].Text)
	}
	if got := MorningBrief(MorningFacts{Yesterday: DayLog{CronRuns: 1}}, nil, "en"); got[0].Text != "Yesterday: 1 cron run" {
		t.Fatalf("cron only = %q", got[0].Text)
	}
}

func TestEveningRecap(t *testing.T) {
	facts := RecapFacts{
		Done:   []WorkItem{{Title: "Ship docs", Href: "/console/focus/a"}, {Title: "Goal", Href: "/console/chat/b"}},
		Failed: []WorkItem{{Title: "nightly", Href: "/console/cron"}, {Title: "backup", Href: "/console/cron"}},
		Open:   []WorkItem{{Title: "Refactor", Stage: "build", Href: "/console/focus/c"}},
	}
	en := EveningRecap(facts, "en")
	ko := EveningRecap(facts, "ko-KR")
	if len(en) != 3 || len(ko) != 3 {
		t.Fatalf("lines = %+v / %+v", en, ko)
	}
	if en[0].Text != `Done today: "Ship docs", "Goal"` || en[0].Href != "/console" {
		t.Fatalf("done = %+v", en[0])
	}
	if en[1].Text != `Failed today: "nightly", "backup"` || en[1].Severity != BriefBlock || en[1].Href != "/console/cron" {
		t.Fatalf("failed = %+v", en[1])
	}
	if en[2].Text != `Still open: "Refactor" (build)` || en[2].Href != "/console/focus/c" {
		t.Fatalf("open = %+v", en[2])
	}
	if ko[0].Text != `오늘 끝낸 일: "Ship docs", "Goal"` || ko[1].Text != `오늘 실패: "nightly", "backup"` || ko[2].Text != `남은 일: "Refactor" (build)` {
		t.Fatalf("ko = %+v", ko)
	}
	if EveningRecap(RecapFacts{}, "en") != nil || !(&RecapFacts{}).Empty() || !(*RecapFacts)(nil).Empty() {
		t.Fatal("an empty recap says nothing")
	}
	speech := EveningRecapSpeech(en, false)
	if !strings.HasPrefix(speech, "Here's today's wrap-up.\nDone today:") || strings.Count(speech, "\n") != 3 {
		t.Fatalf("speech = %q", speech)
	}
	if !strings.HasPrefix(EveningRecapSpeech(ko, true), "오늘 하루 정리예요.\n") || EveningRecapSpeech(nil, true) != "" {
		t.Fatal("korean speech frame")
	}
	many := []WorkItem{{Title: "a"}, {Title: "b"}, {Title: "c"}, {Title: "d"}, {Title: "e"}}
	if got := EveningRecap(RecapFacts{Open: many}, "en")[0]; got.Text != `Still open: "a", "b", "c" and 2 more` || got.Href != "/console" {
		t.Fatalf("many = %+v", got)
	}
}

func TestWithoutSaidAndPrefersKorean(t *testing.T) {
	open := []WorkItem{{Title: "a", Href: "/console/focus/a"}, {Title: "b", Href: "/console/focus/b"}, {Title: "c"}}
	said := []BriefItem{{Href: "/console/focus/a"}}
	got := WithoutSaid(open, said)
	if len(got) != 2 || got[0].Title != "b" || got[1].Title != "c" {
		t.Fatalf("WithoutSaid = %+v", got)
	}
	if len(WithoutSaid(open, nil)) != 3 {
		t.Fatal("nothing said keeps every item")
	}
	if !PrefersKorean([]UserMessage{{Text: "hello"}, {Text: "안녕"}}, "") || PrefersKorean([]UserMessage{{Text: "안녕"}, {Text: "ok then"}}, "") {
		t.Fatal("the newest message picks the language")
	}
	if !PrefersKorean(nil, "이름: 홍길동") || PrefersKorean(nil, "") {
		t.Fatal("the profile decides with no messages")
	}
	for d, want := range map[time.Duration]string{90 * time.Minute: "1h30m", 2 * time.Hour: "2h", 10 * time.Second: "1m", 5 * time.Minute: "5m"} {
		if got := shortDuration(d); got != want {
			t.Fatalf("shortDuration(%v) = %q, want %q", d, got, want)
		}
	}
}

// TestEveningRecapRunsOnceAndNeverComposes: a recap goes out once a day
// with its facts, past the compose gates (it never composes), opens no
// reply window, never reaches the ledger as words, and a restart remembers
// it was said.
func TestEveningRecapRunsOnceAndNeverComposes(t *testing.T) {
	now := at(22, 10)
	ledgerPath := filepath.Join(t.TempDir(), "ledger.jsonl")
	ledger := OpenLedger(ledgerPath, 0)
	obs := Observation{
		ConsolePresentAt: now, ConsoleArrivedAt: now.Add(-3 * time.Hour),
		LastUserAt: now.Add(-40 * time.Minute),
		Recap:      &RecapFacts{Done: []WorkItem{{Title: "결제 리팩터링", Href: "/console/focus/s9"}}},
	}
	speaker := &fakeSpeaker{outcomes: []SpeakOutcome{{Delivered: true}, {Delivered: true}}}
	r := subjectRuntime(t, &now, Config{EveningRecap: true, MorningBrief: true}, &obs, speaker, ledger)
	r.deps.SpeakSendsText = true
	r.mu.Lock()
	r.loadHistoryLocked(now)
	r.hist.SpeakFailCount, r.hist.LastSpeakCallAt = 3, now
	r.mu.Unlock()

	e := r.RunOnce(context.Background())
	if e.Intent != IntentEveningRecap || e.Delivery != DeliveryDelivered {
		t.Fatalf("entry = %+v, want a delivered recap past the compose backoff", e)
	}
	if got := speaker.calls[0]; got.Recap == nil || got.Recap.Done[0].Title != "결제 리팩터링" || got.Subject != nil {
		t.Fatalf("speak request = %+v, want the recap facts", got)
	}
	if len(r.hist.PendingReplies) != 0 {
		t.Fatalf("pending replies = %+v, a recap opens no reply window", r.hist.PendingReplies)
	}
	now = now.Add(50 * time.Minute) // past the cooldown, still in the window
	if e := r.RunOnce(context.Background()); e.Signals.RecapDue || e.Intent == IntentEveningRecap {
		t.Fatalf("second tick = %+v, want no second recap today", e)
	}
	restarted := subjectRuntime(t, &now, Config{EveningRecap: true, MorningBrief: true}, &obs, speaker, ledger)
	if e := restarted.RunOnce(context.Background()); e.Signals.RecapDue {
		t.Fatalf("restarted = %+v, want the recap remembered from the ledger", e)
	}
	if raw := mustReadFile(t, ledgerPath); strings.Contains(raw, "결제") || strings.Contains(raw, "s9") {
		t.Fatalf("ledger holds recap words: %s", raw)
	}
	snap := r.Snapshot()
	if !snap.MorningBrief || !snap.EveningRecap.Enabled || snap.EveningRecap.Window != "22:00-23:00" || snap.EveningRecap.SpokenToday != 1 {
		t.Fatalf("snapshot = %+v / %+v", snap.MorningBrief, snap.EveningRecap)
	}
	if off := subjectRuntime(t, &now, Config{}, &obs, speaker, nil).Snapshot(); off.EveningRecap.Window != "" || off.MorningBrief {
		t.Fatalf("snapshot with both off = %+v", off.EveningRecap)
	}
}
