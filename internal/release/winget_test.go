package release

import (
	"strings"
	"testing"
)

const (
	testServerSHA  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testDesktopSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func wingetFileMap(t *testing.T) map[string]string {
	t.Helper()
	files, err := WingetManifests("devlikebear/tars", "1.2.3", testServerSHA, testDesktopSHA)
	if err != nil {
		t.Fatalf("WingetManifests: %v", err)
	}
	out := map[string]string{}
	for _, f := range files {
		if _, dup := out[f.Path]; dup {
			t.Fatalf("duplicate manifest path %s", f.Path)
		}
		out[f.Path] = f.Content
	}
	return out
}

func TestWingetManifestsLayout(t *testing.T) {
	files := wingetFileMap(t)
	want := []string{
		"d/devlikebear/TARS/1.2.3/devlikebear.TARS.yaml",
		"d/devlikebear/TARS/1.2.3/devlikebear.TARS.installer.yaml",
		"d/devlikebear/TARS/1.2.3/devlikebear.TARS.locale.en-US.yaml",
		"d/devlikebear/TARS/Desktop/1.2.3/devlikebear.TARS.Desktop.yaml",
		"d/devlikebear/TARS/Desktop/1.2.3/devlikebear.TARS.Desktop.installer.yaml",
		"d/devlikebear/TARS/Desktop/1.2.3/devlikebear.TARS.Desktop.locale.en-US.yaml",
	}
	if len(files) != len(want) {
		t.Fatalf("got %d files, want %d: %v", len(files), len(want), files)
	}
	for _, p := range want {
		if _, ok := files[p]; !ok {
			t.Errorf("missing %s", p)
		}
	}
}

func TestWingetServerInstallerKeepsShareNextToExecutable(t *testing.T) {
	got := wingetFileMap(t)["d/devlikebear/TARS/1.2.3/devlikebear.TARS.installer.yaml"]
	for _, want := range []string{
		"PackageIdentifier: devlikebear.TARS\n",
		"PackageVersion: 1.2.3\n",
		"InstallerType: zip\n",
		"NestedInstallerType: portable\n",
		"- RelativeFilePath: tars.exe\n  PortableCommandAlias: tars\n",
		"InstallerUrl: https://github.com/devlikebear/tars/releases/download/v1.2.3/tars_1.2.3_windows_amd64.zip\n",
		"InstallerSha256: " + strings.ToUpper(testServerSHA) + "\n",
		"ManifestType: installer\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("server installer manifest missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Dependencies") {
		t.Errorf("the server depends on nothing:\n%s", got)
	}
}

func TestWingetDesktopInstallerDependsOnServer(t *testing.T) {
	got := wingetFileMap(t)["d/devlikebear/TARS/Desktop/1.2.3/devlikebear.TARS.Desktop.installer.yaml"]
	for _, want := range []string{
		"PackageIdentifier: devlikebear.TARS.Desktop\n",
		"- RelativeFilePath: tars-desktop.exe\n  PortableCommandAlias: tars-desktop\n",
		"PackageDependencies:\n  - PackageIdentifier: devlikebear.TARS\n",
		"InstallerUrl: https://github.com/devlikebear/tars/releases/download/v1.2.3/tars-desktop_1.2.3_windows_amd64.zip\n",
		"InstallerSha256: " + strings.ToUpper(testDesktopSHA) + "\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("desktop installer manifest missing %q:\n%s", want, got)
		}
	}
}

func TestWingetLocaleAndVersionManifests(t *testing.T) {
	files := wingetFileMap(t)
	version := files["d/devlikebear/TARS/1.2.3/devlikebear.TARS.yaml"]
	for _, want := range []string{"DefaultLocale: en-US\n", "ManifestType: version\n", "ManifestVersion: " + wingetManifestVersion + "\n"} {
		if !strings.Contains(version, want) {
			t.Errorf("version manifest missing %q:\n%s", want, version)
		}
	}
	locale := files["d/devlikebear/TARS/Desktop/1.2.3/devlikebear.TARS.Desktop.locale.en-US.yaml"]
	for _, want := range []string{
		"PackageName: TARS Desktop\n",
		"Publisher: devlikebear\n",
		"License: MIT\n",
		"ReleaseNotesUrl: https://github.com/devlikebear/tars/releases/tag/v1.2.3\n",
		"ManifestType: defaultLocale\n",
	} {
		if !strings.Contains(locale, want) {
			t.Errorf("desktop locale manifest missing %q:\n%s", want, locale)
		}
	}
}

func TestWingetManifestsRejectBadInput(t *testing.T) {
	cases := []struct {
		name                     string
		repo, version, srv, desk string
	}{
		{"bad version", "devlikebear/tars", "not-a-version", testServerSHA, testDesktopSHA},
		{"empty repo", " ", "1.2.3", testServerSHA, testDesktopSHA},
		{"empty server sha", "devlikebear/tars", "1.2.3", "", testDesktopSHA},
		{"short desktop sha", "devlikebear/tars", "1.2.3", testServerSHA, "abc123"},
		{"non-hex sha", "devlikebear/tars", "1.2.3", strings.Repeat("z", 64), testDesktopSHA},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := WingetManifests(tc.repo, tc.version, tc.srv, tc.desk); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestWingetManifestsAcceptLowercaseChecksums(t *testing.T) {
	// checksums.txt carries lowercase hex; winget-pkgs wants it upper-cased.
	files, err := WingetManifests("devlikebear/tars", "1.2.3", testServerSHA, testDesktopSHA)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.Contains(f.Content, "InstallerSha256: "+testServerSHA+"\n") {
			t.Fatalf("%s kept a lowercase checksum", f.Path)
		}
	}
}
