package executionplane

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClaudeCodeWorkerLive(t *testing.T) {
	if os.Getenv("TARS_CLAUDE_CODE_HARNESS_LIVE") != "1" {
		t.Skip("set TARS_CLAUDE_CODE_HARNESS_LIVE=1 to use the installed Claude login")
	}

	root := t.TempDir()
	worker, err := NewClaudeCodeWorker(ClaudeCodeWorkerOptions{
		Model: "sonnet", Timeout: 2 * time.Minute, MaxTurns: 5, MaxBudgetUSD: 0.50,
		Tools: []string{"Read", "Write", "Glob"},
		AllowedTools: []string{
			"Read(./**)", "Write(./**)", "Glob(./**)",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	execution := testExecution()
	execution.Work.Title = "Claude Code harness live smoke"
	execution.Work.Objective = "Create live-harness.txt containing exactly: tars-claude-harness-live-ok"
	execution.Claim.Step.Title = "Create the smoke artifact"
	execution.Claim.Step.Description = "Write live-harness.txt with the exact requested content and do not change any other file."

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	result, err := worker.Execute(ctx, WorkerRequest{
		Execution: execution,
		Environment: Environment{
			SchemaVersion: 1, ID: "worktree:attempt-1", Kind: "managed-worktree", RootDir: root,
		},
	})
	if err != nil {
		t.Fatalf("live Claude Code harness: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "live-harness.txt"))
	if err != nil {
		t.Fatalf("read live artifact: %v", err)
	}
	if strings.TrimSpace(string(raw)) != "tars-claude-harness-live-ok" {
		t.Fatalf("live artifact = %q", raw)
	}
	if !result.ExecutionResult.Succeeded || result.ExecutionResult.Usage.Iterations <= 0 ||
		result.ExecutionResult.Usage.Tokens <= 0 || len(result.Transcript) < 2 {
		t.Fatalf("live result = %+v transcript=%+v", result.ExecutionResult, result.Transcript)
	}
}
