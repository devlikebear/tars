package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/devlikebear/tars/internal/ops"
)

func TestOpsTools_StatusPlanApply(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(filepath.Join(home, "Downloads"), 0o755); err != nil {
		t.Fatalf("mkdir downloads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "Downloads", "demo.tmp"), []byte("demo"), 0o644); err != nil {
		t.Fatalf("write demo file: %v", err)
	}
	mgr := ops.NewManager(workspace, ops.Options{HomeDir: home})

	status := NewOpsStatusTool(mgr)
	statusResult, err := status.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute ops_status: %v", err)
	}
	if statusResult.IsError {
		t.Fatalf("expected ops_status success: %s", statusResult.Text())
	}

	planTool := NewOpsCleanupPlanTool(mgr)
	planResult, err := planTool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute ops_cleanup_plan: %v", err)
	}
	if planResult.IsError {
		t.Fatalf("expected ops_cleanup_plan success: %s", planResult.Text())
	}
	var payload struct {
		ApprovalID string `json:"approval_id"`
	}
	if err := json.Unmarshal([]byte(planResult.Text()), &payload); err != nil {
		t.Fatalf("decode plan payload: %v", err)
	}
	if payload.ApprovalID == "" {
		t.Fatalf("expected approval id in plan payload")
	}
	if err := mgr.Approve(payload.ApprovalID); err != nil {
		t.Fatalf("approve plan: %v", err)
	}

	applyTool := NewOpsCleanupApplyTool(mgr)
	applyResult, err := applyTool.Execute(context.Background(), json.RawMessage(`{"approval_id":"`+payload.ApprovalID+`"}`))
	if err != nil {
		t.Fatalf("execute ops_cleanup_apply: %v", err)
	}
	if applyResult.IsError {
		t.Fatalf("expected ops_cleanup_apply success: %s", applyResult.Text())
	}
}
