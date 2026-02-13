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

func TestIsFlagError(t *testing.T) {
	if !IsFlagError(errors.New("unknown flag --foo")) {
		t.Fatal("expected true for unknown flag")
	}
	if IsFlagError(errors.New("connection refused")) {
		t.Fatal("expected false for non-flag error")
	}
}
