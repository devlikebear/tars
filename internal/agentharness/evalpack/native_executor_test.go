package evalpack

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"testing/synctest"
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

// A loaded Windows runner once took over 2s to carry a run from Spawn to its
// prompt, and the executor reported that slowness as a baseline error. Inside
// the bubble the 10s delay costs no real time, while git and session I/O still
// run at their own pace without advancing the fake clock.
func TestNativeExecutorBaselineToleratesSlowPathToPrompt(t *testing.T) {
	pack, err := LoadPack(filepath.Join("..", "..", "..", "testdata", "agent-harness", "scenarios.json"))
	if err != nil {
		t.Fatalf("load pack: %v", err)
	}
	fakePromptDelay = 10 * time.Second
	defer func() { fakePromptDelay = 0 }()
	synctest.Test(t, func(t *testing.T) {
		report, err := (Runner{
			Executor: NativeExecutor{RootDir: t.TempDir()},
			Config:   RunConfig{Mode: ModeDeterministic, Version: "test", Commit: "test"},
		}).Run(t.Context(), pack)
		if err != nil {
			t.Fatalf("run pack: %v", err)
		}
		for _, result := range report.Results {
			if !result.ExpectationMet {
				t.Errorf("scenario %s did not match baseline: status=%s error=%s", result.ID, result.Status, result.Error)
			}
		}
	})
}

func TestAwaitSignalReportsContextEndAndGuardExpiry(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := awaitSignal(canceled, make(chan struct{}), "never"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled wait = %v", err)
	}
	synctest.Test(t, func(t *testing.T) {
		err := awaitSignal(t.Context(), make(chan struct{}), "worker did not start")
		if want := "worker did not start within " + nativeWaitTimeout.String(); err == nil || err.Error() != want {
			t.Fatalf("expired wait = %v, want %s", err, want)
		}
	})
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
