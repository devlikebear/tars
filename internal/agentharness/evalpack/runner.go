package evalpack

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Mode string

const (
	ModeDeterministic Mode = "deterministic"
	ModeLive          Mode = "live"
)

type ResultStatus string

const (
	StatusPassed  ResultStatus = "passed"
	StatusFailed  ResultStatus = "failed"
	StatusError   ResultStatus = "error"
	StatusSkipped ResultStatus = "skipped"
)

type Metrics struct {
	TaskSuccess           bool    `json:"task_success"`
	VerifierPass          bool    `json:"verifier_pass"`
	RestartRecovered      bool    `json:"restart_recovered"`
	DuplicateSideEffects  int     `json:"duplicate_side_effects"`
	OperatorInterventions int     `json:"operator_interventions"`
	TTFTMillis            int64   `json:"ttft_ms"`
	TTFTSource            string  `json:"ttft_source"`
	InputTokens           int     `json:"input_tokens"`
	OutputTokens          int     `json:"output_tokens"`
	EstimatedCostUSD      float64 `json:"estimated_cost_usd"`
}

type ScenarioResult struct {
	ID             string       `json:"id"`
	Title          string       `json:"title"`
	Category       string       `json:"category"`
	Status         ResultStatus `json:"status"`
	ExpectationMet bool         `json:"expectation_met"`
	Metrics
	Details string `json:"details,omitempty"`
	Error   string `json:"error,omitempty"`
}

type Summary struct {
	Total                 int     `json:"total"`
	Completed             int     `json:"completed"`
	Skipped               int     `json:"skipped"`
	Errors                int     `json:"errors"`
	ExpectationsMet       int     `json:"expectations_met"`
	TaskSuccessRate       float64 `json:"task_success_rate"`
	VerifierPassRate      float64 `json:"verifier_pass_rate"`
	RestartRecoveryRate   float64 `json:"restart_recovery_rate"`
	DuplicateSideEffects  int     `json:"duplicate_side_effects"`
	OperatorInterventions int     `json:"operator_interventions"`
	AverageTTFTMillis     float64 `json:"average_ttft_ms"`
	InputTokens           int     `json:"input_tokens"`
	OutputTokens          int     `json:"output_tokens"`
	EstimatedCostUSD      float64 `json:"estimated_cost_usd"`
}

type Report struct {
	SchemaVersion string           `json:"schema_version"`
	Mode          Mode             `json:"mode"`
	Version       string           `json:"version"`
	Commit        string           `json:"commit"`
	GeneratedAt   string           `json:"generated_at"`
	Results       []ScenarioResult `json:"results"`
	Summary       Summary          `json:"summary"`
}

type RunConfig struct {
	Mode    Mode
	Version string
	Commit  string
	Now     func() time.Time
}

type Executor interface {
	Execute(context.Context, Scenario) (Metrics, error)
}

type DetailedExecutor interface {
	ExecuteDetailed(context.Context, Scenario) (Metrics, string, error)
}

type Runner struct {
	Executor Executor
	Config   RunConfig
}

func (r Runner) Run(ctx context.Context, pack Pack) (Report, error) {
	if err := ValidatePack(pack); err != nil {
		return Report{}, err
	}
	if r.Executor == nil {
		return Report{}, fmt.Errorf("executor is required")
	}
	mode := r.Config.Mode
	if mode == "" {
		mode = ModeDeterministic
	}
	if mode != ModeDeterministic && mode != ModeLive {
		return Report{}, fmt.Errorf("unsupported evaluation mode %q", mode)
	}
	nowFn := r.Config.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	report := Report{
		SchemaVersion: pack.SchemaVersion,
		Mode:          mode,
		Version:       fallbackMetadata(r.Config.Version),
		Commit:        fallbackMetadata(r.Config.Commit),
		GeneratedAt:   nowFn().UTC().Format(time.RFC3339),
		Results:       make([]ScenarioResult, 0, len(pack.Scenarios)),
	}
	for _, scenario := range pack.Scenarios {
		result := ScenarioResult{ID: scenario.ID, Title: scenario.Title, Category: scenario.Category}
		if mode == ModeLive && !scenario.LiveSupported {
			result.Status = StatusSkipped
			result.Details = "scenario is deterministic-only"
			report.Results = append(report.Results, result)
			continue
		}
		var (
			metrics Metrics
			details string
			err     error
		)
		if detailed, ok := r.Executor.(DetailedExecutor); ok {
			metrics, details, err = detailed.ExecuteDetailed(ctx, scenario)
		} else {
			metrics, err = r.Executor.Execute(ctx, scenario)
		}
		result.Metrics = metrics
		result.Details = strings.TrimSpace(details)
		if err != nil {
			result.Status = StatusError
			result.Error = err.Error()
		} else {
			result.ExpectationMet = matchesExpected(metrics, scenario.Expected)
			if result.ExpectationMet {
				result.Status = StatusPassed
			} else {
				result.Status = StatusFailed
			}
		}
		report.Results = append(report.Results, result)
	}
	report.Summary = summarize(report.Results)
	return report, nil
}

func fallbackMetadata(value string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return "unknown"
}

func matchesExpected(got Metrics, want ExpectedMetrics) bool {
	return got.TaskSuccess == want.TaskSuccess &&
		got.VerifierPass == want.VerifierPass &&
		got.RestartRecovered == want.RestartRecovered &&
		got.DuplicateSideEffects == want.DuplicateSideEffects &&
		got.OperatorInterventions == want.OperatorInterventions
}

func summarize(results []ScenarioResult) Summary {
	summary := Summary{Total: len(results)}
	var taskSuccess, verifierPass, restartRecovered, ttftCount int
	var ttftTotal int64
	for _, result := range results {
		switch result.Status {
		case StatusSkipped:
			summary.Skipped++
			continue
		case StatusError:
			summary.Errors++
		default:
			if result.ExpectationMet {
				summary.ExpectationsMet++
			}
		}
		summary.Completed++
		if result.TaskSuccess {
			taskSuccess++
		}
		if result.VerifierPass {
			verifierPass++
		}
		if result.RestartRecovered {
			restartRecovered++
		}
		summary.DuplicateSideEffects += result.DuplicateSideEffects
		summary.OperatorInterventions += result.OperatorInterventions
		summary.InputTokens += result.InputTokens
		summary.OutputTokens += result.OutputTokens
		summary.EstimatedCostUSD += result.EstimatedCostUSD
		if result.TTFTMillis > 0 {
			ttftTotal += result.TTFTMillis
			ttftCount++
		}
	}
	if summary.Completed > 0 {
		denominator := float64(summary.Completed)
		summary.TaskSuccessRate = float64(taskSuccess) / denominator
		summary.VerifierPassRate = float64(verifierPass) / denominator
		summary.RestartRecoveryRate = float64(restartRecovered) / denominator
	}
	if ttftCount > 0 {
		summary.AverageTTFTMillis = float64(ttftTotal) / float64(ttftCount)
	}
	return summary
}
