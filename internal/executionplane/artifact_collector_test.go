package executionplane

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileArtifactCollectorCopiesConfinedRedactedArtifacts(t *testing.T) {
	t.Parallel()

	environmentRoot := t.TempDir()
	artifactsRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(environmentRoot, "output.txt"), []byte("result secret-value\n"), 0o600); err != nil {
		t.Fatalf("write output: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(environmentRoot, "logs"), 0o700); err != nil {
		t.Fatalf("create logs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(environmentRoot, "logs", "run.log"), []byte("safe log\n"), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}
	if err := os.WriteFile(filepath.Join(environmentRoot, ".env"), []byte("API_KEY=secret-value\n"), 0o600); err != nil {
		t.Fatalf("write secret file: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(environmentRoot, "outside-link")); err != nil {
		t.Fatalf("create outside symlink: %v", err)
	}
	collector, err := NewFileArtifactCollector(ArtifactCollectorOptions{
		RootDir: artifactsRoot, Paths: []string{"."}, IncludeTranscript: true,
	})
	if err != nil {
		t.Fatalf("new artifact collector: %v", err)
	}
	request := CollectRequest{
		Execution:    testExecution(),
		Environment:  Environment{SchemaVersion: 1, ID: "env-artifact", Kind: "worktree", RootDir: environmentRoot},
		Worker:       WorkerResult{Transcript: []TranscriptEntry{{Sequence: 1, Type: "assistant", Text: "used secret-value"}}},
		RedactValues: []string{"secret-value"},
	}
	artifacts, err := collector.Collect(context.Background(), request)
	if err != nil {
		t.Fatalf("collect artifacts: %v", err)
	}
	if len(artifacts) != 3 {
		t.Fatalf("artifact count = %d, want output, log, transcript: %#v", len(artifacts), artifacts)
	}
	for _, artifact := range artifacts {
		if strings.Contains(artifact.Name, ".env") || strings.Contains(artifact.Name, "outside-link") || artifact.Digest == "" || artifact.SizeBytes < 0 {
			t.Fatalf("unsafe artifact = %#v", artifact)
		}
		path, err := filepathFromURI(artifact.URI)
		if err != nil {
			t.Fatalf("artifact URI %q: %v", artifact.URI, err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read copied artifact: %v", err)
		}
		if strings.Contains(string(raw), "secret-value") {
			t.Fatalf("artifact %q retained credential value: %s", artifact.Name, raw)
		}
	}
	if got, err := os.ReadFile(filepath.Join(environmentRoot, "output.txt")); err != nil || string(got) != "result secret-value\n" {
		t.Fatalf("collector mutated source artifact: %q, %v", got, err)
	}
}

func TestFileArtifactCollectorRejectsTraversalAndDestinationOverlap(t *testing.T) {
	t.Parallel()

	if _, err := NewFileArtifactCollector(ArtifactCollectorOptions{RootDir: t.TempDir(), Paths: []string{"../secret"}}); err == nil {
		t.Fatal("collector accepted traversal path")
	}
	environmentRoot := t.TempDir()
	collector, err := NewFileArtifactCollector(ArtifactCollectorOptions{RootDir: filepath.Join(environmentRoot, "artifacts"), Paths: []string{"."}})
	if err != nil {
		t.Fatalf("new overlapping collector: %v", err)
	}
	_, err = collector.Collect(context.Background(), CollectRequest{
		Execution: testExecution(), Environment: Environment{RootDir: environmentRoot},
	})
	if err == nil || !strings.Contains(err.Error(), "overlap") {
		t.Fatalf("overlapping collection error = %v", err)
	}
}

func TestFileArtifactCollectorCapturesTrackedAndUntrackedGitPatch(t *testing.T) {
	t.Parallel()

	environmentRoot := t.TempDir()
	artifactsRoot := t.TempDir()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.name", "TARS Test"},
		{"config", "user.email", "tars@example.invalid"},
	} {
		if _, err := runGitCommand(context.Background(), environmentRoot, args...); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(environmentRoot, "tracked.go"), []byte("package sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runGitCommand(context.Background(), environmentRoot, "add", "tracked.go"); err != nil {
		t.Fatal(err)
	}
	if _, err := runGitCommand(context.Background(), environmentRoot, "commit", "-m", "base"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(environmentRoot, "tracked.go"), []byte("package changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(environmentRoot, "new.go"), []byte("package newfile // secret-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(environmentRoot, ".tars"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(environmentRoot, ".tars", "execution-environment.json"), []byte("private marker"), 0o600); err != nil {
		t.Fatal(err)
	}
	collector, err := NewFileArtifactCollector(ArtifactCollectorOptions{
		RootDir: artifactsRoot, IncludeTranscript: true, IncludeGitPatch: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := collector.Collect(context.Background(), CollectRequest{
		Execution: testExecution(), Environment: Environment{RootDir: environmentRoot},
		Worker:       WorkerResult{Transcript: []TranscriptEntry{{Sequence: 1, Type: "assistant", Text: "done"}}},
		RedactValues: []string{"secret-value"},
	})
	if err != nil {
		t.Fatalf("collect Git patch: %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("artifacts = %+v, want patch and transcript", artifacts)
	}
	var patchText string
	for _, artifact := range artifacts {
		if artifact.Name != "changes.patch" {
			continue
		}
		path, err := filepathFromURI(artifact.URI)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		patchText = string(raw)
	}
	for _, want := range []string{"tracked.go", "package changed", "new.go", "package newfile"} {
		if !strings.Contains(patchText, want) {
			t.Fatalf("patch missing %q:\n%s", want, patchText)
		}
	}
	if strings.Contains(patchText, "secret-value") || strings.Contains(patchText, "execution-environment.json") || strings.Contains(patchText, "private marker") {
		t.Fatalf("patch leaked redacted/private content:\n%s", patchText)
	}
}
