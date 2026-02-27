package tarsclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_OpsAndApprovalMethods(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/ops/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"disk_used_percent": 71.2, "process_count": 210, "disk_free_bytes": 100})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/ops/cleanup/plan":
			_ = json.NewEncoder(w).Encode(map[string]any{"approval_id": "apr_1", "total_bytes": 1000, "candidates": []map[string]any{{"path": "/tmp/a", "size_bytes": 1000}}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/ops/approvals":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "apr_1", "type": "cleanup", "status": "pending", "plan": map[string]any{"approval_id": "apr_1"}}})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/ops/approvals/apr_1/approve":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/ops/approvals/apr_1/reject":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/ops/cleanup/apply":
			_ = json.NewEncoder(w).Encode(map[string]any{"approval_id": "apr_1", "deleted_count": 1, "deleted_bytes": 1000})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(Config{ServerURL: server.URL})
	status, err := client.OpsStatus(context.Background())
	if err != nil {
		t.Fatalf("OpsStatus: %v", err)
	}
	if status.ProcessCount != 210 {
		t.Fatalf("unexpected status: %+v", status)
	}
	plan, err := client.CreateCleanupPlan(context.Background())
	if err != nil {
		t.Fatalf("CreateCleanupPlan: %v", err)
	}
	if plan.ApprovalID != "apr_1" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	items, err := client.ListApprovals(context.Background())
	if err != nil {
		t.Fatalf("ListApprovals: %v", err)
	}
	if len(items) != 1 || items[0].ID != "apr_1" {
		t.Fatalf("unexpected approvals: %+v", items)
	}
	if err := client.ApproveCleanup(context.Background(), "apr_1"); err != nil {
		t.Fatalf("ApproveCleanup: %v", err)
	}
	if err := client.RejectCleanup(context.Background(), "apr_1"); err != nil {
		t.Fatalf("RejectCleanup: %v", err)
	}
	apply, err := client.ApplyCleanup(context.Background(), "apr_1")
	if err != nil {
		t.Fatalf("ApplyCleanup: %v", err)
	}
	if apply.DeletedCount != 1 {
		t.Fatalf("unexpected apply result: %+v", apply)
	}
}
