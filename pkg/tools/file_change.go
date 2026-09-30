package tools

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// FileChangeOp names what a tool call did to a file.
type FileChangeOp string

const (
	FileChangeCreate FileChangeOp = "create"
	FileChangeModify FileChangeOp = "modify"
	FileChangeDelete FileChangeOp = "delete"
)

// FileChange is what one file-editing tool call did to one file (#1032).
//
// It rides on Result.FileChanges for the UI and is never part of the text the
// model sees. Hunks are capped: a binary file, a file too large to diff, or a
// diff past the hunk budget carries counts only and sets Truncated (for a
// binary file, Binary instead, and no counts).
type FileChange struct {
	Path      string           `json:"path"`
	Op        FileChangeOp     `json:"op"`
	Additions int              `json:"additions"`
	Deletions int              `json:"deletions"`
	Binary    bool             `json:"binary,omitempty"`
	Truncated bool             `json:"truncated,omitempty"`
	Hunks     []FileChangeHunk `json:"hunks,omitempty"`
}

// FileChangeHunk is one @@ block of a unified diff. Lines keep their
// unified-diff prefix: ' ' context, '-' removed, '+' added, and '\' for
// "\ No newline at end of file".
type FileChangeHunk struct {
	OldStart int      `json:"old_start"`
	OldLines int      `json:"old_lines"`
	NewStart int      `json:"new_start"`
	NewLines int      `json:"new_lines"`
	Lines    []string `json:"lines"`
}

const (
	// maxFileChangeBytes is the largest file side that is diffed line by
	// line; a larger one reports the change without counts or hunks.
	maxFileChangeBytes = 1 << 20
	// maxFileChangeCells bounds the line table of the changed region
	// (after the common prefix and suffix are set aside).
	maxFileChangeCells = 1 << 22
	// maxFileChangeHunkLines and maxFileChangeHunkBytes cap the hunks one
	// file carries; later hunks are dropped and Truncated is set.
	maxFileChangeHunkLines = 400
	maxFileChangeHunkBytes = 64 << 10
	fileChangeContext      = 3
	noNewlineMarker        = `\ No newline at end of file`
)

// fileSnapshot is one side of a change: the file's bytes, or that it did not
// exist, or that it was too large to read for a diff.
type fileSnapshot struct {
	exists bool
	tooBig bool
	data   []byte
}

func snapshotFile(absPath string) fileSnapshot {
	// Callers resolve paths inside the workspace already; this guard is
	// defense in depth that path-injection analysis recognizes. After Clean
	// an absolute path has no ".." segments, so the only files it skips are
	// ones whose name contains ".." (a..b.txt): they get no per-call change,
	// and the turn-end change card still covers them.
	clean := filepath.Clean(absPath)
	if !filepath.IsAbs(clean) || strings.Contains(clean, "..") {
		return fileSnapshot{}
	}
	info, err := os.Stat(clean)
	if err != nil || info.IsDir() {
		return fileSnapshot{}
	}
	if info.Size() > maxFileChangeBytes {
		return fileSnapshot{exists: true, tooBig: true}
	}
	data, err := os.ReadFile(clean)
	if err != nil {
		return fileSnapshot{exists: true, tooBig: true}
	}
	return fileSnapshot{exists: true, data: data}
}

// computeFileChange describes the change from before to after. ok is false
// when nothing changed.
func computeFileChange(path string, before, after fileSnapshot) (FileChange, bool) {
	change := FileChange{Path: path, Op: FileChangeModify}
	switch {
	case !before.exists && !after.exists:
		return FileChange{}, false
	case !before.exists:
		change.Op = FileChangeCreate
	case !after.exists:
		change.Op = FileChangeDelete
	}
	if before.tooBig || after.tooBig {
		change.Truncated = true
		return change, true
	}
	if change.Op == FileChangeModify && bytes.Equal(before.data, after.data) {
		return FileChange{}, false
	}
	if isBinaryContent(before.data) || isBinaryContent(after.data) {
		change.Binary = true
		return change, true
	}
	sides := newLineSides(before.data, after.data)
	ops, ok := sides.diff()
	if !ok {
		// Too many lines changed to align; count the changed region whole.
		prefix, suffix := sides.commonEnds()
		change.Deletions = len(sides.a) - prefix - suffix
		change.Additions = len(sides.b) - prefix - suffix
		change.Truncated = true
		return change, true
	}
	change.Additions, change.Deletions = countOps(ops)
	change.Hunks, change.Truncated = sides.hunks(ops)
	return change, true
}

func countOps(ops []diffOp) (additions, deletions int) {
	for _, op := range ops {
		switch op.kind {
		case '-':
			deletions++
		case '+':
			additions++
		}
	}
	return additions, deletions
}

func isBinaryContent(data []byte) bool {
	head := data
	if len(head) > 8<<10 {
		head = head[:8<<10]
	}
	return bytes.IndexByte(head, 0) >= 0 || !utf8.Valid(data)
}

// splitDiffLines splits text into lines without their newlines. noEOL
// reports a last line that has no trailing newline.
func splitDiffLines(text string) (lines []string, noEOL bool) {
	if text == "" {
		return nil, false
	}
	lines = strings.Split(text, "\n")
	if lines[len(lines)-1] == "" {
		return lines[:len(lines)-1], false
	}
	return lines, true
}

type diffOp struct {
	kind   byte // ' ', '-', '+'
	oldPos int  // old lines consumed before this op
	newPos int  // new lines consumed before this op
}

// opBuilder appends ops while tracking how many lines of each side they
// have consumed.
type opBuilder struct {
	ops            []diffOp
	oldPos, newPos int
}

func (o *opBuilder) emit(kind byte, count int) {
	for k := 0; k < count; k++ {
		o.ops = append(o.ops, diffOp{kind: kind, oldPos: o.oldPos, newPos: o.newPos})
		if kind != '+' {
			o.oldPos++
		}
		if kind != '-' {
			o.newPos++
		}
	}
}

// lineSides is both sides of a text diff: a is the old file, b the new.
type lineSides struct {
	a, b           []string
	aNoEOL, bNoEOL bool
}

func newLineSides(before, after []byte) lineSides {
	var s lineSides
	s.a, s.aNoEOL = splitDiffLines(string(before))
	s.b, s.bNoEOL = splitDiffLines(string(after))
	return s
}

// oldLastNoEOL and newLastNoEOL report the last line of a side that has no
// trailing newline.
func (s lineSides) oldLastNoEOL(i int) bool { return s.aNoEOL && i == len(s.a)-1 }
func (s lineSides) newLastNoEOL(j int) bool { return s.bNoEOL && j == len(s.b)-1 }

// same compares old line i with new line j. A last line without a newline
// differs from the same text with one, as git's diff does.
func (s lineSides) same(i, j int) bool {
	return s.a[i] == s.b[j] && s.oldLastNoEOL(i) == s.newLastNoEOL(j)
}

func (s lineSides) commonEnds() (prefix, suffix int) {
	for prefix < len(s.a) && prefix < len(s.b) && s.same(prefix, prefix) {
		prefix++
	}
	for suffix < len(s.a)-prefix && suffix < len(s.b)-prefix && s.same(len(s.a)-1-suffix, len(s.b)-1-suffix) {
		suffix++
	}
	return prefix, suffix
}

// diff aligns the sides by longest common subsequence over the region
// between their common prefix and suffix. ok is false when that region is
// too large to align.
func (s lineSides) diff() ([]diffOp, bool) {
	prefix, suffix := s.commonEnds()
	n := len(s.a) - prefix - suffix
	m := len(s.b) - prefix - suffix
	if n > 0 && m > 0 && n*m > maxFileChangeCells {
		return nil, false
	}
	o := opBuilder{ops: make([]diffOp, 0, len(s.a)+m)}
	o.emit(' ', prefix)
	if n > 0 && m > 0 {
		s.alignMiddle(&o, prefix, n, m)
	} else {
		o.emit('-', n)
		o.emit('+', m)
	}
	o.emit(' ', suffix)
	return o.ops, true
}

// lcsTable holds at [i*(m+1)+j] the LCS length of the middle region's old
// lines from i and new lines from j. The cell cap keeps lengths < 2^16.
func (s lineSides) lcsTable(prefix, n, m int) []uint16 {
	w := m + 1
	lcs := make([]uint16, (n+1)*w)
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if s.same(prefix+i, prefix+j) {
				lcs[i*w+j] = lcs[(i+1)*w+j+1] + 1
			} else {
				lcs[i*w+j] = max(lcs[(i+1)*w+j], lcs[i*w+j+1])
			}
		}
	}
	return lcs
}

// alignMiddle emits the ops for the n old and m new lines after prefix,
// removals before additions where both fit.
func (s lineSides) alignMiddle(o *opBuilder, prefix, n, m int) {
	lcs := s.lcsTable(prefix, n, m)
	w := m + 1
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case s.same(prefix+i, prefix+j):
			o.emit(' ', 1)
			i++
			j++
		case lcs[(i+1)*w+j] >= lcs[i*w+j+1]:
			o.emit('-', 1)
			i++
		default:
			o.emit('+', 1)
			j++
		}
	}
	o.emit('-', n-i)
	o.emit('+', m-j)
}

// nextChange returns the first op at or after i that is not context.
func nextChange(ops []diffOp, i int) int {
	for i < len(ops) && ops[i].kind == ' ' {
		i++
	}
	return i
}

// skipChanges returns the first context op at or after i.
func skipChanges(ops []diffOp, i int) int {
	for i < len(ops) && ops[i].kind != ' ' {
		i++
	}
	return i
}

// hunkEnd returns the end of the changes starting at i, taking in later
// changes separated by at most 2*context lines of context.
func hunkEnd(ops []diffOp, i int) int {
	end := i
	for {
		end = skipChanges(ops, end)
		next := nextChange(ops, end)
		if next >= len(ops) || next-end > 2*fileChangeContext {
			return end
		}
		end = next
	}
}

// hunkBudget is what is left of the line and byte caps for one file.
type hunkBudget struct {
	lines, bytes int
}

// take charges h to the budget; false when it does not fit.
func (b *hunkBudget) take(h FileChangeHunk) bool {
	size := 0
	for _, line := range h.Lines {
		size += len(line) + 1
	}
	if len(h.Lines) > b.lines || size > b.bytes {
		return false
	}
	b.lines -= len(h.Lines)
	b.bytes -= size
	return true
}

// hunks groups ops into unified-diff hunks with three lines of context.
// truncated reports hunks dropped for the line or byte budget.
func (s lineSides) hunks(ops []diffOp) ([]FileChangeHunk, bool) {
	var out []FileChangeHunk
	budget := hunkBudget{lines: maxFileChangeHunkLines, bytes: maxFileChangeHunkBytes}
	for i := nextChange(ops, 0); i < len(ops); {
		// Hunks are at least 2*context+1 lines apart (closer ones merge in
		// hunkEnd), so this never reaches back into the previous hunk.
		start := max(i-fileChangeContext, 0)
		stop := min(hunkEnd(ops, i)+fileChangeContext, len(ops))
		hunk := s.renderHunk(ops[start:stop])
		if !budget.take(hunk) {
			return out, true
		}
		out = append(out, hunk)
		i = nextChange(ops, stop)
	}
	return out, false
}

func (s lineSides) renderHunk(ops []diffOp) FileChangeHunk {
	h := FileChangeHunk{Lines: make([]string, 0, len(ops))}
	for _, op := range ops {
		if op.kind != '+' {
			h.OldLines++
		}
		if op.kind != '-' {
			h.NewLines++
		}
		h.Lines = append(h.Lines, s.renderOp(op)...)
	}
	h.OldStart = hunkStart(ops[0].oldPos, h.OldLines)
	h.NewStart = hunkStart(ops[0].newPos, h.NewLines)
	return h
}

// renderOp is an op's diff line, followed by the no-newline marker when it
// is a side's last line and has no trailing newline.
func (s lineSides) renderOp(op diffOp) []string {
	var line string
	var noEOL bool
	switch op.kind {
	case '-':
		line, noEOL = "-"+s.a[op.oldPos], s.oldLastNoEOL(op.oldPos)
	case '+':
		line, noEOL = "+"+s.b[op.newPos], s.newLastNoEOL(op.newPos)
	default:
		line, noEOL = " "+s.a[op.oldPos], s.oldLastNoEOL(op.oldPos) || s.newLastNoEOL(op.newPos)
	}
	if noEOL {
		return []string{line, noNewlineMarker}
	}
	return []string{line}
}

// hunkStart is a hunk's 1-based start line on one side, or the line before
// it when the hunk has no lines on that side, as unified diffs number them.
func hunkStart(pos, lines int) int {
	if lines > 0 {
		return pos + 1
	}
	return pos
}

// snapshotWritten is the snapshot of content a tool holds in memory, so it
// need not read the file back.
func snapshotWritten(data []byte) fileSnapshot {
	if len(data) > maxFileChangeBytes {
		return fileSnapshot{exists: true, tooBig: true}
	}
	return fileSnapshot{exists: true, data: data}
}

// withFileChange attaches the change from before to after at relPath to a
// tool result, when there is one.
func withFileChange(result Result, relPath string, before, after fileSnapshot) Result {
	if change, ok := computeFileChange(filepath.ToSlash(relPath), before, after); ok {
		result.FileChanges = append(result.FileChanges, change)
	}
	return result
}
