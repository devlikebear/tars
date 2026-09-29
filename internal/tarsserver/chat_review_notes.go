package tarsserver

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/devlikebear/tars/internal/checkpoint"
)

// chatReviewNote is one note from reviewing what earlier turns changed
// (#969): a comment on a hunk or file, or word that the user reverted it.
type chatReviewNote struct {
	TurnID  string `json:"turn_id"`
	Path    string `json:"path"`
	HunkID  string `json:"hunk_id,omitempty"`
	Comment string `json:"comment,omitempty"`
	// Kind is "comment" (the default) or "revert".
	Kind string `json:"kind,omitempty"`
}

const (
	reviewNoteComment = "comment"
	reviewNoteRevert  = "revert"

	maxReviewNotes        = 20
	maxReviewCommentBytes = 2000
	maxReviewExcerptBytes = 4000
	maxReviewBlockBytes   = 32 << 10

	reviewNotesOpen  = "<review-notes>"
	reviewNotesClose = "</review-notes>"
)

var errBadReviewNote = errors.New("invalid review note")

// appendReviewNotes puts the review notes after the user's message as one
// block, with the code each note is about taken from the turn's checkpoint.
// Every provider reads the message text, so this is how the notes reach the
// model; the transcript keeps the block, and the console folds it. A slash
// command keeps its arguments clean, so its message gets no block.
func appendReviewNotes(ctx context.Context, store *checkpoint.Store, sessionID, message string, notes []chatReviewNote) (string, error) {
	if len(notes) == 0 || strings.HasPrefix(strings.TrimSpace(message), "/") {
		return message, nil
	}
	for i, n := range notes {
		kind := strings.TrimSpace(n.Kind)
		switch {
		case strings.TrimSpace(n.TurnID) == "" || strings.TrimSpace(n.Path) == "":
			return "", fmt.Errorf("%w %d: turn_id and path are required", errBadReviewNote, i+1)
		case kind != "" && kind != reviewNoteComment && kind != reviewNoteRevert:
			return "", fmt.Errorf("%w %d: kind %q is not comment or revert", errBadReviewNote, i+1, kind)
		case kind != reviewNoteRevert && strings.TrimSpace(n.Comment) == "":
			return "", fmt.Errorf("%w %d: a comment needs text", errBadReviewNote, i+1)
		}
	}
	excerpts := reviewExcerpts{ctx: ctx, store: store, sessionID: sessionID, diffs: map[string][]checkpoint.FileDiff{}}
	var b strings.Builder
	b.WriteString(reviewNotesOpen + "\n")
	b.WriteString("Notes from the user's review of changes you made in earlier turns. Treat each as a request about that code.\n")
	written := 0
	for i, n := range notes {
		if i >= maxReviewNotes {
			break
		}
		entry := formatReviewNote(written+1, n, excerpts.excerpt(n))
		if b.Len()+len(entry) > maxReviewBlockBytes {
			break
		}
		b.WriteString(entry)
		written++
	}
	if left := len(notes) - written; left > 0 {
		fmt.Fprintf(&b, "\n(%d more notes were left out to keep this short.)\n", left)
	}
	b.WriteString(reviewNotesClose)
	return strings.TrimRight(message, "\n") + "\n\n" + b.String(), nil
}

func formatReviewNote(number int, n chatReviewNote, excerpt string) string {
	var b strings.Builder
	where := strings.TrimSpace(n.Path)
	if hunk := strings.TrimSpace(n.HunkID); hunk != "" {
		where += ", hunk " + hunk
	}
	fmt.Fprintf(&b, "\n%d. %s\n", number, where)
	if strings.TrimSpace(n.Kind) == reviewNoteRevert {
		b.WriteString("The user reverted this change, so it is no longer in the file. Do not make it again unless asked.\n")
	}
	if comment := clipBytes(strings.TrimSpace(n.Comment), maxReviewCommentBytes); comment != "" {
		b.WriteString("Comment: " + comment + "\n")
	}
	if excerpt != "" {
		b.WriteString("````diff\n" + excerpt)
		if !strings.HasSuffix(excerpt, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("````\n")
	}
	return b.String()
}

// reviewExcerpts reads the code a note is about from the turn's diff. A
// note still goes through without one: the checkpoint may be gone or off.
type reviewExcerpts struct {
	ctx       context.Context
	store     *checkpoint.Store
	sessionID string
	diffs     map[string][]checkpoint.FileDiff
}

func (e reviewExcerpts) excerpt(n chatReviewNote) string {
	if e.store == nil {
		return ""
	}
	turnID := strings.TrimSpace(n.TurnID)
	files, ok := e.diffs[turnID]
	if !ok {
		result, err := e.store.Diff(e.ctx, e.sessionID, turnID, checkpoint.ScopeTurn, "")
		if err == nil {
			files = result.Files
		}
		e.diffs[turnID] = files
	}
	pos := slices.IndexFunc(files, func(f checkpoint.FileDiff) bool { return f.Path == strings.TrimSpace(n.Path) })
	if pos < 0 {
		return ""
	}
	file := files[pos]
	patch := file.Patch
	if hunk := strings.TrimSpace(n.HunkID); hunk != "" {
		patch = hunkText(file, hunk)
	} else if i := strings.Index(patch, "\n@@"); i >= 0 {
		patch = patch[i+1:] // drop the diff --git / index / ---/+++ header
	}
	return clipBytes(patch, maxReviewExcerptBytes)
}

// hunkText cuts one hunk out of a file's patch: its header line through the
// line before the next hunk.
func hunkText(file checkpoint.FileDiff, hunkID string) string {
	index := slices.IndexFunc(file.Hunks, func(h checkpoint.Hunk) bool { return h.ID == hunkID })
	if index < 0 {
		return ""
	}
	var out []string
	seen := -1
	for _, line := range strings.SplitAfter(file.Patch, "\n") {
		if strings.HasPrefix(line, "@@") {
			seen++
		}
		if seen == index {
			out = append(out, line)
		} else if seen > index {
			break
		}
	}
	return strings.Join(out, "")
}

func clipBytes(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := strings.LastIndexByte(s[:limit], '\n')
	if cut < 0 {
		cut = limit
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
	}
	return s[:cut] + "\n… (" + strconv.Itoa(len(s)-cut) + " more bytes)\n"
}
