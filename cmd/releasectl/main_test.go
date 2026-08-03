package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseCommandsRenderValidatedBundleAndFormula(t *testing.T) {
	dir := t.TempDir()
	versionPath := filepath.Join(dir, "VERSION.txt")
	changelogPath := filepath.Join(dir, "CHANGELOG.md")
	if err := os.WriteFile(versionPath, []byte("1.2.3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(changelogPath, []byte("## [1.2.3] - 2026-08-03\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr := captureReleaseCommandOutput(t, func() {
		validateRelease([]string{"--version-file", versionPath, "--changelog", changelogPath})
	})
	if stdout != "1.2.3\n" || stderr != "" {
		t.Fatalf("validate release stdout=%q stderr=%q", stdout, stderr)
	}

	stdout, stderr = captureReleaseCommandOutput(t, func() {
		homebrewFormula([]string{
			"--repo", "example/tars", "--version", "1.2.3",
			"--arm64-sha", "arm64-sha", "--amd64-sha", "amd64-sha",
		})
	})
	if stderr != "" || !strings.Contains(stdout, "class Tars < Formula") ||
		!strings.Contains(stdout, "example/tars/releases/download/v1.2.3") ||
		!strings.Contains(stdout, `sha256 "arm64-sha"`) || !strings.Contains(stdout, `sha256 "amd64-sha"`) {
		t.Fatalf("homebrew formula stdout=%q stderr=%q", stdout, stderr)
	}

	_, stderr = captureReleaseCommandOutput(t, usage)
	if !strings.Contains(stderr, "validate-release|homebrew-formula") {
		t.Fatalf("usage stderr=%q", stderr)
	}
}

func captureReleaseCommandOutput(t *testing.T, run func()) (string, string) {
	t.Helper()
	oldStdout, oldStderr := os.Stdout, os.Stderr
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = stdoutWriter, stderrWriter
	t.Cleanup(func() {
		os.Stdout, os.Stderr = oldStdout, oldStderr
	})

	run()
	if err := stdoutWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := stderrWriter.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = oldStdout, oldStderr
	stdout, err := io.ReadAll(stdoutReader)
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := io.ReadAll(stderrReader)
	if err != nil {
		t.Fatal(err)
	}
	if err := stdoutReader.Close(); err != nil {
		t.Fatal(err)
	}
	if err := stderrReader.Close(); err != nil {
		t.Fatal(err)
	}
	return string(stdout), string(stderr)
}
