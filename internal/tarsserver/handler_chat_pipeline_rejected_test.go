package tarsserver

import (
	"errors"
	"net/http"
	"testing"
)

// The focus driver's blocked gate shows this error's text: it has to name
// the cause, not only the handler's summary (#1231).
func TestErrChatTurnRejectedCarriesCause(t *testing.T) {
	cause := errors.New("scan transcript: bufio.Scanner: token too long")
	err := error(&errChatTurnRejected{status: http.StatusInternalServerError, msg: "auto compaction failed", cause: cause})
	if got, want := err.Error(), "auto compaction failed: scan transcript: bufio.Scanner: token too long"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	if !errors.Is(err, cause) {
		t.Fatal("the cause is not unwrapped")
	}

	// A 400 already uses the cause as its message; it is not said twice.
	same := &errChatTurnRejected{status: http.StatusBadRequest, msg: "mention not found", cause: errors.New("mention not found")}
	if got := same.Error(); got != "mention not found" {
		t.Fatalf("Error() = %q", got)
	}
	if got := (&errChatTurnRejected{msg: "turn refused"}).Error(); got != "turn refused" {
		t.Fatalf("Error() = %q", got)
	}
}
