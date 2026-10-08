package tarsserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/apptool"
	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// reviewVerifying puts a reviewing session's pipeline at the point a clean
// review turn leaves it: verification awaited.
func reviewVerifying(t *testing.T, store *session.Store, edit func(*focuspipeline.Plan)) session.Session {
	t.Helper()
	sess := reviewingFocusSession(t, store, "", "")
	if _, _, err := focusStoreFor(store).Update(sess.ID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		p.AwaitingVerification = true
		edit(p.Plan)
		return p, nil
	}); err != nil {
		t.Fatal(err)
	}
	return sess
}

func TestFocusDriverBuildsBeforeEndToEndGoals(t *testing.T) {
	d, _, store, _ := testFocusDriver(t, func(int, string) string { return focusReport("next", false) }, &fakeVerifier{result: passAll})
	steps := &recordingVerifier{}
	d.verify = func(ctx context.Context, id, command string) (focuspipeline.VerificationResult, error) {
		return steps.verify(ctx, id, "sh:"+command)
	}
	d.e2e = func(ctx context.Context, id, goal string) (focuspipeline.VerificationResult, error) {
		return steps.verify(ctx, id, "gui:"+goal)
	}
	sess := reviewVerifying(t, store, func(plan *focuspipeline.Plan) {
		plan.E2E = []string{"@Safari check the new button"}
		plan.E2ESetup = []string{"make build", "./bin/app --port 4399 &"}
		plan.E2ETeardown = []string{"pkill app"}
	})
	d.start(sess.ID, focuspipeline.Action{Kind: focuspipeline.ActionRunVerification}, "")
	waitDriverIdle(t, d, sess.ID)

	want := "sh:make test,sh:make build,sh:./bin/app --port 4399 &,gui:@Safari check the new button,sh:pkill app"
	if got := strings.Join(steps.seen(), ","); got != want {
		t.Fatalf("order = %q\nwant    %q", got, want)
	}
	if got := statuses(pipelineOf(t, store, sess.ID))[focuspipeline.StageReview]; got != focuspipeline.StatusDone {
		t.Fatalf("review = %s", got)
	}
}

func TestFocusDriverFailedSetupSkipsTheGoals(t *testing.T) {
	d, turns, store, _ := testFocusDriver(t, func(int, string) string { return focusReport("fixed", false) }, &fakeVerifier{result: passAll})
	shell := &recordingVerifier{}
	d.verify = func(ctx context.Context, id, command string) (focuspipeline.VerificationResult, error) {
		_, _ = shell.verify(ctx, id, command)
		if command == "make build" && len(shell.seen()) < 4 {
			return focuspipeline.VerificationResult{Command: command, ExitCode: 2, Excerpt: "compile error"}, nil
		}
		return focuspipeline.VerificationResult{Command: command, Passed: true}, nil
	}
	gui := &recordingVerifier{}
	d.e2e = gui.verify
	sess := reviewVerifying(t, store, func(plan *focuspipeline.Plan) {
		plan.E2E = []string{"check the new button"}
		plan.E2ESetup = []string{"make build", "open the app"}
		plan.E2ETeardown = []string{"pkill app"}
	})
	d.start(sess.ID, focuspipeline.Action{Kind: focuspipeline.ActionRunVerification}, "")
	waitDriverIdle(t, d, sess.ID)

	if prompt := turns.seen()[0]; !strings.Contains(prompt, "`make build`") || !strings.Contains(prompt, "compile error") {
		t.Fatalf("fix prompt = %q", prompt)
	}
	// The first run: verify, the failing setup command, teardown — and no
	// goal against a screen the build never reached. The run after the
	// fix turn goes all the way.
	want := "make test,make build,pkill app,make test,make build,open the app,pkill app"
	if got := strings.Join(shell.seen(), ","); got != want {
		t.Fatalf("shell = %q\nwant    %q", got, want)
	}
	if got := gui.seen(); len(got) != 1 {
		t.Fatalf("goals ran %d times, want once after the fix", len(got))
	}
}

func TestFocusPlanEditEndpointAndTool(t *testing.T) {
	d, _, store, _ := testFocusDriver(t, func(int, string) string { return focusNoFindings }, &fakeVerifier{result: passAll})
	planned := plannedFocusSession(t, store)
	h := newFocusPipelineHandler(store, nil, d, zerolog.Nop())
	if rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/"+planned.ID+"/plan", `{"e2e":[]}`, true); rec.Code != http.StatusConflict {
		t.Fatalf("before approval: %d %s", rec.Code, rec.Body.String())
	}
	if focusPlanEditable(store, planned.ID) {
		t.Fatal("the tool must not be offered before the plan is approved")
	}

	sess := reviewingFocusSession(t, store, "", "")
	if err := store.SaveTasks(sess.ID, focuspipeline.SessionTasks(*pipelineOf(t, store, sess.ID).Plan, time.Now())); err != nil {
		t.Fatal(err)
	}
	path := "/v1/focus/pipelines/" + sess.ID + "/plan"
	if rec := focusRequest(t, h, http.MethodPost, path, `{}`, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty edit: %d %s", rec.Code, rec.Body.String())
	}
	rec := focusRequest(t, h, http.MethodPost, path, `{"e2e":[],"verify":["make test","make lint"]}`, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("edit: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Changed  bool                   `json:"changed"`
		Pipeline focuspipeline.Pipeline `json:"pipeline"`
	}
	decodeInto(t, rec, &resp)
	if !resp.Changed || len(resp.Pipeline.Plan.E2E) != 0 {
		t.Fatalf("resp = %+v", resp)
	}
	tasks, err := store.GetTasks(sess.ID)
	if err != nil || strings.Join(tasks.Contract.VerificationCommands, ",") != "make test,make lint" {
		t.Fatalf("contract = %+v err = %v", tasks.Contract, err)
	}
	if verify, e2e := focusVerifyCommands(pipelineOf(t, store, sess.ID)); len(e2e) != 0 || len(verify) != 2 {
		t.Fatalf("next verification runs %q + %q", verify, e2e)
	}

	if !focusPlanEditable(store, sess.ID) {
		t.Fatal("the tool must be offered for an approved plan")
	}
	tool := apptool.NewFocusPlanEditTool(newFocusPlanEditor(store, sess.ID, d, zerolog.Nop()))
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"goal":"a narrower goal","e2e_setup":["make build"]}`))
	if err != nil || result.IsError {
		t.Fatalf("tool: %+v %v", result, err)
	}
	p := pipelineOf(t, store, sess.ID)
	if p.Goal != "a narrower goal" || strings.Join(p.Plan.E2ESetup, ",") != "make build" {
		t.Fatalf("after tool: goal = %q plan = %+v", p.Goal, p.Plan)
	}
	if result, _ := tool.Execute(context.Background(), json.RawMessage(`{}`)); !result.IsError {
		t.Fatal("an empty tool edit must be an error")
	}
}
