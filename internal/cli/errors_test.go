package cli

import (
	"errors"
	"testing"
)

func TestExitError(t *testing.T) {
	err := &ExitError{Code: 42, Err: errors.New("boom")}
	if err.Error() != "boom" {
		t.Fatalf("unexpected: %q", err.Error())
	}
	if err.Code != 42 {
		t.Fatalf("unexpected code: %d", err.Code)
	}
}
