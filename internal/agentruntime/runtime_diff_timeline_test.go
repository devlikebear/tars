package agentruntime

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/session"
)

func TestRuntimeCapturesGitDiffTimelineForShellStyleChanges(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "tars@example.test")
	runGit(t, repo, "config", "user.name", "TARS Test")
	writeFile(t, filepath.Join(repo, "notes.md"), "before\n")
	runGit(t, repo, "add", "notes.md")
	runGit(t, repo, "commit", "-m", "initial notes")

	store := session.NewStore(t.TempDir())
	rt := NewRuntime(RuntimeOptions{
		Enabled:      true,
		WorkspaceDir: repo,
		SessionStore: store,
		RunPrompt: func(_ context.Context, _ string, _ string) (string, error) {
			writeFile(t, filepath.Join(repo, "notes.md"), "before\nafter\n")
			return "updated notes", nil
		},
	})
	t.Cleanup(func() { closeAgentRuntime(t, rt) })

	run, err := rt.Spawn(context.Background(), SpawnRequest{
		Prompt: "append notes",
		FlowID: "flow_1",
		StepID: "step_2",
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	final, err := rt.Wait(waitCtx, run.ID)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if final.Status != RunStatusCompleted {
		t.Fatalf("expected completed run, got %+v", final)
	}
	if len(final.FileAttention) != 0 {
		t.Fatalf("expected no file tool calls, got %+v", final.FileAttention)
	}
	if len(final.DiffTimeline) != 1 {
		t.Fatalf("expected one diff timeline entry, got %+v", final.DiffTimeline)
	}
	entry := final.DiffTimeline[0]
	if entry.RunID != final.ID || entry.SessionID != final.SessionID || entry.Agent != final.Agent {
		t.Fatalf("expected run metadata on diff entry, got %+v", entry)
	}
	if entry.FlowID != "flow_1" || entry.StepID != "step_2" {
		t.Fatalf("expected plan metadata on diff entry, got %+v", entry)
	}
	wantRepoRoot, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	if filepath.Clean(entry.RepoRoot) != filepath.Clean(wantRepoRoot) {
		t.Fatalf("expected repo root %q, got %q", repo, entry.RepoRoot)
	}
	if entry.Summary.Files != 1 || entry.Summary.Additions != 1 || entry.Summary.Deletions != 0 {
		t.Fatalf("unexpected diff summary: %+v", entry.Summary)
	}
	if len(entry.Files) != 1 {
		t.Fatalf("expected one changed file, got %+v", entry.Files)
	}
	file := entry.Files[0]
	if file.Path != "notes.md" || file.Status != "modified" {
		t.Fatalf("unexpected changed file metadata: %+v", file)
	}
	if file.Additions != 1 || file.Deletions != 0 {
		t.Fatalf("unexpected changed file stats: %+v", file)
	}
	if !strings.Contains(file.Patch, "+after") {
		t.Fatalf("expected patch preview to include added line, got:\n%s", file.Patch)
	}
}

func TestGitNoIndexPatchCapturesUntrackedFile(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	writeFile(t, filepath.Join(repo, "notes.md"), "new\n")

	patch, ok := gitNoIndexPatch(repo, "notes.md")
	if !ok {
		t.Fatalf("expected no-index patch for untracked file")
	}
	if !strings.Contains(patch, "+new") {
		t.Fatalf("expected patch to contain added content, got:\n%s", patch)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
