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
		`prefix.install "share" if Dir.exist?("share")`,
		"brew install ffmpeg whisper-cpp",
		"brew install --cask devlikebear/tap/tars-desktop",
	}
	for _, snippet := range wantSnippets {
		if !strings.Contains(formula, snippet) {
			t.Fatalf("formula missing snippet %q:\n%s", snippet, formula)
		}
	}
}

func TestHomebrewCaskInstallsDesktopWithServer(t *testing.T) {
	cask, err := HomebrewCask("devlikebear/tars", "1.2.3", "desk-arm64-sha", "desk-amd64-sha")
	if err != nil {
		t.Fatalf("homebrew cask: %v", err)
	}

	wantSnippets := []string{
		`cask "tars-desktop" do`,
		`version "1.2.3"`,
		`arch arm: "arm64", intel: "amd64"`,
		`sha256 arm:   "desk-arm64-sha",`,
		`intel: "desk-amd64-sha"`,
		`url "https://github.com/devlikebear/tars/releases/download/v#{version}/tars-desktop_#{version}_darwin_#{arch}.tar.gz"`,
		// The desktop app only shows the console of a local server, so the
		// cask pulls in the server formula: one command installs both.
		`depends_on formula: "devlikebear/tap/tars"`,
		// The app replaces itself from GitHub releases.
		"auto_updates true",
		`app "TARS.app"`,
	}
	for _, snippet := range wantSnippets {
		if !strings.Contains(cask, snippet) {
			t.Fatalf("cask missing snippet %q:\n%s", snippet, cask)
		}
	}
}

func TestHomebrewCaskRequiresVersionAndChecksums(t *testing.T) {
	if _, err := HomebrewCask("devlikebear/tars", "not-semver", "a", "b"); err == nil {
		t.Fatal("invalid version must fail")
	}
	if _, err := HomebrewCask("devlikebear/tars", "1.2.3", "a", ""); err == nil {
		t.Fatal("missing amd64 checksum must fail")
	}
	if _, err := HomebrewCask(" ", "1.2.3", "a", "b"); err == nil {
		t.Fatal("empty repository must fail")
	}
}

func TestDesktopArchiveName(t *testing.T) {
	if got := DesktopArchiveName("1.2.3", "darwin", "arm64"); got != "tars-desktop_1.2.3_darwin_arm64.tar.gz" {
		t.Fatalf("unexpected desktop archive name: %q", got)
	}
	if got := DesktopArchiveName("1.2.3", "windows", "amd64"); got != "tars-desktop_1.2.3_windows_amd64.zip" {
		t.Fatalf("unexpected windows desktop archive name: %q", got)
	}
}

func TestReleaseAssetMetadata(t *testing.T) {
	if got := AssetArchiveName("1.2.3", "darwin", "arm64"); got != "tars_1.2.3_darwin_arm64.tar.gz" {
		t.Fatalf("unexpected asset archive name: %q", got)
	}
	if got := AssetArchiveName("1.2.3", "windows", "amd64"); got != "tars_1.2.3_windows_amd64.zip" {
		t.Fatalf("unexpected windows asset archive name: %q", got)
	}
	if got := ReleaseTag("1.2.3"); got != "v1.2.3" {
		t.Fatalf("unexpected release tag: %q", got)
	}
	if got := ReleaseAssetURL("devlikebear/tars", "1.2.3", "darwin", "amd64"); got != "https://github.com/devlikebear/tars/releases/download/v1.2.3/tars_1.2.3_darwin_amd64.tar.gz" {
		t.Fatalf("unexpected release asset url: %q", got)
	}
}
