package main

import (
	"flag"
	"fmt"
	"os"

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

func usage() {
	fmt.Fprintln(os.Stderr, "usage: releasectl <validate-release|homebrew-formula|homebrew-cask> [flags]")
}
