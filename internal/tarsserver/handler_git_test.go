package tarsserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func TestGitAPIStatusAndDiffUseSessionCurrentDir(t *testing.T) {
	workspace := t.TempDir()
	repo := filepath.Join(workspace, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	runHandlerGit(t, repo, "init", "-b", "main")
	runHandlerGit(t, repo, "config", "user.email", "tars@example.test")
	runHandlerGit(t, repo, "config", "user.name", "TARS Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	runHandlerGit(t, repo, "add", "README.md")
	runHandlerGit(t, repo, "commit", "-m", "initial")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\nchanged\n"), 0o644); err != nil {
		t.Fatalf("modify readme: %v", err)
	}

	store := session.NewStore(workspace)
	sess, err := store.Create("git")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := store.SetWorkDirs(sess.ID, []string{repo}, repo); err != nil {
		t.Fatalf("set workdirs: %v", err)
	}
	handler := newGitAPIHandler(workspace, store, nil, zerolog.New(io.Discard))

	statusReq := httptest.NewRequest(http.MethodGet, "/v1/git/status?session_id="+sess.ID, nil)
	statusRec := httptest.NewRecorder()
	handler.ServeHTTP(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%q", statusRec.Code, statusRec.Body.String())
	}
	var status struct {
		IsGit  bool   `json:"is_git"`
		Root   string `json:"root"`
		Branch string `json:"branch"`
		Files  []struct {
			Path     string `json:"path"`
			Status   string `json:"status"`
			Unstaged bool   `json:"unstaged"`
		} `json:"files"`
	}
	if err := json.Unmarshal(statusRec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if !status.IsGit || status.Root != canonicalHandlerGitPath(t, repo) || status.Branch != "main" {
		t.Fatalf("unexpected status payload: %+v", status)
	}
	if len(status.Files) != 1 || status.Files[0].Path != "README.md" || !status.Files[0].Unstaged || status.Files[0].Status != "modified" {
		t.Fatalf("expected README modification, got %+v", status.Files)
	}

	diffReq := httptest.NewRequest(http.MethodGet, "/v1/git/diff?session_id="+sess.ID+"&path=README.md", nil)
	diffRec := httptest.NewRecorder()
	handler.ServeHTTP(diffRec, diffReq)
	if diffRec.Code != http.StatusOK {
		t.Fatalf("expected diff 200, got %d body=%q", diffRec.Code, diffRec.Body.String())
	}
	var diff struct {
		Path  string `json:"path"`
		Patch string `json:"patch"`
	}
	if err := json.Unmarshal(diffRec.Body.Bytes(), &diff); err != nil {
		t.Fatalf("decode diff: %v", err)
	}
	if diff.Path != "README.md" || !strings.Contains(diff.Patch, "+changed") {
		t.Fatalf("unexpected diff payload: %+v", diff)
	}

	logReq := httptest.NewRequest(http.MethodGet, "/v1/git/log?session_id="+sess.ID, nil)
	logRec := httptest.NewRecorder()
	handler.ServeHTTP(logRec, logReq)
	if logRec.Code != http.StatusOK {
		t.Fatalf("expected log 200, got %d body=%q", logRec.Code, logRec.Body.String())
	}
	var log struct {
		Commits []struct {
			Subject string `json:"subject"`
		} `json:"commits"`
	}
	if err := json.Unmarshal(logRec.Body.Bytes(), &log); err != nil {
		t.Fatalf("decode log: %v", err)
	}
	if len(log.Commits) != 1 || log.Commits[0].Subject != "initial" {
		t.Fatalf("unexpected log payload: %+v", log)
	}

	branchesReq := httptest.NewRequest(http.MethodGet, "/v1/git/branches?session_id="+sess.ID, nil)
	branchesRec := httptest.NewRecorder()
	handler.ServeHTTP(branchesRec, branchesReq)
	if branchesRec.Code != http.StatusOK {
		t.Fatalf("expected branches 200, got %d body=%q", branchesRec.Code, branchesRec.Body.String())
	}
	var branches struct {
		Branches []struct {
			Name    string `json:"name"`
			Current bool   `json:"current"`
		} `json:"branches"`
	}
	if err := json.Unmarshal(branchesRec.Body.Bytes(), &branches); err != nil {
		t.Fatalf("decode branches: %v", err)
	}
	if len(branches.Branches) == 0 || branches.Branches[0].Name != "main" || !branches.Branches[0].Current {
		t.Fatalf("unexpected branches payload: %+v", branches)
	}

	headOut, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v\n%s", err, headOut)
	}
	hash := strings.TrimSpace(string(headOut))
	commitReq := httptest.NewRequest(http.MethodGet, "/v1/git/commit?session_id="+sess.ID+"&hash="+hash, nil)
	commitRec := httptest.NewRecorder()
	handler.ServeHTTP(commitRec, commitReq)
	if commitRec.Code != http.StatusOK {
		t.Fatalf("expected commit 200, got %d body=%q", commitRec.Code, commitRec.Body.String())
	}
	var commit struct {
		Hash    string `json:"hash"`
		Subject string `json:"subject"`
		Files   []struct {
			Path   string `json:"path"`
			Status string `json:"status"`
		} `json:"files"`
	}
	if err := json.Unmarshal(commitRec.Body.Bytes(), &commit); err != nil {
		t.Fatalf("decode commit: %v", err)
	}
	if commit.Hash != hash || commit.Subject != "initial" || len(commit.Files) == 0 {
		t.Fatalf("unexpected commit payload: %+v", commit)
	}

	missingHashReq := httptest.NewRequest(http.MethodGet, "/v1/git/commit?session_id="+sess.ID, nil)
	missingHashRec := httptest.NewRecorder()
	handler.ServeHTTP(missingHashRec, missingHashReq)
	if missingHashRec.Code != http.StatusBadRequest {
		t.Fatalf("expected commit without hash to be 400, got %d", missingHashRec.Code)
	}

	worktreeReq := httptest.NewRequest(http.MethodGet, "/v1/git/worktrees?session_id="+sess.ID, nil)
	worktreeRec := httptest.NewRecorder()
	handler.ServeHTTP(worktreeRec, worktreeReq)
	if worktreeRec.Code != http.StatusOK {
		t.Fatalf("expected worktrees 200, got %d body=%q", worktreeRec.Code, worktreeRec.Body.String())
	}
	var trees struct {
		Worktrees []struct {
			Path    string `json:"path"`
			Branch  string `json:"branch"`
			Current bool   `json:"current"`
		} `json:"worktrees"`
	}
	if err := json.Unmarshal(worktreeRec.Body.Bytes(), &trees); err != nil {
		t.Fatalf("decode worktrees: %v", err)
	}
	if len(trees.Worktrees) != 1 || trees.Worktrees[0].Branch != "main" || !trees.Worktrees[0].Current {
		t.Fatalf("unexpected worktrees payload: %+v", trees)
	}
}

func TestGitAPIMutationApprovalRequiresConsentAndApproval(t *testing.T) {
	workspace := t.TempDir()
	repo := filepath.Join(workspace, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	runHandlerGit(t, repo, "init", "-b", "main")
	runHandlerGit(t, repo, "config", "user.email", "tars@example.test")
	runHandlerGit(t, repo, "config", "user.name", "TARS Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	runHandlerGit(t, repo, "add", "README.md")
	runHandlerGit(t, repo, "commit", "-m", "initial")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\nchanged\n"), 0o644); err != nil {
		t.Fatalf("modify readme: %v", err)
	}

	store := session.NewStore(workspace)
	sess, err := store.Create("git")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := store.SetWorkDirs(sess.ID, []string{repo}, repo); err != nil {
		t.Fatalf("set workdirs: %v", err)
	}
	mgr := ops.NewManager(workspace, ops.Options{HomeDir: filepath.Join(t.TempDir(), "home")})
	gitHandler := newGitAPIHandler(workspace, store, mgr, zerolog.New(io.Discard))

	body := `{"session_id":"` + sess.ID + `","action":"stage","path":"README.md"}`
	blockedReq := httptest.NewRequest(http.MethodPost, "/v1/git/mutations", strings.NewReader(body))
	blockedReq.Header.Set("Content-Type", "application/json")
	blockedRec := httptest.NewRecorder()
	gitHandler.ServeHTTP(blockedRec, blockedReq)
	if blockedRec.Code != http.StatusForbidden {
		t.Fatalf("expected mutation approval to require consent, got %d body=%q", blockedRec.Code, blockedRec.Body.String())
	}

	if err := store.SetAutomationConsent(sess.ID, &session.SessionAutomationConsent{GitMutations: true}); err != nil {
		t.Fatalf("set automation consent: %v", err)
	}
	approvalReq := httptest.NewRequest(http.MethodPost, "/v1/git/mutations", strings.NewReader(body))
	approvalReq.Header.Set("Content-Type", "application/json")
	approvalRec := httptest.NewRecorder()
	gitHandler.ServeHTTP(approvalRec, approvalReq)
	if approvalRec.Code != http.StatusOK {
		t.Fatalf("expected approval 200, got %d body=%q", approvalRec.Code, approvalRec.Body.String())
	}
	var plan ops.GitMutationPlan
	if err := json.Unmarshal(approvalRec.Body.Bytes(), &plan); err != nil {
		t.Fatalf("decode mutation plan: %v", err)
	}
	if plan.ApprovalID == "" || plan.Action != ops.GitMutationStage || plan.Path != "README.md" {
		t.Fatalf("unexpected mutation plan: %+v", plan)
	}

	opsHandler := newOpsAPIHandler(mgr, zerolog.New(io.Discard), nil, store)
	approveReq := httptest.NewRequest(http.MethodPost, "/v1/ops/approvals/"+plan.ApprovalID+"/approve", strings.NewReader(`{}`))
	approveReq.Header.Set("Content-Type", "application/json")
	approveRec := httptest.NewRecorder()
	opsHandler.ServeHTTP(approveRec, approveReq)
	if approveRec.Code != http.StatusOK {
		t.Fatalf("expected approve 200, got %d body=%q", approveRec.Code, approveRec.Body.String())
	}
	cached := runHandlerGitOutput(t, repo, "diff", "--cached", "--", "README.md")
	if !strings.Contains(cached, "+changed") {
		t.Fatalf("expected staged README diff, got %q", cached)
	}
	audit, err := mgr.ListAutomationAudit(ops.AutomationAuditListOptions{SessionID: sess.ID})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	foundSuccess := false
	foundBlocked := false
	for _, entry := range audit {
		if entry.Action == "git.stage" && entry.Result == "success" {
			foundSuccess = true
		}
		if entry.Action == "git.stage" && entry.Result == "blocked" {
			foundBlocked = true
		}
	}
	if !foundSuccess || !foundBlocked {
		t.Fatalf("expected blocked and successful git mutation audit entries, got %+v", audit)
	}
}

func TestGitAPIMutationRejectsRootOutsideSessionWorkspace(t *testing.T) {
	workspace := t.TempDir()
	repo := filepath.Join(workspace, "repo")
	outside := filepath.Join(t.TempDir(), "outside")
	for _, dir := range []string{repo, outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		runHandlerGit(t, dir, "init", "-b", "main")
		runHandlerGit(t, dir, "config", "user.email", "tars@example.test")
		runHandlerGit(t, dir, "config", "user.name", "TARS Test")
		if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
			t.Fatalf("write readme: %v", err)
		}
		runHandlerGit(t, dir, "add", "README.md")
		runHandlerGit(t, dir, "commit", "-m", "initial")
	}

	store := session.NewStore(workspace)
	sess, err := store.Create("git")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := store.SetWorkDirs(sess.ID, []string{repo}, repo); err != nil {
		t.Fatalf("set workdirs: %v", err)
	}
	if err := store.SetAutomationConsent(sess.ID, &session.SessionAutomationConsent{GitMutations: true}); err != nil {
		t.Fatalf("set automation consent: %v", err)
	}

	mgr := ops.NewManager(workspace, ops.Options{HomeDir: filepath.Join(t.TempDir(), "home")})
	gitHandler := newGitAPIHandler(workspace, store, mgr, zerolog.New(io.Discard))
	// Marshal rather than concatenate: `outside` is an OS path, and on Windows
	// its backslashes are invalid JSON escapes, so a hand-built body fails
	// decoding with 400 and never reaches the containment check under test.
	body, err := json.Marshal(map[string]string{
		"session_id": sess.ID,
		"root":       outside,
		"action":     "stage",
		"path":       "README.md",
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/git/mutations", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gitHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected outside root to be forbidden, got %d body=%q", rec.Code, rec.Body.String())
	}
}

func runHandlerGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	_ = runHandlerGitOutput(t, dir, args...)
}

func runHandlerGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func canonicalHandlerGitPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(resolved)
}
