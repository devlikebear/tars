package tarsserver

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/easyproject"
	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

type easyListResp struct {
	Projects []easyproject.Listed      `json:"projects"`
	Next     string                    `json:"next_cursor"`
	Counts   map[easyproject.State]int `json:"counts"`
	Archived int                       `json:"archived"`
}

type easyBoardResp struct {
	Project easyproject.Project `json:"project"`
	Board   easyproject.Board   `json:"board"`
}

func listEasy(t *testing.T, h http.Handler, query string) (int, easyListResp) {
	t.Helper()
	rec := focusRequestAs(t, h, http.MethodGet, "/v1/easy/projects"+query, "", serverauth.RoleUser)
	var out easyListResp
	if rec.Code == http.StatusOK {
		decodeInto(t, rec, &out)
	}
	return rec.Code, out
}

// easyPipelineSession makes a session with a webapp pipeline changed by
// edit, for a project to point at.
func easyPipelineSession(t *testing.T, store *session.Store, edit func(*focuspipeline.Pipeline)) string {
	t.Helper()
	sess, err := store.Create("easy")
	if err != nil {
		t.Fatal(err)
	}
	p := focuspipeline.NewFromTemplate(sess.ID, "A page", focuspipeline.WebAppTemplate(), time.Now())
	p.SaveVersion = true
	if edit != nil {
		edit(&p)
	}
	if err := focusStoreFor(store).Save(p); err != nil {
		t.Fatal(err)
	}
	return sess.ID
}

func TestEasyProjectListBoard(t *testing.T) {
	f := newWorktreeFixture(t)
	workspace := f.store.WorkspaceDir()
	h := newFocusPipelineHandler(f.store, f.c, nil, zerolog.Nop())

	planGate := easyPipelineSession(t, f.store, func(p *focuspipeline.Pipeline) {
		p.OpenGate = focuspipeline.GatePlan
		p.Plan = &focuspipeline.Plan{Goal: "A page"}
	})
	working := easyPipelineSession(t, f.store, nil)
	done := easyPipelineSession(t, f.store, nil)
	finishPipeline(t, f.store, done)

	// 30 projects: 10 waiting on the person, 10 at work, 9 done, 1 starting.
	t0 := time.Now().Add(-time.Hour)
	for i := range 30 {
		p := easyproject.Project{ID: fmt.Sprintf("%016x", i+1), Name: fmt.Sprintf("p%d", i), Status: easyproject.StatusReady,
			CreatedAt: t0.Add(time.Duration(i) * time.Minute)}
		p.UpdatedAt = p.CreatedAt
		switch {
		case i == 29:
			p.Status = easyproject.StatusCreating
		case i%3 == 0:
			p.SessionID = planGate
		case i%3 == 1:
			p.SessionID = working
		default:
			p.SessionID = done
		}
		putEasyProject(t, workspace, p)
	}

	code, all := listEasy(t, h, "")
	if code != http.StatusOK || len(all.Projects) != 30 || all.Next != "" {
		t.Fatalf("list = %d, %d projects, next %q", code, len(all.Projects), all.Next)
	}
	want := map[easyproject.State]int{easyproject.StateYourTurn: 10, easyproject.StateWorking: 10, easyproject.StateDone: 9, easyproject.StateStarting: 1}
	for state, n := range want {
		if all.Counts[state] != n {
			t.Fatalf("counts = %v, want %v", all.Counts, want)
		}
	}
	// The pipelines changed after the records: the order follows the
	// latest change, newest first, and the project fields stay where the
	// console reads them.
	first := all.Projects[0]
	if first.ID == "" || first.Name == "" || first.Board.ChangedAt.IsZero() {
		t.Fatalf("first item = %+v", first)
	}
	for i := 1; i < len(all.Projects); i++ {
		if all.Projects[i].Board.ChangedAt.After(all.Projects[i-1].Board.ChangedAt) {
			t.Fatalf("not newest first at %d", i)
		}
	}
	for _, l := range all.Projects {
		if l.SessionID == planGate && l.Board.Waiting != easyproject.WaitingPlan {
			t.Fatalf("plan gate project board = %+v", l.Board)
		}
	}

	// Paging walks the whole board once.
	seen := map[string]bool{}
	cursor := ""
	for range 10 {
		q := "?limit=8"
		if cursor != "" {
			q += "&cursor=" + cursor
		}
		code, page := listEasy(t, h, q)
		if code != http.StatusOK || page.Counts[easyproject.StateDone] != 9 {
			t.Fatalf("page = %d %v", code, page.Counts)
		}
		for _, l := range page.Projects {
			seen[l.ID] = true
		}
		if cursor = page.Next; cursor == "" {
			break
		}
	}
	if len(seen) != 30 {
		t.Fatalf("paged %d projects", len(seen))
	}

	if code, page := listEasy(t, h, "?state=your_turn&limit=3"); code != http.StatusOK || len(page.Projects) != 3 || page.Next == "" {
		t.Fatalf("your_turn page = %d %d %q", code, len(page.Projects), page.Next)
	}
	for _, q := range []string{"?state=nope", "?limit=0", "?limit=x", "?cursor=!!"} {
		if code, _ := listEasy(t, h, q); code != http.StatusBadRequest {
			t.Fatalf("%s = %d", q, code)
		}
	}

	var one easyBoardResp
	decodeInto(t, focusRequestAs(t, h, http.MethodGet, "/v1/easy/projects/"+fmt.Sprintf("%016x", 3), "", serverauth.RoleUser), &one)
	if one.Board.State != easyproject.StateDone {
		t.Fatalf("get board = %+v", one.Board)
	}
}

func TestEasyProjectRenameAndArchive(t *testing.T) {
	previewScript(t, "sleep 30")
	f := newWorktreeFixture(t)
	workspace := f.store.WorkspaceDir()
	d, _ := createFocusDriver(t, f.store, focusPlanReply)
	h := newFocusPipelineHandler(f.store, f.c, d, zerolog.Nop())
	done := easyPipelineSession(t, f.store, nil)
	finishPipeline(t, f.store, done)
	working := easyPipelineSession(t, f.store, nil)
	idle := easyproject.Project{ID: "0123456789abcdef", Name: "shop", Folder: t.TempDir(), Status: easyproject.StatusReady, SessionID: done}
	busy := easyproject.Project{ID: "fedcba9876543210", Name: "blog", Folder: t.TempDir(), Status: easyproject.StatusReady, SessionID: working}
	putEasyProject(t, workspace, idle)
	putEasyProject(t, workspace, busy)
	path := "/v1/easy/projects/" + idle.ID

	for _, tc := range []struct {
		body, role string
		code       int
	}{
		{`{}`, serverauth.RoleUser, http.StatusBadRequest},
		{`{"name":"   "}`, serverauth.RoleUser, http.StatusBadRequest},
		{`{"name":"` + strings.Repeat("x", 61) + `"}`, serverauth.RoleUser, http.StatusBadRequest},
		{`{"name":"x"}`, serverauth.RoleFocus, http.StatusForbidden},
	} {
		if rec := focusRequestAs(t, h, http.MethodPatch, path, tc.body, tc.role); rec.Code != tc.code {
			t.Fatalf("%s as %s = %d %s", tc.body, tc.role, rec.Code, rec.Body.String())
		}
	}
	if rec := focusRequestAs(t, h, http.MethodPatch, "/v1/easy/projects/0000000000000000", `{"name":"x"}`, serverauth.RoleUser); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown = %d", rec.Code)
	}

	var out easyBoardResp
	rec := focusRequestAs(t, h, http.MethodPatch, path, `{"name":"  My   shop "}`, serverauth.RoleUser)
	decodeInto(t, rec, &out)
	if rec.Code != http.StatusOK || out.Project.Name != "My shop" || out.Board.State != easyproject.StateDone {
		t.Fatalf("rename = %d %+v", rec.Code, out)
	}

	// Archiving a project the team works on is refused; renaming it is not.
	if rec := focusRequestAs(t, h, http.MethodPatch, "/v1/easy/projects/"+busy.ID, `{"archived":true}`, serverauth.RoleUser); rec.Code != http.StatusConflict {
		t.Fatalf("archive busy = %d", rec.Code)
	}
	if rec := focusRequestAs(t, h, http.MethodPatch, "/v1/easy/projects/"+busy.ID, `{"name":"Blog"}`, serverauth.RoleUser); rec.Code != http.StatusOK {
		t.Fatalf("rename busy = %d", rec.Code)
	}

	// Archiving stops the preview and moves the project off the board.
	if code, p := previewOf(t, h, http.MethodPost, idle.ID, serverauth.RoleUser); code != http.StatusOK || p.State != easyproject.PreviewStarting {
		t.Fatalf("preview start = %d %+v", code, p)
	}
	rec = focusRequestAs(t, h, http.MethodPatch, path, `{"archived":true}`, serverauth.RoleUser)
	decodeInto(t, rec, &out)
	if rec.Code != http.StatusOK || out.Project.ArchivedAt == nil {
		t.Fatalf("archive = %d %+v", rec.Code, out.Project)
	}
	if code, p := previewOf(t, h, http.MethodGet, idle.ID, serverauth.RoleUser); code != http.StatusOK || p.State != easyproject.PreviewStopped {
		t.Fatalf("preview after archive = %d %+v", code, p)
	}
	_, board := listEasy(t, h, "")
	if len(board.Projects) != 1 || board.Projects[0].ID != busy.ID || board.Archived != 1 {
		t.Fatalf("board after archive = %+v", board)
	}
	_, archived := listEasy(t, h, "?archived=1")
	if len(archived.Projects) != 1 || archived.Projects[0].ID != idle.ID || archived.Counts[easyproject.StateDone] != 1 {
		t.Fatalf("archive list = %+v", archived)
	}

	rec = focusRequestAs(t, h, http.MethodPatch, path, `{"archived":false}`, serverauth.RoleUser)
	out = easyBoardResp{}
	decodeInto(t, rec, &out)
	if rec.Code != http.StatusOK || out.Project.ArchivedAt != nil {
		t.Fatalf("restore = %d %+v", rec.Code, out.Project)
	}
	if _, board := listEasy(t, h, ""); len(board.Projects) != 2 || board.Archived != 0 {
		t.Fatalf("board after restore = %+v", board)
	}
}

func TestEasyProjectDelete(t *testing.T) {
	base := easyFixture(t)
	f := newWorktreeFixture(t)
	workspace := f.store.WorkspaceDir()
	h := newFocusPipelineHandler(f.store, f.c, nil, zerolog.Nop())
	done := easyPipelineSession(t, f.store, nil)
	finishPipeline(t, f.store, done)
	working := easyPipelineSession(t, f.store, nil)

	folder := filepath.Join(base, "shop")
	if err := os.MkdirAll(filepath.Join(folder, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "keep.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	projects := []easyproject.Project{
		{ID: "0000000000000001", Name: "shop", Folder: folder, Status: easyproject.StatusReady, SessionID: done},
		{ID: "0000000000000002", Name: "busy", Folder: filepath.Join(base, "busy"), Status: easyproject.StatusReady, SessionID: working},
		{ID: "0000000000000003", Name: "making", Folder: filepath.Join(base, "making"), Status: easyproject.StatusCreating},
		// A record pointing outside the projects folder (an edited file,
		// or a projects folder moved since): never touched.
		{ID: "0000000000000004", Name: "elsewhere", Folder: outside, Status: easyproject.StatusReady, SessionID: done},
		{ID: "0000000000000005", Name: "failed", Status: easyproject.StatusFailed, ErrorCode: easyproject.ErrorNodeMissing},
	}
	for _, p := range projects {
		putEasyProject(t, workspace, p)
	}
	del := func(id, role string) int {
		return focusRequestAs(t, h, http.MethodDelete, "/v1/easy/projects/"+id, "", role).Code
	}

	if code := del("0000000000000001", serverauth.RoleFocus); code != http.StatusForbidden {
		t.Fatalf("phone = %d", code)
	}
	for _, id := range []string{"0000000000000002", "0000000000000003"} {
		if code := del(id, serverauth.RoleUser); code != http.StatusConflict {
			t.Fatalf("delete while working %s = %d", id, code)
		}
	}
	if code := del("0000000000000004", serverauth.RoleUser); code != http.StatusConflict {
		t.Fatalf("delete outside = %d", code)
	}
	if raw, err := os.ReadFile(filepath.Join(outside, "keep.txt")); err != nil || string(raw) != "mine" {
		t.Fatalf("outside folder touched: %v", err)
	}
	if _, err := easyproject.NewStore(workspace).Get("0000000000000004"); err != nil {
		t.Fatalf("refused project's record removed: %v", err)
	}

	rec := focusRequestAs(t, h, http.MethodDelete, "/v1/easy/projects/0000000000000001", "", serverauth.RoleUser)
	var out struct {
		DeletedTo string `json:"deleted_to"`
	}
	decodeInto(t, rec, &out)
	if rec.Code != http.StatusOK || filepath.Dir(out.DeletedTo) != filepath.Join(base, easyproject.DeletedDirName) {
		t.Fatalf("delete = %d %+v", rec.Code, out)
	}
	if _, err := os.Stat(filepath.Join(out.DeletedTo, "src")); err != nil {
		t.Fatalf("moved folder: %v", err)
	}
	if _, err := os.Stat(folder); !os.IsNotExist(err) {
		t.Fatalf("folder left in place: %v", err)
	}
	if code := del("0000000000000001", serverauth.RoleUser); code != http.StatusNotFound {
		t.Fatalf("delete twice = %d", code)
	}
	// A failed project has no folder: only the record goes.
	if code := del("0000000000000005", serverauth.RoleUser); code != http.StatusOK {
		t.Fatalf("delete failed project = %d", code)
	}
	_, board := listEasy(t, h, "")
	if len(board.Projects) != 3 {
		t.Fatalf("board after deletes = %d projects", len(board.Projects))
	}
}
