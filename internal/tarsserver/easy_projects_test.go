package tarsserver

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/easyproject"
	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/rs/zerolog"
)

// easyFixture points project creation at a temp folder and a stand-in
// scaffolder that writes a package.json and a .gitignore.
func easyFixture(t *testing.T) (base string) {
	t.Helper()
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base = filepath.Join(tmp, "TARS Projects")
	t.Setenv(easyproject.BaseDirEnv, base)
	script := filepath.Join(t.TempDir(), "scaffold.sh")
	body := "#!/bin/sh\necho '{\"name\":\"app\"}' > \"$1/package.json\"\necho node_modules > \"$1/.gitignore\"\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(easyproject.ScaffoldPathEnv, script)
	return base
}

type easyProjectResp struct {
	Project easyproject.Project `json:"project"`
}

func getEasyProject(t *testing.T, h http.Handler, id string) easyproject.Project {
	t.Helper()
	var out easyProjectResp
	rec := focusRequestAs(t, h, http.MethodGet, "/v1/easy/projects/"+id, "", serverauth.RoleUser)
	decodeInto(t, rec, &out)
	return out.Project
}

func TestEasyProjectFromOneSentence(t *testing.T) {
	base := easyFixture(t)
	f := newWorktreeFixture(t)
	d, turns := createFocusDriver(t, f.store, focusPlanReply)
	h := newFocusPipelineHandler(f.store, f.c, d, zerolog.Nop())

	idea := "A sign-up page for a book club"
	rec := focusRequestAs(t, h, http.MethodPost, "/v1/easy/projects", `{"idea":"  `+idea+`  "}`, serverauth.RoleUser)
	var created easyProjectResp
	decodeInto(t, rec, &created)
	if rec.Code != http.StatusAccepted || created.Project.Status != easyproject.StatusCreating ||
		created.Project.Folder != filepath.Join(base, "a-sign-up-page-for-a-book-club") {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}

	var p easyproject.Project
	waitFor(t, "the project to be ready", func() bool {
		p = getEasyProject(t, h, created.Project.ID)
		return p.Status != easyproject.StatusCreating
	})
	if p.Status != easyproject.StatusReady || p.SessionID == "" {
		t.Fatalf("project = %+v", p)
	}
	if _, err := os.Stat(filepath.Join(p.Folder, ".git")); err != nil {
		t.Fatalf("no version control: %v", err)
	}

	pipeline := pipelineOf(t, f.store, p.SessionID)
	if pipeline.Template != focuspipeline.WebAppTemplateID || !pipeline.WebUI || !pipeline.SaveVersion || pipeline.Goal != idea {
		t.Fatalf("pipeline: template %q web ui %v goal %q", pipeline.Template, pipeline.WebUI, pipeline.Goal)
	}
	sess, err := f.store.Get(p.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if sess.CurrentDir != p.Folder {
		t.Fatalf("the session works in %q, not the project folder", sess.CurrentDir)
	}
	// The server sends the first turn: nobody else has a chat turn to send.
	waitFor(t, "the plan gate", func() bool { return pipelineOf(t, f.store, p.SessionID).OpenGate == focuspipeline.GatePlan })
	waitDriverIdle(t, d, p.SessionID)
	if got := turns.seen(); len(got) != 1 || got[0] != idea {
		t.Fatalf("turns = %q", got)
	}

	// The list holds it, newest first.
	rec = focusRequestAs(t, h, http.MethodPost, "/v1/easy/projects", `{"idea":"A second idea"}`, serverauth.RoleUser)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("second create = %d %s", rec.Code, rec.Body.String())
	}
	var listed struct {
		Projects []easyproject.Project `json:"projects"`
	}
	waitFor(t, "both projects", func() bool {
		decodeInto(t, focusRequestAs(t, h, http.MethodGet, "/v1/easy/projects", "", serverauth.RoleUser), &listed)
		return len(listed.Projects) == 2
	})
	if listed.Projects[1].ID != p.ID {
		t.Fatalf("list order = %+v", listed.Projects)
	}
	waitFor(t, "the second project to settle", func() bool {
		return getEasyProject(t, h, listed.Projects[0].ID).Status != easyproject.StatusCreating
	})
	waitDriverIdle(t, d, getEasyProject(t, h, listed.Projects[0].ID).SessionID)
}

func TestEasyProjectRefusals(t *testing.T) {
	easyFixture(t)
	f := newWorktreeFixture(t)
	d, _ := createFocusDriver(t, f.store, focusPlanReply)
	h := newFocusPipelineHandler(f.store, f.c, d, zerolog.Nop())

	for _, tc := range []struct {
		name, body, role, code string
		status                 int
	}{
		{"empty idea", `{"idea":"   "}`, serverauth.RoleUser, "idea_required", http.StatusBadRequest},
		{"long idea", `{"idea":"` + strings.Repeat("a", easyproject.MaxIdeaRunes+1) + `"}`, serverauth.RoleUser, "idea_too_long", http.StatusBadRequest},
		{"phone, empty idea", `{"idea":""}`, serverauth.RoleFocus, "idea_required", http.StatusBadRequest},
	} {
		rec := focusRequestAs(t, h, http.MethodPost, "/v1/easy/projects", tc.body, tc.role)
		var body map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.code) {
			t.Errorf("%s: %d %s", tc.name, rec.Code, rec.Body.String())
		}
	}
	if rec := focusRequestAs(t, h, http.MethodGet, "/v1/easy/projects/0123456789abcdef", "", serverauth.RoleUser); rec.Code != http.StatusNotFound {
		t.Errorf("unknown project = %d", rec.Code)
	}

	// No npm (here: the stand-in is gone): refused before a folder is made.
	t.Setenv(easyproject.ScaffoldPathEnv, filepath.Join(t.TempDir(), "missing"))
	h = newFocusPipelineHandler(f.store, f.c, d, zerolog.Nop())
	rec := focusRequestAs(t, h, http.MethodPost, "/v1/easy/projects", `{"idea":"a page"}`, serverauth.RoleUser)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), easyproject.ErrorNodeMissing) {
		t.Fatalf("no node = %d %s", rec.Code, rec.Body.String())
	}

	// A server that cannot send turns says so.
	h = newFocusPipelineHandler(f.store, f.c, nil, zerolog.Nop())
	rec = focusRequestAs(t, h, http.MethodPost, "/v1/easy/projects", `{"idea":"a page"}`, serverauth.RoleUser)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "cannot_run") {
		t.Fatalf("no driver = %d %s", rec.Code, rec.Body.String())
	}
}

// A creation cut off by a restart reads as failed afterwards, never as
// creating forever.
func TestEasyProjectInterruptedByARestart(t *testing.T) {
	easyFixture(t)
	f := newWorktreeFixture(t)
	store := easyproject.NewStore(f.store.WorkspaceDir())
	stale := easyproject.Project{ID: "0123456789abcdef", Name: "x", Status: easyproject.StatusCreating, CreatedAt: time.Now()}
	if err := store.Put(stale); err != nil {
		t.Fatal(err)
	}
	h := newFocusPipelineHandler(f.store, f.c, nil, zerolog.Nop())
	if p := getEasyProject(t, h, stale.ID); p.Status != easyproject.StatusFailed || p.ErrorCode != easyproject.ErrorInterrupted {
		t.Fatalf("after restart: %+v", p)
	}
}

// A pipeline that cannot be opened fails the project with a code, and the
// half-made folder is gone.
func TestEasyProjectPipelineFailure(t *testing.T) {
	base := easyFixture(t)
	f := newWorktreeFixture(t)
	d, _ := createFocusDriver(t, f.store, focusPlanReply)
	api := &focusAPI{sessions: f.store, worktrees: nil, driver: d, logger: zerolog.Nop(), now: time.Now}
	mux := http.NewServeMux()
	registerEasyProjects(mux, api, zerolog.Nop())

	rec := focusRequestAs(t, mux, http.MethodPost, "/v1/easy/projects", `{"idea":"a page"}`, serverauth.RoleUser)
	var created easyProjectResp
	decodeInto(t, rec, &created)
	var p easyproject.Project
	waitFor(t, "the project to fail", func() bool {
		p = getEasyProject(t, mux, created.Project.ID)
		return p.Status != easyproject.StatusCreating
	})
	if p.Status != easyproject.StatusFailed || p.ErrorCode != easyproject.ErrorPipelineFailed {
		t.Fatalf("project = %+v", p)
	}
	if entries, _ := os.ReadDir(base); len(entries) != 0 {
		t.Fatalf("left behind: %v", entries)
	}
}
