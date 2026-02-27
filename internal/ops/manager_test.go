package ops

import (
	"context"
	"os"
	"path/filepath"
	"testing"
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
