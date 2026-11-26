package proofverifier

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/devlikebear/tars/internal/workscheduler"
	"github.com/devlikebear/tars/internal/workstore"
)

// A verification that names no paths snapshots what git lists, and git lists
// a tracked file the work deleted without committing. That file is gone, not
// a broken subject: the command still runs and its exit code decides.
func TestEngineCommandPassesWhenATrackedFileWasDeleted(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for name, text := range map[string]string{"keep.txt": "keep\n", "gone.css": "body{}\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "-A"},
		{"-c", "user.name=test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "start"},
	} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("git %v unavailable: %v: %s", args, err, out)
		}
	}
	if err := os.Remove(filepath.Join(root, "gone.css")); err != nil {
		t.Fatalf("delete tracked file: %v", err)
	}
	engine, err := New(Options{ID: "verifier-1", RootDir: root, Timeout: commandTestBudget})
	if err != nil {
		t.Fatalf("new proof verifier: %v", err)
	}
	result, err := engine.Verify(context.Background(), workscheduler.VerificationRequest{
		Requirement: workstore.ProofRequirement{Kind: "test", Verifier: engine.Name(), Command: "exit 0"},
	})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if result.Status != workstore.ProofStatusPassed || len(result.InputJSON) == 0 {
		t.Fatalf("result = %+v, want the command to run and pass", result)
	}
	var tree []treeDigest
	if err := json.Unmarshal(result.ArtifactDigestsJSON, &tree); err != nil || len(tree) != 1 || tree[0].Files != 1 {
		t.Fatalf("artifact digests = %s (%v), want one tree of the one file left", result.ArtifactDigestsJSON, err)
	}
}

// A path the requirement names itself must still exist.
func TestEngineCommandFailsWhenADeclaredPathIsMissing(t *testing.T) {
	t.Parallel()

	engine, err := New(Options{ID: "verifier-1", RootDir: t.TempDir(), Timeout: commandTestBudget})
	if err != nil {
		t.Fatalf("new proof verifier: %v", err)
	}
	result, err := engine.Verify(context.Background(), workscheduler.VerificationRequest{
		Requirement: workstore.ProofRequirement{Kind: "test", Verifier: engine.Name(), Command: "exit 0", Paths: []string{"missing.txt"}},
	})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if result.Status != workstore.ProofStatusFailed || result.Summary != "verification subject is unavailable" {
		t.Fatalf("result = %+v, want a failed subject", result)
	}
}
