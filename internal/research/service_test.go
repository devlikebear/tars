package research

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestService_RunCreatesReportAndSummary(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 2, 27, 14, 30, 0, 0, time.UTC)
	svc := NewService(workspace, Options{Now: func() time.Time { return now }})

	report, err := svc.Run(RunInput{
		Topic:   "MCP runtime health",
		Summary: "Weekly update",
		Body:    "- finding A\n- finding B",
	})
	if err != nil {
		t.Fatalf("run research: %v", err)
	}
	if report.Path == "" {
		t.Fatalf("expected report path")
	}
	if _, err := os.Stat(report.Path); err != nil {
		t.Fatalf("expected report file to exist: %v", err)
	}
	summaryPath := filepath.Join(workspace, "reports", "summary.jsonl")
	if _, err := os.Stat(summaryPath); err != nil {
		t.Fatalf("expected summary jsonl to exist: %v", err)
	}
}
