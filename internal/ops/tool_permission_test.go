package ops

import (
	"testing"
)

func TestToolPermissionApprovalLifecycle(t *testing.T) {
	mgr := NewManager(t.TempDir(), Options{HomeDir: t.TempDir()})

	if _, err := mgr.CreateToolPermissionApproval(ToolPermissionRequest{SessionID: "s"}); err == nil {
		t.Fatal("expected an error without a tool name")
	}
	first, err := mgr.CreateToolPermissionApproval(ToolPermissionRequest{SessionID: " s1 ", Source: "cron", ToolName: " exec ", Preview: "go test ./..."})
	if err != nil {
		t.Fatal(err)
	}
	if first.Type != ApprovalTypeToolPermission || first.Status != ApprovalStatusPending {
		t.Fatalf("created %+v", first)
	}
	if first.ToolPermission == nil || first.ToolPermission.SessionID != "s1" || first.ToolPermission.ToolName != "exec" {
		t.Fatalf("request not normalized: %+v", first.ToolPermission)
	}
	second, err := mgr.CreateToolPermissionApproval(ToolPermissionRequest{SessionID: "s1", ToolName: "write_file"})
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID {
		t.Fatalf("ids collide: %s", first.ID)
	}

	if err := mgr.ReviewToolPermission(first.ID, true); err != nil {
		t.Fatal(err)
	}
	got, _ := mgr.GetApproval(first.ID)
	if got.Status != ApprovalStatusApproved || got.ReviewedAt == nil {
		t.Fatalf("after approve: %+v", got)
	}
	// A reviewed question cannot be reviewed again or expired.
	if err := mgr.ReviewToolPermission(first.ID, false); !IsApprovalNotPending(err) {
		t.Fatalf("second review err = %v", err)
	}
	if err := mgr.ExpireToolPermission(first.ID); err != nil {
		t.Fatalf("expire after review should be a no-op, got %v", err)
	}
	if got, _ := mgr.GetApproval(first.ID); got.Status != ApprovalStatusApproved {
		t.Fatalf("expire changed a reviewed approval: %s", got.Status)
	}

	if err := mgr.ReviewToolPermission(second.ID, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := mgr.GetApproval(second.ID); got.Status != ApprovalStatusRejected {
		t.Fatalf("after reject: %s", got.Status)
	}
	if err := mgr.ReviewToolPermission("missing", true); err == nil || IsApprovalNotPending(err) {
		t.Fatalf("missing approval err = %v", err)
	}
}

func TestToolPermissionRejectsOtherApprovalTypes(t *testing.T) {
	mgr := NewManager(t.TempDir(), Options{HomeDir: t.TempDir()})
	plan, err := mgr.CreateCleanupPlan(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.ReviewToolPermission(plan.ApprovalID, true); err == nil {
		t.Fatal("expected a cleanup approval to be refused")
	}
}

func TestExpirePendingToolPermissions(t *testing.T) {
	mgr := NewManager(t.TempDir(), Options{HomeDir: t.TempDir()})
	if n, err := mgr.ExpirePendingToolPermissions(); err != nil || n != 0 {
		t.Fatalf("empty queue: n=%d err=%v", n, err)
	}
	pending, _ := mgr.CreateToolPermissionApproval(ToolPermissionRequest{ToolName: "exec"})
	answered, _ := mgr.CreateToolPermissionApproval(ToolPermissionRequest{ToolName: "exec"})
	_ = mgr.ReviewToolPermission(answered.ID, true)

	n, err := mgr.ExpirePendingToolPermissions()
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	got, _ := mgr.GetApproval(pending.ID)
	if got.Status != ApprovalStatusExpired || got.Note == "" {
		t.Fatalf("pending after restart: %+v", got)
	}
	if got, _ := mgr.GetApproval(answered.ID); got.Status != ApprovalStatusApproved {
		t.Fatalf("answered changed: %s", got.Status)
	}
	if err := mgr.ExpireToolPermission(pending.ID); err != nil {
		t.Fatal(err)
	}
}

func TestToolPermissionNilManager(t *testing.T) {
	var mgr *Manager
	if _, err := mgr.CreateToolPermissionApproval(ToolPermissionRequest{ToolName: "exec"}); err == nil {
		t.Fatal("create")
	}
	if err := mgr.ReviewToolPermission("x", true); err == nil {
		t.Fatal("review")
	}
	if _, err := mgr.ExpirePendingToolPermissions(); err == nil {
		t.Fatal("expire pending")
	}
}
