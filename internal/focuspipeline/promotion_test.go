package focuspipeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestRecurrenceKey(t *testing.T) {
	cases := []struct {
		name string
		it   EvidenceItem
		want string
	}{
		{"finding", EvidenceItem{Kind: EvidenceFinding, Source: EvidenceSourceAgent, File: "frontend/console/src/i18n/sections/focus.ts", Title: "Missing ko translation for `focus.notice`"},
			"finding:frontend/console/src/i18n/sections/*.ts|missing ko translation"},
		{"finding quoted, numbers and identifiers dropped", EvidenceItem{Kind: EvidenceFinding, File: "frontend/console/src/i18n/sections/chat.ts", Title: `Missing KO translation for "chat.title" (3 keys) in chat_panel`},
			"finding:frontend/console/src/i18n/sections/*.ts|missing ko translation keys"},
		{"no file", EvidenceItem{Kind: EvidenceFinding, Title: "Error ignored"}, "finding:*|error ignored"},
		{"top-level file", EvidenceItem{Kind: EvidenceFinding, File: "Makefile", Title: "Error ignored"}, "finding:*|error ignored"},
		{"hangul kept", EvidenceItem{Kind: EvidenceFinding, File: "a/b.go", Title: "에러 무시"}, "finding:a/*.go|에러 무시"},
		{"title of nothing", EvidenceItem{Kind: EvidenceFinding, File: "a.go", Title: "`x` 42"}, ""},
		{"merge conflict", EvidenceItem{Kind: EvidenceFinding, Source: EvidenceSourcePRProbe, Title: "Merge conflict"}, ""},
		{"untrusted comment", EvidenceItem{Kind: EvidenceFinding, Source: EvidenceSourcePRComment, Title: untrustedCommentTitle}, ""},
		{"trusted comment", EvidenceItem{Kind: EvidenceFinding, Source: EvidenceSourcePRComment, File: "x.go", Title: "Close the body"}, "finding:*.go|close body"},
		{"verify", EvidenceItem{Kind: EvidenceVerifyFailure, Command: "  make   lint-diff "}, "verify:make lint-diff"},
		{"verify empty", EvidenceItem{Kind: EvidenceVerifyFailure}, ""},
		{"ci", EvidenceItem{Kind: EvidenceCIFailure, Check: "pr-diff"}, "ci:pr-diff"},
		{"ci empty", EvidenceItem{Kind: EvidenceCIFailure}, ""},
		{"unknown", EvidenceItem{Kind: "other", Title: "x"}, ""},
	}
	for _, c := range cases {
		if got := RecurrenceKey(c.it); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
	if RecurrenceKind("finding:a|b") != RecurrenceFinding || RecurrenceKind("ci:x") != RecurrenceCI {
		t.Fatal("kind")
	}
	long := normalizeFindingTitle("one two three four five six seven eight nine ten")
	if long != "one two three four five six seven eight" {
		t.Fatalf("cap: %q", long)
	}
	if got := normalizeFindingTitle("Wrong “smart quoted” value"); got != "wrong value" {
		t.Fatalf("smart quotes: %q", got)
	}
}

func promotionPipeline(id string) Pipeline {
	p := New(id, "Add export", time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
	raw, _ := json.Marshal(PRFinding{Finding: Finding{File: "web/i18n/ko.ts", Title: "Missing ko key"}, Key: "f1"})
	p.Cards = append(p.Cards,
		Card{ID: "c1", Kind: CardFinding, Title: "Missing ko key", Payload: raw, State: CardDecided, Decision: DecisionFix},
		Card{ID: "c2", Kind: CardFinding, Title: "Missing ko key", Payload: raw, State: CardDecided, Decision: DecisionFix})
	raw, _ = json.Marshal(FailureFact{Command: "make lint", ExitCode: 1})
	p.Cards = append(p.Cards, Card{ID: "c3", Kind: CardFailure, Payload: raw})
	return p
}

func TestRecurrenceHits(t *testing.T) {
	hits := RecurrenceHits(CollectHarnessEvidence(promotionPipeline("s1")))
	if len(hits) != 2 || hits[0].Key != "finding:web/i18n/*.ts|missing ko key" || hits[0].CardID != "c1" ||
		hits[1].Kind != RecurrenceVerify || hits[1].Command != "make lint" || hits[1].CardID != "c3" {
		t.Fatalf("hits = %+v", hits)
	}
}

func ledgerOf(keys ...[]string) RecurrenceLedger {
	var l RecurrenceLedger
	for i, ks := range keys {
		var hits []RecurrenceHit
		for _, k := range ks {
			hits = append(hits, RecurrenceHit{Key: k, Kind: RecurrenceKind(k)})
		}
		l = l.withPipeline(RecurrencePipeline{SessionID: fmt.Sprintf("s%d", i+1), Hits: hits})
	}
	return l
}

func TestRecurrenceCandidates(t *testing.T) {
	a, b, c := "finding:*|a", "ci:b", "verify:c"
	l := ledgerOf([]string{a, b}, []string{a}, []string{c}, []string{a, b, c}, []string{b, c})
	current := []RecurrenceHit{{Key: a, Kind: RecurrenceFinding}, {Key: b, Kind: RecurrenceCI}, {Key: c, Kind: RecurrenceVerify}, {Key: "x:new"}}
	got := RecurrenceCandidates(l, current, nil)
	if len(got) != 3 || got[0].Key != b || got[0].Count != 3 || got[1].Key != a || got[2].Key != c {
		t.Fatalf("candidates = %+v", got)
	}
	if !slices.Equal(got[1].Sessions, []string{"s1", "s2", "s4"}) || len(got[1].Hits) != 3 {
		t.Fatalf("sessions = %+v", got[1])
	}
	// Only keys of the current pipeline.
	if got := RecurrenceCandidates(l, current[:1], nil); len(got) != 1 || got[0].Key != a {
		t.Fatalf("current only = %+v", got)
	}
	// Excluded (proposed or enforced) keys are left out.
	if got := RecurrenceCandidates(l, current, map[string]bool{a: true, b: true}); len(got) != 1 || got[0].Key != c {
		t.Fatalf("exclude = %+v", got)
	}
	// A key tried at its count waits until it recurs once more.
	l.Attempts = map[string]PromotionAttempt{a: {Count: 3}}
	if got := RecurrenceCandidates(l, current, nil); len(got) != 2 {
		t.Fatalf("attempted = %+v", got)
	}
	l = l.withPipeline(RecurrencePipeline{SessionID: "s6", Hits: []RecurrenceHit{{Key: a}}})
	if got := RecurrenceCandidates(l, current, nil); len(got) != 3 || got[0].Key != a || got[0].Count != 4 {
		t.Fatalf("recurred again = %+v", got)
	}
	// The window: only the last PromotionWindow pipelines count.
	old := ledgerOf([]string{a}, []string{a}, []string{a})
	for i := 0; i < PromotionWindow; i++ {
		old = old.withPipeline(RecurrencePipeline{SessionID: fmt.Sprintf("n%d", i)})
	}
	if got := RecurrenceCandidates(old, current, nil); len(got) != 0 {
		t.Fatalf("outside the window = %+v", got)
	}
	// The cap.
	var many [][]string
	var cur []RecurrenceHit
	for i := 0; i < 3; i++ {
		var ks []string
		for k := 0; k < PromotionMaxCandidates+2; k++ {
			ks = append(ks, fmt.Sprintf("ci:k%d", k))
		}
		many = append(many, ks)
	}
	for k := 0; k < PromotionMaxCandidates+2; k++ {
		cur = append(cur, RecurrenceHit{Key: fmt.Sprintf("ci:k%d", k)})
	}
	if got := RecurrenceCandidates(ledgerOf(many...), cur, nil); len(got) != PromotionMaxCandidates {
		t.Fatalf("cap = %d", len(got))
	}
}

func TestRecurrenceLedgerKeepsAndReplaces(t *testing.T) {
	var l RecurrenceLedger
	for i := 0; i < recurrenceKeep+5; i++ {
		l = l.withPipeline(RecurrencePipeline{SessionID: fmt.Sprintf("s%d", i)})
	}
	if len(l.Pipelines) != recurrenceKeep || l.Pipelines[0].SessionID != "s5" {
		t.Fatalf("kept %d from %s", len(l.Pipelines), l.Pipelines[0].SessionID)
	}
	l = l.withPipeline(RecurrencePipeline{SessionID: "s10", Hits: []RecurrenceHit{{Key: "ci:x"}}})
	if len(l.Pipelines) != recurrenceKeep || l.Pipelines[len(l.Pipelines)-1].SessionID != "s10" || len(l.Pipelines[len(l.Pipelines)-1].Hits) != 1 {
		t.Fatal("a pipeline recorded twice is replaced, not counted twice")
	}
}

func TestHarnessLibraryRecurrenceAndChecks(t *testing.T) {
	shared := t.TempDir()
	lib, err := OpenHarnessLibrary(shared, filepath.Join(t.TempDir(), "x"))
	if err != nil {
		t.Fatal(err)
	}
	if l, err := lib.Recurrence(); err != nil || len(l.Pipelines) != 0 {
		t.Fatalf("empty ledger: %+v %v", l, err)
	}
	now := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	src := RecurrencePipeline{SessionID: "s1", Goal: "g", PRURL: "https://example.test/pr/1", RecordedAt: now,
		Hits: []RecurrenceHit{{Key: "verify:make lint", Kind: RecurrenceVerify, Command: "make lint"}}}
	l, err := lib.RecordRecurrence(src)
	if err != nil || len(l.Pipelines) != 1 {
		t.Fatalf("record: %+v %v", l, err)
	}
	cands := []RecurrenceCandidate{{Key: "verify:make lint", Count: 3}, {Key: "ci:x", Count: 4}}
	if err := lib.RecordPromotionAttempts(cands, nil, now); err != nil {
		t.Fatal(err)
	}
	if err := lib.RecordPromotionAttempts(cands, map[string]string{"ci:x": "fails_on_fix"}, now); err != nil {
		t.Fatal(err)
	}
	l, _ = lib.Recurrence()
	if a := l.Attempts["ci:x"]; a.Count != 4 || a.Reason != "fails_on_fix" || len(l.Attempts) != 1 || len(l.Pipelines) != 1 {
		t.Fatalf("attempts = %+v", l)
	}

	check := RecordCheck(RecurrenceCandidate{Key: "verify:make lint", Kind: RecurrenceVerify, Count: 3, Sessions: []string{"a", "b", "s1"},
		Hits: []RecurrenceHit{{Key: "verify:make lint", Kind: RecurrenceVerify, Command: "make lint"}}})
	script := VerifiedCheck{PromotionCheck: PromotionCheck{Key: "finding:*|x", Kind: PromotionCheckScript, Command: "sh scripts/x.sh",
		Files: []PromotionFile{{Path: "scripts/x.sh", Content: "exit 0"}}}, Count: 3, VerifiedBy: VerifiedByReplay,
		Hits: []RecurrenceHit{{Kind: RecurrenceFinding, Title: "Bad x", File: "a.go"}}}
	ids, err := lib.RecordChecks(src, "prop", []VerifiedCheck{check, script}, now)
	if err != nil || len(ids) != 2 {
		t.Fatalf("record checks: %v %v", ids, err)
	}
	all, _ := lib.CheckRecurrences(false)
	enforced, _ := lib.CheckRecurrences(true)
	if len(all) != 2 || len(enforced) != 0 {
		t.Fatalf("recurrences = %v %v", all, enforced)
	}
	_, lines, _ := lib.Known()
	if len(lines) != 2 || !strings.Contains(lines[0], "checked by the verify command `make lint`") || !strings.HasPrefix(lines[0], "[proposed] Recurring in 3 pipelines: `make lint` failed") {
		t.Fatalf("known = %v", lines)
	}
	lessons, _ := lib.Lessons()
	if lessons[0].Path != RepoVerifyFile || lessons[1].Path != "scripts/x.sh" || lessons[1].Evidence[0].Kind != EvidenceFinding || lessons[1].Lesson != "Recurring in 3 pipelines: Bad x" {
		t.Fatalf("lessons = %+v", lessons)
	}
	if n, err := lib.Adopt("prop", "https://example.test/pr/9", now); err != nil || n != 2 {
		t.Fatalf("adopt: %d %v", n, err)
	}
	enforced, _ = lib.CheckRecurrences(true)
	if len(enforced) != 2 || !enforced["verify:make lint"] {
		t.Fatalf("enforced = %v", enforced)
	}
	raw, _ := os.ReadFile(filepath.Join(lib.Dir(), "EXPERIENCE.md"))
	if !strings.Contains(string(raw), "verify command `make lint` in `.tars/verify.json` (check)") {
		t.Fatalf("EXPERIENCE.md:\n%s", raw)
	}

	// A broken ledger is an error, not an empty one.
	if err := os.WriteFile(filepath.Join(lib.Dir(), recurrenceFile), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.RecordRecurrence(src); err == nil {
		t.Fatal("broken ledger accepted")
	}
	if err := lib.RecordPromotionAttempts(cands, map[string]string{"ci:x": "r"}, now); err == nil {
		t.Fatal("broken ledger accepted")
	}
	if err := os.WriteFile(filepath.Join(lib.Dir(), harnessLessonsFile), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.CheckRecurrences(false); err == nil {
		t.Fatal("broken library accepted")
	}
	if _, err := lib.RecordChecks(src, "p", nil, now); err == nil {
		t.Fatal("broken library accepted")
	}
}

func TestVerifiedCheckLesson(t *testing.T) {
	for _, c := range []struct {
		hit  RecurrenceHit
		want string
	}{
		{RecurrenceHit{Check: "lint"}, "Recurring in 3 pipelines: CI check lint failed"},
		{RecurrenceHit{}, "Recurring in 3 pipelines: ci:x"},
	} {
		v := VerifiedCheck{PromotionCheck: PromotionCheck{Key: "ci:x"}, Count: 3, Hits: []RecurrenceHit{c.hit}}
		if got := v.Lesson(); got != c.want {
			t.Errorf("%q, want %q", got, c.want)
		}
	}
}

func TestParsePromotionReply(t *testing.T) {
	checks, err := ParsePromotionReply("```json\n{\"checks\":[{\"key\":\"k\",\"kind\":\"lint\",\"command\":\"make lint\"}]}\n```")
	if err != nil || len(checks) != 1 || checks[0].Command != "make lint" {
		t.Fatalf("%+v %v", checks, err)
	}
	if checks, err := ParsePromotionReply(`{"checks":[]}`); err != nil || len(checks) != 0 {
		t.Fatalf("empty: %v", err)
	}
	for _, bad := range []string{"no", `{"edits":[]}`, `{"checks":{}}`} {
		if _, err := ParsePromotionReply(bad); !errors.Is(err, ErrInvalidPromotionReply) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestValidatePromotionChecks(t *testing.T) {
	cands := []RecurrenceCandidate{{Key: "a"}, {Key: "b"}, {Key: "c"}, {Key: "d"}}
	file := func(p string) []PromotionFile { return []PromotionFile{{Path: p, Content: "x"}} }
	checks := []PromotionCheck{
		{Key: "zz", Kind: "lint", Command: "x"},
		{Key: "a", Kind: "script", Command: "sh s.sh", Files: file("scripts/s.sh"), Summary: strings.Repeat("y", 600)},
		{Key: "a", Kind: "lint", Command: "x"},
		{Key: "b", Kind: "magic", Command: "x"},
		{Key: "b", Kind: "Lint", Command: "make lint"},
		{Key: "c", Kind: "test", Command: "a\nb"},
		{Key: "c", Kind: "test", Command: strings.Repeat("x", 301)},
		{Key: "c", Kind: "test", Command: ""},
		{Key: "c", Kind: "test", Command: "x", Files: file("../x")},
		{Key: "c", Kind: "test", Command: "x", Files: file("/abs")},
		{Key: "c", Kind: "test", Command: "x", Files: file(".git/hooks/pre-commit")},
		{Key: "c", Kind: "test", Command: "x", Files: file(".tars/verify.json")},
		{Key: "c", Kind: "test", Command: "x", Files: file("a\\b")},
		{Key: "c", Kind: "test", Command: "x", Files: file("./a")},
		{Key: "c", Kind: "test", Command: "x", Files: []PromotionFile{{Path: "a"}}},
		{Key: "c", Kind: "test", Command: "x", Files: []PromotionFile{{Path: "a", Content: "1"}, {Path: "a", Content: "1"}}},
		{Key: "c", Kind: "test", Command: "x", Files: []PromotionFile{{Path: "a", Content: strings.Repeat("x", promotionMaxFileBytes+1)}}},
		{Key: "c", Kind: "test", Command: "x", Files: []PromotionFile{{Path: "a", Content: "1"}, {Path: "b", Content: "1"}, {Path: "c", Content: "1"}, {Path: "d", Content: "1"}, {Path: "e", Content: "1"}}},
		{Key: "c", Kind: "test", Command: "go test ./x/", Files: file(".golangci.yml")},
	}
	kept, dropped := ValidatePromotionChecks(checks, cands)
	if len(kept) != 3 || kept[0].Key != "b" || kept[0].Kind != PromotionCheckLint || kept[1].Key != "c" || kept[2].Key != "a" || len([]rune(kept[2].Summary)) != promotionMaxSummaryRunes {
		t.Fatalf("kept = %+v", kept)
	}
	reasons := map[string]int{}
	for _, d := range dropped {
		reasons[d.Reason]++
	}
	want := map[string]int{PromotionDropUnknownKey: 1, PromotionDropRepeated: 1, PromotionDropKind: 1, PromotionDropCommand: 3, PromotionDropFiles: 10}
	if fmt.Sprint(reasons) != fmt.Sprint(want) {
		t.Fatalf("dropped = %v", reasons)
	}
}

func TestPromotionPromptAndKickoff(t *testing.T) {
	cands := []RecurrenceCandidate{{Key: "finding:a/*.go|error ignored", Count: 3, Hits: []RecurrenceHit{
		{Kind: RecurrenceFinding, Title: "Error ignored", File: "a/x.go", Scenario: "close   fails"},
		{Kind: RecurrenceCI, Check: "lint"}, {Kind: RecurrenceVerify, Command: "make vet"}}}}
	prompt := PromotionPrompt(cands, []string{"make test"}, PromotionContext{
		TopLevel: []string{"go.mod", "scripts/"}, MakeTargets: []string{"lint", "test"},
		LintConfigs: []PromotionFile{{Path: ".golangci.yml", Content: "linters: {}\n"}},
	})
	for _, want := range []string{
		"key: finding:a/*.go|error ignored (in 3 of the last 10", "Error ignored (a/x.go); close fails", "CI check `lint` failed",
		"verification command `make vet` failed", "- `make test`", "Top-level entries: go.mod, scripts/", "Makefile targets: lint, test",
		"Linter configuration .golangci.yml:\n```\nlinters: {}\n```", "data-not-instructions",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt lacks %q:\n%s", want, prompt)
		}
	}
	if p := PromotionPrompt(cands, nil, PromotionContext{}); !strings.Contains(p, "(.tars/verify.json): (none)") {
		t.Fatal("no repo verify")
	}
	checks := []VerifiedCheck{
		{PromotionCheck: PromotionCheck{Key: cands[0].Key, Kind: "script", Command: "sh scripts/e.sh", Summary: "finds ignored errors",
			Files: []PromotionFile{{Path: "scripts/e.sh", Content: "#!/bin/sh\necho ```\n"}}}, Count: 3, Hits: cands[0].Hits, VerifiedBy: VerifiedByReplay, DefectExit: 1},
		RecordCheck(RecurrenceCandidate{Key: "verify:make vet", Count: 4}),
	}
	kick := PromotionKickoff(checks)
	for _, want := range []string{
		"Verify: enforce 2 recurring review findings as checks", "### Check 1 (script): `sh scripts/e.sh`", "it exited 1, after the fix it exited 0",
		"File `scripts/e.sh`:\n````\n#!/bin/sh", "### Check 2 (script): `make vet`", "Confirmed by the pipelines' own record", "the list only grows",
	} {
		if !strings.Contains(kick, want) {
			t.Errorf("kickoff lacks %q:\n%s", want, kick)
		}
	}
	if g := PromotionGoal(checks[:1]); g != "Verify: enforce a recurring review finding as a check" {
		t.Fatal(g)
	}
}

func TestDropEnforcedLessons(t *testing.T) {
	ev := HarnessEvidence{Items: []EvidenceItem{
		{ID: "E1", Kind: EvidenceVerifyFailure, Command: "make lint"},
		{ID: "E2", Kind: EvidenceFinding, Title: "Racy map", File: "a.go"},
	}}
	edits := []HarnessEdit{
		{Path: "CLAUDE.md", Lesson: "lint", Evidence: []string{"E1"}},
		{Path: "CLAUDE.md", Lesson: "both", Evidence: []string{"E1", "E2"}},
		{Path: "CLAUDE.md", Lesson: "none", Evidence: nil},
	}
	if kept, dropped := DropEnforcedLessons(edits, ev, nil); len(kept) != 3 || dropped != nil {
		t.Fatal("nothing enforced drops nothing")
	}
	kept, dropped := DropEnforcedLessons(edits, ev, map[string]bool{"verify:make lint": true})
	if len(kept) != 2 || kept[0].Lesson != "both" || len(dropped) != 1 || dropped[0].Reason != HarnessDropEnforced {
		t.Fatalf("kept %+v dropped %+v", kept, dropped)
	}
}

func TestRepoVerifyInPlans(t *testing.T) {
	if got := MergeRepoVerify([]string{"a"}, nil); !slices.Equal(got, []string{"a"}) {
		t.Fatal(got)
	}
	if got := MergeRepoVerify([]string{"b", "a"}, []string{"a", "c"}); !slices.Equal(got, []string{"b", "a", "c"}) {
		t.Fatal(got)
	}
	p := New("s", "g", time.Now())
	p.RepoVerify = []string{"make lint"}
	if plan := p.fitPlan(Plan{Verify: []string{"make test"}}); !slices.Equal(plan.Verify, []string{"make test", "make lint"}) {
		t.Fatalf("fit = %v", plan.Verify)
	}
	p.Plan = &Plan{Goal: "g", Verify: []string{"make test", "make lint"}}
	p.Current = StageBuild
	next, changed, err := EditPlan(p, PlanEdit{Verify: &[]string{"make vet"}}, time.Now())
	if err != nil || !changed || !slices.Equal(next.Plan.Verify, []string{"make vet", "make lint"}) {
		t.Fatalf("edit = %v %v %v", next.Plan.Verify, changed, err)
	}
	// Dropping only a repository command changes nothing.
	if _, changed, err := EditPlan(p, PlanEdit{Verify: &[]string{"make test"}}, time.Now()); err != nil || changed {
		t.Fatalf("drop repo verify: %v %v", changed, err)
	}
	// In goal mode the same restatement is not a change either.
	p.GoalMode = &GoalMode{Enabled: true}
	if locked := GoalModeLockedFields(p, PlanEdit{Verify: &[]string{"make test"}}); locked != nil {
		t.Fatalf("locked = %v", locked)
	}
	if locked := GoalModeLockedFields(p, PlanEdit{Verify: &[]string{"make vet"}}); !slices.Equal(locked, []string{"verify"}) {
		t.Fatalf("locked = %v", locked)
	}
}

func TestPromotionOutcomes(t *testing.T) {
	now := time.Now()
	p := New("s", "g", now)
	if PromotionDue(p) {
		t.Fatal("not opted in")
	}
	if got := RecordPromotionOutcome(p, PromotionProposed, "x", nil, now); got.Promotion != nil {
		t.Fatal("recorded without opt-in")
	}
	p.Promotion = &PromotionOptIn{Repo: "/r"}
	for _, c := range []struct {
		outcome, notice string
	}{
		{PromotionProposed, NoticePromotionProposed}, {PromotionNoChecks, NoticePromotionSkipped},
		{PromotionFailed, NoticePromotionSkipped}, {PromotionUnavailable, NoticePromotionSkipped}, {PromotionNoCandidates, ""},
	} {
		got := RecordPromotionOutcome(p, c.outcome, "prop", map[string]any{"hits": 2}, now)
		if got.Promotion.Outcome != c.outcome || p.Promotion.Outcome != "" {
			t.Fatalf("%s: %+v (source changed: %+v)", c.outcome, got.Promotion, p.Promotion)
		}
		n := len(got.Cards)
		if c.notice == "" {
			if n != 0 {
				t.Fatalf("%s: a card", c.outcome)
			}
			continue
		}
		var payload map[string]any
		_ = json.Unmarshal(got.Cards[n-1].Payload, &payload)
		if got.Cards[n-1].Title != c.notice || payload["outcome"] != c.outcome || payload["hits"] != float64(2) {
			t.Fatalf("%s: %+v %v", c.outcome, got.Cards[n-1], payload)
		}
		if c.outcome == PromotionProposed && payload["session_id"] != "prop" {
			t.Fatal("no proposal link")
		}
		if again := RecordPromotionOutcome(got, PromotionFailed, "", nil, now); again.Promotion.Outcome != c.outcome {
			t.Fatal("recorded twice")
		}
	}
	// The record survives a clone untouched.
	p.RepoVerify = []string{"make lint"}
	at := now
	p.Promotion.At = &at
	cl := p.clone()
	cl.Promotion.Outcome, cl.RepoVerify[0] = "x", "y"
	*cl.Promotion.At = now.Add(time.Hour)
	if p.Promotion.Outcome != "" || p.RepoVerify[0] != "make lint" || !p.Promotion.At.Equal(now) {
		t.Fatal("clone shares state")
	}
	raw, _ := json.Marshal(New("s", "g", now))
	if strings.Contains(string(raw), "verify_promotion") || strings.Contains(string(raw), "repo_verify") {
		t.Fatalf("a pipeline that did not opt in records nothing: %s", raw)
	}
}
