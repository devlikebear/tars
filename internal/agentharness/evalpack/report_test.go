package evalpack

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

type reportFailWriter struct{}

func (reportFailWriter) Write([]byte) (int, error) {
	return 0, errors.New("report write failed")
}

func TestWriteJSONLEmitsMetadataResultsAndSummary(t *testing.T) {
	report := Report{
		SchemaVersion: SchemaVersion,
		Mode:          ModeDeterministic,
		Version:       "0.34.3",
		Commit:        "abc123",
		GeneratedAt:   "2026-08-02T03:04:05Z",
		Results: []ScenarioResult{{
			ID: "single", Title: "Single", Category: "correctness", Status: StatusPassed,
			ExpectationMet: true, Metrics: Metrics{TaskSuccess: true, VerifierPass: true},
		}},
		Summary: Summary{Total: 1, Completed: 1, ExpectationsMet: 1, TaskSuccessRate: 1, VerifierPassRate: 1},
	}
	var out bytes.Buffer
	if err := WriteJSONL(&out, report); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("line count = %d, want 3\n%s", len(lines), out.String())
	}
	for _, needle := range []string{`"record_type":"metadata"`, `"record_type":"scenario"`, `"record_type":"summary"`} {
		if !strings.Contains(out.String(), needle) {
			t.Errorf("missing %s in %s", needle, out.String())
		}
	}
}

func TestWriteMarkdownHighlightsKnownGapsAndCoreMetrics(t *testing.T) {
	report := Report{
		SchemaVersion: SchemaVersion,
		Mode:          ModeDeterministic, Version: "0.34.3", Commit: "abc123", GeneratedAt: "2026-08-02T03:04:05Z",
		Results: []ScenarioResult{{
			ID: "restart", Title: "Restart recovery", Category: "durability", Status: StatusPassed,
			ExpectationMet: true,
			Metrics:        Metrics{TaskSuccess: false, VerifierPass: false, DuplicateSideEffects: 1, OperatorInterventions: 1, TTFTMillis: 4, TTFTSource: "deterministic"},
			Details:        "active work is canceled on process restart",
		}},
		Summary: Summary{Total: 1, Completed: 1, ExpectationsMet: 1, DuplicateSideEffects: 1, OperatorInterventions: 1},
	}
	var out bytes.Buffer
	if err := WriteMarkdown(&out, report); err != nil {
		t.Fatalf("write markdown: %v", err)
	}
	for _, needle := range []string{"TARS Agent Harness Evaluation", "0.34.3", "abc123", "Duplicate side effects", "Known gaps", "active work is canceled"} {
		if !strings.Contains(out.String(), needle) {
			t.Errorf("missing %q in markdown:\n%s", needle, out.String())
		}
	}
}

func TestReportWritersHandleNoGapsSkipsAndWriteFailures(t *testing.T) {
	report := Report{
		SchemaVersion: SchemaVersion,
		Mode:          ModeDeterministic,
		Results: []ScenarioResult{
			{ID: "ok", Title: "A | B", Status: StatusPassed, ExpectationMet: true, Metrics: Metrics{TaskSuccess: true, VerifierPass: true}},
			{ID: "skip", Title: "Skipped", Status: StatusSkipped},
		},
		Summary: Summary{Total: 2, Completed: 1, Skipped: 1, ExpectationsMet: 1, TaskSuccessRate: 1, VerifierPassRate: 1},
	}
	var markdown bytes.Buffer
	if err := WriteMarkdown(&markdown, report); err != nil {
		t.Fatalf("write markdown: %v", err)
	}
	if !strings.Contains(markdown.String(), "A \\| B") || !strings.Contains(markdown.String(), "- None in this run.") {
		t.Fatalf("unexpected no-gap report:\n%s", markdown.String())
	}
	if err := WriteJSONL(reportFailWriter{}, report); err == nil {
		t.Fatal("expected JSONL write failure")
	}
	if err := WriteMarkdown(reportFailWriter{}, report); err == nil {
		t.Fatal("expected Markdown write failure")
	}
}
