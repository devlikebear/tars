package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := run([]string{}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%q", code, stderr.String())
	}

	if !strings.Contains(stdout.String(), "cased sentinel starting") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}
