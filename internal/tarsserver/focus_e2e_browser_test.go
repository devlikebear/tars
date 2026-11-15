package tarsserver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/aside"
	"github.com/devlikebear/tars/internal/focuspipeline"
)

func TestWithBrowserE2ERoutesOnlyBrowserGoals(t *testing.T) {
	var nextCalls []string
	next := func(_ context.Context, _ string, goal string) (focuspipeline.VerificationResult, error) {
		nextCalls = append(nextCalls, goal)
		return focuspipeline.VerificationResult{Command: goal, Passed: true, E2E: true}, nil
	}
	var got aside.Check
	verify := func(ctx context.Context, c aside.Check) (aside.CheckResult, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("browser check must run with a deadline")
		}
		got = c
		return aside.CheckResult{Passed: true, Transcript: "looked\nVERDICT: PASS"}, nil
	}
	run := withBrowserE2E(verify, next)

	res, err := run(context.Background(), "s1", "@Calculator add 2 and 2")
	if err != nil || len(nextCalls) != 1 || !res.Passed {
		t.Fatalf("non-browser goal must go to computer_use: %+v %v %v", res, err, nextCalls)
	}

	res, err = run(context.Background(), "s1", "@browser http://localhost:5173/cart the cart shows 2 items")
	if err != nil || !res.Passed || res.Skipped || !res.E2E || len(nextCalls) != 1 {
		t.Fatalf("browser goal: %+v %v", res, err)
	}
	if got.StartURL != "http://localhost:5173/cart" || got.Goal != "the cart shows 2 items" {
		t.Fatalf("check %+v", got)
	}
	if !strings.Contains(res.Excerpt, "VERDICT: PASS") {
		t.Fatalf("excerpt %q", res.Excerpt)
	}
}

func TestRunBrowserE2EOutcomes(t *testing.T) {
	const goal = "@Browser http://127.0.0.1:3000 home loads"
	long := strings.Repeat("line\n", 50) + "VERDICT: FAIL: blank page"
	cases := []struct {
		name    string
		goal    string
		res     aside.CheckResult
		err     error
		passed  bool
		skipped bool
		excerpt string
		called  bool
	}{
		{name: "pass", goal: goal, res: aside.CheckResult{Passed: true}, passed: true, excerpt: "done", called: true},
		{name: "fail", goal: goal, res: aside.CheckResult{Reason: "blank page", Transcript: long}, excerpt: "failed: blank page", called: true},
		{name: "no cli", goal: goal, err: fmt.Errorf("%w: install it", aside.ErrNotFound), passed: true, skipped: true, excerpt: "unavailable", called: true},
		{name: "exec error", goal: goal, err: errors.New("aside exec failed: not signed in"), excerpt: "error: aside exec failed: not signed in", called: true},
		{name: "remote site", goal: "@Browser https://mail.google.com read my inbox", excerpt: "not run"},
		{name: "no url", goal: "@Browser check the home page", excerpt: "not run"},
		{name: "no goal", goal: "@Browser http://localhost:3000", excerpt: "not run"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			called := false
			verify := func(context.Context, aside.Check) (aside.CheckResult, error) {
				called = true
				return c.res, c.err
			}
			_, text := splitE2EGoal(c.goal)
			got := runBrowserE2E(context.Background(), verify, c.goal, text)
			if called != c.called || got.Passed != c.passed || got.Skipped != c.skipped || !got.E2E || got.Command != c.goal {
				t.Fatalf("got %+v called=%v", got, called)
			}
			if !strings.HasPrefix(got.Excerpt, c.excerpt) {
				t.Fatalf("excerpt %q, want prefix %q", got.Excerpt, c.excerpt)
			}
			if !got.Passed && got.ExitCode != -1 {
				t.Fatalf("failed check needs exit code -1, got %d", got.ExitCode)
			}
		})
	}
}

func TestTranscriptTailKeepsLastLines(t *testing.T) {
	if transcriptTail("  ") != "" {
		t.Fatal("empty transcript adds nothing")
	}
	tail := transcriptTail(strings.Repeat("x\n", 100) + "end")
	// "\nLast output:\n" adds two line breaks before the kept lines.
	if n := strings.Count(tail, "\n"); n != browserE2EExcerptLines+1 {
		t.Fatalf("got %d lines", n)
	}
	if !strings.HasSuffix(tail, "end") {
		t.Fatal("must keep the last line")
	}
}
