package tarsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/sessionoverride"
	"github.com/devlikebear/tars/internal/sessionworktree"
	"github.com/rs/zerolog"
)

func TestRepoLeases(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	l := newRepoLeases()
	l.now = func() time.Time { return now }

	if _, ok := l.acquire("/r", "a"); !ok {
		t.Fatal("free repo")
	}
	if holder, ok := l.acquire("/r", "b"); ok || holder != "a" {
		t.Fatalf("held repo: holder=%s ok=%v", holder, ok)
	}
	if _, ok := l.acquire("/r", "a"); !ok {
		t.Fatal("holder takes it again")
	}
	l.finish("/r", "a")
	l.finish("/r", "a")
	l.finish("/r", "other") // not the holder: ignored
	if l.holder("/r") != "a" {
		t.Fatal("lease kept for the grace period")
	}
	if _, ok := l.acquire("/r", "b"); ok {
		t.Fatal("grace period still holds")
	}
	now = now.Add(repoLeaseGrace + time.Second)
	if l.holder("/r") != "" {
		t.Fatal("lease should lapse after the grace period")
	}
	if _, ok := l.acquire("/r", "b"); !ok {
		t.Fatal("lapsed lease is free")
	}
	l.release("b")
	if l.holder("/r") != "" {
		t.Fatal("released")
	}
}

type worktreeFixture struct {
	c     *chatWorktrees
	store *session.Store
	repo  string
	audit *auditRecorder
}

type auditRecorder struct {
	mu      sync.Mutex
	entries []ops.AutomationAuditEntry
}

func (a *auditRecorder) add(e ops.AutomationAuditEntry) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = append(a.entries, e)
}

func (a *auditRecorder) results() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []string
	for _, e := range a.entries {
		out = append(out, e.Result)
	}
	return out
}

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func newWorktreeFixture(t *testing.T) *worktreeFixture {
	t.Helper()
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".tars"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "init", "-q", "-b", "main")
	gitRun(t, repo, "config", "commit.gpgSign", "false")
	gitRun(t, repo, "config", "core.autocrlf", "false")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".env\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".tars", "settings.json"), []byte(`{"worktree_include":[".env"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-q", "-m", "init")
	if err := os.WriteFile(filepath.Join(repo, ".env"), []byte("X=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if resolved, err := filepath.EvalSymlinks(repo); err == nil {
		repo = resolved
	}
	store := session.NewStore(root)
	audit := &auditRecorder{}
	c := &chatWorktrees{
		manager:   sessionworktree.New(filepath.Join(root, "_shared", "session-worktrees")),
		store:     store,
		leases:    newRepoLeases(),
		overrides: sessionoverride.NewService(store),
		audit:     audit.add,
	}
	return &worktreeFixture{c: c, store: store, repo: repo, audit: audit}
}

func (f *worktreeFixture) session(t *testing.T, title string) session.Session {
	t.Helper()
	sess, err := f.store.Create(title)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.SetWorkDirs(sess.ID, []string{f.repo}, f.repo); err != nil {
		t.Fatal(err)
	}
	got, _ := f.store.Get(sess.ID)
	return got
}

func TestBeginTurnIsolatesTheSecondSession(t *testing.T) {
	ctx := context.Background()
	f := newWorktreeFixture(t)
	first := f.session(t, "first")
	second := f.session(t, "second")

	notice, end := f.c.beginTurn(ctx, first.ID, false)
	if notice != nil {
		t.Fatal("the first session works in the checkout")
	}
	moved, endSecond := f.c.beginTurn(ctx, second.ID, false)
	endSecond()
	if moved == nil || moved.Holder != first.ID || moved.Worktree.Reason != "lease" {
		t.Fatalf("second session notice = %+v", moved)
	}
	if strings.Join(moved.Copied, ",") != ".env" {
		t.Fatalf("copied = %v", moved.Copied)
	}
	got, _ := f.store.Get(second.ID)
	if got.Worktree == nil || got.CurrentDir != got.Worktree.Dir {
		t.Fatalf("second session not moved: %+v", got)
	}
	end()

	// Already isolated: later turns just run there.
	if again, _ := f.c.beginTurn(ctx, second.ID, false); again != nil {
		t.Fatal("isolated session isolated twice")
	}
	view := f.c.view(ctx, got)
	if view.Worktree == nil || view.Status == nil || view.RepoRoot != f.repo {
		t.Fatalf("view = %+v", view)
	}
	firstView := f.c.view(ctx, first)
	if firstView.LeaseHolder != first.ID || firstView.LeaseHolderTitle != "first" {
		t.Fatalf("first view = %+v", firstView)
	}
}

func TestBeginTurnWithIsolationOff(t *testing.T) {
	ctx := context.Background()
	f := newWorktreeFixture(t)
	first := f.session(t, "first")
	second := f.session(t, "second")
	if err := f.store.SetIsolation(second.ID, session.IsolationOff); err != nil {
		t.Fatal(err)
	}
	_, end := f.c.beginTurn(ctx, first.ID, false)
	defer end()
	if notice, _ := f.c.beginTurn(ctx, second.ID, false); notice != nil {
		t.Fatal("isolation off must not isolate")
	}
	if notice, _ := f.c.beginTurn(ctx, second.ID, true); notice != nil {
		t.Fatal("isolation off must not isolate unattended runs either")
	}
}

func TestBeginTurnIsolatesUnattendedRuns(t *testing.T) {
	ctx := context.Background()
	f := newWorktreeFixture(t)
	sess := f.session(t, "nightly")
	notice, end := f.c.beginTurn(ctx, sess.ID, true)
	end()
	if notice == nil || notice.Worktree.Reason != "unattended" {
		t.Fatalf("notice = %+v", notice)
	}
	var nilService *chatWorktrees
	if n, e := nilService.beginTurn(ctx, sess.ID, true); n != nil || e == nil {
		t.Fatal("nil service is a no-op")
	}
	plain, _ := f.store.Create("no folder")
	if n, _ := f.c.beginTurn(ctx, plain.ID, true); n != nil {
		t.Fatal("a session without a folder is not isolated")
	}
}

func TestWorktreeEndpoint(t *testing.T) {
	f := newWorktreeFixture(t)
	sess := f.session(t, "manual")
	running := false
	f.c.running = func(string) bool { return running }
	mux := http.NewServeMux()
	mux.Handle("/v1/admin/sessions/{id}/worktree", newSessionWorktreeHandler(f.c))
	srv := httptest.NewServer(mux)
	defer srv.Close()
	url := srv.URL + "/v1/admin/sessions/" + sess.ID + "/worktree"

	call := func(method, body string) (int, map[string]any) {
		req, _ := http.NewRequest(method, url, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}

	if code, _ := call(http.MethodPost, `{"action":"apply"}`); code != http.StatusConflict {
		t.Fatalf("apply without a worktree = %d", code)
	}
	if code, _ := call(http.MethodPost, `{"action":"bogus"}`); code != http.StatusBadRequest {
		t.Fatalf("bogus action = %d", code)
	}
	running = true
	if code, _ := call(http.MethodPost, `{"action":"isolate"}`); code != http.StatusConflict {
		t.Fatalf("isolate while running = %d", code)
	}
	running = false
	code, out := call(http.MethodPost, `{"action":"isolate"}`)
	if code != http.StatusOK {
		t.Fatalf("isolate = %d %v", code, out)
	}
	got, _ := f.store.Get(sess.ID)
	if got.Worktree == nil {
		t.Fatal("not isolated")
	}
	if err := os.WriteFile(filepath.Join(got.Worktree.Dir, "b.txt"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out = call(http.MethodGet, "")
	if code != http.StatusOK || out["status"] == nil {
		t.Fatalf("get = %d %v", code, out)
	}
	code, out = call(http.MethodPost, `{"action":"apply"}`)
	if code != http.StatusOK {
		t.Fatalf("apply = %d %v", code, out)
	}
	if _, err := os.Stat(filepath.Join(f.repo, "b.txt")); err != nil {
		t.Fatal("applied file missing in the checkout")
	}
	got, _ = f.store.Get(sess.ID)
	if got.Worktree != nil || got.CurrentDir != f.repo {
		t.Fatalf("session not moved back: %+v", got)
	}

	// Keep and discard.
	call(http.MethodPost, `{"action":"isolate"}`)
	if code, out := call(http.MethodPost, `{"action":"keep"}`); code != http.StatusOK || !strings.HasPrefix(out["result"].(map[string]any)["branch"].(string), "tars/session-") {
		t.Fatalf("keep = %d %v", code, out)
	}
	call(http.MethodPost, `{"action":"isolate"}`)
	if code, _ := call(http.MethodPost, `{"action":"discard"}`); code != http.StatusOK {
		t.Fatalf("discard = %d", code)
	}

	// Isolation setting.
	if code, out := call(http.MethodPut, `{"isolation":"off"}`); code != http.StatusOK || out["isolation"] != "off" {
		t.Fatalf("put isolation = %d %v", code, out)
	}
	if code, _ := call(http.MethodPut, `{"isolation":"sometimes"}`); code != http.StatusBadRequest {
		t.Fatalf("bad isolation = %d", code)
	}
	if code, _ := call(http.MethodDelete, ""); code != http.StatusMethodNotAllowed {
		t.Fatalf("delete = %d", code)
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/admin/sessions/missing/worktree", nil)
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing session = %v %v", resp, err)
	}

	results := f.audit.results()
	want := []string{"isolated", "applied", "isolated", "kept", "isolated", "discarded"}
	if strings.Join(results, ",") != strings.Join(want, ",") {
		t.Fatalf("audit = %v", results)
	}
}

func TestApplyConflictKeepsTheWorktree(t *testing.T) {
	ctx := context.Background()
	f := newWorktreeFixture(t)
	sess := f.session(t, "conflict")
	if _, err := f.c.isolate(ctx, sess, "manual", ""); err != nil {
		t.Fatal(err)
	}
	got, _ := f.store.Get(sess.ID)
	if err := os.WriteFile(filepath.Join(got.Worktree.Path, "a.txt"), []byte("session\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.repo, "a.txt"), []byte("checkout\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.c.finish(ctx, sess.ID, "apply"); err == nil {
		t.Fatal("expected a conflict")
	}
	still, _ := f.store.Get(sess.ID)
	if still.Worktree == nil {
		t.Fatal("conflict must leave the session in its worktree")
	}
	if results := f.audit.results(); results[len(results)-1] != "apply_failed" {
		t.Fatalf("audit = %v", results)
	}
}

func TestRetireAndSweepKeepWorkOnBranches(t *testing.T) {
	ctx := context.Background()
	f := newWorktreeFixture(t)
	gone := f.session(t, "gone")
	deleted := f.session(t, "deleted")
	for _, s := range []session.Session{gone, deleted} {
		if _, err := f.c.isolate(ctx, s, "manual", ""); err != nil {
			t.Fatal(err)
		}
	}
	// Deleting through the session API keeps the work.
	sessions := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = f.store.Delete(strings.TrimPrefix(r.URL.Path, "/v1/admin/sessions/"))
		w.WriteHeader(http.StatusOK)
	})
	handler := withWorktreeRetire(sessions, f.c)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/v1/admin/sessions/"+deleted.ID, nil))
	if branches := gitRun(t, f.repo, "branch", "--list", "tars/session-"+deleted.ID); branches == "" {
		t.Fatal("deleted session's branch should be kept")
	}
	// Other requests pass straight through.
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/admin/sessions/"+gone.ID, nil))
	if withWorktreeRetire(sessions, nil) == nil {
		t.Fatal("nil service returns the handler")
	}

	// A session removed behind the server's back is swept at startup.
	if err := f.store.Delete(gone.ID); err != nil {
		t.Fatal(err)
	}
	if swept := f.c.sweep(ctx); swept != 1 {
		t.Fatalf("swept = %d", swept)
	}
	if records, _ := f.c.manager.Records(); len(records) != 0 {
		t.Fatalf("records left: %+v", records)
	}
	var nilService *chatWorktrees
	if nilService.sweep(ctx) != 0 {
		t.Fatal("nil sweep")
	}
	nilService.retire(ctx, gone)
}

func TestSessionWorktreeStoreRoundTrip(t *testing.T) {
	f := newWorktreeFixture(t)
	sess := f.session(t, "store")
	wt := session.SessionWorktree{Path: "/tmp/wt", Dir: filepath.Join(t.TempDir(), "wt"), Branch: "tars/session-x", SourceDir: f.repo}
	if err := f.store.SetWorktree(sess.ID, &wt); err != nil {
		t.Fatal(err)
	}
	got, _ := f.store.Get(sess.ID)
	if got.Worktree == nil || got.CurrentDir != got.Worktree.Dir || len(got.WorkDirs) != 3 {
		t.Fatalf("after set: %+v", got)
	}
	if err := f.store.SetWorktree(sess.ID, nil); err != nil {
		t.Fatal(err)
	}
	got, _ = f.store.Get(sess.ID)
	if got.Worktree != nil || got.CurrentDir != f.repo || len(got.WorkDirs) != 2 {
		t.Fatalf("after clear: %+v", got)
	}
	if err := f.store.SetWorktree("missing", nil); err == nil {
		t.Fatal("missing session")
	}
	if err := f.store.SetIsolation("missing", ""); err == nil {
		t.Fatal("missing session")
	}
}

func TestWorktreeStreamEventAndRunningRegistry(t *testing.T) {
	rec := httptest.NewRecorder()
	stream := newChatStreamWriter(rec, "s1", zerolog.Nop())
	stream.worktree(worktreeNotice{Worktree: session.SessionWorktree{Path: "/wt", Dir: "/wt/app", Branch: "tars/session-s1", Reason: "lease"}, Holder: "s0", Copied: []string{".env"}, Pending: []string{"node_modules"}})
	body := rec.Body.String()
	for _, want := range []string{`"type":"worktree"`, `"branch":"tars/session-s1"`, `"lease_holder":"s0"`, `"copied":[".env"]`, `"pending":["node_modules"]`} {
		if !strings.Contains(body, want) {
			t.Fatalf("stream %s missing %s", body, want)
		}
	}
	registry := newChatCancelRegistry()
	if registry.Running("s1") {
		t.Fatal("nothing registered")
	}
	registry.Register("s1", func() {})
	if !registry.Running("s1") {
		t.Fatal("registered turn")
	}
	var nilRegistry *chatCancelRegistry
	if nilRegistry.Running("s1") {
		t.Fatal("nil registry")
	}
}

// TestBackgroundIncludesAreAudited records the includes that finish after the
// turn already started in the worktree.
func TestBackgroundIncludesAreAudited(t *testing.T) {
	f := newWorktreeFixture(t)
	f.c.watchIncludes()
	wt := session.SessionWorktree{Branch: "tars/session-s1", SourceDir: f.repo}
	f.c.includeDone(sessionworktree.IncludeDone{SessionID: "s1", Worktree: wt, Copied: []string{"node_modules"}})
	f.c.includeDone(sessionworktree.IncludeDone{SessionID: "s1", Worktree: wt, Skipped: []string{"dist: canceled"}})
	if got := strings.Join(f.audit.results(), ","); got != "include_copied,include_skipped" {
		t.Fatalf("audit = %s", got)
	}
	f.audit.mu.Lock()
	defer f.audit.mu.Unlock()
	first := f.audit.entries[0]
	if first.SessionID != "s1" || first.CWD != f.repo || fmt.Sprint(first.Details["copied"]) != "[node_modules]" {
		t.Fatalf("entry = %+v", first)
	}
	if fmt.Sprint(f.audit.entries[1].Details["skipped"]) != "[dist: canceled]" {
		t.Fatalf("entry = %+v", f.audit.entries[1])
	}
}

// TestIsolateCopiesDependencyFolders brings a gitignored folder into the
// worktree as a copy the session can change freely.
func TestIsolateCopiesDependencyFolders(t *testing.T) {
	ctx := context.Background()
	f := newWorktreeFixture(t)
	for name, body := range map[string]string{
		".gitignore":               ".env\nnode_modules/\n",
		".tars/settings.json":      `{"worktree_include":[".env","node_modules","../outside"]}`,
		"node_modules/pkg/main.js": "dep\n",
	} {
		path := filepath.Join(f.repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sess := f.session(t, "deps")
	notice, err := f.c.isolate(ctx, sess, "manual", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(notice.Copied, ","); got != ".env,node_modules" {
		t.Fatalf("copied = %s", got)
	}
	copyPath := filepath.Join(notice.Worktree.Path, "node_modules", "pkg", "main.js")
	if err := os.WriteFile(copyPath, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(filepath.Join(f.repo, "node_modules", "pkg", "main.js")); string(raw) != "dep\n" {
		t.Fatal("changing the worktree's copy changed the checkout")
	}
	f.audit.mu.Lock()
	details := f.audit.entries[len(f.audit.entries)-1].Details
	f.audit.mu.Unlock()
	// The loader already dropped "../outside" with a warning, so the copy
	// never saw it.
	if _, ok := details["skipped"]; ok {
		t.Fatalf("details = %v", details)
	}
}

func TestIncludeWithoutOverrides(t *testing.T) {
	f := newWorktreeFixture(t)
	f.c.overrides = nil
	if f.c.include(session.Session{}) != nil {
		t.Fatal("no overrides service, no includes")
	}
	f.c.audit = nil
	f.c.record("s", "", "isolated", nil)
	if _, err := f.c.isolate(context.Background(), session.Session{ID: "missing"}, "manual", ""); err == nil {
		t.Fatal("isolate a missing session")
	}
	plain, _ := f.store.Create("plain")
	if _, err := f.c.isolate(context.Background(), plain, "manual", ""); err == nil {
		t.Fatal("isolate without a folder")
	}
	if _, err := f.c.finish(context.Background(), "missing", "apply"); err == nil {
		t.Fatal("finish a missing session")
	}
}

func TestChatTurnInABusyRepositoryRunsInAWorktree(t *testing.T) {
	f := newWorktreeFixture(t)
	holder := f.session(t, "holder")
	busy := f.session(t, "busy")
	if _, ok := f.c.leases.acquire(f.repo, holder.ID); !ok {
		t.Fatal("holder lease")
	}
	client := &mockLLMClient{response: llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "ok"}}}
	tooling := defaultChatToolingOptions()
	tooling.Worktrees = f.c
	root := filepath.Dir(filepath.Dir(f.c.manager.Root()))
	handler := newChatAPIHandlerWithRuntimeConfig(root, f.store, client, nil, zerolog.Nop(), 2, nil, "", tooling)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(`{"session_id":"`+busy.ID+`","message":"hi"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), `"type":"worktree"`) {
		t.Fatalf("no worktree event in %s", rec.Body.String())
	}
	got, _ := f.store.Get(busy.ID)
	if got.Worktree == nil {
		t.Fatal("busy session not isolated")
	}
	if len(client.seenWorkDirs) == 0 || client.seenWorkDirs[0] != got.Worktree.Dir {
		t.Fatalf("turn ran in %v, want %s", client.seenWorkDirs, got.Worktree.Dir)
	}
	if f.c.running == nil {
		t.Fatal("the chat handler should wire the running check")
	}
}

func TestCronRunIsolatesItsSession(t *testing.T) {
	f := newWorktreeFixture(t)
	sess := f.session(t, "nightly")
	client := &mockLLMClient{response: llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "ok"}}, disableDelta: true}
	tooling := defaultChatToolingOptions()
	tooling.Worktrees = f.c
	root := filepath.Dir(filepath.Dir(f.c.manager.Root()))
	runner := newCronPromptRunnerWithSessionContext(nil, chatHandlerDeps{workspaceDir: root, store: f.store, client: client, logger: zerolog.Nop(), maxIters: 1, tooling: tooling})
	ctx := withCronExecutionContext(context.Background(), cronExecutionContext{SessionID: sess.ID})
	if _, err := runner(ctx, "cron:nightly", "tidy up", nil, "", nil); err != nil {
		t.Fatal(err)
	}
	got, _ := f.store.Get(sess.ID)
	if got.Worktree == nil || got.Worktree.Reason != "unattended" {
		t.Fatalf("cron session = %+v", got.Worktree)
	}
}
