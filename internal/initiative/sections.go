package initiative

import "strings"

// identitySpeakFirstHeadings and userSpeakFirstHeadings are the en/ko
// headings recognized for IDENTITY.md's speaking-first tone section and
// USER.md's speaking-first preferences section (tars#1221). Matched
// case-insensitively against a "## <heading>" line. Documented in
// CLAUDE.md's Initiative section — a renamed heading on either side would
// otherwise silently stop being read.
var (
	identitySpeakFirstHeadings = []string{"When I speak first", "먼저 말할 때"}
	userSpeakFirstHeadings     = []string{"Speaking first", "먼저 말하기"}
)

// sectionBounds returns the line range [start, end) of the first "## "
// heading in lines matching one of headings case-insensitively: start is
// the heading line itself, end is the next "## " heading or len(lines).
// start is -1 when no matching heading exists.
func sectionBounds(lines []string, headings []string) (start, end int) {
	start = -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "## ") {
			continue
		}
		title := strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
		for _, h := range headings {
			if strings.EqualFold(title, h) {
				start = i
				break
			}
		}
		if start >= 0 {
			break
		}
	}
	if start < 0 {
		return -1, -1
	}
	end = len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "## ") {
			end = i
			break
		}
	}
	return start, end
}

// joinLinesTrimmingCR re-joins lines (as split on "\n") with "\n",
// stripping each line's trailing "\r" first. A markdown file saved with
// CRLF line endings splits on "\n" into lines each still ending in "\r";
// joining them back without stripping that leaves a "\r" embedded after
// every line but the last — a plain TrimSpace of the whole joined string
// only cleans the very end, not those embedded ones.
func joinLinesTrimmingCR(lines []string) string {
	trimmed := make([]string, len(lines))
	for i, l := range lines {
		trimmed[i] = strings.TrimRight(l, "\r")
	}
	return strings.Join(trimmed, "\n")
}

// extractMarkdownSection returns the body (heading line excluded) of the
// first "## <heading>" section matching one of headings, up to the next
// "## " heading or the end of the document, trimmed of surrounding
// whitespace. ok is false when no such heading exists or its body is
// empty.
func extractMarkdownSection(markdown string, headings []string) (section string, ok bool) {
	lines := strings.Split(markdown, "\n")
	start, end := sectionBounds(lines, headings)
	if start < 0 {
		return "", false
	}
	body := strings.TrimSpace(joinLinesTrimmingCR(lines[start+1 : end]))
	return body, body != ""
}

// IdentitySpeakFirstSection extracts IDENTITY.md's speaking-first tone
// section (en "When I speak first", ko "먼저 말할 때"). ok is false when
// the section is absent or empty — the caller then falls back to the
// whole file, as before tars#1221.
func IdentitySpeakFirstSection(identityMarkdown string) (string, bool) {
	return extractMarkdownSection(identityMarkdown, identitySpeakFirstHeadings)
}

// UserSpeakFirstSection extracts USER.md's speaking-first preferences
// section (en "Speaking first", ko "먼저 말하기") — free-form sentences the
// user wrote about when/how often TARS should speak first. ok is false
// when the section is absent or empty.
func UserSpeakFirstSection(userMarkdown string) (string, bool) {
	return extractMarkdownSection(userMarkdown, userSpeakFirstHeadings)
}

// prioritizedProfile reorders userMarkdown so its speaking-first section
// (if any) comes first, then truncates to limit runes. A length cap always
// keeps the front of a string, so the user's own speak-first preferences
// survive truncation even when the rest of USER.md does not (tars#1221) —
// used wherever a profile goes into a text-signal read or the speak
// composer's context, the two places that already gate this on the
// same-provider "may this text be sent at all" rule.
func prioritizedProfile(userMarkdown string, limit int) string {
	if _, ok := UserSpeakFirstSection(userMarkdown); !ok {
		return truncateRunes(strings.TrimSpace(userMarkdown), limit)
	}
	lines := strings.Split(userMarkdown, "\n")
	start, end := sectionBounds(lines, userSpeakFirstHeadings)
	section := strings.TrimSpace(joinLinesTrimmingCR(lines[start:end]))
	rest := strings.TrimSpace(joinLinesTrimmingCR(append(append([]string{}, lines[:start]...), lines[end:]...)))
	reordered := section
	if rest != "" {
		reordered = section + "\n\n" + rest
	}
	return truncateRunes(reordered, limit)
}
