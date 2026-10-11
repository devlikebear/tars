package initiative

import (
	"strings"
	"testing"
)

func TestExtractMarkdownSection(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		headings []string
		want     string
		wantOK   bool
	}{
		{
			name:     "english heading",
			markdown: "# IDENTITY.md\n\n## When I speak first\nBe warm and brief.\n\n## Other\nignored",
			headings: identitySpeakFirstHeadings,
			want:     "Be warm and brief.",
			wantOK:   true,
		},
		{
			name:     "korean heading",
			markdown: "# IDENTITY.md\n\n## 먼저 말할 때\n짧고 다정하게.\n",
			headings: identitySpeakFirstHeadings,
			want:     "짧고 다정하게.",
			wantOK:   true,
		},
		{
			name:     "heading at end of document",
			markdown: "# USER.md\n\n## Speaking first\n오후에는 말 걸지 마.",
			headings: userSpeakFirstHeadings,
			want:     "오후에는 말 걸지 마.",
			wantOK:   true,
		},
		{
			name:     "no matching heading",
			markdown: "# IDENTITY.md\n\n## Communication Style\nsomething else",
			headings: identitySpeakFirstHeadings,
			want:     "",
			wantOK:   false,
		},
		{
			name:     "heading present but empty body",
			markdown: "## When I speak first\n\n## Next\nbody",
			headings: identitySpeakFirstHeadings,
			want:     "",
			wantOK:   false,
		},
		{
			name:     "case-insensitive match",
			markdown: "## WHEN I SPEAK FIRST\nStay calm.",
			headings: identitySpeakFirstHeadings,
			want:     "Stay calm.",
			wantOK:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := extractMarkdownSection(tt.markdown, tt.headings)
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("extractMarkdownSection = %q, %v; want %q, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestIdentityAndUserSpeakFirstSectionWrappers(t *testing.T) {
	if section, ok := IdentitySpeakFirstSection("## 먼저 말할 때\n다정하게."); !ok || section != "다정하게." {
		t.Fatalf("IdentitySpeakFirstSection = %q, %v", section, ok)
	}
	if section, ok := UserSpeakFirstSection("## 먼저 말하기\n주말엔 인사만."); !ok || section != "주말엔 인사만." {
		t.Fatalf("UserSpeakFirstSection = %q, %v", section, ok)
	}
	if _, ok := UserSpeakFirstSection("# USER.md\nno such section here"); ok {
		t.Fatal("UserSpeakFirstSection should report false when absent")
	}
}

func TestPrioritizedProfileMovesSectionFirstAndSurvivesTruncation(t *testing.T) {
	markdown := "# USER.md\n\n## Preferences\n" + strings.Repeat("filler ", 100) +
		"\n\n## Speaking first\nDon't message me in the afternoon.\n\n## Other\nmore filler"

	// With a generous limit, the speak-first section is moved to the
	// front (before the unrelated Preferences content).
	got := prioritizedProfile(markdown, 2000)
	if idx := strings.Index(got, "Don't message me"); idx < 0 || idx > 50 {
		t.Fatalf("speak-first section not near the front: %q", got)
	}

	// With a tight limit that would otherwise cut off a section near the
	// end of the document, the speak-first content still survives because
	// it was moved to the front before truncation.
	tight := prioritizedProfile(markdown, 40)
	if !strings.Contains(tight, "Don't message me") {
		t.Fatalf("speak-first section lost to truncation: %q", tight)
	}
}

func TestPrioritizedProfileWithoutSectionIsUnchanged(t *testing.T) {
	markdown := "# USER.md\n\n## Preferences\nlikes tea"
	got := prioritizedProfile(markdown, 2000)
	want := truncateRunes(markdown, 2000)
	if got != want {
		t.Fatalf("prioritizedProfile = %q, want unchanged %q", got, want)
	}
}

// TestExtractMarkdownSectionStripsCRFromCRLFFiles guards a file saved with
// CRLF line endings (e.g. edited on Windows): splitting on "\n" leaves each
// line ending in "\r", and re-joining a multi-line body without stripping
// that would embed a stray "\r" after every line but the last — not caught
// by the outer TrimSpace, which only cleans the very ends of the whole
// string.
func TestExtractMarkdownSectionStripsCRFromCRLFFiles(t *testing.T) {
	markdown := "## When I speak first\r\nBe warm.\r\nKeep it short.\r\n\r\n## Next\r\nignored\r\n"
	got, ok := extractMarkdownSection(markdown, identitySpeakFirstHeadings)
	if !ok {
		t.Fatal("expected a section to be found")
	}
	if strings.Contains(got, "\r") {
		t.Fatalf("section still carries an embedded \\r: %q", got)
	}
	if got != "Be warm.\nKeep it short." {
		t.Fatalf("section = %q", got)
	}
}

// TestPrioritizedProfileStripsCRFromCRLFFiles is the same guard for the
// reordered profile (section + rest), both of which are reconstructed via
// the same line-join path.
func TestPrioritizedProfileStripsCRFromCRLFFiles(t *testing.T) {
	markdown := "# USER.md\r\n\r\n## Speaking first\r\nDon't message me\r\nin the afternoon.\r\n\r\n## Other\r\nmore stuff\r\nhere\r\n"
	got := prioritizedProfile(markdown, 2000)
	if strings.Contains(got, "\r") {
		t.Fatalf("reordered profile still carries an embedded \\r: %q", got)
	}
	if !strings.Contains(got, "Don't message me\nin the afternoon.") {
		t.Fatalf("section body was not preserved correctly: %q", got)
	}
}
