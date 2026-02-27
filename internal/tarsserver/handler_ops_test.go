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

	"github.com/devlikebear/tarsncase/internal/ops"
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
	handler := newOpsAPIHandler(mgr, zerolog.New(io.Discard), nil)

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

	approveReq := httptest.NewRequest(http.MethodPost, "/v1/ops/approvals/"+plan.ApprovalID+"/approve", strings.NewReader(`{}`))
	approveReq.Header.Set("Content-Type", "application/json")
	approveRec := httptest.NewRecorder()
	handler.ServeHTTP(approveRec, approveReq)
	if approveRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for approve, got %d body=%q", approveRec.Code, approveRec.Body.String())
	}

	applyReq := httptest.NewRequest(http.MethodPost, "/v1/ops/cleanup/apply", strings.NewReader(`{"approval_id":"`+plan.ApprovalID+`"}`))
	applyReq.Header.Set("Content-Type", "application/json")
	applyRec := httptest.NewRecorder()
	handler.ServeHTTP(applyRec, applyReq)
	if applyRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for apply, got %d body=%q", applyRec.Code, applyRec.Body.String())
	}
}
