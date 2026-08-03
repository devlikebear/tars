package evalpack

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestNativeExecutorMatchesCanonicalBaseline(t *testing.T) {
	pack, err := LoadPack(filepath.Join("..", "..", "..", "testdata", "agent-harness", "scenarios.json"))
	if err != nil {
		t.Fatalf("load pack: %v", err)
	}
	report, err := (Runner{
		Executor: NativeExecutor{RootDir: t.TempDir()},
		Config: RunConfig{
			Mode: ModeDeterministic, Version: "test", Commit: "test",
			Now: func() time.Time { return time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC) },
		},
	}).Run(context.Background(), pack)
	if err != nil {
		t.Fatalf("run pack: %v", err)
	}
	if report.Summary.Total < 10 || report.Summary.Completed != report.Summary.Total {
		t.Fatalf("unexpected completion summary: %+v", report.Summary)
	}
	if report.Summary.ExpectationsMet != report.Summary.Total {
		for _, result := range report.Results {
			if !result.ExpectationMet {
				t.Errorf("scenario %s did not match baseline: status=%s metrics=%+v error=%s", result.ID, result.Status, result.Metrics, result.Error)
			}
		}
	}
	if report.Summary.TaskSuccessRate >= 1 {
		t.Fatalf("baseline must expose current reliability gaps, got %+v", report.Summary)
	}
	if report.Summary.DuplicateSideEffects == 0 {
		t.Fatalf("baseline must expose replayed side effects, got %+v", report.Summary)
	}
}

func TestNativeExecutorConvenienceAndValidationBranches(t *testing.T) {
	executor := NativeExecutor{RootDir: t.TempDir()}
	metrics, err := executor.Execute(context.Background(), validScenario("single-wrapper"))
	if err != nil || !metrics.TaskSuccess {
		t.Fatalf("execute wrapper: %+v, %v", metrics, err)
	}
	unknown := validScenario("unknown")
	unknown.Kind = "not_supported"
	if _, _, err := executor.ExecuteDetailed(context.Background(), unknown); err == nil {
		t.Fatal("expected unsupported kind error")
	}
	invalidApproval := validScenario("approval")
	if _, _, err := executeApprovalGate(invalidApproval); err == nil {
		t.Fatal("expected invalid approval decision")
	}
	invalidBudget := validScenario("budget")
	invalidBudget.Parameters = map[string]string{"estimated_tokens": "bad", "token_budget": "also-bad"}
	if _, _, err := executeBudgetGuard(invalidBudget); err == nil {
		t.Fatal("expected invalid estimated token count")
	}
	invalidBudget.Parameters["estimated_tokens"] = "10"
	if _, _, err := executeBudgetGuard(invalidBudget); err == nil {
		t.Fatal("expected invalid budget")
	}
	if got := sanitizePathPart(" "); got != "scenario" {
		t.Fatalf("empty path part = %q", got)
	}
	if got := sanitizePathPart("a/b"); got != "a-b" {
		t.Fatalf("sanitized path part = %q", got)
	}
	if estimateTokens("  ") != 0 || boolCount(false) != 0 {
		t.Fatal("empty metrics helpers returned non-zero")
	}
}
