package evalpack

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubExecutor struct {
	results map[string]Metrics
	errors  map[string]error
}

func (s stubExecutor) Execute(_ context.Context, scenario Scenario) (Metrics, error) {
	return s.results[scenario.ID], s.errors[scenario.ID]
}

func TestRunnerAggregatesQualityReliabilityAndCostMetrics(t *testing.T) {
	pack := Pack{SchemaVersion: SchemaVersion, Scenarios: []Scenario{
		{
			ID: "passes", Title: "Passes", Category: "correctness", Kind: "single_agent",
			Prompt: "ok", SuccessToken: "OK", LiveSupported: true,
			Expected: ExpectedMetrics{TaskSuccess: true, VerifierPass: true, RestartRecovered: true},
		},
		{
			ID: "known-gap", Title: "Known gap", Category: "durability", Kind: "restart_recovery",
			Prompt: "recover", SuccessToken: "OK",
			Expected: ExpectedMetrics{TaskSuccess: false, VerifierPass: false, DuplicateSideEffects: 1, OperatorInterventions: 1},
		},
	}}
	now := time.Date(2026, time.August, 2, 3, 4, 5, 0, time.UTC)
	runner := Runner{
		Executor: stubExecutor{results: map[string]Metrics{
			"passes": {
				TaskSuccess: true, VerifierPass: true, RestartRecovered: true,
				TTFTMillis: 12, TTFTSource: "measured", InputTokens: 10, OutputTokens: 5, EstimatedCostUSD: 0.002,
			},
			"known-gap": {
				TaskSuccess: false, VerifierPass: false, DuplicateSideEffects: 1,
				OperatorInterventions: 1, TTFTMillis: 8, TTFTSource: "deterministic",
			},
		}},
		Config: RunConfig{Mode: ModeDeterministic, Version: "0.34.3", Commit: "abc123", Now: func() time.Time { return now }},
	}

	report, err := runner.Run(context.Background(), pack)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if report.GeneratedAt != "2026-08-02T03:04:05Z" || report.Version != "0.34.3" || report.Commit != "abc123" {
		t.Fatalf("unexpected metadata: %+v", report)
	}
	if report.Summary.Total != 2 || report.Summary.Completed != 2 || report.Summary.ExpectationsMet != 2 {
		t.Fatalf("unexpected counts: %+v", report.Summary)
	}
	if report.Summary.TaskSuccessRate != 0.5 || report.Summary.VerifierPassRate != 0.5 || report.Summary.RestartRecoveryRate != 0.5 {
		t.Fatalf("unexpected rates: %+v", report.Summary)
	}
	if report.Summary.DuplicateSideEffects != 1 || report.Summary.OperatorInterventions != 1 {
		t.Fatalf("unexpected reliability totals: %+v", report.Summary)
	}
	if report.Summary.InputTokens != 10 || report.Summary.OutputTokens != 5 || report.Summary.EstimatedCostUSD != 0.002 {
		t.Fatalf("unexpected usage totals: %+v", report.Summary)
	}
}

func TestRunnerKeepsExecutionErrorsAsMachineReadableResults(t *testing.T) {
	scenario := validScenario("broken")
	runner := Runner{
		Executor: stubExecutor{errors: map[string]error{"broken": errors.New("tool crashed")}},
		Config:   RunConfig{Mode: ModeDeterministic, Now: time.Now},
	}
	report, err := runner.Run(context.Background(), Pack{SchemaVersion: SchemaVersion, Scenarios: []Scenario{scenario}})
	if err != nil {
		t.Fatalf("run should preserve scenario errors in the report: %v", err)
	}
	if len(report.Results) != 1 || report.Results[0].Status != StatusError || report.Results[0].Error != "tool crashed" {
		t.Fatalf("unexpected result: %+v", report.Results)
	}
	if report.Summary.Errors != 1 || report.Summary.ExpectationsMet != 0 {
		t.Fatalf("unexpected summary: %+v", report.Summary)
	}
}

func TestLiveRunnerSkipsUnsupportedScenarios(t *testing.T) {
	scenario := validScenario("deterministic-only")
	runner := Runner{
		Executor: stubExecutor{},
		Config:   RunConfig{Mode: ModeLive, Now: time.Now},
	}
	report, err := runner.Run(context.Background(), Pack{SchemaVersion: SchemaVersion, Scenarios: []Scenario{scenario}})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if report.Results[0].Status != StatusSkipped || report.Summary.Skipped != 1 {
		t.Fatalf("expected skipped live-only result: %+v", report)
	}
}

func TestRunnerValidatesConfigurationAndDefaults(t *testing.T) {
	pack := Pack{SchemaVersion: SchemaVersion, Scenarios: []Scenario{validScenario("defaulted")}}
	if _, err := (Runner{}).Run(context.Background(), pack); err == nil {
		t.Fatal("expected missing executor error")
	}
	if _, err := (Runner{Executor: stubExecutor{}, Config: RunConfig{Mode: "invalid"}}).Run(context.Background(), pack); err == nil {
		t.Fatal("expected invalid mode error")
	}
	if _, err := (Runner{Executor: stubExecutor{}}).Run(context.Background(), Pack{}); err == nil {
		t.Fatal("expected invalid pack error")
	}
	report, err := (Runner{Executor: stubExecutor{results: map[string]Metrics{
		"defaulted": {TaskSuccess: true, VerifierPass: true},
	}}}).Run(context.Background(), pack)
	if err != nil {
		t.Fatalf("run with defaults: %v", err)
	}
	if report.Mode != ModeDeterministic || report.Version != "unknown" || report.Commit != "unknown" || report.GeneratedAt == "" {
		t.Fatalf("defaults not applied: %+v", report)
	}
}
