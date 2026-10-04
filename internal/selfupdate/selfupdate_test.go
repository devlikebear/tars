package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type entry struct {
	name string
	body string
	dir  bool
}

func zipArchive(t *testing.T, entries []entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		name := e.name
		if e.dir {
			name += "/"
		}
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if !e.dir {
			if _, err := w.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func tarGzArchive(t *testing.T, entries []entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Mode: 0o755, Size: int64(len(e.body)), Typeflag: tar.TypeReg}
		if e.dir {
			hdr = &tar.Header{Name: e.name + "/", Mode: 0o755, Typeflag: tar.TypeDir}
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if !e.dir {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fakeGitHub serves a latest release with the given assets. A nil checksum
// override lists the real sha256 of every asset.
type fakeGitHub struct {
	tag       string
	assets    map[string][]byte
	checksums *string
}

func (f fakeGitHub) serve(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/devlikebear/tars/releases/latest":
			type asset struct {
				Name string `json:"name"`
				URL  string `json:"browser_download_url"`
			}
			out := struct {
				TagName string  `json:"tag_name"`
				Assets  []asset `json:"assets"`
			}{TagName: f.tag}
			for name := range f.assets {
				out.Assets = append(out.Assets, asset{Name: name, URL: srv.URL + "/download/" + name})
			}
			out.Assets = append(out.Assets, asset{Name: "checksums.txt", URL: srv.URL + "/download/checksums.txt"})
			_ = json.NewEncoder(w).Encode(out)
		case r.URL.Path == "/download/checksums.txt":
			if f.checksums != nil {
				_, _ = w.Write([]byte(*f.checksums))
				return
			}
			for name, body := range f.assets {
				sum := sha256.Sum256(body)
				_, _ = fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), name)
			}
		case strings.HasPrefix(r.URL.Path, "/download/"):
			body, ok := f.assets[strings.TrimPrefix(r.URL.Path, "/download/")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func writeExe(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func windowsOpts(srv *httptest.Server, exe string) Options {
	return Options{CurrentVersion: "1.2.3", GOOS: "windows", GOARCH: "amd64", ExePath: exe, APIBaseURL: srv.URL, HTTPClient: srv.Client()}
}

func TestCheckReportsANewerRelease(t *testing.T) {
	srv := fakeGitHub{tag: "v1.3.0", assets: map[string][]byte{
		"tars_1.3.0_windows_amd64.zip":   []byte("zip"),
		"tars_1.3.0_darwin_arm64.tar.gz": []byte("tgz"),
		// The desktop archive shares the platform suffix; it must not be picked.
		"tars-desktop_1.3.0_windows_amd64.zip": []byte("desktop"),
	}}.serve(t)

	st, err := Check(context.Background(), windowsOpts(srv, `C:\TARS\tars.exe`))
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !st.Available || st.Current != "1.2.3" || st.Latest != "1.3.0" {
		t.Fatalf("status = %+v", st)
	}
	if st.Release.ArchiveName != "tars_1.3.0_windows_amd64.zip" || !strings.HasSuffix(st.Release.ArchiveURL, "/tars_1.3.0_windows_amd64.zip") {
		t.Fatalf("picked %+v", st.Release)
	}

	opts := windowsOpts(srv, `C:\TARS\tars.exe`)
	opts.CurrentVersion = "v1.3.0"
	if st, err := Check(context.Background(), opts); err != nil || st.Available {
		t.Fatalf("same version: status=%+v err=%v", st, err)
	}
}

func TestCheckRefusesWhatItCannotUpdate(t *testing.T) {
	srv := fakeGitHub{tag: "v1.3.0", assets: map[string][]byte{"tars_1.3.0_darwin_arm64.tar.gz": []byte("tgz")}}.serve(t)

	for _, dev := range []string{"", "dev", "0.0.0", "0.0.0-dev"} {
		opts := windowsOpts(srv, `C:\TARS\tars.exe`)
		opts.CurrentVersion = dev
		if _, err := Check(context.Background(), opts); !errors.Is(err, ErrDevelopmentBuild) {
			t.Fatalf("version %q: err = %v, want ErrDevelopmentBuild", dev, err)
		}
	}

	brew := windowsOpts(srv, "/opt/homebrew/Cellar/tars/1.2.3/bin/tars")
	brew.GOOS, brew.GOARCH = "darwin", "arm64"
	if _, err := Check(context.Background(), brew); !errors.Is(err, ErrHomebrew) {
		t.Fatalf("homebrew: err = %v", err)
	}

	// Until a release carries the Windows archive, Check says so plainly.
	if _, err := Check(context.Background(), windowsOpts(srv, `C:\TARS\tars.exe`)); !errors.Is(err, ErrNoAsset) {
		t.Fatalf("missing asset: err = %v", err)
	}
}

func TestInstallReplacesWindowsExecutableAndShare(t *testing.T) {
	archive := zipArchive(t, []entry{
		{name: "tars.exe", body: "new"},
		{name: "share", dir: true},
		{name: "share/tars", dir: true},
		{name: "share/tars/skills/demo/SKILL.md", body: "skill v2"},
		{name: "README.txt", body: "ignored"},
	})
	srv := fakeGitHub{tag: "v1.3.0", assets: map[string][]byte{"tars_1.3.0_windows_amd64.zip": archive}}.serve(t)

	dir := t.TempDir()
	exe := filepath.Join(dir, "tars.exe")
	writeExe(t, exe, "old")
	writeExe(t, exe+".old", "older") // left by the previous update
	writeExe(t, filepath.Join(dir, "share", "tars", "skills", "gone", "SKILL.md"), "stale")

	opts := windowsOpts(srv, exe)
	st, err := Check(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(context.Background(), opts, st.Release); err != nil {
		t.Fatalf("install: %v", err)
	}

	if got := readFile(t, exe); got != "new" {
		t.Fatalf("tars.exe = %q, want the new binary", got)
	}
	// Windows keeps the replaced binary until the next update: a running
	// server may still have it open.
	if got := readFile(t, exe+".old"); got != "old" {
		t.Fatalf("tars.exe.old = %q, want the replaced binary", got)
	}
	if got := readFile(t, filepath.Join(dir, "share", "tars", "skills", "demo", "SKILL.md")); got != "skill v2" {
		t.Fatalf("share not installed: %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "share", "tars", "skills", "gone")); !os.IsNotExist(err) {
		t.Fatalf("stale share entry survived: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "README.txt")); !os.IsNotExist(err) {
		t.Fatalf("an entry outside tars.exe and share/ was installed")
	}
	assertNoStaging(t, dir)
}

func TestInstallReplacesUnixBinaryAndPrefixShare(t *testing.T) {
	archive := tarGzArchive(t, []entry{
		{name: "tars", body: "new"},
		{name: "share/tars", dir: true},
		{name: "share/tars/plugins/p.json", body: "{}"},
	})
	srv := fakeGitHub{tag: "v1.3.0", assets: map[string][]byte{"tars_1.3.0_linux_amd64.tar.gz": archive}}.serve(t)

	prefix := t.TempDir()
	exe := filepath.Join(prefix, "bin", "tars")
	writeExe(t, exe, "old")

	opts := Options{CurrentVersion: "1.2.3", GOOS: "linux", GOARCH: "amd64", ExePath: exe, APIBaseURL: srv.URL, HTTPClient: srv.Client()}
	st, err := Check(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(context.Background(), opts, st.Release); err != nil {
		t.Fatalf("install: %v", err)
	}
	if got := readFile(t, exe); got != "new" {
		t.Fatalf("tars = %q", got)
	}
	if _, err := os.Stat(exe + ".old"); !os.IsNotExist(err) {
		t.Fatalf("unix install should drop tars.old: %v", err)
	}
	// install.sh puts share/tars in the prefix above bin/.
	if got := readFile(t, filepath.Join(prefix, "share", "tars", "plugins", "p.json")); got != "{}" {
		t.Fatalf("prefix share = %q", got)
	}
}

func TestInstallLeavesTheInstallAloneOnBadArchives(t *testing.T) {
	cases := map[string]struct {
		archive   []byte
		checksums *string
		wantErr   string
	}{
		"checksum mismatch": {
			archive:   zipArchive(t, []entry{{name: "tars.exe", body: "evil"}}),
			checksums: ptr(strings.Repeat("0", 64) + "  tars_1.3.0_windows_amd64.zip\n"),
			wantErr:   "checksum mismatch",
		},
		"not in checksums": {
			archive:   zipArchive(t, []entry{{name: "tars.exe", body: "new"}}),
			checksums: ptr(""),
			wantErr:   "no entry for tars_1.3.0_windows_amd64.zip",
		},
		"escaping entry": {
			archive: zipArchive(t, []entry{{name: "tars.exe", body: "new"}, {name: "../evil.txt", body: "x"}}),
			wantErr: "outside the archive",
		},
		"no executable": {
			archive: zipArchive(t, []entry{{name: "share/tars/x", body: "x"}}),
			wantErr: "has no tars.exe",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := fakeGitHub{tag: "v1.3.0", assets: map[string][]byte{"tars_1.3.0_windows_amd64.zip": tc.archive}, checksums: tc.checksums}.serve(t)
			dir := t.TempDir()
			exe := filepath.Join(dir, "tars.exe")
			writeExe(t, exe, "old")

			opts := windowsOpts(srv, exe)
			st, err := Check(context.Background(), opts)
			if err != nil {
				t.Fatal(err)
			}
			err = Install(context.Background(), opts, st.Release)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
			if got := readFile(t, exe); got != "old" {
				t.Fatalf("tars.exe = %q after a failed install", got)
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "evil.txt")); !os.IsNotExist(err) {
				t.Fatalf("escaping entry was written")
			}
			assertNoStaging(t, dir)
		})
	}
}

func assertNoStaging(t *testing.T, dir string) {
	t.Helper()
	matches, _ := filepath.Glob(filepath.Join(dir, ".tars-update-*"))
	if len(matches) != 0 {
		t.Fatalf("staging left behind: %v", matches)
	}
}

func ptr(s string) *string { return &s }

func TestVersionHelpers(t *testing.T) {
	for _, tc := range []struct {
		candidate, current string
		want               bool
	}{
		{"1.3.0", "1.2.9", true},
		{"v2.0.0", "1.99.99", true},
		{"1.2.3", "1.2.3", false},
		{"1.2.3", "1.2.4", false},
		{"1.2.4", "1.2.3-dev", true},
		{"garbage", "1.2.3", false},
	} {
		if got := Newer(tc.candidate, tc.current); got != tc.want {
			t.Errorf("Newer(%q, %q) = %v", tc.candidate, tc.current, got)
		}
	}
	if !Updatable("0.43.4") || Updatable("dev") || Updatable("0.0.0") {
		t.Fatal("Updatable misjudged a version")
	}
	for p, want := range map[string]bool{
		"/opt/homebrew/bin/tars":                true,
		"/usr/local/Cellar/tars/1.0.0/bin/tars": true,
		"/home/linuxbrew/.linuxbrew/bin/tars":   true,
		`D:\Programs\TARS\tars.exe`:             false,
		"/srv/tars/bin/tars":                    false,
	} {
		if got := ManagedByHomebrew(p); got != want {
			t.Errorf("ManagedByHomebrew(%q) = %v", p, got)
		}
	}
}
