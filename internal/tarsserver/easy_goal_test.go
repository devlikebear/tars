package tarsserver

import (
	"net/http"
	"testing"

	"github.com/devlikebear/tars/internal/easyproject"
	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/rs/zerolog"
)

const easyPlanReply = "Plan below.\n<focus-plan>" +
	`{"goal":"A sign-up page","tasks":[{"title":"A sign-up form","done":"the form saves a name"}],"stages":["plan","make","review"],"verify":["npm run build"]}` +
	"</focus-plan>"

// The whole easy flow on the server: one sentence, a person approves the
// plan (as a user, not an admin), and from there goal mode decides
// everything — the build, the review — until the work is saved as a
// version and the session has its own permission mode back.
func TestEasyProjectRunsInGoalModeOnceThePlanIsApproved(t *testing.T) {
	easyFixture(t)
	f := newWorktreeFixture(t)
	d, turns := createFocusDriver(t, f.store, easyPlanReply)
	var sessionID string
	turns.reply = func(_ int, prompt string) string {
		if prompt == "A sign-up page for a book club" {
			return easyPlanReply
		}
		if sessionID != "" && pipelineOf(t, f.store, sessionID).CurrentKind() == focuspipeline.StageReview {
			return focusNoFindings
		}
		return focusReport("done", true)
	}
	d.verify = (&fakeVerifier{result: passAll}).verify
	h := newFocusPipelineHandler(f.store, f.c, d, zerolog.Nop())

	rec := focusRequestAs(t, h, http.MethodPost, "/v1/easy/projects", `{"idea":"A sign-up page for a book club"}`, serverauth.RoleUser)
	var created easyProjectResp
	decodeInto(t, rec, &created)
	var project easyproject.Project
	waitFor(t, "the project", func() bool {
		project = getEasyProject(t, h, created.Project.ID)
		return project.Status == easyproject.StatusReady
	})
	id := project.SessionID
	sessionID = id
	waitFor(t, "the plan gate", func() bool { return pipelineOf(t, f.store, id).OpenGate == focuspipeline.GatePlan })
	waitDriverIdle(t, d, id)
	if p := pipelineOf(t, f.store, id); !p.GoalAfterPlan || p.GoalMode != nil {
		t.Fatalf("before approval: goal after plan %v, goal mode %+v", p.GoalAfterPlan, p.GoalMode)
	}

	// The person approves the plan; nothing else is clicked.
	rec = focusRequestAs(t, h, http.MethodPost, "/v1/focus/pipelines/"+id+"/gates/plan", `{"action":"approve"}`, serverauth.RoleUser)
	var approved struct {
		Pipeline focuspipeline.Pipeline `json:"pipeline"`
	}
	decodeInto(t, rec, &approved)
	if rec.Code != http.StatusOK {
		t.Fatalf("approve = %d %s", rec.Code, rec.Body.String())
	}
	// Goal mode is on, and its permission mode was set before the build
	// turn the approval started.
	if g := approved.Pipeline.GoalMode; g == nil || !g.Enabled || !g.PermissionSet {
		t.Fatalf("goal mode on approval: %+v", g)
	}

	defer func() {
		if t.Failed() {
			p := pipelineOf(t, f.store, id)
			t.Logf("current %s gate %q pending %q awaiting %v goal %+v", p.Current, p.OpenGate, p.PendingTurn, p.AwaitingVerification, p.GoalMode)
			for _, s := range p.Stages {
				t.Logf("stage %s %s", s.ID, s.Status)
			}
			for _, c := range p.Cards {
				t.Logf("card %s %s %s %q", c.Kind, c.Stage, c.State, c.Title)
			}
			t.Logf("turns %q", turns.seen())
		}
	}()
	waitFor(t, "the saved version", func() bool { return pipelineOf(t, f.store, id).Saved != nil })
	waitFor(t, "goal mode to end", func() bool { return !pipelineOf(t, f.store, id).GoalActive() })
	p := pipelineOf(t, f.store, id)
	if !focuspipeline.Finished(p) || p.GoalMode.EndReason != focuspipeline.GoalEndFinished || p.Saved.Error != "" || p.Saved.Commit == "" {
		t.Fatalf("finished %v, goal %+v, saved %+v", focuspipeline.Finished(p), p.GoalMode, p.Saved)
	}
	if sess, _ := f.store.Get(id); sess.PermissionMode == chatPermissionModeAuto {
		t.Fatal("the session kept goal mode's permission mode")
	}
	waitDriverIdle(t, d, id)
}

// Approving the plan of any other pipeline does not turn goal mode on.
func TestPlanApprovalStartsGoalModeOnlyWhenAsked(t *testing.T) {
	f := newWorktreeFixture(t)
	d, _ := createFocusDriver(t, f.store, focusPlanReply)
	h := newFocusPipelineHandler(f.store, f.c, d, zerolog.Nop())
	rec := focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines", `{"goal":"g","start":true,"cwd":`+jsonString(f.repo)+`}`, true)
	var out focusCreated
	decodeInto(t, rec, &out)
	waitFor(t, "the plan gate", func() bool { return pipelineOf(t, f.store, out.SessionID).OpenGate == focuspipeline.GatePlan })
	waitDriverIdle(t, d, out.SessionID)
	rec = focusRequest(t, h, http.MethodPost, "/v1/focus/pipelines/"+out.SessionID+"/gates/plan", `{"action":"approve"}`, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("approve = %d %s", rec.Code, rec.Body.String())
	}
	if p := pipelineOf(t, f.store, out.SessionID); p.GoalMode != nil || p.GoalAfterPlan {
		t.Fatalf("goal mode on a pipeline that did not ask: %+v", p.GoalMode)
	}
	waitDriverIdle(t, d, out.SessionID)
}
