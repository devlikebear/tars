package checkpoint

import (
	"bytes"
	"fmt"
	"slices"
)

// splitLines cuts b after each "\n", keeping the terminators, so joining the
// pieces gives back b byte for byte: CRLF and a missing final newline
// included.
func splitLines(b []byte) [][]byte {
	if len(b) == 0 {
		return nil
	}
	lines := bytes.SplitAfter(b, []byte("\n"))
	if len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// hunkIndex turns a hunk header's 1-based start into a slice index. An empty
// range names the line before it, so its index is the start itself.
func hunkIndex(start, count int) int {
	if count == 0 {
		return start
	}
	return start - 1
}

// spliceHunks returns end with the chosen hunks of the start→end diff put
// back to start's lines. Choosing every hunk gives start; choosing none gives
// end. Hunk ranges come from both files, so context lines need no matching.
func spliceHunks(start, end []byte, hunks []Hunk, chosen []int) ([]byte, error) {
	old, cur := splitLines(start), splitLines(end)
	picked := slices.Clone(chosen)
	slices.Sort(picked)
	picked = slices.Compact(picked)
	// Bottom-up, so earlier ranges keep their positions.
	for i := len(picked) - 1; i >= 0; i-- {
		n := picked[i]
		if n < 0 || n >= len(hunks) {
			return nil, fmt.Errorf("%w: no hunk h%d", ErrInvalid, n)
		}
		h := hunks[n]
		oi, ni := hunkIndex(h.OldStart, h.OldLines), hunkIndex(h.NewStart, h.NewLines)
		if oi < 0 || oi+h.OldLines > len(old) || ni < 0 || ni+h.NewLines > len(cur) {
			return nil, fmt.Errorf("checkpoint: hunk %s is out of range", h.ID)
		}
		cur = slices.Concat(cur[:ni], old[oi:oi+h.OldLines], cur[ni+h.NewLines:])
	}
	return bytes.Join(cur, nil), nil
}

// binaryContent follows git's rule: a NUL in the first 8000 bytes.
func binaryContent(b []byte) bool {
	return bytes.IndexByte(b[:min(len(b), 8000)], 0) >= 0
}
