//go:build darwin

package assistant

import (
	"strings"
	"testing"
)

func TestQuoteAppleScriptEscapesQuotesBackslashesNewlinesAndCJK(t *testing.T) {
	raw := "say \"hi\" \\ path\n다음"
	got := quoteAppleScript(raw)
	want := "\"say \\\"hi\\\" \\\\ path\n다음\""
	if got != want {
		t.Fatalf("unexpected AppleScript quote:\nwant %q\n got %q", want, got)
	}
}

func TestBuildResultDialogScriptEscapesRawMessageOnce(t *testing.T) {
	script := buildResultDialogScript(VoiceTurnResult{
		Transcript:     "안녕 \"세계\" \\ root",
		AssistantReply: "line1\nline2",
	})

	if strings.Contains(script, `You: \"`) {
		t.Fatalf("message appears to have been quoted before outer escaping:\n%s", script)
	}
	for _, want := range []string{
		`display dialog "You: 안녕 \"세계\" \\ root`,
		`TARS: line1 line2"`,
		`with title "TARS replied"`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("expected script to contain %q, got:\n%s", want, script)
		}
	}
}
