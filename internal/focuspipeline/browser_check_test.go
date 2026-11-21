package focuspipeline

import (
	"encoding/json"
	"strings"
	"testing"
)

// A pipeline whose desk has no scripted-browser CLI, and a template with
// no pull request stage, read exactly as they did before the feature.
func TestBrowserCheckGuidanceOnlyWhereItCanBeUsed(t *testing.T) {
	plain := New("s1", "add a cart page", t0)
	with := plain
	with.BrowserCheck = BrowserCheckCLIName

	base := Guidance(plain)
	if strings.Contains(base, "agent-browser") || strings.Contains(base, checkFailedMarker) {
		t.Fatalf("a desk without the CLI hears of it:\n%s", base)
	}
	got := Guidance(with)
	for _, want := range []string{
		"Scripted browser checks", "`agent-browser skills get core`", `"${AGENT_BROWSER:-agent-browser}"`,
		"CHECK FAILED", "screen:", "expected:", "seen:", "playwright.config",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("plan guidance lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "not on PATH here") {
		t.Errorf("a CLI on PATH needs no AGENT_BROWSER in the verify command:\n%s", got)
	}

	// Off PATH: the verify command names the CLI, the script does not.
	with.BrowserCheck = "/opt/tools/agent-browser"
	if got := Guidance(with); !strings.Contains(got, "AGENT_BROWSER='/opt/tools/agent-browser' sh <script>") ||
		!strings.Contains(got, "`/opt/tools/agent-browser skills get core`") {
		t.Errorf("off-PATH guidance:\n%s", got)
	}

	// A template without a pull request stage never hears of it.
	writing, ok := FindTemplate("", "writing")
	if !ok {
		t.Fatal("no writing template")
	}
	w := NewFromTemplate("s", "a novel", writing, t0)
	before := Guidance(w)
	w.BrowserCheck = BrowserCheckCLIName
	if after := Guidance(w); after != before {
		t.Fatalf("a template with no PR stage changed:\n%s", after)
	}

	// Nothing is stored for a desk without the CLI.
	raw, err := json.Marshal(plain)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "browser_check") {
		t.Fatalf("stored without the CLI:\n%s", raw)
	}
}

func TestBrowserCheckBuildNote(t *testing.T) {
	p := inBuild(t)
	base := Guidance(p)
	p.BrowserCheck = "./node_modules/.bin/agent-browser"
	got := Guidance(p)
	if got == base || !strings.Contains(got, "`./node_modules/.bin/agent-browser skills get core`") ||
		!strings.Contains(got, "CHECK FAILED") || !strings.Contains(got, "never by loosening the check") {
		t.Fatalf("build note:\n%s", got)
	}
	if strings.Contains(got, "Scripted browser checks:") {
		t.Fatalf("the plan paragraph is repeated in build:\n%s", got)
	}
}

func TestParseCheckReport(t *testing.T) {
	long := strings.Repeat("가", checkReportRunes+10)
	tests := []struct {
		name   string
		output string
		want   *CheckReport
	}{
		{"none", "ok\nall good\n", nil},
		{"full", "starting app\nCHECK FAILED\nscreen: Cart (http://127.0.0.1:4173/)\nexpected: total reads 30\nseen: total reads 25\ncleanup\n",
			&CheckReport{Screen: "Cart (http://127.0.0.1:4173/)", Expected: "total reads 30", Seen: "total reads 25"}},
		{"any order, any case, no screen", "  CHECK FAILED  \nSeen: nothing\nExpected: a Save button\n",
			&CheckReport{Expected: "a Save button", Seen: "nothing"}},
		{"the last block wins", "CHECK FAILED\nexpected: a\nseen: b\nretry\nCHECK FAILED\nscreen: Home\nexpected: c\nseen: d\n",
			&CheckReport{Screen: "Home", Expected: "c", Seen: "d"}},
		{"the first value of a field wins", "CHECK FAILED\nexpected: a\nexpected: z\nseen: b\n", &CheckReport{Expected: "a", Seen: "b"}},
		{"missing seen", "CHECK FAILED\nscreen: Home\nexpected: a\n", nil},
		{"fields too far below the marker", "CHECK FAILED\n1\n2\n3\n4\n5\n6\nexpected: a\nseen: b\n", nil},
		{"marker inside a sentence is not a block", "the CHECK FAILED earlier\nexpected: a\nseen: b\n", nil},
		{"a long field is clipped", "CHECK FAILED\nexpected: " + long + "\nseen: b\n",
			&CheckReport{Expected: strings.Repeat("가", checkReportRunes) + "…", Seen: "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseCheckReport(tt.output)
			switch {
			case got == nil && tt.want == nil:
			case got == nil || tt.want == nil || *got != *tt.want:
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

// A failed check that printed its report is told in those words on the
// failure card; any other failure keeps the command and exit code.
func TestFailureCardTellsACheckInPlainWords(t *testing.T) {
	report := "CHECK FAILED\nscreen: Cart page (http://127.0.0.1:4173/)\nexpected: the total reads 30\nseen: the total reads 25"
	fact := failureFact(*failed("sh checks/cart.sh", "app started\n"+report), 1)
	if fact.Check == nil || fact.Check.Screen != "Cart page (http://127.0.0.1:4173/)" {
		t.Fatalf("check = %+v", fact.Check)
	}
	want := "Check failed on Cart page (http://127.0.0.1:4173/): expected the total reads 30, but saw the total reads 25"
	if got := failureTitle(fact); got != want {
		t.Fatalf("title = %q", got)
	}
	if !strings.Contains(fact.Excerpt, "seen: the total reads 25") || fact.Command != "sh checks/cart.sh" || fact.ExitCode != 2 {
		t.Fatalf("the facts under the words are gone: %+v", fact)
	}
	if got := failureTitle(checkReportless(t)); got != "Verification failed: make test (exit 2)" {
		t.Fatalf("plain title = %q", got)
	}

	// The card on a real pipeline carries the same title, and the stage
	// does not advance.
	p, _ := turn(inBuild(t), t, &Report{Summary: "done", TasksDone: true})
	p, _ = verify(p, t, failed("sh checks/cart.sh", report))
	if p.Current != StageBuild {
		t.Fatalf("a failed check advanced to %s", p.Current)
	}
	var card *Card
	for i := range p.Cards {
		if p.Cards[i].Kind == CardFailure {
			card = &p.Cards[i]
		}
	}
	if card == nil || card.Title != want {
		t.Fatalf("failure card = %+v", card)
	}

	// A timed-out command and an end-to-end goal are never read as one.
	v := failed("sh checks/cart.sh", report)
	v.Results[1].TimedOut = true
	if f := failureFact(*v, 1); f.Check != nil || !strings.HasPrefix(failureTitle(f), "Verification timed out") {
		t.Fatalf("timed out: %+v", f)
	}
	v = failed("@Browser http://localhost/ cart", report)
	v.Results[1].E2E = true
	if f := failureFact(*v, 1); f.Check != nil {
		t.Fatalf("e2e: %+v", f)
	}
}

func checkReportless(t *testing.T) FailureFact {
	t.Helper()
	return failureFact(*failed("make test", "--- FAIL: TestX"), 1)
}
