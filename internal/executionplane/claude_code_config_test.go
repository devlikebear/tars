package executionplane

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenConfiguredClaudeCodeWorkerLoadsPrivateBoundedConfig(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "claude-code.json")
	raw := `{
  "schema_version": 1,
  "adapter": "claude-code",
  "model": "sonnet",
  "timeout_seconds": 900,
  "max_turns": 20,
  "max_budget_usd": 5,
  "tools": ["Read", "Edit", "Write", "Glob", "Grep", "Bash"],
  "allowed_tools": ["Read(./**)", "Edit(./**)", "Write(./**)", "Glob(./**)", "Grep(./**)", "Bash(git status *)", "Bash(git diff *)"]
}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	worker, err := OpenConfiguredClaudeCodeWorker(path)
	if err != nil {
		t.Fatalf("open configured worker: %v", err)
	}
	if worker.Name() != "claude-code" || worker.timeout.Seconds() != 900 || worker.maxTurns != 20 || worker.maxBudgetUSD != 5 {
		t.Fatalf("configured worker = %+v", worker)
	}
}

func TestOpenConfiguredClaudeCodeWorkerRejectsUnsafeConfig(t *testing.T) {
	t.Parallel()

	valid := `{"schema_version":1,"adapter":"claude-code","model":"sonnet","timeout_seconds":60,"max_turns":2,"max_budget_usd":1,"tools":["Read"],"allowed_tools":["Read(./**)"]}`
	t.Run("relative path", func(t *testing.T) {
		if _, err := OpenConfiguredClaudeCodeWorker("claude-code.json"); err == nil {
			t.Fatal("accepted relative config path")
		}
	})
	t.Run("world readable", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(valid), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenConfiguredClaudeCodeWorker(path); err == nil || !strings.Contains(err.Error(), "owner-only") {
			t.Fatalf("unsafe mode error = %v", err)
		}
	})
	t.Run("symlink", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, "target.json")
		link := filepath.Join(root, "link.json")
		if err := os.WriteFile(target, []byte(valid), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenConfiguredClaudeCodeWorker(link); err == nil {
			t.Fatal("accepted symlink config")
		}
	})
	for _, testCase := range []struct {
		name string
		raw  string
	}{
		{name: "unknown field", raw: strings.TrimSuffix(valid, "}") + `,"env":{"ANTHROPIC_API_KEY":"secret"}}`},
		{name: "trailing data", raw: valid + `{}`},
		{name: "wrong adapter", raw: strings.Replace(valid, `"claude-code"`, `"other"`, 1)},
		{name: "unbounded rule", raw: strings.Replace(valid, `"Read(./**)"`, `"Read"`, 1)},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(testCase.raw), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := OpenConfiguredClaudeCodeWorker(path); err == nil {
				t.Fatalf("accepted unsafe config: %s", testCase.raw)
			}
		})
	}
}
