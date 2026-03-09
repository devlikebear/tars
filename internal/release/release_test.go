package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateReleaseBundle(t *testing.T) {
	dir := t.TempDir()
	versionFile := filepath.Join(dir, "VERSION.txt")
	changelogFile := filepath.Join(dir, "CHANGELOG.md")
	if err := os.WriteFile(versionFile, []byte("1.2.3\n"), 0o644); err != nil {
		t.Fatalf("write version file: %v", err)
	}
	if err := os.WriteFile(changelogFile, []byte("# Changelog\n\n## [1.2.3] - 2026-03-08\n"), 0o644); err != nil {
		t.Fatalf("write changelog file: %v", err)
	}

	version, err := ValidateReleaseBundle(versionFile, changelogFile)
	if err != nil {
		t.Fatalf("validate release bundle: %v", err)
	}
	if version != "1.2.3" {
		t.Fatalf("unexpected version: %q", version)
	}
}

func TestValidateReleaseBundle_RequiresSemverAndMatchingChangelog(t *testing.T) {
	dir := t.TempDir()
	versionFile := filepath.Join(dir, "VERSION.txt")
	changelogFile := filepath.Join(dir, "CHANGELOG.md")
	if err := os.WriteFile(versionFile, []byte("not-a-version\n"), 0o644); err != nil {
		t.Fatalf("write version file: %v", err)
	}
	if err := os.WriteFile(changelogFile, []byte("# Changelog\n\n## [0.1.0] - 2026-03-08\n"), 0o644); err != nil {
		t.Fatalf("write changelog file: %v", err)
	}

	if _, err := ValidateReleaseBundle(versionFile, changelogFile); err == nil {
		t.Fatal("expected invalid semver to fail")
	}

	if err := os.WriteFile(versionFile, []byte("1.2.3\n"), 0o644); err != nil {
		t.Fatalf("rewrite version file: %v", err)
	}
	if _, err := ValidateReleaseBundle(versionFile, changelogFile); err == nil {
		t.Fatal("expected missing changelog version section to fail")
	}
}

func TestHomebrewFormulaIncludesBothMacArchitectures(t *testing.T) {
	formula, err := HomebrewFormula("devlikebear/tars", "1.2.3", "arm64-sha", "amd64-sha")
	if err != nil {
		t.Fatalf("homebrew formula: %v", err)
	}

	wantSnippets := []string{
		"class Tars < Formula",
		"https://github.com/devlikebear/tars/releases/download/v1.2.3/tars_1.2.3_darwin_arm64.tar.gz",
		"https://github.com/devlikebear/tars/releases/download/v1.2.3/tars_1.2.3_darwin_amd64.tar.gz",
		"sha256 \"arm64-sha\"",
		"sha256 \"amd64-sha\"",
		"brew install ffmpeg whisper-cpp",
	}
	for _, snippet := range wantSnippets {
		if !strings.Contains(formula, snippet) {
			t.Fatalf("formula missing snippet %q:\n%s", snippet, formula)
		}
	}
}

func TestReleaseAssetMetadata(t *testing.T) {
	if got := AssetArchiveName("1.2.3", "darwin", "arm64"); got != "tars_1.2.3_darwin_arm64.tar.gz" {
		t.Fatalf("unexpected asset archive name: %q", got)
	}
	if got := ReleaseTag("1.2.3"); got != "v1.2.3" {
		t.Fatalf("unexpected release tag: %q", got)
	}
	if got := ReleaseAssetURL("devlikebear/tars", "1.2.3", "darwin", "amd64"); got != "https://github.com/devlikebear/tars/releases/download/v1.2.3/tars_1.2.3_darwin_amd64.tar.gz" {
		t.Fatalf("unexpected release asset url: %q", got)
	}
}
