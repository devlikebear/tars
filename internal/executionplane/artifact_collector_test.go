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
