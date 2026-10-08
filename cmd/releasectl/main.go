package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/devlikebear/tars/internal/release"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "validate-release":
		validateRelease(os.Args[2:])
	case "homebrew-formula":
		homebrewFormula(os.Args[2:])
	case "homebrew-cask":
		homebrewCask(os.Args[2:])
	case "winget-manifests":
		wingetManifests(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func validateRelease(args []string) {
	fs := flag.NewFlagSet("validate-release", flag.ExitOnError)
	versionFile := fs.String("version-file", "VERSION.txt", "path to VERSION.txt")
	changelogFile := fs.String("changelog", "CHANGELOG.md", "path to CHANGELOG.md")
	_ = fs.Parse(args)

	version, err := release.ValidateReleaseBundle(*versionFile, *changelogFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, version)
}

func homebrewFormula(args []string) {
	fs := flag.NewFlagSet("homebrew-formula", flag.ExitOnError)
	repoSlug := fs.String("repo", "devlikebear/tars", "GitHub repo slug")
	version := fs.String("version", "", "release version without v prefix")
	arm64SHA := fs.String("arm64-sha", "", "SHA256 for darwin arm64 asset")
	amd64SHA := fs.String("amd64-sha", "", "SHA256 for darwin amd64 asset")
	_ = fs.Parse(args)

	formula, err := release.HomebrewFormula(*repoSlug, *version, *arm64SHA, *amd64SHA)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Fprint(os.Stdout, formula)
}

func homebrewCask(args []string) {
	fs := flag.NewFlagSet("homebrew-cask", flag.ExitOnError)
	repoSlug := fs.String("repo", "devlikebear/tars", "GitHub repo slug")
	version := fs.String("version", "", "release version without v prefix")
	arm64SHA := fs.String("arm64-sha", "", "SHA256 for the darwin arm64 desktop archive")
	amd64SHA := fs.String("amd64-sha", "", "SHA256 for the darwin amd64 desktop archive")
	_ = fs.Parse(args)

	cask, err := release.HomebrewCask(*repoSlug, *version, *arm64SHA, *amd64SHA)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if _, err := fmt.Fprint(os.Stdout, cask); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// wingetManifests writes the winget-pkgs manifests of a release under --out,
// at the paths they have below winget-pkgs' manifests/ directory.
func wingetManifests(args []string) {
	fs := flag.NewFlagSet("winget-manifests", flag.ExitOnError)
	repoSlug := fs.String("repo", "devlikebear/tars", "GitHub repo slug")
	version := fs.String("version", "", "release version without v prefix")
	serverSHA := fs.String("server-sha", "", "SHA256 for the windows amd64 server archive")
	desktopSHA := fs.String("desktop-sha", "", "SHA256 for the windows amd64 desktop archive")
	out := fs.String("out", "", "directory to write the manifests under (a winget-pkgs checkout's manifests/)")
	_ = fs.Parse(args)

	if err := writeWingetManifests(*out, *repoSlug, *version, *serverSHA, *desktopSHA); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func writeWingetManifests(out, repoSlug, version, serverSHA, desktopSHA string) error {
	if strings.TrimSpace(out) == "" {
		return fmt.Errorf("--out is required")
	}
	files, err := release.WingetManifests(repoSlug, version, serverSHA, desktopSHA)
	if err != nil {
		return err
	}
	for _, f := range files {
		target := filepath.Join(out, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, []byte(f.Content), 0o644); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(os.Stdout, target); err != nil {
			return err
		}
	}
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: releasectl <validate-release|homebrew-formula|homebrew-cask|winget-manifests> [flags]")
}
