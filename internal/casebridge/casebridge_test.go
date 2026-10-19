package casebridge

import (
	"strings"
	"testing"
	"time"
)

var at = time.Date(2026, 10, 18, 21, 0, 0, 0, time.UTC)

func fullSnapshot() Snapshot {
	return Snapshot{
		At: at,
		Pipelines: []Pipeline{
			{SessionID: "p-wait", Title: "Fix login", Stage: "review", Gate: "G2", NeedsInput: true, UpdatedAt: at.Add(-12 * time.Minute)},
			{SessionID: "p-run", Title: "Add export", Stage: "build", GoalMode: true, UpdatedAt: at.Add(-time.Minute)},
			{SessionID: "p-done", Title: "Old work", Stage: "merge", Finished: true, UpdatedAt: at.Add(-48 * time.Hour)},
		},
		Running:   []Turn{{SessionID: "s-run", Title: "Refactor", StartedAt: at.Add(-3 * time.Minute)}},
		Approvals: []Approval{{SessionID: "s-ask", Title: "Deploy", Tool: "exec", At: at.Add(-2 * time.Minute)}, {SessionID: "s-cron", Title: "Nightly", Tool: "write_file", Queued: true, Source: "cron", At: at.Add(-time.Hour)}},
		Cron:      []CronFailure{{JobID: "c1", Name: "Backup", Error: "exit status 1\nmore", At: at.Add(-30 * time.Minute)}},
		Usage:     Usage{Known: true, TodayUSD: 1.234, WeekUSD: 8.4},
		Pulse:     []Signal{{Kind: "disk", Severity: "warning", Summary: "disk 91% full"}},
	}
}

func TestRenderNamesEverythingWithIDs(t *testing.T) {
	out := Render(fullSnapshot())
	for _, want := range []string{
		"TARS status at 2026-10-18 21:00 UTC.",
		"Waiting on the user:",
		`approval of exec in "Deploy", 2 minutes ago [session:s-ask]`,
		`unattended run from cron waits for approval of write_file in "Nightly", 1 hour ago [approvals]`,
		`focus pipeline "Fix login" waits at gate G2 (stage review) since 12 minutes ago [focus:p-wait]`,
		"Running:",
		`a turn in "Refactor", running for 3 minutes [session:s-run]`,
		`focus pipeline "Add export" at stage build, goal mode [focus:p-run]`,
		`cron job "Backup" failed 30 minutes ago: exit status 1 more [cron:c1]`,
		"warning disk: disk 91% full [pulse]",
		"Usage: today $1.23, this week $8.40 [usage]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("render is missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "Old work") {
		t.Errorf("a finished pipeline is shown:\n%s", out)
	}
	if strings.Contains(out, "Nothing is waiting") {
		t.Errorf("a busy snapshot says nothing is going on:\n%s", out)
	}
}

func TestRenderEmptyAndLimits(t *testing.T) {
	if out := Render(Snapshot{At: at}); !strings.Contains(out, "Nothing is waiting, running or failing right now.") || strings.Contains(out, "Usage:") {
		t.Fatalf("empty render = %q", out)
	}

	s := Snapshot{At: at}
	for i := 0; i < 8; i++ {
		s.Running = append(s.Running, Turn{SessionID: "s", Title: strings.Repeat("t", 200) + "\n", StartedAt: at.Add(-time.Duration(i) * time.Hour)})
	}
	out := Render(s)
	if n := strings.Count(out, "a turn in"); n != maxPerSection {
		t.Fatalf("showed %d turns, want %d", n, maxPerSection)
	}
	if !strings.Contains(out, "- and 3 more") {
		t.Fatalf("no overflow line:\n%s", out)
	}
	if strings.Contains(out, strings.Repeat("t", maxTitleBytes+1)) {
		t.Fatal("title not clipped")
	}

	big := Snapshot{At: at}
	for i := 0; i < 5; i++ {
		big.Cron = append(big.Cron, CronFailure{JobID: strings.Repeat("x", 900), Name: "n", At: at})
	}
	if out := Render(big); len(out) > MaxBytes || !strings.HasSuffix(out, "…") {
		t.Fatalf("render is %d bytes, want <= %d and clipped", len(out), MaxBytes)
	}
}

func TestSpanAndAgo(t *testing.T) {
	cases := map[time.Duration]string{
		10 * time.Second: "under a minute",
		time.Minute:      "1 minute",
		59 * time.Minute: "59 minutes",
		time.Hour:        "1 hour",
		47 * time.Hour:   "47 hours",
		72 * time.Hour:   "3 days",
	}
	for d, want := range cases {
		if got := span(d); got != want {
			t.Errorf("span(%v) = %q, want %q", d, got, want)
		}
	}
	if got := ago(at, time.Time{}); got != "at an unknown time" {
		t.Errorf("ago(zero) = %q", got)
	}
	if got := since(at, time.Time{}); got != "an unknown time" {
		t.Errorf("since(zero) = %q", got)
	}
	if got := quoted(""); got != "an untitled session" {
		t.Errorf("quoted(empty) = %q", got)
	}
	if got := quoted(`say "hi"`); got != `"say 'hi'"` {
		t.Errorf("quoted = %q", got)
	}
	// "가" is 3 bytes and "…" is 3: a 5-byte clip cannot keep a whole
	// syllable and must not split one.
	if got := clip("가나다", 5); got != "…" {
		t.Errorf("clip = %q", got)
	}
	if got := clip("가나다", 6); got != "가…" {
		t.Errorf("clip = %q", got)
	}
}

func TestSplitCards(t *testing.T) {
	text, cards := SplitCards("Two things wait.\n<case-cards>[{\"kind\":\"focus\",\"id\":\"p-wait\",\"title\":\"Fix login\",\"severity\":\"block\"}]</case-cards>")
	if text != "Two things wait." || len(cards) != 1 || cards[0].ID != "p-wait" {
		t.Fatalf("split = %q %+v", text, cards)
	}
	text, cards = SplitCards("No block here.")
	if text != "No block here." || cards != nil {
		t.Fatalf("split without a block = %q %+v", text, cards)
	}
	text, cards = SplitCards("Broken <case-cards>{not json</case-cards> end")
	if text != "Broken\nend" || cards != nil {
		t.Fatalf("broken block = %q %+v", text, cards)
	}
	text, _ = SplitCards("a <case-cards>[]</case-cards> b <case-cards>[]</case-cards>")
	if strings.Contains(text, "case-cards") {
		t.Fatalf("a second block stayed in the text: %q", text)
	}
}

func TestValidCards(t *testing.T) {
	s := fullSnapshot()
	proposed := []Card{
		{Kind: "focus", ID: "p-wait", Title: "Fix login", Sub: "waits at G2", Severity: "BLOCK", Href: "https://evil.example"},
		{Kind: "focus", ID: "p-wait", Title: "duplicate"},
		{Kind: "focus", ID: "nope", Title: "made up"},
		{Kind: "session", ID: "s-run", Title: ""},
		{Kind: "Cron", ID: "c1", Title: "Backup"},
		{Kind: "approvals", Title: "Queued approvals"},
		{Kind: "usage", Title: "Spend"},
		{Kind: "pulse", Title: "Disk"},
		{Kind: "rm -rf", ID: "x"},
	}
	got := ValidCards(proposed, s)
	if len(got) != MaxCards {
		t.Fatalf("kept %d cards, want %d: %+v", len(got), MaxCards, got)
	}
	want := []Card{
		{Kind: "focus", ID: "p-wait", Title: "Fix login", Sub: "waits at G2", Severity: "block", Href: "/console/focus/p-wait"},
		{Kind: "session", ID: "s-run", Title: "Refactor", Severity: "info", Href: "/console/chat/s-run"},
		{Kind: "cron", ID: "c1", Title: "Backup", Severity: "info", Href: "/console/cron"},
		{Kind: "approvals", Title: "Queued approvals", Severity: "info", Href: "/console/ops"},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("card %d = %+v, want %+v", i, got[i], want[i])
		}
	}

	rest := ValidCards([]Card{{Kind: "usage"}, {Kind: "pulse"}, {Kind: "session", ID: "s-ask"}, {Kind: "session", ID: "p-run"}}, s)
	hrefs := []string{}
	for _, c := range rest {
		hrefs = append(hrefs, c.Href)
	}
	if strings.Join(hrefs, " ") != "/console/analytics /console/pulse /console/chat/s-ask /console/chat/p-run" {
		t.Fatalf("hrefs = %v", hrefs)
	}

	// Screens with nothing to show are not linked.
	empty := Snapshot{At: at}
	if got := ValidCards([]Card{{Kind: "approvals"}, {Kind: "usage"}, {Kind: "pulse"}, {Kind: "focus"}}, empty); len(got) != 0 {
		t.Fatalf("empty snapshot kept %+v", got)
	}
	// A chat approval (not queued) does not make the Ops card valid.
	chatOnly := Snapshot{Approvals: []Approval{{SessionID: "s", Tool: "exec"}}}
	if got := ValidCards([]Card{{Kind: "approvals"}}, chatOnly); len(got) != 0 {
		t.Fatalf("approvals card kept without a queued approval: %+v", got)
	}
}

func TestQuestionContextFitsAndScopes(t *testing.T) {
	long := QuestionContext(View{Route: "focus", SessionID: "p-wait", Title: strings.Repeat("긴 제목 ", 100)}, ScopeScreen)
	// appendConsoleContext clips at 2000 bytes; the instructions and the
	// view must fit whole.
	if len(long) > 2000 {
		t.Fatalf("question context is %d bytes", len(long))
	}
	if !strings.Contains(long, "Answer only about what this screen shows") || !strings.Contains(long, "[p-wait]") {
		t.Fatalf("screen scope missing:\n%s", long)
	}
	all := QuestionContext(View{Route: "board"}, ScopeScreen)
	if !strings.Contains(all, "Answer about TARS as a whole.") || !strings.Contains(all, "looking at the board screen.") {
		t.Fatalf("a screen scope without a session should fall back to all:\n%s", all)
	}
	if !strings.Contains(QuestionContext(View{}, ""), "looking at the board screen.") {
		t.Fatal("an empty view should read as the board")
	}
	if got := QuestionContext(View{Route: "chat", SessionID: "s1"}, ScopeScreen); !strings.Contains(got, "looking at the chat screen, on [s1].") {
		t.Fatalf("an untitled view = %q", got)
	}
	if TurnContext("  ") != "" || TurnContext("x") != "<tars-status>\nx\n</tars-status>" {
		t.Fatal("turn context wrapping")
	}
}
