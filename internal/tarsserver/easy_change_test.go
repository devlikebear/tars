package tarsserver

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/easyproject"
	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// finishPipeline marks a pipeline as run to its end with its version saved.
func finishPipeline(t *testing.T, store *session.Store, id string) {
	t.Helper()
	if _, _, err := focusStoreFor(store).Update(id, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		ids := make([]focuspipeline.StageID, 0, len(p.Stages))
		for i := range p.Stages {
			p.Stages[i].Status = focuspipeline.StatusDone
			ids = append(ids, p.Stages[i].ID)
		}
		p.OpenGate = ""
		if p.Plan == nil {
			p.Plan = &focuspipeline.Plan{}
		}
		p.Plan.Stages = ids
		p.Saved = &focuspipeline.SavedVersion{Commit: "abc"}
		return p, nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestEasyChangeOpensTheNextPipelineInTheSameFolder(t *testing.T) {
	easyFixture(t)
	f := newWorktreeFixture(t)
	d, turns := createFocusDriver(t, f.store, focusPlanReply)
	h := newFocusPipelineHandler(f.store, f.c, d, zerolog.Nop())

	rec := focusRequestAs(t, h, http.MethodPost, "/v1/easy/projects", `{"idea":"A sign-up page"}`, serverauth.RoleUser)
	var created easyProjectResp
	decodeInto(t, rec, &created)
	var p easyproject.Project
	waitFor(t, "the project to be ready", func() bool {
		p = getEasyProject(t, h, created.Project.ID)
		return p.Status == easyproject.StatusReady
	})
	first := p.SessionID
	waitFor(t, "the plan gate", func() bool { return pipelineOf(t, f.store, first).OpenGate == focuspipeline.GatePlan })
	waitDriverIdle(t, d, first)
	if sess, _ := f.store.Get(first); sess.Isolation != session.IsolationOff {
		t.Fatalf("the first pipeline's session may move into a worktree: isolation %q", sess.Isolation)
	}

	changes := "/v1/easy/projects/" + p.ID + "/changes"
	// The team is still on the first version.
	if rec := focusRequestAs(t, h, http.MethodPost, changes, `{"request":"Make the button green"}`, serverauth.RoleUser); rec.Code != http.StatusConflict {
		t.Fatalf("change while busy = %d %s", rec.Code, rec.Body.String())
	}

	finishPipeline(t, f.store, first)
	for _, tc := range []struct {
		body, role string
		code       int
	}{
		{`{"request":"  "}`, serverauth.RoleUser, http.StatusBadRequest},
		{`{"request":""}`, serverauth.RoleFocus, http.StatusBadRequest},
	} {
		if rec := focusRequestAs(t, h, http.MethodPost, changes, tc.body, tc.role); rec.Code != tc.code {
			t.Fatalf("%s as %s = %d %s", tc.body, tc.role, rec.Code, rec.Body.String())
		}
	}
	if rec := focusRequestAs(t, h, http.MethodPost, "/v1/easy/projects/0000000000000000/changes", `{"request":"x"}`, serverauth.RoleUser); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown project = %d", rec.Code)
	}

	// A paired phone may ask for a change (vibe redesign unit 6).
	rec = focusRequestAs(t, h, http.MethodPost, changes, `{"request":"  Make the button green  "}`, serverauth.RoleFocus)
	var changed easyProjectResp
	decodeInto(t, rec, &changed)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("change = %d %s", rec.Code, rec.Body.String())
	}
	next := changed.Project.SessionID
	if next == "" || next == first || len(changed.Project.Changes) != 1 ||
		changed.Project.Changes[0].Request != "Make the button green" || changed.Project.Changes[0].SessionID != next {
		t.Fatalf("project after change = %+v", changed.Project)
	}
	if got := getEasyProject(t, h, p.ID); got.SessionID != next || len(got.Changes) != 1 {
		t.Fatalf("stored project = %+v", got)
	}

	pipeline := pipelineOf(t, f.store, next)
	goal := easyproject.ChangeGoal("Make the button green")
	if pipeline.Template != focuspipeline.WebAppTemplateID || !pipeline.SaveVersion || !pipeline.GoalAfterPlan || pipeline.Goal != goal {
		t.Fatalf("change pipeline: template %q save %v goal-after-plan %v goal %q", pipeline.Template, pipeline.SaveVersion, pipeline.GoalAfterPlan, pipeline.Goal)
	}
	sess, err := f.store.Get(next)
	if err != nil {
		t.Fatal(err)
	}
	if sess.CurrentDir != p.Folder || sess.Isolation != session.IsolationOff || sess.Worktree != nil {
		t.Fatalf("change session: dir %q isolation %q worktree %v", sess.CurrentDir, sess.Isolation, sess.Worktree)
	}
	waitFor(t, "the change's plan gate", func() bool { return pipelineOf(t, f.store, next).OpenGate == focuspipeline.GatePlan })
	waitDriverIdle(t, d, next)
	if got := turns.seen(); len(got) != 2 || got[1] != goal {
		t.Fatalf("turns = %q", got)
	}
	// The change's pipeline is working now: a second change waits.
	if rec := focusRequestAs(t, h, http.MethodPost, changes, `{"request":"And bigger"}`, serverauth.RoleUser); rec.Code != http.StatusConflict {
		t.Fatalf("second change while busy = %d", rec.Code)
	}
}

func TestEasyChangeRefusals(t *testing.T) {
	easyFixture(t)
	f := newWorktreeFixture(t)
	workspace := f.store.WorkspaceDir()
	putEasyProject(t, workspace, easyproject.Project{ID: "0123456789abcdef", Name: "making", Status: easyproject.StatusCreating})
	// Ready, but its folder is gone and it has no pipeline: opening the
	// change's pipeline fails.
	putEasyProject(t, workspace, easyproject.Project{ID: "fedcba9876543210", Name: "gone", Status: easyproject.StatusReady,
		Folder: filepath.Join(t.TempDir(), "missing"), SessionID: "no-such-session"})

	noDriver := newFocusPipelineHandler(f.store, f.c, nil, zerolog.Nop())
	if rec := focusRequestAs(t, noDriver, http.MethodPost, "/v1/easy/projects/fedcba9876543210/changes", `{"request":"x"}`, serverauth.RoleUser); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("without a driver = %d", rec.Code)
	}

	d, _ := createFocusDriver(t, f.store, focusPlanReply)
	h := newFocusPipelineHandler(f.store, f.c, d, zerolog.Nop())
	for _, tc := range []struct {
		id, body string
		code     int
	}{
		{"0123456789abcdef", `{"request":"` + strings.Repeat("a", easyproject.MaxIdeaRunes+1) + `"}`, http.StatusBadRequest},
		{"0123456789abcdef", `{"request":"x"}`, http.StatusConflict},
		{"fedcba9876543210", `{"request":"x"}`, http.StatusInternalServerError},
	} {
		if rec := focusRequestAs(t, h, http.MethodPost, "/v1/easy/projects/"+tc.id+"/changes", tc.body, serverauth.RoleUser); rec.Code != tc.code {
			t.Fatalf("%s %.40s = %d %s", tc.id, tc.body, rec.Code, rec.Body.String())
		}
	}
	if p := getEasyProject(t, h, "fedcba9876543210"); len(p.Changes) != 0 || p.SessionID != "no-such-session" {
		t.Fatalf("a failed change was recorded: %+v", p)
	}
}

func TestEasyChangeReady(t *testing.T) {
	done := focuspipeline.Pipeline{
		Stages: []focuspipeline.Stage{{ID: "plan", Status: focuspipeline.StatusDone}, {ID: "make", Status: focuspipeline.StatusDone}},
		Plan:   &focuspipeline.Plan{Stages: []focuspipeline.StageID{"plan", "make"}},
	}
	if !easyChangeReady(done) {
		t.Fatal("a finished pipeline that saves nothing takes a change")
	}
	saving := done
	saving.SaveVersion = true
	if easyChangeReady(saving) {
		t.Fatal("a change waits until the finished version is saved")
	}
	saving.Saved = &focuspipeline.SavedVersion{Error: "save_failed"}
	if !easyChangeReady(saving) {
		t.Fatal("a failed save still frees the folder")
	}
	running := done
	running.Stages = []focuspipeline.Stage{{ID: "plan", Status: focuspipeline.StatusDone}, {ID: "make", Status: focuspipeline.StatusActive}}
	if easyChangeReady(running) {
		t.Fatal("a running pipeline takes no change")
	}
}
