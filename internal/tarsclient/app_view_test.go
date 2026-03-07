package tarsclient

import (
	"strings"
	"testing"
)

func TestRenderPanelLines_WrapsLongLinesBeforeTailCut(t *testing.T) {
	lines := []string{
		"header",
		"01234567890123456789",
	}

	got := renderPanelLines(lines, 6, 4, nil)
	parts := strings.Split(got, "\n")
	if len(parts) != 4 {
		t.Fatalf("expected 4 visible lines, got %d: %q", len(parts), got)
	}
	if parts[0] != "012345" {
		t.Fatalf("expected wrapped tail to keep recent visual lines, got %q", parts[0])
	}
	if parts[3] != "89" {
		t.Fatalf("expected final wrapped fragment to be visible, got %q", parts[3])
	}
}

func TestFormatChatLine_StylesUserAndAssistantLines(t *testing.T) {
	user := formatChatLine("You > hello", "You > hello")
	if !strings.Contains(user, "You > hello") {
		t.Fatalf("expected user content to remain visible, got %q", user)
	}

	assistant := formatChatLine("TARS > hi there", "TARS > hi there")
	if !strings.Contains(assistant, "TARS > hi there") {
		t.Fatalf("expected assistant content to remain visible, got %q", assistant)
	}
}
