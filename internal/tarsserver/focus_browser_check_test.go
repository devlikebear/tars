package tarsserver

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/focusprobe"
	"github.com/rs/zerolog"
)

// A new pipeline records how its desk runs the scripted-browser CLI and its
// plan stage is taught to use it; a desk without the CLI records nothing.
func TestFocusCreateRecordsBrowserCheckCLI(t *testing.T) {
	f := newWorktreeFixture(t)
	h := newFocusPipelineHandler(f.store, f.c, nil, zerolog.Nop())
	create := func() (string, focuspipeline.Pipeline) {
		rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines", `{"goal":"add a cart page","cwd":`+jsonString(f.repo)+`}`, true)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
		}
		var out struct {
			SessionID string `json:"session_id"`
		}
		decodeInto(t, rec, &out)
		return out.SessionID, pipelineOf(t, f.store, out.SessionID)
	}

	cli := filepath.Join(t.TempDir(), "agent-browser")
	if err := os.WriteFile(cli, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(focusprobe.AgentBrowserPathEnv, cli)
	_, p := create()
	if p.BrowserCheck != cli {
		t.Fatalf("browser check = %q, want %q", p.BrowserCheck, cli)
	}
	if g := focuspipeline.Guidance(p); !strings.Contains(g, "Scripted browser checks") || !strings.Contains(g, cli) {
		t.Fatalf("plan guidance does not teach the check:\n%s", g)
	}

	// The CLI-missing path: an override that names nothing is not guessed
	// around, and the pipeline file has no trace of the feature.
	t.Setenv(focusprobe.AgentBrowserPathEnv, filepath.Join(t.TempDir(), "gone"))
	id, p := create()
	if p.BrowserCheck != "" || strings.Contains(focuspipeline.Guidance(p), "agent-browser") {
		t.Fatalf("a desk without the CLI was taught it: %q", p.BrowserCheck)
	}
	raw, err := os.ReadFile(filepath.Join(f.store.WorkspaceDir(), "sessions", id+".pipeline.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "browser_check") {
		t.Fatalf("a pipeline without the CLI records nothing:\n%s", raw)
	}
}

// A check script that finds the page wrong stops the pipeline, and its own
// three lines become the failure card's words: the real verification runner
// (shell, exit code, excerpt) into the real state machine, no model.
func TestFocusFailedBrowserCheckStopsThePipeline(t *testing.T) {
	const check = `for i in $(seq 1 400); do echo "building chunk $i"; done; ` +
		`echo "CHECK FAILED"; echo "screen: Cart page (http://127.0.0.1:4173/)"; ` +
		`echo "expected: the total reads 30"; echo "seen: the total reads 25"; ` +
		`for i in $(seq 1 400); do echo "stopping server $i"; done; exit 1`
	store, id := verificationSession(t, check)
	results, ok, err := runTaskVerificationCommands(context.Background(), store, id, "", []string{check}, 0, focusprobe.FailureExcerpt)
	if err != nil || ok || len(results) != 1 {
		t.Fatalf("results = %+v ok = %v err = %v", results, ok, err)
	}
	p, act := applyCheckResult(t, id, results[0])

	if p.Current != focuspipeline.StageBuild {
		t.Fatalf("a failed check advanced the pipeline to %s", p.Current)
	}
	if act.Kind != focuspipeline.ActionSendTurn || !strings.Contains(act.Prompt, "seen: the total reads 25") {
		t.Fatalf("the fix turn does not carry the failure: %+v", act)
	}
	want := "Check failed on Cart page (http://127.0.0.1:4173/): expected the total reads 30, but saw the total reads 25"
	found := false
	for _, c := range p.Cards {
		if c.Kind == focuspipeline.CardFailure {
			found = c.Title == want
		}
	}
	if !found {
		t.Fatalf("no failure card titled %q in %+v", want, p.Cards)
	}
}

// applyCheckResult takes a pipeline through an approved plan and a build
// report, then gives it r as the result of its one verify command.
func applyCheckResult(t *testing.T, id string, r taskVerificationResult) (focuspipeline.Pipeline, focuspipeline.Action) {
	t.Helper()
	v := &focuspipeline.Verification{Passed: r.Status == "passed", Results: []focuspipeline.VerificationResult{{
		Command: "sh checks/cart.sh", ExitCode: r.ExitCode, TimedOut: r.TimedOut, Passed: r.Status == "passed", Excerpt: r.Output,
	}}}
	now := time.Now()
	p := focuspipeline.New(id, "add a cart page", now)
	plan := &focuspipeline.Plan{
		Goal: "add a cart page", Tasks: []focuspipeline.PlanTask{{Title: "t1", Done: "d1"}},
		Stages: []focuspipeline.StageID{focuspipeline.StagePlan, focuspipeline.StageBuild, focuspipeline.StagePR, focuspipeline.StageMerge},
		Verify: []string{"sh checks/cart.sh"},
	}
	apply := func(e focuspipeline.Event) focuspipeline.Action {
		t.Helper()
		next, act, err := focuspipeline.Apply(p, e, now)
		if err != nil {
			t.Fatal(err)
		}
		p = next
		return act
	}
	apply(focuspipeline.Event{Kind: focuspipeline.EventTurnCompleted, Turn: 1, Blocks: focuspipeline.Blocks{Plan: plan}})
	apply(focuspipeline.Event{Kind: focuspipeline.EventGate, Gate: focuspipeline.GatePlan, Action: focuspipeline.GateApprove})
	apply(focuspipeline.Event{Kind: focuspipeline.EventTurnCompleted, Turn: 2, Blocks: focuspipeline.Blocks{Report: &focuspipeline.Report{Summary: "done", TasksDone: true}}})
	act := apply(focuspipeline.Event{Kind: focuspipeline.EventVerification, Verification: v})
	return p, act
}
