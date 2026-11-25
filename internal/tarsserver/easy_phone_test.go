package tarsserver

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/easyproject"
	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/rs/zerolog"
)

// Vibe redesign unit 6: a paired phone makes a project from one sentence
// and approves its plan, and nothing in the request lets it choose a
// folder, a template, a tier or an opt-in.
func TestEasyProjectFromAPhone(t *testing.T) {
	base := easyFixture(t)
	f := newWorktreeFixture(t)
	d, _ := createFocusDriver(t, f.store, easyPlanReply)
	h := newFocusPipelineHandler(f.store, f.c, d, zerolog.Nop())
	phone := asRole(h, serverauth.RoleFocus)

	outside := t.TempDir()
	body := `{"idea":"A recipe box","cwd":"` + filepath.ToSlash(outside) + `","folder":"` + filepath.ToSlash(outside) +
		`","template":"dev","goal_mode":true,"e2e":true,"exec_tier":"heavy","harness_proposal":true,"tier_recommendation":{"tier":"heavy","pin":true}}`
	rec := sendTo(t, phone, http.MethodPost, "/v1/easy/projects", body)
	var created easyProjectResp
	decodeInto(t, rec, &created)
	if rec.Code != http.StatusAccepted || created.Project.Folder != filepath.Join(base, "a-recipe-box") {
		t.Fatalf("phone create = %d %s", rec.Code, rec.Body.String())
	}
	var p easyproject.Project
	waitFor(t, "the project to be ready", func() bool {
		p = getEasyProject(t, phone, created.Project.ID)
		return p.Status == easyproject.StatusReady
	})
	pipeline := pipelineOf(t, f.store, p.SessionID)
	if pipeline.Template != focuspipeline.WebAppTemplateID || pipeline.GoalMode != nil || pipeline.E2E ||
		pipeline.ExecTier != nil || pipeline.Harness != nil || !pipeline.GoalAfterPlan {
		t.Fatalf("a phone's project pipeline: template %q goal %+v e2e %v tier %+v harness %v", pipeline.Template, pipeline.GoalMode, pipeline.E2E, pipeline.ExecTier, pipeline.Harness)
	}
	if sess, _ := f.store.Get(p.SessionID); sess.CurrentDir != p.Folder || sess.TierPin != "" {
		t.Fatalf("session dir %q tier pin %q", sess.CurrentDir, sess.TierPin)
	}

	// The project's pipeline session is one the phone reaches through the
	// session routes (turn feed, chat turn, approvals).
	if ok, err := newFocusDeviceScope(f.store).allows(httptest.NewRequest(http.MethodGet, "/", nil), p.SessionID, true); !ok || err != nil {
		t.Fatalf("easy pipeline session in the phone's scope: %v %v", ok, err)
	}

	// The list and the project read the same for the phone.
	rec = sendTo(t, phone, http.MethodGet, "/v1/easy/projects", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), p.ID) {
		t.Fatalf("phone list = %d %s", rec.Code, rec.Body.String())
	}

	// The phone approves the plan: goal mode starts, and the answer is
	// recorded as the phone's.
	waitFor(t, "the plan gate", func() bool { return pipelineOf(t, f.store, p.SessionID).OpenGate == focuspipeline.GatePlan })
	waitDriverIdle(t, d, p.SessionID)
	rec = sendTo(t, phone, http.MethodPost, "/v1/focus/pipelines/"+p.SessionID+"/gates/plan", `{"action":"approve"}`)
	var approved struct {
		Pipeline focuspipeline.Pipeline `json:"pipeline"`
	}
	decodeInto(t, rec, &approved)
	if rec.Code != http.StatusOK || !approved.Pipeline.GoalActive() {
		t.Fatalf("phone approve = %d goal %+v", rec.Code, approved.Pipeline.GoalMode)
	}
	by := ""
	for _, c := range approved.Pipeline.Cards {
		if c.Kind == focuspipeline.CardGate && c.State == focuspipeline.CardDecided {
			by = c.DecidedBy
		}
	}
	if by != focuspipeline.AnswerPhone {
		t.Fatalf("plan approval recorded as %q", by)
	}
	// Turning goal mode off from the phone stops the run (and the driver's
	// watcher with it) before the test ends.
	if rec := sendTo(t, phone, http.MethodPost, "/v1/focus/pipelines/"+p.SessionID+"/goal", `{"enabled":false}`); rec.Code != http.StatusOK {
		t.Fatalf("goal off = %d %s", rec.Code, rec.Body.String())
	}
	waitDriverIdle(t, d, p.SessionID)
}

// A phone reaches easy projects by their own ids only: a session id is not
// a project id, the preview and deleting stay refused, and the routes do
// not open a plain session to the phone.
func TestEasyPhoneScope(t *testing.T) {
	easyFixture(t)
	f := newWorktreeFixture(t)
	h := newFocusPipelineHandler(f.store, f.c, nil, zerolog.Nop())
	phone := asRole(h, serverauth.RoleFocus)

	plain, err := f.store.Create("plain chat")
	if err != nil {
		t.Fatal(err)
	}
	focusTask := plannedFocusSession(t, f.store)
	const id = "0123456789abcdef"
	putEasyProject(t, f.store.WorkspaceDir(), easyproject.Project{ID: id, Name: "x", Folder: t.TempDir(), Status: easyproject.StatusReady})

	for _, tc := range []struct {
		method, path string
		code         int
	}{
		{http.MethodGet, "/v1/easy/projects/" + plain.ID, http.StatusNotFound},
		{http.MethodGet, "/v1/easy/projects/" + focusTask.ID, http.StatusNotFound},
		{http.MethodPatch, "/v1/easy/projects/" + plain.ID, http.StatusNotFound},
		{http.MethodGet, "/v1/easy/projects/" + focusTask.ID + "/versions", http.StatusNotFound},
		{http.MethodGet, "/v1/easy/projects/" + id + "/preview", http.StatusForbidden},
		{http.MethodPost, "/v1/easy/projects/" + id + "/preview", http.StatusForbidden},
		{http.MethodDelete, "/v1/easy/projects/" + id, http.StatusForbidden},
	} {
		body := ""
		switch tc.method {
		case http.MethodPatch:
			body = `{"name":"y"}`
		case http.MethodPost:
			body = `{"request":"y"}`
		}
		if rec := sendTo(t, phone, tc.method, tc.path, body); rec.Code != tc.code {
			t.Errorf("phone %s %s = %d %s, want %d", tc.method, tc.path, rec.Code, rec.Body.String(), tc.code)
		}
	}
	if ok, _ := newFocusDeviceScope(f.store).allows(httptest.NewRequest(http.MethodGet, "/", nil), plain.ID, false); ok {
		t.Fatalf("a plain session must stay out of the phone's scope")
	}
	// Still there: the phone's delete moved nothing.
	if got := getEasyProject(t, h, id); got.ID != id {
		t.Fatalf("project after a phone's delete: %+v", got)
	}
}
