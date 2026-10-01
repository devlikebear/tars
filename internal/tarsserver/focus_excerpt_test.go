package tarsserver

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func utf8Valid(s string) bool { return utf8.ValidString(s) }

func TestFocusFailureExcerpt(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 300; i++ {
		b.WriteString("ok  \tgithub.com/x/pkg/a\t0.1s\n")
	}
	b.WriteString("--- FAIL: TestGreet (0.00s)\n    greet_test.go:12: got \"Hi\", want \"Hello\"\nFAIL\nFAIL\tgithub.com/x/pkg/greet\t0.2s\n")
	for i := 0; i < 300; i++ {
		b.WriteString("ok  \tgithub.com/x/pkg/z\t0.1s\n")
	}
	got := focusFailureExcerpt(b.String(), 4096)
	if !strings.Contains(got, "--- FAIL: TestGreet") || !strings.Contains(got, `want "Hello"`) {
		t.Fatalf("the failure is missing:\n%s", got)
	}
	if len(got) > 4096 {
		t.Fatalf("excerpt is %d bytes", len(got))
	}

	// Two different failures give different excerpts (repeat detection).
	other := strings.Replace(b.String(), "TestGreet", "TestFarewell", 1)
	if focusFailureExcerpt(other, 4096) == got {
		t.Fatal("different failures must not look the same")
	}

	// No failure marker: the tail, which holds the summary.
	plain := strings.Repeat("line\n", 2000) + "the end"
	if tail := focusFailureExcerpt(plain, 100); !strings.HasSuffix(tail, "the end") || len(tail) > 100 {
		t.Fatalf("tail = %q", tail)
	}
	if focusFailureExcerpt("short", 100) != "short" {
		t.Fatal("short output stays whole")
	}
	// Many failures: the excerpt still fits, ending on whole UTF-8.
	many := strings.Repeat("--- FAIL: TestÉ\n    détail\n", 2000)
	if got := focusFailureExcerpt(many, 4096); len(got) > 4096 || !strings.Contains(got, "--- FAIL") || !utf8Valid(got) {
		t.Fatalf("many failures: %d bytes", len(got))
	}
}
