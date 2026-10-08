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

	stdout, stderr = captureReleaseCommandOutput(t, func() {
		homebrewCask([]string{
			"--repo", "example/tars", "--version", "1.2.3",
			"--arm64-sha", "desk-arm64", "--amd64-sha", "desk-amd64",
		})
	})
	if stderr != "" || !strings.Contains(stdout, `cask "tars-desktop" do`) ||
		!strings.Contains(stdout, "example/tars/releases/download/v#{version}") ||
		!strings.Contains(stdout, `"desk-arm64"`) || !strings.Contains(stdout, `"desk-amd64"`) {
		t.Fatalf("homebrew cask stdout=%q stderr=%q", stdout, stderr)
	}

	_, stderr = captureReleaseCommandOutput(t, usage)
	if !strings.Contains(stderr, "validate-release|homebrew-formula|homebrew-cask") {
		t.Fatalf("usage stderr=%q", stderr)
	}
}

func TestWriteWingetManifestsWritesBothPackages(t *testing.T) {
	out := t.TempDir()
	server := strings.Repeat("a", 64)
	desktop := strings.Repeat("b", 64)
	stdout, _ := captureReleaseCommandOutput(t, func() {
		if err := writeWingetManifests(out, "devlikebear/tars", "1.2.3", server, desktop); err != nil {
			t.Error(err)
		}
	})
	for _, rel := range []string{
		"d/devlikebear/TARS/1.2.3/devlikebear.TARS.installer.yaml",
		"d/devlikebear/TARS/Desktop/1.2.3/devlikebear.TARS.Desktop.installer.yaml",
	} {
		path := filepath.Join(out, filepath.FromSlash(rel))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("manifest %s not written: %v", rel, err)
		}
		if !strings.Contains(string(data), "NestedInstallerType: portable") {
			t.Errorf("%s is not a portable installer manifest:\n%s", rel, data)
		}
		if !strings.Contains(stdout, path) {
			t.Errorf("stdout does not list %s:\n%s", path, stdout)
		}
	}
}

func TestWriteWingetManifestsRequiresOutAndValidInput(t *testing.T) {
	sha := strings.Repeat("a", 64)
	if err := writeWingetManifests(" ", "devlikebear/tars", "1.2.3", sha, sha); err == nil {
		t.Fatal("expected an error without --out")
	}
	if err := writeWingetManifests(t.TempDir(), "devlikebear/tars", "1.2.3", "", sha); err == nil {
		t.Fatal("expected an error without a server checksum")
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
