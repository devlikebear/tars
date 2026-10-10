package focusprobe

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// focusFailureMarker matches the lines test runners and builds print for a
// failure: Go's --- FAIL / FAIL / panic, node:test's "not ok", Playwright's
// ✘ and Error:, and compiler/linter "error:" lines.
var focusFailureMarker = regexp.MustCompile(`--- FAIL|^FAIL\b|panic:|^not ok\b|✘|\bError:|\berror:|\bFAILED\b`)

// Lines kept around each failure line.
const (
	focusExcerptBefore = 2
	focusExcerptAfter  = 8
)

// FailureExcerpt is the proof excerpt of a focus build-loop
// verification command (proofverifier.Options.Excerpt): the lines around
// each failure line, wherever they sit in the output — `go test ./...`
// prints a failing package's detail between hundreds of "ok" lines — or,
// with no failure line, the tail, which holds a run's summary. It never
// exceeds limit bytes and always ends on whole UTF-8.
func FailureExcerpt(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	lines := strings.Split(text, "\n")
	keep := make([]bool, len(lines))
	found := false
	for i, line := range lines {
		if !focusFailureMarker.MatchString(line) {
			continue
		}
		found = true
		for j := max(0, i-focusExcerptBefore); j <= min(len(lines)-1, i+focusExcerptAfter); j++ {
			keep[j] = true
		}
	}
	if !found {
		return tailBytes(text, limit)
	}
	var b strings.Builder
	gap := false
	for i, line := range lines {
		if !keep[i] {
			gap = true
			continue
		}
		if gap && b.Len() > 0 {
			b.WriteString("…\n")
		}
		gap = false
		if b.Len()+len(line)+1 > limit {
			break
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// tailBytes is the last limit bytes of s, starting on a whole rune.
func tailBytes(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	tail := s[len(s)-limit:]
	for len(tail) > 0 && !utf8.RuneStart(tail[0]) {
		tail = tail[1:]
	}
	return tail
}
