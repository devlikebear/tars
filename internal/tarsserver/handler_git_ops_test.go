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

	"github.com/devlikebear/tars/internal/apihandlers"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// The git handler lives in internal/apihandlers; this test stays here because
// it approves the mutation through the ops handler, which is this package's.
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
	gitHandler := apihandlers.NewGitHandler(workspace, store, mgr, zerolog.New(io.Discard))

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
