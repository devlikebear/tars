package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunClientCommandWithoutMessagePointsAtTheConsole(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opts := defaultClientOptions()
	opts.serverURL = " http://127.0.0.1:1 "
	err := runClientCommand(context.Background(), strings.NewReader(""), &stdout, &stderr, opts)
	if err == nil || !strings.Contains(err.Error(), "interactive terminal UI has been removed") {
		t.Fatalf("expected the removed-terminal-UI error, got %v", err)
	}
}
