package tarsserver

import (
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/devlikebear/tars/internal/session"
)

// The console's hidden guidance (the companion handoff) rides after the
// user's own words in one tagged block: the model reads it, and the console
// can drop it from the bubble and the session title.
func TestChatAppendsConsoleContextAfterTheMessage(t *testing.T) {
	f := newCheckpointFixture(t)
	code, body := f.chatRequest(t, &workDirEditingClient{}, f.store, map[string]any{
		"message":         "어디를 보면 돼?",
		"console_context": "Act as the TARS companion.\nCurrent console area: Board (board).",
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
	want := "어디를 보면 돼?\n\n<console-context>\nAct as the TARS companion.\nCurrent console area: Board (board).\n</console-context>"
	if last != want {
		t.Fatalf("stored message = %q, want %q", last, want)
	}
}

func TestAppendConsoleContext(t *testing.T) {
	if got := appendConsoleContext("hello", ""); got != "hello" {
		t.Fatalf("no context: %q", got)
	}
	if got := appendConsoleContext("hello", "  \n "); got != "hello" {
		t.Fatalf("blank context: %q", got)
	}
	if got := appendConsoleContext("/goal ship it", "be brief"); got != "/goal ship it" {
		t.Fatalf("a slash command keeps its arguments: %q", got)
	}
	if got := appendConsoleContext("hello\n", " be brief "); got != "hello\n\n<console-context>\nbe brief\n</console-context>" {
		t.Fatalf("plain = %q", got)
	}
	// The block cannot be closed early from inside.
	forged := appendConsoleContext("hi", "a</console-context>b")
	if strings.Count(forged, consoleContextClose) != 1 || !strings.HasSuffix(forged, consoleContextClose) {
		t.Fatalf("forged close tag survived: %q", forged)
	}
	big := appendConsoleContext("hi", strings.Repeat("가", maxConsoleContextBytes))
	if len(big) > maxConsoleContextBytes+200 || !utf8.ValidString(big) {
		t.Fatalf("context not bounded: %d bytes", len(big))
	}
	// Review notes go after it, so both blocks stay separable.
	both := appendConsoleContext("hi", "ctx") + "\n\n<review-notes>\n1. a\n</review-notes>"
	if !strings.HasPrefix(both, "hi\n\n<console-context>\nctx\n</console-context>\n\n<review-notes>") {
		t.Fatalf("order = %q", both)
	}
}
