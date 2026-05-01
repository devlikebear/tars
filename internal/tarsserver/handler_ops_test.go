package tarsserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/ops"
	"github.com/rs/zerolog"
)

func TestOpsAPI_StatusAndApprovalFlow(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(filepath.Join(home, "Downloads"), 0o755); err != nil {
		t.Fatalf("mkdir downloads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "Downloads", "x.tmp"), []byte("demo"), 0o644); err != nil {
		t.Fatalf("write cleanup file: %v", err)
	}
	mgr := ops.NewManager(workspace, ops.Options{HomeDir: home})
	handler := newOpsAPIHandler(mgr, zerolog.New(io.Discard), nil, nil)

	statusReq := httptest.NewRequest(http.MethodGet, "/v1/ops/status", nil)
	statusRec := httptest.NewRecorder()
	handler.ServeHTTP(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for status, got %d body=%q", statusRec.Code, statusRec.Body.String())
	}

	planReq := httptest.NewRequest(http.MethodPost, "/v1/ops/cleanup/plan", strings.NewReader(`{}`))
	planReq.Header.Set("Content-Type", "application/json")
	planRec := httptest.NewRecorder()
	handler.ServeHTTP(planRec, planReq)
	if planRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for cleanup plan, got %d body=%q", planRec.Code, planRec.Body.String())
	}
	var plan ops.CleanupPlan
	if err := json.Unmarshal(planRec.Body.Bytes(), &plan); err != nil {
		t.Fatalf("decode cleanup plan: %v", err)
	}

	// Approve now auto-applies the cleanup plan
	approveReq := httptest.NewRequest(http.MethodPost, "/v1/ops/approvals/"+plan.ApprovalID+"/approve", strings.NewReader(`{}`))
	approveReq.Header.Set("Content-Type", "application/json")
	approveRec := httptest.NewRecorder()
	handler.ServeHTTP(approveRec, approveReq)
	if approveRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for approve, got %d body=%q", approveRec.Code, approveRec.Body.String())
	}
}

func TestOpsAPI_CleanupApplyRejectsOversizedBody(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(filepath.Join(home, "Downloads"), 0o755); err != nil {
		t.Fatalf("mkdir downloads: %v", err)
	}
	mgr := ops.NewManager(workspace, ops.Options{HomeDir: home})
	handler := newOpsAPIHandler(mgr, zerolog.New(io.Discard), nil, nil)

	body := `{"approval_id":"` + strings.Repeat("a", int(defaultJSONBodyLimitBytes+1)) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/ops/cleanup/apply", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d body=%q", rec.Code, rec.Body.String())
	}
	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if payload["error"] != "request body too large" {
		t.Fatalf("unexpected body: %+v", payload)
	}
}

func TestOpsAPI_AutomationAuditList(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	home := filepath.Join(t.TempDir(), "home")
	mgr := ops.NewManager(workspace, ops.Options{HomeDir: home})
	if _, err := mgr.RecordAutomationAudit(ops.AutomationAuditEntry{
		Actor:     "git",
		Action:    "git_stage",
		Reason:    "user approved staging",
		SessionID: "sess_1",
		CWD:       "/tmp/workspace",
		Result:    "success",
	}); err != nil {
		t.Fatalf("record automation audit: %v", err)
	}
	handler := newOpsAPIHandler(mgr, zerolog.New(io.Discard), nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/ops/automation-audit?limit=10", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for automation audit, got %d body=%q", rec.Code, rec.Body.String())
	}
	var payload struct {
		Items []ops.AutomationAuditEntry `json:"items"`
		Count int                        `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode automation audit payload: %v", err)
	}
	if payload.Count != 1 || len(payload.Items) != 1 {
		t.Fatalf("expected one audit entry, got %+v", payload)
	}
	if payload.Items[0].Action != "git_stage" || payload.Items[0].Result != "success" {
		t.Fatalf("unexpected audit entry: %+v", payload.Items[0])
	}
}
