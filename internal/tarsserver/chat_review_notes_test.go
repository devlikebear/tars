package tarsserver

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/devlikebear/tars/internal/checkpoint"
	"github.com/devlikebear/tars/internal/session"
)

// Review notes ride on the next message: the stored message carries the
// block, with the hunk each note is about taken from the turn's checkpoint.
func TestChatAppendsReviewNotesToTheMessage(t *testing.T) {
	f := newCheckpointFixture(t)
	events := f.chat(t, &workDirEditingClient{edit: func(dir string) {
		writeTestFile(t, filepath.Join(dir, "base.txt"), "one\n2\n")
		writeTestFile(t, filepath.Join(dir, "new.txt"), "fresh\n")
	}}, f.store)
	turnID, _ := eventOfType(events, "turn_started")["user_message_id"].(string)

	code, body := f.chatRequest(t, &workDirEditingClient{}, f.store, map[string]any{
		"message": "please rework",
		"review_notes": []map[string]any{
			{"turn_id": turnID, "path": "base.txt", "hunk_id": "h0", "comment": "spell the number out"},
			{"turn_id": turnID, "path": "new.txt", "kind": "revert"},
		},
	})
	if code != http.StatusOK {
		t.Fatalf("chat status %d body=%q", code, body)
	}
	history, err := session.ReadMessages(f.sessions.TranscriptPath(f.sessionID))
	if err != nil {
		t.Fatal(err)
	}
	var last string
	for _, m := range history {
		if m.Role == "user" {
			last = m.Content
		}
	}
	for _, want := range []string{
		"please rework\n\n<review-notes>",
		"1. base.txt, hunk h0\nComment: spell the number out\n````diff\n@@ ",
		"-two\n+2\n````",
		"2. new.txt\nThe user reverted this change",
		"+fresh\n",
		"</review-notes>",
	} {
		if !strings.Contains(last, want) {
			t.Fatalf("stored message lacks %q:\n%s", want, last)
		}
	}
}

func TestChatRejectsBadReviewNotes(t *testing.T) {
	f := newCheckpointFixture(t)
	for _, note := range []map[string]any{
		{"turn_id": "t1", "path": "a.txt", "kind": "praise", "comment": "nice"},
		{"turn_id": "t1", "path": "a.txt"},
		{"turn_id": "t1", "comment": "no path"},
	} {
		code, body := f.chatRequest(t, &workDirEditingClient{}, f.store, map[string]any{
			"message":      "hello",
			"review_notes": []map[string]any{note},
		})
		if code != http.StatusBadRequest || !strings.Contains(body, "invalid review note") {
			t.Errorf("note %v: %d %s", note, code, body)
		}
	}
}

func TestAppendReviewNotes(t *testing.T) {
	ctx := context.Background()
	note := chatReviewNote{TurnID: "t1", Path: "a.txt", Comment: "tidy this"}

	same, err := appendReviewNotes(ctx, nil, "s1", "hello", nil)
	if err != nil || same != "hello" {
		t.Fatalf("no notes: %q %v", same, err)
	}
	slash, err := appendReviewNotes(ctx, nil, "s1", "/goal ship it", []chatReviewNote{note})
	if err != nil || slash != "/goal ship it" {
		t.Fatalf("a slash command keeps its arguments: %q %v", slash, err)
	}
	// Without a checkpoint store the note still goes through, code-less.
	plain, err := appendReviewNotes(ctx, nil, "s1", "hello\n", []chatReviewNote{note})
	if err != nil || !strings.HasPrefix(plain, "hello\n\n<review-notes>") || strings.Contains(plain, "````") {
		t.Fatalf("plain = %q %v", plain, err)
	}

	many := make([]chatReviewNote, maxReviewNotes+3)
	for i := range many {
		many[i] = note
	}
	capped, err := appendReviewNotes(ctx, nil, "s1", "hello", many)
	if err != nil || !strings.Contains(capped, "(3 more notes were left out") || strings.Contains(capped, "\n21. ") {
		t.Fatalf("capped = %q %v", capped, err)
	}
	huge := []chatReviewNote{
		{TurnID: "t1", Path: "a.txt", Comment: strings.Repeat("x", maxReviewCommentBytes)},
	}
	for len(huge) < 30 {
		huge = append(huge, huge[0])
	}
	big, err := appendReviewNotes(ctx, nil, "s1", "hello", huge)
	if err != nil || len(big) > maxReviewBlockBytes+100 || !strings.Contains(big, "more notes were left out") {
		t.Fatalf("block of %d bytes, %v", len(big), err)
	}
	if _, err := appendReviewNotes(ctx, nil, "s1", "hello", []chatReviewNote{{TurnID: "t1", Path: "a"}}); !errors.Is(err, errBadReviewNote) {
		t.Fatalf("empty comment: %v", err)
	}
}

func TestReviewNoteHelpers(t *testing.T) {
	long := strings.Repeat("가", 2000)
	clipped := clipBytes(long, 100)
	if !utf8.ValidString(clipped) || !strings.Contains(clipped, "more bytes") {
		t.Fatalf("clip broke a character: %q", clipped)
	}
	if clipBytes("short", 100) != "short" {
		t.Fatal("short text clipped")
	}
	file := checkpoint.FileDiff{
		Patch: "diff --git a/a b/a\n@@ -1 +1 @@\n-a\n+b\n@@ -9 +9 @@\n-c\n+d\n",
		Hunks: []checkpoint.Hunk{{ID: "h0"}, {ID: "h1"}},
	}
	if got := hunkText(file, "h1"); got != "@@ -9 +9 @@\n-c\n+d\n" {
		t.Fatalf("h1 = %q", got)
	}
	if got := hunkText(file, "h0"); got != "@@ -1 +1 @@\n-a\n+b\n" {
		t.Fatalf("h0 = %q", got)
	}
	if hunkText(file, "h7") != "" {
		t.Fatal("unknown hunk")
	}
	// A note about a turn or file the checkpoint does not know has no code.
	f := newCheckpointFixture(t)
	excerpts := reviewExcerpts{ctx: context.Background(), store: f.store, sessionID: f.sessionID, diffs: map[string][]checkpoint.FileDiff{}}
	if got := excerpts.excerpt(chatReviewNote{TurnID: "gone", Path: "a.txt"}); got != "" {
		t.Fatalf("excerpt = %q", got)
	}
}
