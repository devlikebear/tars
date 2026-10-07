package tarsserver

import (
	"bytes"
	"context"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/pkg/session"
	"github.com/rs/zerolog"
)

// The review loop's git facts (docs/decisions/focus-mode.md §5, P3): the
// commit a pipeline's review diff starts from, and the diff excerpt each
// finding card shows. Git runs outside focuspipeline.Store.Update's lock.

const (
	// focusExcerptRadius is how many lines around a finding's line its
	// excerpt keeps; focusExcerptMaxBytes caps the excerpt.
	focusExcerptRadius   = 6
	focusExcerptMaxBytes = 4000
	// focusUntrackedMaxBytes caps how much of an untracked file is read for
	// its excerpt.
	focusUntrackedMaxBytes = 64 << 10
)

// focusRecordBaseCommit stores HEAD of the session's folder as the
// pipeline's base commit the first time a focus turn runs, before any
// change. A folder outside git records nothing.
func focusRecordBaseCommit(sessions *session.Store, sessionID string, p focuspipeline.Pipeline, logger zerolog.Logger) focuspipeline.Pipeline {
	if p.BaseCommit != "" || !p.Active() {
		return p
	}
	dir, err := sessions.GetCurrentDir(sessionID)
	if err != nil || strings.TrimSpace(dir) == "" {
		return p
	}
	head, err := runGit(context.Background(), releaseGitTimeout, dir, nil, "rev-parse", "--verify", "HEAD")
	if err != nil || head == "" {
		return p
	}
	updated, _, err := focusStoreFor(sessions).Update(sessionID, func(cur focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		if cur.BaseCommit == "" {
			cur.BaseCommit = head
		}
		return cur, nil
	})
	if err != nil {
		logger.Warn().Err(err).Str("session_id", sessionID).Msg("focus: record base commit failed")
		p.BaseCommit = head
		return p
	}
	return updated
}

// focusEnrichFindings adds diff excerpts to a review turn's findings. It
// reads the pipeline and the session folder before the update that applies
// the turn, so git never runs under the pipeline lock.
func focusEnrichFindings(sessions *session.Store, sessionID string, blocks focuspipeline.Blocks) focuspipeline.Blocks {
	if len(blocks.Findings) == 0 {
		return blocks
	}
	p, ok, err := focusStoreFor(sessions).Get(sessionID)
	if err != nil || !ok || p.CurrentKind() != focuspipeline.StageReview {
		return blocks
	}
	dir, err := sessions.GetCurrentDir(sessionID)
	if err != nil {
		return blocks
	}
	blocks.Findings = focusFindingExcerpts(context.Background(), dir, p.BaseCommit, blocks.Findings)
	return blocks
}

// focusFindingExcerpts returns findings with Excerpt set from `git diff
// <base>` in dir's repository (committed and uncommitted changes since
// base; HEAD when base is unknown), or, for a file git does not track yet,
// its lines around the finding as additions. Finding paths are relative to
// the repository's top level, wherever in it dir is; paths that leave the
// repository — by text or through a symlink — get no excerpt. The input is
// not modified.
func focusFindingExcerpts(ctx context.Context, dir, base string, findings []focuspipeline.Finding) []focuspipeline.Finding {
	out := append([]focuspipeline.Finding(nil), findings...)
	if strings.TrimSpace(dir) == "" {
		return out
	}
	top, err := runGit(ctx, releaseGitTimeout, dir, nil, "rev-parse", "--show-toplevel")
	if err != nil || top == "" {
		return out
	}
	dir = top
	if base = strings.TrimSpace(base); base == "" {
		base = "HEAD"
	}
	diffs := map[string]string{}
	for i, f := range out {
		file, ok := focusRepoPath(f.File)
		if !ok {
			continue
		}
		diff, seen := diffs[file]
		if !seen {
			diff, _ = runGit(ctx, releaseGitTimeout, dir, nil, "diff", "--no-color", "--no-ext-diff", "-U3", base, "--", file)
			diffs[file] = diff
		}
		excerpt := focusDiffExcerpt(diff, f.Line, focusExcerptRadius)
		if excerpt == "" && diff == "" {
			excerpt = focusUntrackedExcerpt(ctx, dir, file, f.Line)
		}
		out[i].Excerpt = capExcerpt(excerpt)
	}
	return out
}

// focusRepoPath cleans a finding's file into a slash path relative to the
// repository, or false when it is absolute, leaves the repository, or could
// read as a git option.
func focusRepoPath(file string) (string, bool) {
	file = strings.TrimSpace(filepath.ToSlash(file))
	if file == "" || strings.HasPrefix(file, "-") || strings.HasPrefix(file, "/") || filepath.IsAbs(file) {
		return "", false
	}
	clean := path.Clean(file)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false
	}
	return clean, true
}

// focusUntrackedExcerpt shows a file git does not track as added lines
// around line. The file must resolve, symlinks evaluated, to a regular file
// inside the repository top level dir; at most focusUntrackedMaxBytes of it
// are read.
func focusUntrackedExcerpt(ctx context.Context, dir, file string, line int) string {
	if tracked, err := runGit(ctx, releaseGitTimeout, dir, nil, "ls-files", "--", file); err != nil || tracked != "" {
		return ""
	}
	real, ok := focusInsideRepo(dir, filepath.Join(dir, filepath.FromSlash(file)))
	if !ok {
		return ""
	}
	data, err := readHead(real, focusUntrackedMaxBytes)
	if err != nil || len(data) == 0 {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	from, to := excerptWindow(line, len(lines), focusExcerptRadius)
	var b strings.Builder
	b.WriteString("@@ new file +" + strconv.Itoa(from) + " @@\n")
	for i := from; i <= to; i++ {
		b.WriteString("+" + lines[i-1] + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func excerptWindow(line, count, radius int) (int, int) {
	if line <= 0 || line > count {
		line = 1
	}
	return max(1, line-radius), min(count, line+radius)
}

var hunkHeader = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

type diffLine struct {
	text string
	// at is the new-file line the diff line sits at (a removed line: the
	// line it was removed before).
	at int
}

type diffHunk struct {
	header      string
	start, size int
	lines       []diffLine
}

// focusDiffExcerpt is the hunk of a unified diff that holds line (new-file
// numbering), cut to radius lines around it with the hunk header kept; with
// line 0 the first hunk's start. "" when no hunk holds line.
func focusDiffExcerpt(diff string, line, radius int) string {
	hunks := parseHunks(diff)
	if len(hunks) == 0 {
		return ""
	}
	var h *diffHunk
	if line <= 0 {
		h = &hunks[0]
		line = h.start
	} else {
		for i := range hunks {
			if line >= hunks[i].start && line < hunks[i].start+max(hunks[i].size, 1) {
				h = &hunks[i]
				break
			}
		}
	}
	if h == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(h.header + "\n")
	for _, l := range h.lines {
		if l.at >= line-radius && l.at <= line+radius {
			b.WriteString(l.text + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func parseHunks(diff string) []diffHunk {
	var hunks []diffHunk
	next := 0
	for _, text := range strings.Split(diff, "\n") {
		if m := hunkHeader.FindStringSubmatch(text); m != nil {
			start, _ := strconv.Atoi(m[1])
			size := 1
			if m[2] != "" {
				size, _ = strconv.Atoi(m[2])
			}
			hunks = append(hunks, diffHunk{header: text, start: start, size: size})
			next = start
			continue
		}
		if len(hunks) == 0 || text == "" || strings.HasPrefix(text, `\`) {
			continue
		}
		h := &hunks[len(hunks)-1]
		switch text[0] {
		case ' ', '+':
			h.lines = append(h.lines, diffLine{text: text, at: next})
			next++
		case '-':
			h.lines = append(h.lines, diffLine{text: text, at: next})
		}
	}
	return hunks
}

func capExcerpt(s string) string {
	if len(s) <= focusExcerptMaxBytes {
		return s
	}
	cut := strings.LastIndex(s[:focusExcerptMaxBytes], "\n")
	if cut <= 0 {
		cut = focusExcerptMaxBytes
	}
	return strings.ToValidUTF8(s[:cut], "") + "\n…"
}

// focusInsideRepo resolves path's symlinks and reports the real path when it
// is a regular file under the repository top level (also resolved).
func focusInsideRepo(top, path string) (string, bool) {
	realTop, err := filepath.EvalSymlinks(top)
	if err != nil {
		return "", false
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(realTop, real)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	info, err := os.Lstat(real)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	return real, true
}

// readHead reads at most limit bytes of path, cut back to its last whole
// line when the file is longer.
func readHead(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }() // read-only: a close error loses nothing
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) <= limit {
		return data, nil
	}
	data = data[:limit]
	if i := bytes.LastIndexByte(data, '\n'); i >= 0 {
		data = data[:i+1]
	}
	return data, nil
}
