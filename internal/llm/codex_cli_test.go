package llm

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexCLIClientAsk(t *testing.T) {
	cmdPath := writeFakeCodexCLI(t, "#!/bin/sh\n"+
		"outfile=\"\"\n"+
		"while [ \"$#\" -gt 0 ]; do\n"+
		"  if [ \"$1\" = \"-o\" ] || [ \"$1\" = \"--output-last-message\" ]; then\n"+
		"    outfile=\"$2\"\n"+
		"    shift 2\n"+
		"    continue\n"+
		"  fi\n"+
		"  shift\n"+
		"done\n"+
		"cat >/dev/null\n"+
		"echo \"codex cli response\" > \"$outfile\"\n")
	t.Setenv("CODEX_CLI_BIN", cmdPath)

	client, err := NewCodexCLIClient("gpt-5.3-codex")
	if err != nil {
		t.Fatalf("new codex client: %v", err)
	}

	got, err := client.Ask(context.Background(), "hello")
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if got != "codex cli response" {
		t.Fatalf("unexpected response: %q", got)
	}
}

func TestCodexCLIClientAsk_CommandFailure(t *testing.T) {
	cmdPath := writeFakeCodexCLI(t, "#!/bin/sh\nexit 9\n")
	t.Setenv("CODEX_CLI_BIN", cmdPath)

	client, err := NewCodexCLIClient("gpt-5.3-codex")
	if err != nil {
		t.Fatalf("new codex client: %v", err)
	}

	_, err = client.Ask(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "codex exec failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewCodexCLIClient_RequiresModel(t *testing.T) {
	_, err := NewCodexCLIClient("")
	if err == nil {
		t.Fatal("expected error for empty model")
	}
}

func writeFakeCodexCLI(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake codex cli: %v", err)
	}
	return path
}
