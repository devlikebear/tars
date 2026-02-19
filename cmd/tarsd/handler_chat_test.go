package main

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
