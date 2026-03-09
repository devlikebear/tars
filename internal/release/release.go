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

func AssetArchiveName(version, goos, goarch string) string {
	return fmt.Sprintf("tars_%s_%s_%s.tar.gz", strings.TrimSpace(version), strings.TrimSpace(goos), strings.TrimSpace(goarch))
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
  end

  def caveats
    <<~EOS
      Optional assistant dependencies are not installed by this formula.
      Install them separately when needed:
        brew install ffmpeg whisper-cpp
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/tars --version")
  end
end
`, repoSlug, version, ReleaseAssetURL(repoSlug, version, "darwin", "arm64"), arm64SHA, ReleaseAssetURL(repoSlug, version, "darwin", "amd64"), amd64SHA), nil
}
