package ops

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
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
