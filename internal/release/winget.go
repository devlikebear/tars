package release

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

// The winget package identifiers. winget-pkgs lays manifests out by
// publisher and package, so the Desktop package is a sibling directory.
const (
	WingetServerID  = "devlikebear.TARS"
	WingetDesktopID = "devlikebear.TARS.Desktop"

	// wingetManifestVersion is the manifest schema both packages are written
	// against. Zip with a nested portable installer needs 1.4.0 or later.
	wingetManifestVersion = "1.6.0"
)

var sha256Pattern = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

// WingetFile is one manifest file: Path is relative to winget-pkgs'
// manifests/ directory.
type WingetFile struct {
	Path    string
	Content string
}

// WingetManifests renders the winget-pkgs manifests of a release: the
// server (devlikebear.TARS) and the desktop app (devlikebear.TARS.Desktop),
// each as the version, installer and default-locale files.
//
// Both release zips are installed as portable packages: winget unpacks the
// whole zip into its package directory and links the nested executable onto
// PATH, so the server keeps its share/ directory next to tars.exe. The app
// depends on the server, as the Homebrew cask does.
func WingetManifests(repoSlug, version, serverSHA, desktopSHA string) ([]WingetFile, error) {
	if err := ValidateVersion(version); err != nil {
		return nil, err
	}
	version = strings.TrimSpace(version)
	repoSlug = strings.TrimSpace(repoSlug)
	if repoSlug == "" {
		return nil, fmt.Errorf("repository slug must not be empty")
	}
	serverSHA = strings.ToUpper(strings.TrimSpace(serverSHA))
	desktopSHA = strings.ToUpper(strings.TrimSpace(desktopSHA))
	if !sha256Pattern.MatchString(serverSHA) || !sha256Pattern.MatchString(desktopSHA) {
		return nil, fmt.Errorf("server and desktop SHA256 values are required (64 hex characters each)")
	}

	publisher := repoSlug
	if i := strings.Index(repoSlug, "/"); i >= 0 {
		publisher = repoSlug[:i]
	}
	homepage := "https://github.com/" + repoSlug

	server := wingetPackage{
		id:         WingetServerID,
		name:       "TARS",
		short:      "Local-first automation runtime written in Go",
		desc:       "TARS runs a local server with a web console, chat sessions, cron, memory and tools. The server keeps its skills and plugins in a share directory next to tars.exe.",
		tags:       []string{"ai", "agent", "automation", "cli", "llm"},
		url:        ReleaseAssetURL(repoSlug, version, "windows", "amd64"),
		sha:        serverSHA,
		executable: "tars.exe",
		alias:      "tars",
	}
	desktop := wingetPackage{
		id:         WingetDesktopID,
		name:       "TARS Desktop",
		short:      "Desktop app for the local TARS server",
		desc:       "A native window, tray icon, approval notifications and a global hotkey for the console of a local TARS server. Installs the TARS server with it.",
		tags:       []string{"ai", "agent", "automation", "desktop", "llm"},
		url:        wingetDesktopURL(repoSlug, version),
		sha:        desktopSHA,
		executable: "tars-desktop.exe",
		alias:      "tars-desktop",
		dependsOn:  WingetServerID,
	}

	var files []WingetFile
	for _, pkg := range []wingetPackage{server, desktop} {
		dir := wingetManifestDir(pkg.id, version)
		files = append(files,
			WingetFile{Path: path.Join(dir, pkg.id+".yaml"), Content: pkg.versionManifest(version)},
			WingetFile{Path: path.Join(dir, pkg.id+".installer.yaml"), Content: pkg.installerManifest(version)},
			WingetFile{Path: path.Join(dir, pkg.id+".locale.en-US.yaml"), Content: pkg.localeManifest(version, publisher, homepage)},
		)
	}
	return files, nil
}

// wingetManifestDir is winget-pkgs' d/devlikebear/TARS/Desktop/<version>
// layout: the lowercase first letter of the publisher, then one segment per
// identifier part, then the version.
func wingetManifestDir(id, version string) string {
	parts := strings.Split(id, ".")
	segments := append([]string{strings.ToLower(parts[0][:1])}, parts...)
	return path.Join(append(segments, version)...)
}

// wingetDesktopURL is the desktop archive's download URL.
func wingetDesktopURL(repoSlug, version string) string {
	return fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repoSlug, ReleaseTag(version), fmt.Sprintf("tars-desktop_%s_windows_amd64%s", version, ArchiveExt("windows")))
}

type wingetPackage struct {
	id, name, short, desc string
	tags                  []string
	url, sha              string
	executable, alias     string
	dependsOn             string
}

func wingetHeader(kind string) string {
	return fmt.Sprintf("# yaml-language-server: $schema=https://aka.ms/winget-manifest.%s.%s.schema.json\n\n", kind, wingetManifestVersion)
}

func (p wingetPackage) versionManifest(version string) string {
	return wingetHeader("version") + fmt.Sprintf(`PackageIdentifier: %s
PackageVersion: %s
DefaultLocale: en-US
ManifestType: version
ManifestVersion: %s
`, p.id, version, wingetManifestVersion)
}

func (p wingetPackage) installerManifest(version string) string {
	var b strings.Builder
	b.WriteString(wingetHeader("installer"))
	fmt.Fprintf(&b, "PackageIdentifier: %s\nPackageVersion: %s\n", p.id, version)
	b.WriteString("InstallerLocale: en-US\nMinimumOSVersion: 10.0.0.0\n")
	b.WriteString("InstallerType: zip\nNestedInstallerType: portable\n")
	fmt.Fprintf(&b, "NestedInstallerFiles:\n- RelativeFilePath: %s\n  PortableCommandAlias: %s\n", p.executable, p.alias)
	fmt.Fprintf(&b, "Commands:\n- %s\n", p.alias)
	if p.dependsOn != "" {
		fmt.Fprintf(&b, "Dependencies:\n  PackageDependencies:\n  - PackageIdentifier: %s\n", p.dependsOn)
	}
	fmt.Fprintf(&b, "Installers:\n- Architecture: x64\n  InstallerUrl: %s\n  InstallerSha256: %s\n", p.url, p.sha)
	fmt.Fprintf(&b, "ManifestType: installer\nManifestVersion: %s\n", wingetManifestVersion)
	return b.String()
}

func (p wingetPackage) localeManifest(version, publisher, homepage string) string {
	var b strings.Builder
	b.WriteString(wingetHeader("defaultLocale"))
	fmt.Fprintf(&b, "PackageIdentifier: %s\nPackageVersion: %s\nPackageLocale: en-US\n", p.id, version)
	fmt.Fprintf(&b, "Publisher: %s\nPublisherUrl: https://github.com/%s\n", publisher, publisher)
	fmt.Fprintf(&b, "PublisherSupportUrl: %s/issues\n", homepage)
	fmt.Fprintf(&b, "PackageName: %s\nPackageUrl: %s\n", p.name, homepage)
	fmt.Fprintf(&b, "License: MIT\nLicenseUrl: %s/blob/main/LICENSE\n", homepage)
	fmt.Fprintf(&b, "ShortDescription: %s\nDescription: %s\n", p.short, p.desc)
	b.WriteString("Tags:\n")
	for _, tag := range p.tags {
		fmt.Fprintf(&b, "- %s\n", tag)
	}
	fmt.Fprintf(&b, "ReleaseNotesUrl: %s/releases/tag/%s\n", homepage, ReleaseTag(version))
	fmt.Fprintf(&b, "ManifestType: defaultLocale\nManifestVersion: %s\n", wingetManifestVersion)
	return b.String()
}
