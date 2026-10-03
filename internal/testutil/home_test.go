package testutil

import (
	"os"
	"testing"
)

func TestSetHomeRedirectsUserHomeDir(t *testing.T) {
	dir := t.TempDir()
	SetHome(t, dir)

	got, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	if got != dir {
		t.Fatalf("UserHomeDir = %q, want %q", got, dir)
	}
}
