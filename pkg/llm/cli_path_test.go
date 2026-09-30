package llm

import (
	"os"
	"path/filepath"
	"testing"
)

// A launchd service runs with a minimal PATH that leaves out ~/.local/bin,
// where the native installers put both claude and agy.
func TestFindCLIPath_FallsBackToUserLocalBin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PATH", t.TempDir())
	t.Setenv(claudeCodeCLIPathEnv, "")
	t.Setenv(antigravityCLIPathEnv, "")

	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, name := range []string{"claude", "agy"} {
		if err := os.WriteFile(filepath.Join(binDir, name+exeSuffix()), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	for name, find := range map[string]func() (string, error){
		"claude": FindClaudeCodeCLIPath,
		"agy":    FindAntigravityCLIPath,
	} {
		got, err := find()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if want := filepath.Join(binDir, name+exeSuffix()); got != want {
			t.Errorf("%s path = %q, want %q", name, got, want)
		}
	}
}

func TestFindCLIPath_MissingEverywhereStillFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PATH", t.TempDir())
	t.Setenv(claudeCodeCLIPathEnv, "")
	t.Setenv(antigravityCLIPathEnv, "")

	if _, err := FindClaudeCodeCLIPath(); err == nil {
		t.Error("claude: expected an error when the CLI is nowhere")
	}
	if _, err := FindAntigravityCLIPath(); err == nil {
		t.Error("agy: expected an error when the CLI is nowhere")
	}
}
