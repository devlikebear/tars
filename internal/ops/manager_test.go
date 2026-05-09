package ops

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/config"
)

func TestManager_CleanupRequiresApprovalThenApplies(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(filepath.Join(home, "Downloads"), 0o755); err != nil {
		t.Fatalf("mkdir downloads: %v", err)
	}
	target := filepath.Join(home, "Downloads", "old.bin")
	if err := os.WriteFile(target, []byte("1234567890"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	mgr := NewManager(workspace, Options{HomeDir: home})
	plan, err := mgr.CreateCleanupPlan(context.Background())
	if err != nil {
		t.Fatalf("create cleanup plan: %v", err)
	}
	if plan.ApprovalID == "" {
		t.Fatalf("expected approval id")
	}

	if _, err := mgr.ApplyCleanup(context.Background(), plan.ApprovalID); err == nil {
		t.Fatalf("expected apply to fail before approval")
	}
	if err := mgr.Approve(plan.ApprovalID); err != nil {
		t.Fatalf("approve plan: %v", err)
	}
	result, err := mgr.ApplyCleanup(context.Background(), plan.ApprovalID)
	if err != nil {
		t.Fatalf("apply cleanup: %v", err)
	}
	if result.DeletedCount == 0 {
		t.Fatalf("expected deleted files > 0")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("expected target removed, stat err=%v", err)
	}
}

func TestManager_ListApprovals(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(filepath.Join(home, "Desktop"), 0o755); err != nil {
		t.Fatalf("mkdir desktop: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "Desktop", "a.log"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write desktop file: %v", err)
	}
	mgr := NewManager(workspace, Options{HomeDir: home})
	plan, err := mgr.CreateCleanupPlan(context.Background())
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	items, err := mgr.ListApprovals()
	if err != nil {
		t.Fatalf("list approvals: %v", err)
	}
	if len(items) == 0 {
		t.Fatalf("expected at least one approval")
	}
	if items[0].ID != plan.ApprovalID {
		t.Fatalf("expected latest approval %q, got %+v", plan.ApprovalID, items[0])
	}
}

func TestManager_RecordAutomationAuditListsNewestFirst(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	first := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	current := first
	mgr := NewManager(workspace, Options{
		HomeDir: filepath.Join(t.TempDir(), "home"),
		Now: func() time.Time {
			return current
		},
	})

	if _, err := mgr.RecordAutomationAudit(AutomationAuditEntry{
		Actor:     "pulse",
		Action:    "auto_resume_chat",
		Reason:    "stalled chat nudge",
		SessionID: "sess_1",
		CWD:       "/tmp/workspace",
		Result:    "blocked",
	}); err != nil {
		t.Fatalf("record first audit: %v", err)
	}
	current = first.Add(time.Minute)
	latest, err := mgr.RecordAutomationAudit(AutomationAuditEntry{
		Actor:     "git",
		Action:    "git_commit",
		Reason:    "user approved commit",
		SessionID: "sess_1",
		CWD:       "/tmp/workspace",
		Result:    "success",
	})
	if err != nil {
		t.Fatalf("record second audit: %v", err)
	}

	items, err := mgr.ListAutomationAudit(AutomationAuditListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("list automation audit: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected two audit entries, got %+v", items)
	}
	if items[0].ID != latest.ID || items[0].Action != "git_commit" {
		t.Fatalf("expected newest audit first, got %+v", items)
	}
	if items[1].Actor != "pulse" || items[1].Result != "blocked" {
		t.Fatalf("unexpected first audit entry: %+v", items[1])
	}
}

func TestManager_GitMutationRequiresApprovalAndAudits(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	runOpsGit(t, repo, "init", "-b", "main")
	runOpsGit(t, repo, "config", "user.email", "tars@example.test")
	runOpsGit(t, repo, "config", "user.name", "TARS Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	runOpsGit(t, repo, "add", "README.md")
	runOpsGit(t, repo, "commit", "-m", "initial")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\nchanged\n"), 0o644); err != nil {
		t.Fatalf("modify readme: %v", err)
	}

	mgr := NewManager(workspace, Options{HomeDir: filepath.Join(t.TempDir(), "home")})
	plan, err := mgr.CreateGitMutationApproval(context.Background(), GitMutationPlan{
		SessionID: "sess_1",
		Root:      repo,
		Action:    GitMutationStage,
		Path:      "README.md",
		Reason:    "stage selected file",
	})
	if err != nil {
		t.Fatalf("create git mutation approval: %v", err)
	}
	if plan.ApprovalID == "" || plan.Type != "git_mutation" || plan.Destructive {
		t.Fatalf("unexpected git mutation plan: %+v", plan)
	}
	if _, err := mgr.ApplyGitMutation(context.Background(), plan.ApprovalID); err == nil {
		t.Fatalf("expected apply to fail before approval")
	}
	if err := mgr.Approve(plan.ApprovalID); err != nil {
		t.Fatalf("approve git mutation: %v", err)
	}
	result, err := mgr.ApplyGitMutation(context.Background(), plan.ApprovalID)
	if err != nil {
		t.Fatalf("apply git mutation: %v", err)
	}
	if result.Action != GitMutationStage || result.Result != "success" {
		t.Fatalf("unexpected git mutation result: %+v", result)
	}
	cached := runOpsGitOutput(t, repo, "diff", "--cached", "--", "README.md")
	if !strings.Contains(cached, "+changed") {
		t.Fatalf("expected staged README diff, got %q", cached)
	}
	audit, err := mgr.ListAutomationAudit(AutomationAuditListOptions{SessionID: "sess_1"})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(audit) != 1 || audit[0].Action != "git.stage" || audit[0].Result != "success" {
		t.Fatalf("expected successful git audit entry, got %+v", audit)
	}
}

func TestManager_GitMutationCheckoutCommitMarksDestructiveOnDetach(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	runOpsGit(t, repo, "init", "-b", "main")
	runOpsGit(t, repo, "config", "user.email", "tars@example.test")
	runOpsGit(t, repo, "config", "user.name", "TARS Test")
	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runOpsGit(t, repo, "add", "f")
	runOpsGit(t, repo, "commit", "-m", "first")
	hash := strings.TrimSpace(runOpsGitOutput(t, repo, "rev-parse", "HEAD"))

	mgr := NewManager(workspace, Options{HomeDir: filepath.Join(t.TempDir(), "home")})
	plan, err := mgr.CreateGitMutationApproval(context.Background(), GitMutationPlan{
		SessionID: "sess_1",
		Root:      repo,
		Action:    GitMutationCheckoutCommit,
		Hash:      hash,
		Reason:    "checkout commit detached",
	})
	if err != nil {
		t.Fatalf("create checkout plan: %v", err)
	}
	if !plan.Destructive {
		t.Fatalf("expected detached checkout to be destructive, got %+v", plan)
	}
	if !strings.Contains(plan.Command, "--detach") {
		t.Fatalf("expected --detach in command, got %q", plan.Command)
	}

	plan2, err := mgr.CreateGitMutationApproval(context.Background(), GitMutationPlan{
		SessionID: "sess_1",
		Root:      repo,
		Action:    GitMutationCheckoutCommit,
		Hash:      hash,
		NewBranch: "feature-x",
	})
	if err != nil {
		t.Fatalf("create checkout w/ branch: %v", err)
	}
	if plan2.Destructive {
		t.Fatalf("expected branch-creating checkout to NOT be destructive: %+v", plan2)
	}
	if !strings.Contains(plan2.Command, "checkout -b feature-x") {
		t.Fatalf("expected checkout -b in command, got %q", plan2.Command)
	}
}

func TestManager_GitMutationFetchPlanIsNonDestructive(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	runOpsGit(t, repo, "init", "-b", "main")
	runOpsGit(t, repo, "config", "user.email", "tars@example.test")
	runOpsGit(t, repo, "config", "user.name", "TARS Test")
	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runOpsGit(t, repo, "add", "f")
	runOpsGit(t, repo, "commit", "-m", "first")

	mgr := NewManager(workspace, Options{HomeDir: filepath.Join(t.TempDir(), "home")})
	plan, err := mgr.CreateGitMutationApproval(context.Background(), GitMutationPlan{
		SessionID: "sess_1",
		Root:      repo,
		Action:    GitMutationFetch,
		Reason:    "refresh remote refs",
	})
	if err != nil {
		t.Fatalf("create fetch plan: %v", err)
	}
	if plan.Destructive {
		t.Fatalf("expected fetch plan NOT destructive: %+v", plan)
	}
	if plan.Command != "git fetch --all --prune" {
		t.Fatalf("unexpected fetch command: %q", plan.Command)
	}
}

func TestManager_GitMutationWorktreeAddRemoveValidatesPaths(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	runOpsGit(t, repo, "init", "-b", "main")
	runOpsGit(t, repo, "config", "user.email", "tars@example.test")
	runOpsGit(t, repo, "config", "user.name", "TARS Test")
	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runOpsGit(t, repo, "add", "f")
	runOpsGit(t, repo, "commit", "-m", "first")

	mgr := NewManager(workspace, Options{HomeDir: filepath.Join(t.TempDir(), "home")})

	if _, err := mgr.CreateGitMutationApproval(context.Background(), GitMutationPlan{
		SessionID: "sess_1",
		Root:      repo,
		Action:    GitMutationWorktreeAdd,
	}); err == nil {
		t.Fatalf("expected error for missing worktree_path")
	}

	wtPath := filepath.Join(t.TempDir(), "wt")
	plan, err := mgr.CreateGitMutationApproval(context.Background(), GitMutationPlan{
		SessionID:    "sess_1",
		Root:         repo,
		Action:       GitMutationWorktreeAdd,
		WorktreePath: wtPath,
		NewBranch:    "feature",
	})
	if err != nil {
		t.Fatalf("create worktree add plan: %v", err)
	}
	if plan.Destructive {
		t.Fatalf("expected worktree_add NOT destructive, got %+v", plan)
	}
	if !strings.Contains(plan.Command, "worktree add -b feature") {
		t.Fatalf("unexpected worktree_add command: %q", plan.Command)
	}

	removePlan, err := mgr.CreateGitMutationApproval(context.Background(), GitMutationPlan{
		SessionID:    "sess_1",
		Root:         repo,
		Action:       GitMutationWorktreeRemove,
		WorktreePath: wtPath,
	})
	if err != nil {
		t.Fatalf("create worktree remove plan: %v", err)
	}
	if !removePlan.Destructive {
		t.Fatalf("expected worktree_remove destructive: %+v", removePlan)
	}
}

func TestManager_GitMutationRejectsUnsafePath(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	runOpsGit(t, repo, "init", "-b", "main")

	mgr := NewManager(workspace, Options{HomeDir: filepath.Join(t.TempDir(), "home")})
	if _, err := mgr.CreateGitMutationApproval(context.Background(), GitMutationPlan{
		SessionID: "sess_1",
		Root:      repo,
		Action:    GitMutationStage,
		Path:      "../outside.txt",
	}); err == nil {
		t.Fatalf("expected unsafe path to be rejected")
	}
}

func TestManager_UpdateApprovalStatus_SetsReviewedAtAndPersists(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	fixedNow := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	mgr := NewManager(workspace, Options{
		HomeDir: filepath.Join(t.TempDir(), "home"),
		Now: func() time.Time {
			return fixedNow
		},
	})

	initial := []Approval{
		{
			ID:          "apr_1",
			Type:        "cleanup",
			Status:      "pending",
			RequestedAt: fixedNow.Add(-time.Hour),
			UpdatedAt:   fixedNow.Add(-time.Hour),
			Plan: CleanupPlan{
				ApprovalID: "apr_1",
				CreatedAt:  fixedNow.Add(-time.Hour),
			},
		},
	}
	if err := os.MkdirAll(filepath.Dir(mgr.approvalsPath), 0o755); err != nil {
		t.Fatalf("mkdir approvals dir: %v", err)
	}
	raw, err := json.Marshal(initial)
	if err != nil {
		t.Fatalf("marshal approvals: %v", err)
	}
	if err := os.WriteFile(mgr.approvalsPath, raw, 0o644); err != nil {
		t.Fatalf("write approvals: %v", err)
	}

	if err := mgr.updateApprovalStatus("apr_1", "approved"); err != nil {
		t.Fatalf("updateApprovalStatus: %v", err)
	}

	items, err := mgr.ListApprovals()
	if err != nil {
		t.Fatalf("list approvals: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 approval, got %+v", items)
	}
	if items[0].Status != "approved" {
		t.Fatalf("expected approved status, got %+v", items[0])
	}
	if items[0].ReviewedAt == nil || !items[0].ReviewedAt.Equal(fixedNow) {
		t.Fatalf("expected reviewed_at %s, got %+v", fixedNow, items[0].ReviewedAt)
	}
	if !items[0].UpdatedAt.Equal(fixedNow) {
		t.Fatalf("expected updated_at %s, got %s", fixedNow, items[0].UpdatedAt)
	}
	requireOpsFileMode(t, mgr.approvalsPath, 0o600)
}

func runOpsGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	_ = runOpsGitOutput(t, dir, args...)
}

func runOpsGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func TestNewManager_EmptyWorkspaceUsesCoreDefault(t *testing.T) {
	mgr := NewManager("", Options{})

	if mgr.workspaceDir != config.DefaultWorkspaceDir() {
		t.Fatalf("expected workspace default %q, got %q", config.DefaultWorkspaceDir(), mgr.workspaceDir)
	}
	if mgr.approvalsPath != filepath.Join(config.DefaultWorkspaceDir(), "ops", "approvals.json") {
		t.Fatalf("expected approvals path under default workspace, got %q", mgr.approvalsPath)
	}
}

func TestManager_SaveApprovalsPreservesExistingFileWhenAtomicTempCannotBeCreated(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
	workspace := filepath.Join(t.TempDir(), "workspace")
	mgr := NewManager(workspace, Options{HomeDir: filepath.Join(t.TempDir(), "home")})
	if err := os.MkdirAll(filepath.Dir(mgr.approvalsPath), 0o755); err != nil {
		t.Fatalf("mkdir approvals dir: %v", err)
	}
	original := []byte(`[{"id":"apr_1","type":"cleanup","status":"pending","requested_at":"2026-03-07T11:00:00Z","updated_at":"2026-03-07T11:00:00Z","plan":{"approval_id":"apr_1","created_at":"2026-03-07T11:00:00Z","candidates":null}}]`)
	if err := os.WriteFile(mgr.approvalsPath, original, 0o644); err != nil {
		t.Fatalf("seed approvals: %v", err)
	}
	if err := os.Chmod(filepath.Dir(mgr.approvalsPath), 0o500); err != nil {
		t.Fatalf("chmod approvals dir: %v", err)
	}
	defer os.Chmod(filepath.Dir(mgr.approvalsPath), 0o755)

	err := mgr.saveApprovalsLocked([]Approval{
		{
			ID:          "apr_2",
			Type:        "cleanup",
			Status:      "approved",
			RequestedAt: time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC),
			Plan: CleanupPlan{
				ApprovalID: "apr_2",
				CreatedAt:  time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC),
			},
		},
	})
	if err == nil {
		t.Fatalf("expected save approvals to fail when temp file cannot be created")
	}
	got, readErr := os.ReadFile(mgr.approvalsPath)
	if readErr != nil {
		t.Fatalf("read approvals: %v", readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("expected original approvals to be preserved, got %q", got)
	}
}

func requireOpsFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("expected %s mode %04o, got %04o", path, want, got)
	}
}

func TestManager_IsSafeCleanupPath_OnlyAllowsConfiguredRoots(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	home := filepath.Join(t.TempDir(), "home")
	downloads := filepath.Join(home, "Downloads")
	if err := os.MkdirAll(downloads, 0o755); err != nil {
		t.Fatalf("mkdir downloads: %v", err)
	}
	allowedPath := filepath.Join(downloads, "nested", "old.bin")
	if err := os.MkdirAll(filepath.Dir(allowedPath), 0o755); err != nil {
		t.Fatalf("mkdir allowed path dir: %v", err)
	}
	outsidePath := filepath.Join(home, "Documents", "unsafe.txt")

	mgr := NewManager(workspace, Options{HomeDir: home})

	if !mgr.isSafeCleanupPath(allowedPath) {
		t.Fatalf("expected downloads descendant to be allowed")
	}
	if mgr.isSafeCleanupPath(outsidePath) {
		t.Fatalf("expected documents path to be rejected")
	}
}
