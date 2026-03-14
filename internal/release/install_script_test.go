package release

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallScript_FetchesLatestReleaseAndInstallsBinary(t *testing.T) {
	t.Parallel()

	rootDir := repoRoot(t)
	installScript := filepath.Join(rootDir, "install.sh")
	for _, tc := range []struct {
		name string
		arch string
	}{
		{name: "arm64", arch: "arm64"},
		{name: "amd64", arch: "x86_64"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			binDir := filepath.Join(tempDir, "bin")
			homeDir := filepath.Join(tempDir, "home")
			installDir := filepath.Join(tempDir, "install")
			if err := os.MkdirAll(binDir, 0o755); err != nil {
				t.Fatalf("mkdir bin dir: %v", err)
			}
			if err := os.MkdirAll(homeDir, 0o755); err != nil {
				t.Fatalf("mkdir home dir: %v", err)
			}

			archivePath := filepath.Join(tempDir, "archive.tar.gz")
			if err := writeTarballWithFiles(archivePath, map[string]string{
				"tars": "#!/bin/sh\necho 'tars 0.1.0 (test123, 2026-03-08T00:00:00Z)'\n",
			}); err != nil {
				t.Fatalf("write tarball: %v", err)
			}
			curlLog := filepath.Join(tempDir, "curl.log")
			writeExecutable(t, filepath.Join(binDir, "uname"), "#!/bin/sh\nif [ \"$1\" = \"-s\" ]; then\n  printf 'Darwin\\n'\n  exit 0\nfi\nif [ \"$1\" = \"-m\" ]; then\n  printf '"+tc.arch+"\\n'\n  exit 0\nfi\nprintf 'unsupported uname args: %s\\n' \"$*\" >&2\nexit 1\n")
			writeExecutable(t, filepath.Join(binDir, "curl"), "#!/bin/sh\nfor arg in \"$@\"; do url=\"$arg\"; done\nprintf '%s\\n' \"$url\" >> \"$TEST_CURL_LOG\"\ncase \"$url\" in\n  */releases/latest)\n    printf 'https://github.com/devlikebear/tars/releases/tag/v0.1.0'\n    ;;\n  *tars_0.1.0_darwin_arm64.tar.gz|*tars_0.1.0_darwin_amd64.tar.gz)\n    cat \"$TEST_ARCHIVE_SOURCE\"\n    ;;\n  *)\n    printf 'unexpected url: %s\\n' \"$url\" >&2\n    exit 22\n    ;;\nesac\n")

			cmd := exec.Command("sh", installScript)
			cmd.Env = append(os.Environ(),
				"PATH="+binDir+":"+os.Getenv("PATH"),
				"HOME="+homeDir,
				"INSTALL_DIR="+installDir,
				"TEST_ARCHIVE_SOURCE="+archivePath,
				"TEST_CURL_LOG="+curlLog,
			)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("install.sh failed: %v\n%s", err, output)
			}

			installedBinary := filepath.Join(installDir, "tars")
			data, err := os.ReadFile(installedBinary)
			if err != nil {
				t.Fatalf("read installed binary: %v", err)
			}
			if !strings.Contains(string(data), "tars 0.1.0") {
				t.Fatalf("installed binary missing expected version output: %s", data)
			}

			logData, err := os.ReadFile(curlLog)
			if err != nil {
				t.Fatalf("read curl log: %v", err)
			}
			gotLog := string(logData)
			if !strings.Contains(gotLog, "/releases/latest") {
				t.Fatalf("expected install.sh to query latest release, log:\n%s", gotLog)
			}
			if strings.Contains(gotLog, "VERSION.txt") {
				t.Fatalf("install.sh should not fetch VERSION.txt anymore, log:\n%s", gotLog)
			}
		})
	}
}

func TestInstallScript_InstallsBundledShareAssets(t *testing.T) {
	t.Parallel()

	rootDir := repoRoot(t)
	installScript := filepath.Join(rootDir, "install.sh")
	tempDir := t.TempDir()
	binDir := filepath.Join(tempDir, "bin")
	homeDir := filepath.Join(tempDir, "home")
	installDir := filepath.Join(tempDir, "install")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin dir: %v", err)
	}
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("mkdir home dir: %v", err)
	}

	archivePath := filepath.Join(tempDir, "archive.tar.gz")
	if err := writeTarballWithFiles(archivePath, map[string]string{
		"tars": "#!/bin/sh\necho 'tars 0.1.0 (test123, 2026-03-08T00:00:00Z)'\n",
		"share/tars/plugins/project-swarm/tars.plugin.json": `{"id":"project-swarm"}`,
	}); err != nil {
		t.Fatalf("write tarball: %v", err)
	}

	writeExecutable(t, filepath.Join(binDir, "uname"), "#!/bin/sh\nif [ \"$1\" = \"-s\" ]; then\n  printf 'Darwin\\n'\n  exit 0\nfi\nif [ \"$1\" = \"-m\" ]; then\n  printf 'arm64\\n'\n  exit 0\nfi\nprintf 'unsupported uname args: %s\\n' \"$*\" >&2\nexit 1\n")
	writeExecutable(t, filepath.Join(binDir, "curl"), "#!/bin/sh\nfor arg in \"$@\"; do url=\"$arg\"; done\ncase \"$url\" in\n  */releases/latest)\n    printf 'https://github.com/devlikebear/tars/releases/tag/v0.1.0'\n    ;;\n  *tars_0.1.0_darwin_arm64.tar.gz)\n    cat \"$TEST_ARCHIVE_SOURCE\"\n    ;;\n  *)\n    printf 'unexpected url: %s\\n' \"$url\" >&2\n    exit 22\n    ;;\nesac\n")

	cmd := exec.Command("sh", installScript)
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"HOME="+homeDir,
		"INSTALL_DIR="+installDir,
		"TEST_ARCHIVE_SOURCE="+archivePath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install.sh failed: %v\n%s", err, output)
	}

	installedPlugin := filepath.Join(tempDir, "share", "tars", "plugins", "project-swarm", "tars.plugin.json")
	data, err := os.ReadFile(installedPlugin)
	if err != nil {
		t.Fatalf("read installed plugin: %v", err)
	}
	if !strings.Contains(string(data), "project-swarm") {
		t.Fatalf("installed plugin missing manifest content: %s", data)
	}
}

func TestInstallScript_ReportsMissingAssetURL(t *testing.T) {
	t.Parallel()

	rootDir := repoRoot(t)
	installScript := filepath.Join(rootDir, "install.sh")
	tempDir := t.TempDir()
	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin dir: %v", err)
	}

	writeExecutable(t, filepath.Join(binDir, "uname"), "#!/bin/sh\nif [ \"$1\" = \"-s\" ]; then\n  printf 'Darwin\\n'\n  exit 0\nfi\nif [ \"$1\" = \"-m\" ]; then\n  printf 'arm64\\n'\n  exit 0\nfi\nexit 1\n")
	writeExecutable(t, filepath.Join(binDir, "curl"), "#!/bin/sh\nfor arg in \"$@\"; do url=\"$arg\"; done\ncase \"$url\" in\n  */releases/latest)\n    printf 'https://github.com/devlikebear/tars/releases/tag/v9.9.9'\n    ;;\n  *)\n    printf 'curl 404 for %s\\n' \"$url\" >&2\n    exit 22\n    ;;\nesac\n")

	cmd := exec.Command("sh", installScript)
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"HOME="+filepath.Join(tempDir, "home"),
		"INSTALL_DIR="+filepath.Join(tempDir, "install"),
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected install.sh to fail for missing asset")
	}
	got := string(output)
	want := "https://github.com/devlikebear/tars/releases/download/v9.9.9/tars_9.9.9_darwin_arm64.tar.gz"
	if !strings.Contains(got, want) {
		t.Fatalf("expected output to mention missing asset URL %q, got:\n%s", want, got)
	}
}

func TestInstallScript_ReportsLatestReleaseLookupFailure(t *testing.T) {
	t.Parallel()

	rootDir := repoRoot(t)
	installScript := filepath.Join(rootDir, "install.sh")
	tempDir := t.TempDir()
	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin dir: %v", err)
	}

	writeExecutable(t, filepath.Join(binDir, "uname"), "#!/bin/sh\nif [ \"$1\" = \"-s\" ]; then\n  printf 'Darwin\\n'\n  exit 0\nfi\nif [ \"$1\" = \"-m\" ]; then\n  printf 'arm64\\n'\n  exit 0\nfi\nexit 1\n")
	writeExecutable(t, filepath.Join(binDir, "curl"), "#!/bin/sh\nfor arg in \"$@\"; do url=\"$arg\"; done\nprintf 'curl 404 for %s\\n' \"$url\" >&2\nexit 22\n")

	cmd := exec.Command("sh", installScript)
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"HOME="+filepath.Join(tempDir, "home"),
		"INSTALL_DIR="+filepath.Join(tempDir, "install"),
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected install.sh to fail when latest release lookup fails")
	}
	got := string(output)
	want := "https://github.com/devlikebear/tars/releases/latest"
	if !strings.Contains(got, want) {
		t.Fatalf("expected output to mention latest release URL %q, got:\n%s", want, got)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

func writeExecutable(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", path, err)
	}
}

func writeTarballWithFiles(path string, files map[string]string) error {
	var archive bytes.Buffer
	gzipWriter := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, contents := range files {
		mode := int64(0o644)
		if name == "tars" {
			mode = 0o755
		}
		if err := tarWriter.WriteHeader(&tar.Header{
			Name: name,
			Mode: mode,
			Size: int64(len(contents)),
		}); err != nil {
			return err
		}
		if _, err := tarWriter.Write([]byte(contents)); err != nil {
			return err
		}
	}
	if err := tarWriter.Close(); err != nil {
		return err
	}
	if err := gzipWriter.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, archive.Bytes(), 0o644)
}
