package tarsserver

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/apihandlers"
	"github.com/devlikebear/tars/internal/consoleauth"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/rs/zerolog"
)

func focusRequestAs(t *testing.T, h http.Handler, method, path, body, role string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if role != "" {
		req = req.WithContext(serverauth.WithRoleContext(req.Context(), role))
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func registerDeviceFolders(t *testing.T, f *worktreeFixture, folders ...string) {
	t.Helper()
	if _, err := consoleauth.NewStore(f.store.WorkspaceDir()).SetDeviceFolders(folders); err != nil {
		t.Fatalf("SetDeviceFolders: %v", err)
	}
}

func TestFocusDeviceCreatesInARegisteredFolder(t *testing.T) {
	f := newWorktreeFixture(t)
	d, turns := createFocusDriver(t, f.store, focusPlanReply)
	h := newFocusPipelineHandler(f.store, f.c, d, zerolog.Nop())
	body := `{"goal":"idea from the train","cwd":` + jsonString(f.repo) + `}`

	if rec := focusRequestAs(t, h, http.MethodPost, "/v1/focus/pipelines", body, serverauth.RoleFocus); rec.Code != http.StatusForbidden {
		t.Fatalf("unregistered folder: %d %s", rec.Code, rec.Body.String())
	}
	if list, _ := f.store.List(); len(list) != 0 {
		t.Fatalf("a refused create left sessions: %+v", list)
	}

	registerDeviceFolders(t, f, f.repo)
	rec := focusRequestAs(t, h, http.MethodPost, "/v1/focus/pipelines", body, serverauth.RoleFocus)
	var out focusCreated
	decodeInto(t, rec, &out)
	if rec.Code != http.StatusCreated || !out.Started {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	// The phone sends no chat turn of its own: the server sends the first one.
	waitFor(t, "the first turn", func() bool { return len(turns.seen()) == 1 })
	waitDriverIdle(t, d, out.SessionID)
	if got := turns.seen(); got[0] != "idea from the train" {
		t.Fatalf("first turn = %q", got)
	}
}

func TestFocusDeviceCreateRefusesAdminOptions(t *testing.T) {
	f := newWorktreeFixture(t)
	d, _ := createFocusDriver(t, f.store, focusPlanReply)
	h := newFocusPipelineHandler(f.store, f.c, d, zerolog.Nop())
	registerDeviceFolders(t, f, f.repo)
	cwd := `"cwd":` + jsonString(f.repo)

	for _, extra := range []string{
		`"goal_mode":true`,
		`"kind":"release"`,
		`"e2e":true`,
		`"harness_proposal":true`,
		`"verify_promotion":true`,
		`"extra_dirs":[` + jsonString(t.TempDir()) + `]`,
	} {
		body := `{"goal":"g",` + cwd + `,` + extra + `}`
		if rec := focusRequestAs(t, h, http.MethodPost, "/v1/focus/pipelines", body, serverauth.RoleFocus); rec.Code != http.StatusForbidden {
			t.Fatalf("%s: %d %s", extra, rec.Code, rec.Body.String())
		}
	}
	if list, _ := f.store.List(); len(list) != 0 {
		t.Fatalf("refused creates left sessions: %+v", list)
	}

	// A registered extra folder is fine.
	extra := t.TempDir()
	registerDeviceFolders(t, f, f.repo, extra)
	body := `{"goal":"g",` + cwd + `,"extra_dirs":[` + jsonString(extra) + `]}`
	if rec := focusRequestAs(t, h, http.MethodPost, "/v1/focus/pipelines", body, serverauth.RoleFocus); rec.Code != http.StatusCreated {
		t.Fatalf("registered extra folder: %d %s", rec.Code, rec.Body.String())
	}
}

func TestFocusDeviceCreateOtherRoles(t *testing.T) {
	f := newWorktreeFixture(t)
	h := newFocusPipelineHandler(f.store, f.c, nil, zerolog.Nop())
	registerDeviceFolders(t, f, f.repo)
	body := `{"goal":"g","cwd":` + jsonString(f.repo) + `}`
	// Registering a folder for phones does not open it to the user role.
	if rec := focusRequestAs(t, h, http.MethodPost, "/v1/focus/pipelines", body, serverauth.RoleUser); rec.Code != http.StatusForbidden {
		t.Fatalf("user role: %d", rec.Code)
	}
	// Without a driver a phone's create cannot send its first turn.
	if rec := focusRequestAs(t, h, http.MethodPost, "/v1/focus/pipelines", body, serverauth.RoleFocus); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("focus role without a driver: %d %s", rec.Code, rec.Body.String())
	}
}

func TestFocusDeviceCreateUnreadableFolders(t *testing.T) {
	f := newWorktreeFixture(t)
	d, _ := createFocusDriver(t, f.store, focusPlanReply)
	h := newFocusPipelineHandler(f.store, f.c, d, zerolog.Nop())
	authDir := filepath.Join(f.store.WorkspaceDir(), "auth")
	if err := os.MkdirAll(authDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, "device-folders.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	body := `{"goal":"g","cwd":` + jsonString(f.repo) + `}`
	if rec := focusRequestAs(t, h, http.MethodPost, "/v1/focus/pipelines", body, serverauth.RoleFocus); rec.Code != http.StatusInternalServerError {
		t.Fatalf("broken folders file on create: %d", rec.Code)
	}
	if rec := focusRequestAs(t, h, http.MethodGet, "/v1/focus/folders", "", serverauth.RoleFocus); rec.Code != http.StatusInternalServerError {
		t.Fatalf("broken folders file on list: %d", rec.Code)
	}
}

func TestFocusFoldersList(t *testing.T) {
	f := newWorktreeFixture(t)
	h := newFocusPipelineHandler(f.store, f.c, nil, zerolog.Nop())

	rec := focusRequestAs(t, h, http.MethodGet, "/v1/focus/folders", "", serverauth.RoleFocus)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"folders":[]`) {
		t.Fatalf("empty: %d %s", rec.Code, rec.Body.String())
	}

	gone := filepath.Join(t.TempDir(), "gone")
	registerDeviceFolders(t, f, f.repo, gone)
	rec = focusRequestAs(t, h, http.MethodGet, "/v1/focus/folders", "", serverauth.RoleFocus)
	var out struct {
		Folders []apihandlers.DeviceFolderView `json:"folders"`
	}
	decodeInto(t, rec, &out)
	if len(out.Folders) != 2 {
		t.Fatalf("folders = %+v", out.Folders)
	}
	if out.Folders[0].Path != filepath.Clean(f.repo) || out.Folders[0].Name != filepath.Base(f.repo) || !out.Folders[0].Exists {
		t.Fatalf("first folder = %+v", out.Folders[0])
	}
	if out.Folders[1].Exists {
		t.Fatalf("a missing folder must say so: %+v", out.Folders[1])
	}
}

func TestCheckDeviceCreateWithoutStore(t *testing.T) {
	if status, _ := checkDeviceCreate(nil, focusCreateRequest{Goal: "g", Cwd: "/x"}); status != http.StatusServiceUnavailable {
		t.Fatalf("no store: %d", status)
	}
	if focusDeviceStore(nil) != nil {
		t.Fatal("no sessions, no store")
	}
}
