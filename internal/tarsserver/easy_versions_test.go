package tarsserver

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/easyproject"
	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/rs/zerolog"
)

type easyVersionsResp struct {
	Version  easyproject.Version   `json:"version"`
	Versions []easyproject.Version `json:"versions"`
}

func TestEasyVersionsListAndGoBack(t *testing.T) {
	ctx := context.Background()
	f := newWorktreeFixture(t)
	dir := t.TempDir()
	write := func(body string) {
		if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("empty\n")
	if err := easyproject.InitGit(ctx, dir); err != nil {
		t.Fatal(err)
	}
	write("sign-up\n")
	if _, err := easyproject.SaveVersion(ctx, dir, "A sign-up page"); err != nil {
		t.Fatal(err)
	}
	write("green\n")
	if _, err := easyproject.SaveVersion(ctx, dir, "Make the button green"); err != nil {
		t.Fatal(err)
	}
	const id = "0123456789abcdef"
	// Its pipeline is still working: going back waits.
	running := focuspipeline.Pipeline{Version: 1, SessionID: "busy-session",
		Stages: []focuspipeline.Stage{{ID: "plan", Status: focuspipeline.StatusActive}}}
	if err := focusStoreFor(f.store).Save(running); err != nil {
		t.Fatal(err)
	}
	putEasyProject(t, f.store.WorkspaceDir(), easyproject.Project{ID: id, Name: "x", Folder: dir, Status: easyproject.StatusReady, SessionID: "busy-session"})
	putEasyProject(t, f.store.WorkspaceDir(), easyproject.Project{ID: "fedcba9876543210", Name: "y", Status: easyproject.StatusCreating})
	h := newFocusPipelineHandler(f.store, f.c, nil, zerolog.Nop())

	var listed easyVersionsResp
	rec := focusRequestAs(t, h, http.MethodGet, "/v1/easy/projects/"+id+"/versions", "", serverauth.RoleUser)
	decodeInto(t, rec, &listed)
	if rec.Code != http.StatusOK || len(listed.Versions) != 3 || !listed.Versions[0].Current ||
		listed.Versions[1].Title != "A sign-up page" || listed.Versions[2].Kind != easyproject.VersionStart {
		t.Fatalf("versions = %d %+v", rec.Code, listed.Versions)
	}
	signUp := listed.Versions[1].Commit
	restore := "/v1/easy/projects/" + id + "/versions/" + signUp + "/restore"

	for _, tc := range []struct {
		method, path, role string
		code               int
	}{
		{http.MethodPost, restore, serverauth.RoleUser, http.StatusConflict},
		{http.MethodPost, restore, serverauth.RoleFocus, http.StatusForbidden},
		{http.MethodGet, "/v1/easy/projects/0000000000000000/versions", serverauth.RoleUser, http.StatusNotFound},
		{http.MethodGet, "/v1/easy/projects/fedcba9876543210/versions", serverauth.RoleUser, http.StatusConflict},
	} {
		if rec := focusRequestAs(t, h, tc.method, tc.path, "", tc.role); rec.Code != tc.code {
			t.Fatalf("%s %s as %s = %d %s", tc.method, tc.path, tc.role, rec.Code, rec.Body.String())
		}
	}

	finishPipeline(t, f.store, "busy-session")
	bogus := "/v1/easy/projects/" + id + "/versions/" + "0000000000000000000000000000000000000000" + "/restore"
	if rec := focusRequestAs(t, h, http.MethodPost, bogus, "", serverauth.RoleUser); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown version = %d", rec.Code)
	}
	var back easyVersionsResp
	rec = focusRequestAs(t, h, http.MethodPost, restore, "", serverauth.RoleUser)
	decodeInto(t, rec, &back)
	if rec.Code != http.StatusOK || back.Version.Kind != easyproject.VersionRestored || back.Version.Title != "A sign-up page" ||
		len(back.Versions) != 4 || back.Versions[0].Commit != back.Version.Commit {
		t.Fatalf("restore = %d %+v", rec.Code, back)
	}
	if raw, _ := os.ReadFile(filepath.Join(dir, "index.html")); string(raw) != "sign-up\n" {
		t.Fatalf("index.html = %q", raw)
	}

	// A git step that fails is a 500 with a fixed code, and nothing moved.
	if err := os.WriteFile(filepath.Join(dir, ".git", "index.lock"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	green := back.Versions[2].Commit
	rec = focusRequestAs(t, h, http.MethodPost, "/v1/easy/projects/"+id+"/versions/"+green+"/restore", "", serverauth.RoleUser)
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "restore_failed") {
		t.Fatalf("restore with git failing = %d %s", rec.Code, rec.Body.String())
	}
}

// A project without a pipeline record and a folder without history.
func TestEasyVersionsOfAFolderWithoutHistory(t *testing.T) {
	f := newWorktreeFixture(t)
	putEasyProject(t, f.store.WorkspaceDir(), easyproject.Project{ID: "0123456789abcdef", Name: "x", Folder: t.TempDir(), Status: easyproject.StatusReady})
	h := newFocusPipelineHandler(f.store, f.c, nil, zerolog.Nop())
	if rec := focusRequestAs(t, h, http.MethodGet, "/v1/easy/projects/0123456789abcdef/versions", "", serverauth.RoleUser); rec.Code != http.StatusInternalServerError {
		t.Fatalf("versions = %d", rec.Code)
	}
	restore := "/v1/easy/projects/0123456789abcdef/versions/" + strings.Repeat("a", 40) + "/restore"
	if rec := focusRequestAs(t, h, http.MethodPost, restore, "", serverauth.RoleUser); rec.Code != http.StatusNotFound {
		t.Fatalf("restore = %d", rec.Code)
	}
}
