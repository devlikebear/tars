package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOptionsDefaults(t *testing.T) {
	var o Options
	if o.apiBase() != DefaultAPIBaseURL || o.repository() != DefaultRepository {
		t.Fatalf("defaults = %q, %q", o.apiBase(), o.repository())
	}
	if c := o.client(); c == nil || c.Timeout == 0 {
		t.Fatal("the default client must have a timeout")
	}
	o = Options{APIBaseURL: " https://ghe.example/api/ ", Repository: " me/fork "}
	if o.apiBase() != "https://ghe.example/api" || o.repository() != "me/fork" {
		t.Fatalf("overrides = %q, %q", o.apiBase(), o.repository())
	}
	if c := (Server{}).client(); c == nil || c.Timeout == 0 {
		t.Fatal("the default server client must have a timeout")
	}
}

// releaseAPI serves one fixed answer for the latest-release call.
func releaseAPI(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestLatestRejectsUnusableReleases(t *testing.T) {
	cases := map[string]struct {
		status  int
		body    string
		wantErr string
	}{
		"rate limited":  {http.StatusForbidden, `{"message":"rate limit"}`, "403"},
		"not json":      {http.StatusOK, `<html>`, "decode latest release"},
		"not a version": {http.StatusOK, `{"tag_name":"nightly"}`, `"nightly" is not a version`},
		"no checksums": {http.StatusOK, `{"tag_name":"v1.3.0","assets":[{"name":"tars_1.3.0_windows_amd64.zip","browser_download_url":"http://x/a.zip"}]}`,
			"has no checksums.txt"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := releaseAPI(t, tc.status, tc.body)
			_, err := Latest(context.Background(), windowsOpts(srv, "tars.exe"))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
		})
	}

	gone := releaseAPI(t, http.StatusOK, "")
	opts := windowsOpts(gone, "tars.exe")
	gone.Close()
	if _, err := Latest(context.Background(), opts); err == nil || !strings.Contains(err.Error(), "read latest release") {
		t.Fatalf("unreachable API: err = %v", err)
	}
}

func TestInstallRefusesBeforeDownloading(t *testing.T) {
	rel := Release{Version: "1.3.0", ArchiveName: "tars_1.3.0_windows_amd64.zip"}
	if err := Install(context.Background(), Options{GOOS: "windows"}, rel); err == nil || !strings.Contains(err.Error(), "path is unknown") {
		t.Fatalf("empty exe: err = %v", err)
	}
	brew := Options{GOOS: "darwin", ExePath: "/opt/homebrew/Cellar/tars/1.2.3/bin/tars"}
	if err := Install(context.Background(), brew, rel); !errors.Is(err, ErrHomebrew) {
		t.Fatalf("homebrew: err = %v", err)
	}
	winget := Options{GOOS: "windows", ExePath: `D:\fake\AppData\Local\Microsoft\WinGet\Packages\Devlikebear.TARS_Microsoft.Winget.Source_8wekyb3d8bbwe\tars.exe`}
	if err := Install(context.Background(), winget, rel); !errors.Is(err, ErrWinget) {
		t.Fatalf("winget: err = %v", err)
	}
}

func TestInstallReportsFailedDownloads(t *testing.T) {
	archive := zipArchive(t, []entry{{name: "tars.exe", body: "new"}})
	for _, missing := range []string{"checksums.txt", "tars_1.3.0_windows_amd64.zip"} {
		t.Run(missing, func(t *testing.T) {
			gh := fakeGitHub{tag: "v1.3.0", assets: map[string][]byte{"tars_1.3.0_windows_amd64.zip": archive}}.serve(t)
			dir := t.TempDir()
			exe := filepath.Join(dir, "tars.exe")
			writeExe(t, exe, "old")
			opts := windowsOpts(gh, exe)
			st, err := Check(context.Background(), opts)
			if err != nil {
				t.Fatal(err)
			}
			// Point the missing asset at a path the fake does not serve.
			if missing == "checksums.txt" {
				st.Release.ChecksumsURL = gh.URL + "/download/absent.txt"
			} else {
				st.Release.ArchiveURL = gh.URL + "/download/absent.zip"
			}
			err = Install(context.Background(), opts, st.Release)
			if err == nil || !strings.Contains(err.Error(), "404") {
				t.Fatalf("err = %v, want a 404", err)
			}
			if got := readFile(t, exe); got != "old" {
				t.Fatalf("tars.exe = %q after a failed download", got)
			}
			assertNoStaging(t, dir)
		})
	}
}

func TestInstallReportsAMissingExecutable(t *testing.T) {
	archive := zipArchive(t, []entry{{name: "tars.exe", body: "new"}})
	gh := fakeGitHub{tag: "v1.3.0", assets: map[string][]byte{"tars_1.3.0_windows_amd64.zip": archive}}.serve(t)
	dir := t.TempDir()
	opts := windowsOpts(gh, filepath.Join(dir, "tars.exe")) // never written
	st, err := Check(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(context.Background(), opts, st.Release); err == nil || !strings.Contains(err.Error(), "stat ") {
		t.Fatalf("err = %v", err)
	}
	assertNoStaging(t, dir)
}

func installLinuxArchive(t *testing.T, archive []byte) (string, error) {
	t.Helper()
	gh := fakeGitHub{tag: "v1.3.0", assets: map[string][]byte{"tars_1.3.0_linux_amd64.tar.gz": archive}}.serve(t)
	exe := filepath.Join(t.TempDir(), "bin", "tars")
	writeExe(t, exe, "old")
	opts := Options{CurrentVersion: "1.2.3", GOOS: "linux", GOARCH: "amd64", ExePath: exe, APIBaseURL: gh.URL, HTTPClient: gh.Client()}
	st, err := Check(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	return exe, Install(context.Background(), opts, st.Release)
}

func TestInstallRefusesBadTarballs(t *testing.T) {
	var link bytes.Buffer
	gz := gzip.NewWriter(&link)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "share/tars/evil", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}); err != nil {
		t.Fatal(err)
	}
	_ = tw.Close()
	_ = gz.Close()

	cases := map[string]struct {
		archive []byte
		wantErr string
	}{
		"link entry":     {link.Bytes(), "is not a regular file"},
		"escaping entry": {tarGzArchive(t, []entry{{name: "../../evil", body: "x"}}), "outside the archive"},
		"not gzip":       {[]byte("plainly not a tarball"), "open tars_1.3.0_linux_amd64.tar.gz"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			exe, err := installLinuxArchive(t, tc.archive)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
			if got := readFile(t, exe); got != "old" {
				t.Fatalf("tars = %q after a refused archive", got)
			}
		})
	}

	// A truncated gzip stream fails while reading entries, not at open.
	whole := tarGzArchive(t, []entry{{name: "tars", body: strings.Repeat("x", 4096)}})
	if _, err := installLinuxArchive(t, whole[:len(whole)/2]); err == nil {
		t.Fatal("a truncated tarball was installed")
	}
}

// A name that survives cleaning but still holds ".." is refused where the
// path is used, in both archive formats.
func TestInstallRefusesDotDotInsideAName(t *testing.T) {
	if _, err := installLinuxArchive(t, tarGzArchive(t, []entry{{name: "share/tars/a..b", body: "x"}})); err == nil || !strings.Contains(err.Error(), "outside the archive") {
		t.Fatalf("tar.gz: err = %v", err)
	}

	archive := zipArchive(t, []entry{{name: "share/tars/a..b", body: "x"}, {name: "tars.exe", body: "new"}})
	gh := fakeGitHub{tag: "v1.3.0", assets: map[string][]byte{"tars_1.3.0_windows_amd64.zip": archive}}.serve(t)
	exe := filepath.Join(t.TempDir(), "tars.exe")
	writeExe(t, exe, "old")
	opts := windowsOpts(gh, exe)
	st, err := Check(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(context.Background(), opts, st.Release); err == nil || !strings.Contains(err.Error(), "outside the archive") {
		t.Fatalf("zip: err = %v", err)
	}
	if got := readFile(t, exe); got != "old" {
		t.Fatalf("tars.exe = %q after a refused archive", got)
	}
}

func TestInstallRefusesACorruptZip(t *testing.T) {
	gh := fakeGitHub{tag: "v1.3.0", assets: map[string][]byte{"tars_1.3.0_windows_amd64.zip": []byte("not a zip")}}.serve(t)
	exe := filepath.Join(t.TempDir(), "tars.exe")
	writeExe(t, exe, "old")
	opts := windowsOpts(gh, exe)
	st, err := Check(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(context.Background(), opts, st.Release); err == nil || !strings.Contains(err.Error(), "open tars_1.3.0_windows_amd64.zip") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(exe + ".old"); !os.IsNotExist(err) {
		t.Fatalf("a refused archive moved the executable aside: %v", err)
	}
}

func TestRestartFailures(t *testing.T) {
	// The server answers the restart with an error.
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"resolve executable failed"}`))
	}))
	defer failing.Close()
	err := Server{URL: failing.URL, HTTPClient: failing.Client()}.Restart(context.Background(), "1.3.0")
	if err == nil || !strings.Contains(err.Error(), "resolve executable failed") {
		t.Fatalf("500: err = %v", err)
	}

	// Nothing listens.
	closed := httptest.NewServer(http.NotFoundHandler())
	url := closed.URL
	closed.Close()
	if err := (Server{URL: url, HTTPClient: &http.Client{Timeout: time.Second}}).Restart(context.Background(), "1.3.0"); err == nil || !strings.Contains(err.Error(), "restart the server") {
		t.Fatalf("no server: err = %v", err)
	}

	// The server accepts the restart and never answers /v1/healthz again.
	silent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/admin/restart" {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer silent.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	err = Server{URL: silent.URL, HTTPClient: silent.Client(), PollEvery: time.Millisecond}.Restart(ctx, "1.3.0")
	if err == nil || !strings.Contains(err.Error(), "did not come back") {
		t.Fatalf("silent server: err = %v", err)
	}
}
