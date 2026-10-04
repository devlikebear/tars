package tarsserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/usage"
)

func gitInit(t *testing.T, dir, branch string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q", "-b", branch},
		{"-c", "user.email=t@example.com", "-c", "user.name=t", "add", "-A"},
		{"-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-q", "-m", "init"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("git %v: %v %s", args, err, out)
		}
	}
}

func TestSessionBoardShowsStatusRepoChangeAndCost(t *testing.T) {
	fx := newCheckpointFixture(t)
	gitInit(t, fx.project, "feat/board")
	ctx := context.Background()

	turn, err := fx.store.BeginTurn(ctx, fx.sessionID, "turn-1", fx.project, "edit base")
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(fx.project, "base.txt"), "one\ntwo\nthree\n")
	if _, err := turn.End(ctx); err != nil {
		t.Fatal(err)
	}

	idle, err := fx.sessions.Create("idle session")
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := fx.sessions.CreateWithOptions("worker", "worker", true)
	if err != nil {
		t.Fatal(err)
	}

	activity := newChatActivity(fx.sessions, nil)
	end := activity.begin(fx.sessionID)
	defer end()
	activity.observe(fx.sessionID, map[string]any{"type": "permission_request", "request_id": "r1", "tool_name": "Bash"})
	endIdle := activity.begin(idle.ID)
	endIdle()

	board := newSessionBoard(fx.sessions, activity, fx.store, func() (map[string]boardCost, error) {
		return map[string]boardCost{fx.sessionID: {USD: 1.25, UnpricedCalls: 3}}, nil
	})
	resp, err := board.build(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if resp.CostPeriod != "month" {
		t.Fatalf("cost period = %q", resp.CostPeriod)
	}
	byID := map[string]boardSession{}
	for _, s := range resp.Sessions {
		byID[s.ID] = s
	}
	if _, ok := byID[hidden.ID]; ok {
		t.Fatal("a hidden worker session must not be on the board")
	}
	got, ok := byID[fx.sessionID]
	if !ok {
		t.Fatalf("board = %+v", resp.Sessions)
	}
	if got.Status != boardStatusNeedsInput || got.PendingApprovals != 1 || got.RunningSince == nil {
		t.Fatalf("status = %q pending=%d running=%v", got.Status, got.PendingApprovals, got.RunningSince)
	}
	wantRepo, _ := filepath.EvalSymlinks(fx.project)
	// The board reports the cwd as the store saved it, with symlinks
	// resolved, so on macOS, where t.TempDir() is under /var ->
	// /private/var, it differs from fx.project.
	saved, err := fx.sessions.Get(fx.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Repo != wantRepo || got.Branch != "feat/board" || got.Cwd != saved.CurrentDir {
		t.Fatalf("repo = %q (want %q) branch = %q cwd = %q (want %q)", got.Repo, wantRepo, got.Branch, got.Cwd, saved.CurrentDir)
	}
	if got.LastChange == nil || got.LastChange.Files != 1 || got.LastChange.Additions != 1 || got.LastTurnAt == nil {
		t.Fatalf("last change = %+v last turn = %v", got.LastChange, got.LastTurnAt)
	}
	if got.CostUSD != 1.25 || got.UnpricedCalls != 3 {
		t.Fatalf("cost = %v, unpriced = %d", got.CostUSD, got.UnpricedCalls)
	}
	if other := byID[idle.ID]; other.Status != boardStatusIdle || other.Repo != "" || other.LastChange != nil {
		t.Fatalf("idle session = %+v", other)
	}

	activity.observe(fx.sessionID, map[string]any{"type": "permission_resolved", "request_id": "r1"})
	resp, _ = board.build(ctx)
	for _, s := range resp.Sessions {
		if s.ID == fx.sessionID && s.Status != boardStatusRunning {
			t.Fatalf("after the answer the turn is running, got %q", s.Status)
		}
	}
}

func TestSessionBoardCachesRepoLookups(t *testing.T) {
	fx := newCheckpointFixture(t)
	gitInit(t, fx.project, "main")
	board := newSessionBoard(fx.sessions, newChatActivity(fx.sessions, nil), fx.store, nil)
	calls := 0
	board.branchOf = func(context.Context, string) string { calls++; return "main" }
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	board.now = func() time.Time { return now }
	for range 3 {
		if _, err := board.build(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("branch looked up %d times within the TTL", calls)
	}
	now = now.Add(boardRepoTTL)
	if _, err := board.build(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("branch looked up %d times after the TTL", calls)
	}
}

func TestSessionBoardWithoutCheckpointsOrCosts(t *testing.T) {
	fx := newCheckpointFixture(t)
	board := newSessionBoard(fx.sessions, nil, nil, func() (map[string]boardCost, error) {
		return nil, errors.New("usage unavailable")
	})
	resp, err := board.build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Sessions) == 0 {
		t.Fatal("sessions must still be listed")
	}
	for _, s := range resp.Sessions {
		if s.Repo != "" || s.CostUSD != 0 || s.Status != boardStatusIdle {
			t.Fatalf("session = %+v", s)
		}
	}
}

// A new session starts in its own artifact folder, which the store saves with
// symlinks resolved. When the workspace is reached through a link (macOS's
// /var, a relocated ~/.tars) that folder must still be recognized, or every
// fresh session shows up as working in a repository of its own.
func TestSessionBoardArtifactFolderThroughSymlinkedWorkspace(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	sessions := session.NewStore(link)
	sess, err := sessions.Create("fresh")
	if err != nil {
		t.Fatal(err)
	}
	board := newSessionBoard(sessions, nil, nil, nil)
	resp, err := board.build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range resp.Sessions {
		if s.ID == sess.ID && (s.Cwd != "" || s.Repo != "") {
			t.Fatalf("a session still in its artifact folder is not in a project: %+v", s)
		}
	}
}

func TestSessionBoardHTTP(t *testing.T) {
	fx := newCheckpointFixture(t)
	archived, err := fx.sessions.Create("old")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.sessions.SetArchived(archived.ID, true); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(newSessionBoard(fx.sessions, newChatActivity(fx.sessions, nil), fx.store, nil).handle))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var body boardResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	for _, s := range body.Sessions {
		if s.ID == archived.ID {
			t.Fatal("an archived session must not be on the board")
		}
	}
	post, err := http.Post(srv.URL, "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	_ = post.Body.Close()
	if post.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST = %d", post.StatusCode)
	}
}

func TestGitBranch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitInit(t, dir, "topic")
	if got := gitBranch(context.Background(), dir); got != "topic" {
		t.Fatalf("branch = %q", got)
	}
	cmd := exec.Command("git", "-C", dir, "checkout", "-q", "--detach")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("detach: %v %s", err, out)
	}
	if got := gitBranch(context.Background(), dir); got == "" || got == "topic" {
		t.Fatalf("detached = %q, want a short commit", got)
	}
	if got := gitBranch(context.Background(), t.TempDir()); got != "" {
		t.Fatalf("outside a repo = %q", got)
	}
}

func TestSessionBoardCountsUnattendedApprovals(t *testing.T) {
	f := newUnattendedFixture(t, chatPermissionModeManual)
	other, err := f.store.Create("quiet")
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := f.ops.CreateToolPermissionApproval(ops.ToolPermissionRequest{SessionID: f.session, Source: "cron", ToolName: "exec"})
	if err != nil {
		t.Fatal(err)
	}
	answered, _ := f.ops.CreateToolPermissionApproval(ops.ToolPermissionRequest{SessionID: other.ID, Source: "cron", ToolName: "exec"})
	if err := f.ops.ReviewToolPermission(answered.ID, true); err != nil {
		t.Fatal(err)
	}

	board := newSessionBoard(f.store, nil, nil, nil)
	board.queued = f.perms.queuedBySession
	statusOf := func() map[string]boardSession {
		t.Helper()
		resp, err := board.build(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]boardSession{}
		for _, s := range resp.Sessions {
			out[s.ID] = s
		}
		return out
	}
	got := statusOf()
	if s := got[f.session]; s.Status != boardStatusNeedsInput || s.QueuedApprovals != 1 || s.PendingApprovals != 0 {
		t.Fatalf("waiting session = %+v", s)
	}
	if s := got[other.ID]; s.Status != boardStatusIdle || s.QueuedApprovals != 0 {
		t.Fatalf("answered session = %+v", s)
	}

	if err := f.ops.ReviewToolPermission(waiting.ID, false); err != nil {
		t.Fatal(err)
	}
	if s := statusOf()[f.session]; s.Status != boardStatusIdle || s.QueuedApprovals != 0 {
		t.Fatalf("after the review = %+v", s)
	}

	board.queued = func() (map[string]int, error) { return nil, errors.New("ops unavailable") }
	if s := statusOf()[f.session]; s.Status != boardStatusIdle {
		t.Fatalf("an unreadable queue counts nothing, got %+v", s)
	}
}

func TestSessionCostsFromReadsCostAndUnpricedCalls(t *testing.T) {
	if sessionCostsFrom(nil) != nil {
		t.Fatal("no tracker should mean no cost source")
	}
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	tracker, err := usage.NewTracker(t.TempDir(), usage.TrackerOptions{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []usage.Entry{
		{Timestamp: now, Provider: "anthropic", Model: "m", InputTokens: 10, OutputTokens: 1, EstimatedCostUSD: 0.5, PricingKnown: true, SessionID: "priced"},
		{Timestamp: now, Provider: "openai-codex", Model: "m", InputTokens: 10, OutputTokens: 1, SessionID: "unpriced"},
		{Timestamp: now, Provider: "openai-codex", Model: "m", InputTokens: 10, OutputTokens: 1, SessionID: "unpriced"},
	} {
		if err := tracker.Record(entry); err != nil {
			t.Fatal(err)
		}
	}
	costs, err := sessionCostsFrom(tracker)()
	if err != nil {
		t.Fatal(err)
	}
	if got := costs["priced"]; got.USD != 0.5 || got.UnpricedCalls != 0 {
		t.Fatalf("priced session: %+v", got)
	}
	if got := costs["unpriced"]; got.USD != 0 || got.UnpricedCalls != 2 {
		t.Fatalf("unpriced session: %+v", got)
	}
}
