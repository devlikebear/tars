// Package update picks the desktop shell's own archive out of a TARS
// GitHub release.
//
// A release carries the CLI archives (tars_<version>_<os>_<arch>.tar.gz)
// and the shell's (tars-desktop_<version>_<os>_<arch>.tar.gz or .zip) under
// one tag, with one checksums.txt. The stock matcher picks the first asset
// naming the platform, which would be the CLI; installing that in place of
// the shell would break it. This matcher takes only the shell's archive.
package update

import (
	"strings"

	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

// Repository is where releases are published.
const Repository = "devlikebear/tars"

// ChecksumAsset is the release's sha256 list, covering every archive.
const ChecksumAsset = "checksums.txt"

// AssetPrefix starts every shell archive name.
const AssetPrefix = "tars-desktop_"

// AssetName is the archive name the release workflow publishes.
func AssetName(version, goos, goarch string) string {
	return AssetPrefix + version + "_" + goos + "_" + goarch + ArchiveExt(goos)
}

// ArchiveExt is .zip on Windows and .tar.gz elsewhere.
func ArchiveExt(goos string) string {
	if goos == "windows" {
		return ".zip"
	}
	return ".tar.gz"
}

// MatchAsset returns the index of the shell archive for req's platform, or -1.
func MatchAsset(req updater.CheckRequest, assets []github.ReleaseAsset) int {
	suffix := "_" + strings.ToLower(req.Platform) + "_" + strings.ToLower(req.Arch) + ArchiveExt(strings.ToLower(req.Platform))
	for i, a := range assets {
		name := strings.ToLower(a.Name)
		if strings.HasPrefix(name, AssetPrefix) && strings.HasSuffix(name, suffix) {
			return i
		}
	}
	return -1
}

// Provider is the GitHub release source for the shell.
func Provider() (*github.Provider, error) {
	return github.New(github.Config{
		Repository:    Repository,
		AssetMatcher:  MatchAsset,
		ChecksumAsset: ChecksumAsset,
	})
}

// Enabled reports whether a build can update itself: a development build
// has no release version to compare against.
func Enabled(version string) bool {
	v := strings.TrimPrefix(strings.TrimSpace(version), "v")
	return v != "" && v != "dev" && !strings.HasPrefix(v, "0.0.0")
}
