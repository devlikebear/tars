package llm

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestClaudeCodeCLIModelCatalogIsACopy(t *testing.T) {
	got := ClaudeCodeCLIModelCatalog()
	if !slices.Contains(got, "sonnet") || !slices.Contains(got, "claude-opus-5-5") {
		t.Fatalf("catalog: %v", got)
	}
	got[0] = "changed"
	if ClaudeCodeCLIModelCatalog()[0] == "changed" {
		t.Fatal("catalog shares its backing array with the caller")
	}
}

func TestParseClaudeCodeCLIModelCache(t *testing.T) {
	got := parseClaudeCodeCLIModelCache([]byte(`{"oauthAccount":{"x":"secret"},"additionalModelOptionsCache":[{"value":"claude-fable-5-1[1m]"},{"value":" "},{"value":"opus"}]}`))
	if !slices.Equal(got, []string{"claude-fable-5-1", "opus"}) {
		t.Fatalf("got %v", got)
	}
	for _, bad := range []string{"", "{", `{"additionalModelOptionsCache":"x"}`, `{}`} {
		if got := parseClaudeCodeCLIModelCache([]byte(bad)); len(got) != 0 {
			t.Fatalf("%q => %v", bad, got)
		}
	}
}

func TestReadClaudeCodeCLIModelCache(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	if got := ReadClaudeCodeCLIModelCache(); got != nil {
		t.Fatalf("missing file: %v", got)
	}
	if err := os.WriteFile(filepath.Join(dir, ".claude.json"), []byte(`{"additionalModelOptionsCache":[{"value":"claude-x-1"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := ReadClaudeCodeCLIModelCache(); !slices.Equal(got, []string{"claude-x-1"}) {
		t.Fatalf("got %v", got)
	}
	if err := os.WriteFile(filepath.Join(dir, ".claude.json"), []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := ReadClaudeCodeCLIModelCache(); got != nil {
		t.Fatalf("corrupt: %v", got)
	}
}

func TestReadClaudeCodeCLIModelCacheFallsBackToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"additionalModelOptionsCache":[{"value":"claude-y-2"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := ReadClaudeCodeCLIModelCache(); !slices.Equal(got, []string{"claude-y-2"}) {
		t.Fatalf("got %v", got)
	}
}

func TestClaudeModelIDs(t *testing.T) {
	got := ClaudeModelIDs([]string{" claude-opus-9 ", "gpt-5", "", "claude-haiku-5-5"})
	if !slices.Equal(got, []string{"claude-opus-9", "claude-haiku-5-5"}) {
		t.Fatalf("got %v", got)
	}
	if got := ClaudeModelIDs([]string{"other"}); got != nil {
		t.Fatalf("no claude ids should be nil, got %v", got)
	}
}

func TestMergeClaudeCodeCLIModels(t *testing.T) {
	got := MergeClaudeCodeCLIModels(
		[]string{"claude-opus-9", "sonnet"},
		[]string{" ", "claude-opus-9", "opus"},
		nil,
		[]string{"claude-haiku-5-5", "sonnet"},
	)
	want := []string{"sonnet", "opus", "claude-opus-9", "claude-haiku-5-5"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
