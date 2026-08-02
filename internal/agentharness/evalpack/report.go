package evalpack

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func WriteJSONL(w io.Writer, report Report) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	metadata := struct {
		RecordType    string `json:"record_type"`
		SchemaVersion string `json:"schema_version"`
		Mode          Mode   `json:"mode"`
		Version       string `json:"version"`
		Commit        string `json:"commit"`
		GeneratedAt   string `json:"generated_at"`
	}{"metadata", report.SchemaVersion, report.Mode, report.Version, report.Commit, report.GeneratedAt}
	if err := encoder.Encode(metadata); err != nil {
		return fmt.Errorf("encode evaluation metadata: %w", err)
	}
	for _, result := range report.Results {
		record := struct {
			RecordType string `json:"record_type"`
			ScenarioResult
		}{"scenario", result}
		if err := encoder.Encode(record); err != nil {
			return fmt.Errorf("encode scenario %q: %w", result.ID, err)
		}
	}
	summary := struct {
		RecordType string `json:"record_type"`
		Summary
	}{"summary", report.Summary}
	if err := encoder.Encode(summary); err != nil {
		return fmt.Errorf("encode evaluation summary: %w", err)
	}
	return nil
}

func WriteMarkdown(w io.Writer, report Report) error {
	write := func(format string, args ...any) error {
		_, err := fmt.Fprintf(w, format, args...)
		return err
	}
	if err := write("# TARS Agent Harness Evaluation\n\n"); err != nil {
		return err
	}
	if err := write("- Schema: `%s`\n- Mode: `%s`\n- TARS version: `%s`\n- Commit: `%s`\n- Generated: `%s`\n\n", report.SchemaVersion, report.Mode, report.Version, report.Commit, report.GeneratedAt); err != nil {
		return err
	}
	if err := write("## Summary\n\n"); err != nil {
		return err
	}
	if err := write("| Metric | Value |\n| --- | ---: |\n"); err != nil {
		return err
	}
	rows := [][2]string{
		{"Scenarios completed", fmt.Sprintf("%d / %d", report.Summary.Completed, report.Summary.Total)},
		{"Baseline expectations met", fmt.Sprintf("%d / %d", report.Summary.ExpectationsMet, report.Summary.Total-report.Summary.Skipped)},
		{"Task success", percent(report.Summary.TaskSuccessRate)},
		{"Independent verifier pass", percent(report.Summary.VerifierPassRate)},
		{"Restart recovery", percent(report.Summary.RestartRecoveryRate)},
		{"Duplicate side effects", fmt.Sprintf("%d", report.Summary.DuplicateSideEffects)},
		{"Operator interventions", fmt.Sprintf("%d", report.Summary.OperatorInterventions)},
		{"Average TTFT", fmt.Sprintf("%.1f ms", report.Summary.AverageTTFTMillis)},
		{"Tokens (input / output)", fmt.Sprintf("%d / %d", report.Summary.InputTokens, report.Summary.OutputTokens)},
		{"Estimated cost", fmt.Sprintf("$%.6f", report.Summary.EstimatedCostUSD)},
	}
	for _, row := range rows {
		if err := write("| %s | %s |\n", row[0], row[1]); err != nil {
			return err
		}
	}
	if err := write("\n## Scenarios\n\n| Scenario | Category | Baseline | Task | Verifier | Restart | Duplicates | Interventions | TTFT |\n| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |\n"); err != nil {
		return err
	}
	for _, result := range report.Results {
		if err := write("| %s | %s | %s | %s | %s | %s | %d | %d | %d ms |\n",
			markdownCell(result.Title), markdownCell(result.Category), result.Status,
			yesNo(result.TaskSuccess), yesNo(result.VerifierPass), yesNo(result.RestartRecovered),
			result.DuplicateSideEffects, result.OperatorInterventions, result.TTFTMillis); err != nil {
			return err
		}
	}
	if err := write("\n## Known gaps\n\n"); err != nil {
		return err
	}
	gapCount := 0
	for _, result := range report.Results {
		if result.Status == StatusSkipped || (result.Status == StatusPassed && result.VerifierPass && result.DuplicateSideEffects == 0) {
			continue
		}
		gapCount++
		detail := strings.TrimSpace(result.Details)
		if detail == "" {
			detail = "The measured quality or durability signal is below the desired end state."
		}
		if err := write("- `%s`: %s\n", result.ID, detail); err != nil {
			return err
		}
	}
	if gapCount == 0 {
		if err := write("- None in this run.\n"); err != nil {
			return err
		}
	}
	return nil
}

func percent(value float64) string {
	return fmt.Sprintf("%.1f%%", value*100)
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func markdownCell(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), "|", "\\|")
}
