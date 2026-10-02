package tarsserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/testutil"
)

// passthrough stands in for the session API handler: it records the body a
// plain create reached it with.
type passthrough struct {
	body string
	hits int
}

func (p *passthrough) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	p.body = string(raw)
	p.hits++
	writeJSON(w, http.StatusCreated, map[string]string{"id": "plain"})
}

func postNewSession(t *testing.T, h http.Handler, body string, admin bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/sessions", strings.NewReader(body))
	if admin {
		req.Header.Set("Tars-Debug-Auth-Role", "admin")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeSession(t *testing.T, rec *httptest.ResponseRecorder) session.Session {
	t.Helper()
	var sess session.Session
	if err := json.Unmarshal(rec.Body.Bytes(), &sess); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return sess
}

func realDir(t *testing.T, dir string) string {
	t.Helper()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		return resolved
	}
	return dir
}

func TestNewSessionWithoutFolderPassesThrough(t *testing.T) {
	f := newWorktreeFixture(t)
	next := &passthrough{}
	h := withSessionCreateIn(next, f.c)

	rec := postNewSession(t, h, `{"title":"Plain"}`, false)
	if rec.Code != http.StatusCreated || next.hits != 1 || !strings.Contains(next.body, `"Plain"`) {
		t.Fatalf("plain create: code=%d hits=%d body=%q", rec.Code, next.hits, next.body)
	}
	// An empty body is still the old "New Chat" create.
	if rec := postNewSession(t, h, ``, false); rec.Code != http.StatusCreated || next.hits != 2 {
		t.Fatalf("empty create: code=%d hits=%d", rec.Code, next.hits)
	}
	// Other routes under the prefix are none of its business.
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/sessions", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)
	if next.hits != 3 {
		t.Fatal("GET should reach the session handler")
	}
}

func TestNewSessionInFolder(t *testing.T) {
	f := newWorktreeFixture(t)
	next := &passthrough{}
	h := withSessionCreateIn(next, f.c)
	dir := realDir(t, t.TempDir())

	rec := postNewSession(t, h, `{"title":"Notes","cwd":`+jsonString(dir)+`}`, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if next.hits != 0 {
		t.Fatal("a create with a folder is handled here")
	}
	sess := decodeSession(t, rec)
	if sess.Title != "Notes" || sess.CurrentDir != dir || sess.Worktree != nil {
		t.Fatalf("session = %+v", sess)
	}
	stored, err := f.store.Get(sess.ID)
	if err != nil || stored.CurrentDir != dir {
		t.Fatalf("stored = %+v, %v", stored, err)
	}
	found := false
	for _, d := range stored.WorkDirs {
		found = found || d == dir
	}
	if !found {
		t.Fatalf("work dirs = %v", stored.WorkDirs)
	}
}

func TestNewSessionIsolated(t *testing.T) {
	f := newWorktreeFixture(t)
	h := withSessionCreateIn(&passthrough{}, f.c)

	rec := postNewSession(t, h, `{"cwd":`+jsonString(f.repo)+`,"isolate":true}`, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	sess := decodeSession(t, rec)
	if sess.Title != "New Chat" {
		t.Fatalf("title = %q", sess.Title)
	}
	if sess.Worktree == nil || sess.Worktree.Reason != worktreeReasonNewChat || sess.CurrentDir != sess.Worktree.Dir {
		t.Fatalf("session = %+v", sess)
	}
	if sess.Worktree.SourceDir != f.repo {
		t.Fatalf("source = %q", sess.Worktree.SourceDir)
	}
	if _, err := os.Stat(filepath.Join(sess.Worktree.Dir, ".env")); err != nil {
		t.Fatalf("worktree_include not copied: %v", err)
	}
	if got := strings.Join(f.audit.results(), ","); got != "isolated" {
		t.Fatalf("audit = %s", got)
	}
}

func TestNewSessionRejectsBadRequestsWithoutCreating(t *testing.T) {
	f := newWorktreeFixture(t)
	h := withSessionCreateIn(&passthrough{}, f.c)
	plain := realDir(t, t.TempDir())
	file := filepath.Join(plain, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		body  string
		admin bool
		code  int
	}{
		{"not admin", `{"cwd":` + jsonString(plain) + `}`, false, http.StatusForbidden},
		{"isolate without folder", `{"isolate":true}`, true, http.StatusBadRequest},
		{"relative folder", `{"cwd":"some/where"}`, true, http.StatusBadRequest},
		{"missing folder", `{"cwd":` + jsonString(filepath.Join(plain, "nope")) + `}`, true, http.StatusNotFound},
		{"a file", `{"cwd":` + jsonString(file) + `}`, true, http.StatusBadRequest},
		{"isolate outside a repository", `{"cwd":` + jsonString(plain) + `,"isolate":true}`, true, http.StatusBadRequest},
		{"bad json", `{"cwd":`, true, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := postNewSession(t, h, tc.body, tc.admin)
			if rec.Code != tc.code {
				t.Fatalf("code=%d want %d body=%s", rec.Code, tc.code, rec.Body.String())
			}
		})
	}
	list, err := f.store.List()
	if err != nil || len(list) != 0 {
		t.Fatalf("sessions created: %d, %v", len(list), err)
	}
}

func TestResolveChatFolderExpandsHome(t *testing.T) {
	home := realDir(t, t.TempDir())
	testutil.SetHome(t, home)
	if err := os.Mkdir(filepath.Join(home, "proj"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := resolveChatFolder(t.Context(), "~/proj")
	if err != nil || got.Path != filepath.Join(home, "proj") || got.RepoRoot != "" {
		t.Fatalf("~/proj = %+v, %v", got, err)
	}
	if got, err := resolveChatFolder(t.Context(), " ~ "); err != nil || got.Path != home {
		t.Fatalf("~ = %+v, %v", got, err)
	}
}

func TestSessionFoldersRecentAndProbe(t *testing.T) {
	f := newWorktreeFixture(t)
	h := newSessionFoldersHandler(f.c)
	other := realDir(t, t.TempDir())

	// Newest first: an isolated session counts for the folder it came from,
	// a session still in its own artifact folder counts for nothing.
	f.session(t, "repo")
	if _, err := f.store.Create("artifacts only"); err != nil {
		t.Fatal(err)
	}
	iso := f.session(t, "isolated")
	if _, err := f.c.isolate(t.Context(), iso, "manual", ""); err != nil {
		t.Fatal(err)
	}
	notes, _ := f.store.Create("notes")
	if err := f.store.SetWorkDirs(notes.ID, []string{other}, other); err != nil {
		t.Fatal(err)
	}
	gone, _ := f.store.Create("gone")
	missing := filepath.Join(other, "removed")
	if err := os.Mkdir(missing, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := f.store.SetWorkDirs(gone.ID, []string{missing}, missing); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(missing); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/admin/session-folders", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Recent []chatFolder `json:"recent"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Recent) != 2 {
		t.Fatalf("recent = %+v", resp.Recent)
	}
	if resp.Recent[0].Path != other || resp.Recent[0].RepoRoot != "" {
		t.Fatalf("first = %+v", resp.Recent[0])
	}
	if resp.Recent[1].Path != f.repo || resp.Recent[1].RepoRoot != f.repo || resp.Recent[1].LastUsedAt == nil {
		t.Fatalf("second = %+v", resp.Recent[1])
	}

	probe := httptest.NewRecorder()
	h.ServeHTTP(probe, httptest.NewRequest(http.MethodGet, "/v1/admin/session-folders?path="+url.QueryEscape(f.repo), nil))
	var one struct {
		Folder chatFolder `json:"folder"`
	}
	if err := json.Unmarshal(probe.Body.Bytes(), &one); err != nil || probe.Code != http.StatusOK || one.Folder.RepoRoot != f.repo {
		t.Fatalf("probe = %d %s", probe.Code, probe.Body.String())
	}
	bad := httptest.NewRecorder()
	h.ServeHTTP(bad, httptest.NewRequest(http.MethodGet, "/v1/admin/session-folders?path="+url.QueryEscape(missing), nil))
	if bad.Code != http.StatusNotFound {
		t.Fatalf("missing probe = %d", bad.Code)
	}
	post := httptest.NewRecorder()
	h.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/v1/admin/session-folders", nil))
	if post.Code != http.StatusMethodNotAllowed {
		t.Fatalf("post = %d", post.Code)
	}
}

func TestRecentFoldersLimit(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	var list []session.Session
	base := t.TempDir()
	for i := range recentFolderLimit + 3 {
		dir := filepath.Join(base, "d"+strings.Repeat("x", i+1))
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		list = append(list, session.Session{ID: "s" + dir, CurrentDir: dir, UpdatedAt: now.Add(time.Duration(i) * time.Minute)})
	}
	list = append(list, session.Session{ID: "hidden", CurrentDir: base, Hidden: true, UpdatedAt: now.Add(time.Hour)})
	got := recentFolders(list, func(session.Session) bool { return false })
	if len(got) != recentFolderLimit {
		t.Fatalf("len = %d", len(got))
	}
	if !strings.HasSuffix(got[0].Path, strings.Repeat("x", recentFolderLimit+3)) {
		t.Fatalf("newest first: %s", got[0].Path)
	}
}

func jsonString(s string) string {
	raw, _ := json.Marshal(s)
	return string(raw)
}
