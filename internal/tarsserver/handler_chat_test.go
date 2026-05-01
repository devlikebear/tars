package tarsserver

import (
	"strings"
	"testing"
)

func TestStatusPreview_RedactsSensitiveFields(t *testing.T) {
	input := `{"password":"p@ss","token":"abc123","path":"README.md"}`
	preview := statusPreview(input, 240)
	if strings.Contains(preview, "p@ss") || strings.Contains(preview, "abc123") {
		t.Fatalf("expected redaction in preview, got %q", preview)
	}
	if !strings.Contains(preview, `"password":"***"`) {
		t.Fatalf("expected password redaction, got %q", preview)
	}
	if !strings.Contains(preview, `"path":"README.md"`) {
		t.Fatalf("expected non-sensitive fields preserved, got %q", preview)
	}
}

func TestStatusPreview_RedactsBearerToken(t *testing.T) {
	preview := statusPreview("authorization=Bearer tok_abcdef123", 240)
	if strings.Contains(preview, "tok_abcdef123") {
		t.Fatalf("expected bearer token redaction, got %q", preview)
	}
	if !strings.Contains(strings.ToLower(preview), "authorization=***") {
		t.Fatalf("expected authorization redaction, got %q", preview)
	}
}

func TestStatusPreviewForTool_CompactsSubagentsRunArgs(t *testing.T) {
	input := `{"agent":"explorer","mode":"parallel","tasks":[{"title":"Check API","prompt":"Inspect the API carefully and report findings","tier":"light"},{"prompt":"Review frontend behavior"}]}`
	preview := statusPreviewForTool("subagents_run", input, 40)
	if !strings.Contains(preview, `"count":2`) {
		t.Fatalf("expected compact count, got %q", preview)
	}
	if !strings.Contains(preview, `"title":"Check API"`) {
		t.Fatalf("expected task title, got %q", preview)
	}
	if strings.Contains(preview, "Inspect the API carefully") {
		t.Fatalf("expected prompt body to be omitted, got %q", preview)
	}
	if strings.Contains(preview, "...") {
		t.Fatalf("expected compact subagent preview to avoid generic truncation, got %q", preview)
	}
}

func TestStatusPreviewForTool_CompactsSubagentsRunResult(t *testing.T) {
	input := `{"count":2,"agent":"explorer","subagents":[{"run_id":"run_1234567890","session_id":"session_a","agent":"explorer","title":"Check API","status":"completed","tier":"light","summary":"done"},{"run_id":"run_failed","title":"Review UI","status":"failed","error":"model failed"}]}`
	preview := statusPreviewForTool("subagents_run", input, 40)
	if !strings.Contains(preview, `"run_id":"run_1234567890"`) {
		t.Fatalf("expected run id, got %q", preview)
	}
	if !strings.Contains(preview, `"status":"failed"`) {
		t.Fatalf("expected failed status, got %q", preview)
	}
	if !strings.Contains(preview, `"error":"model failed"`) {
		t.Fatalf("expected compact error, got %q", preview)
	}
}
