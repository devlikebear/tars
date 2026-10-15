package release

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var semverPattern = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

func ValidateReleaseBundle(versionFile, changelogFile string) (string, error) {
	versionBytes, err := os.ReadFile(versionFile)
	if err != nil {
		return "", fmt.Errorf("read version file: %w", err)
	}
	version := strings.TrimSpace(string(versionBytes))
	if err := ValidateVersion(version); err != nil {
		return "", err
	}

	changelogBytes, err := os.ReadFile(changelogFile)
	if err != nil {
		return "", fmt.Errorf("read changelog file: %w", err)
	}
	if !strings.Contains(string(changelogBytes), fmt.Sprintf("## [%s]", version)) {
		return "", fmt.Errorf("missing changelog section for version %s", version)
	}
	return version, nil
}

func ValidateVersion(version string) error {
	version = strings.TrimSpace(version)
	if version == "" {
		return fmt.Errorf("release version must not be empty")
	}
	if !semverPattern.MatchString(version) {
		return fmt.Errorf("release version must be valid semver, got %q", version)
	}
	return nil
}

func ReleaseTag(version string) string {
	return "v" + strings.TrimSpace(version)
}

// AssetArchiveName is the server's release archive for goos/goarch; it must
// match the Makefile's release-asset target.
func AssetArchiveName(version, goos, goarch string) string {
	return fmt.Sprintf("tars_%s_%s_%s%s", strings.TrimSpace(version), strings.TrimSpace(goos), strings.TrimSpace(goarch), ArchiveExt(goos))
}

// ArchiveExt is .zip for Windows archives and .tar.gz elsewhere.
func ArchiveExt(goos string) string {
	if strings.TrimSpace(goos) == "windows" {
		return ".zip"
	}
	return ".tar.gz"
}

func ReleaseAssetURL(repoSlug, version, goos, goarch string) string {
	return fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", strings.TrimSpace(repoSlug), ReleaseTag(version), AssetArchiveName(version, goos, goarch))
}

func HomebrewFormula(repoSlug, version, arm64SHA, amd64SHA string) (string, error) {
	if err := ValidateVersion(version); err != nil {
		return "", err
	}
	repoSlug = strings.TrimSpace(repoSlug)
	if repoSlug == "" {
		return "", fmt.Errorf("repository slug must not be empty")
	}
	arm64SHA = strings.TrimSpace(arm64SHA)
	amd64SHA = strings.TrimSpace(amd64SHA)
	if arm64SHA == "" || amd64SHA == "" {
		return "", fmt.Errorf("arm64 and amd64 SHA256 values are required")
	}

	return fmt.Sprintf(`class Tars < Formula
  desc "Local-first automation runtime written in Go"
  homepage "https://github.com/%[1]s"
  version "%[2]s"

  on_macos do
    if Hardware::CPU.arm?
      url "%[3]s"
      sha256 "%[4]s"
    else
      url "%[5]s"
      sha256 "%[6]s"
    end
  end

  def install
    bin.install "tars"
    prefix.install "share" if Dir.exist?("share")
  end

  def caveats
    <<~EOS
      Optional assistant dependencies are not installed by this formula.
      Install them separately when needed:
        brew install ffmpeg whisper-cpp

      This formula is the command-line install. The desktop app (tray,
      approval notifications, its own window) ships its own tars, so with
      it you do not need this formula:
        brew install --cask devlikebear/tap/tars-desktop
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/tars --version")
  end
end
`, repoSlug, version, ReleaseAssetURL(repoSlug, version, "darwin", "arm64"), arm64SHA, ReleaseAssetURL(repoSlug, version, "darwin", "amd64"), amd64SHA), nil
}

// HomebrewCask renders the tap's tars-desktop cask. TARS.app carries the
// tars server (Contents/Helpers/tars), so the cask is the whole install:
// the app, and a bin/tars link to the server it ships. The tars formula
// stays as the command-line install for machines without the app.
func HomebrewCask(repoSlug, version, arm64SHA, amd64SHA string) (string, error) {
	if err := ValidateVersion(version); err != nil {
		return "", err
	}
	repoSlug = strings.TrimSpace(repoSlug)
	if repoSlug == "" {
		return "", fmt.Errorf("repository slug must not be empty")
	}
	arm64SHA = strings.TrimSpace(arm64SHA)
	amd64SHA = strings.TrimSpace(amd64SHA)
	if arm64SHA == "" || amd64SHA == "" {
		return "", fmt.Errorf("arm64 and amd64 SHA256 values are required")
	}

	return fmt.Sprintf(`cask "tars-desktop" do
  arch arm: "arm64", intel: "amd64"

  version "%[2]s"
  sha256 arm:   "%[3]s",
         intel: "%[4]s"

  url "https://github.com/%[1]s/releases/download/v#{version}/tars-desktop_#{version}_darwin_#{arch}.tar.gz"
  name "TARS"
  desc "Desktop app for the local TARS server"
  homepage "https://github.com/%[1]s"

  livecheck do
    url :url
    strategy :github_latest
  end

  # The app replaces itself from GitHub releases (tray: Check for updates).
  auto_updates true
  depends_on :macos

  app "TARS.app"

  # TARS.app ships the tars server and command-line tool. The formula
  # devlikebear/tap/tars installs the same command without the app; when it
  # is installed it keeps bin/tars, and linking over it would fail.
  binary "#{appdir}/TARS.app/Contents/Helpers/tars" unless (HOMEBREW_PREFIX/"opt/tars").exist?

  zap launchctl: "io.tars.server",
      trash:     [
        "~/Library/Application Support/tars-desktop",
        "~/Library/Caches/com.devlikebear.tars.desktop",
        "~/Library/LaunchAgents/io.tars.server.plist",
        "~/Library/WebKit/com.devlikebear.tars.desktop",
      ]

  caveats <<~EOS
    TARS.app includes the tars server and the tars command; nothing else
    needs installing.

    If the formula devlikebear/tap/tars is installed, it keeps the tars
    command on PATH (the app still runs its own server). To use the app's:
      brew uninstall devlikebear/tap/tars
      brew reinstall --cask devlikebear/tap/tars-desktop
  EOS
end
`, repoSlug, version, arm64SHA, amd64SHA), nil
}
