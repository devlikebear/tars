package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestResolveFromEnv(t *testing.T) {
	cfg, err := Resolve("", env(nil), FileConfig{})
	if err != nil || cfg.URL != DefaultURL {
		t.Fatalf("default = %+v, %v", cfg, err)
	}
	cfg, err = Resolve("", env(map[string]string{
		"TARS_SERVER_URL":      " http://localhost:9000/console ",
		"TARS_API_TOKEN":       " user ",
		"TARS_ADMIN_API_TOKEN": "admin",
	}), FileConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.URL != "http://localhost:9000" || cfg.Token != "user" || cfg.AdminToken != "admin" {
		t.Fatalf("env config = %+v", cfg)
	}
	cfg, err = Resolve("http://[::1]:8000", env(map[string]string{"TARS_SERVER_URL": "http://127.0.0.1:1"}), FileConfig{})
	if err != nil || cfg.URL != "http://[::1]:8000" {
		t.Fatalf("flag wins: %+v, %v", cfg, err)
	}
	if _, err := Resolve("https://tars.example.com", env(nil), FileConfig{}); err == nil {
		t.Fatal("a remote host must be refused")
	}
}

func TestNormalizeURLRejects(t *testing.T) {
	for _, raw := range []string{"ftp://127.0.0.1", "http://", "127.0.0.1:43180", "http://10.0.0.2:43180", "http://%zz"} {
		if _, err := NormalizeURL(raw); err == nil {
			t.Errorf("NormalizeURL(%q) accepted", raw)
		}
	}
}

func TestConsoleURL(t *testing.T) {
	cfg := Config{URL: "http://127.0.0.1:43180"}
	cases := map[string]string{
		"":                  "http://127.0.0.1:43180/console",
		"/console/chat/abc": "http://127.0.0.1:43180/console/chat/abc",
		"/console?x=1":      "http://127.0.0.1:43180/console?x=1",
		"/v1/admin/secrets": "http://127.0.0.1:43180/console",
		"//evil.example":    "http://127.0.0.1:43180/console",
		"/consolexyz":       "http://127.0.0.1:43180/console",
	}
	for in, want := range cases {
		if got := cfg.ConsoleURL(in); got != want {
			t.Errorf("ConsoleURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestProbe(t *testing.T) {
	tars := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/healthz" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"component":"tars","needs_setup":true}`))
	}))
	defer tars.Close()
	h, err := Probe(context.Background(), tars.Client(), Config{URL: tars.URL})
	if err != nil || !h.OK || !h.NeedsSetup {
		t.Fatalf("probe = %+v, %v", h, err)
	}

	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer other.Close()
	if _, err := Probe(context.Background(), other.Client(), Config{URL: other.URL}); !errors.Is(err, ErrNotTARS) {
		t.Fatalf("other server: %v", err)
	}

	other.Close()
	if _, err := Probe(context.Background(), http.DefaultClient, Config{URL: other.URL}); err == nil || errors.Is(err, ErrNotTARS) {
		t.Fatalf("nothing listening: %v", err)
	}
}

func TestWaitReady(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"component":"tars"}`))
	}))
	defer srv.Close()
	// A non-TARS answer ends the wait at once; so the first two answers
	// must be connection errors, not 503s. Use a closed server for that.
	closed := httptest.NewServer(http.NotFoundHandler())
	closedURL := closed.URL
	closed.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := WaitReady(ctx, http.DefaultClient, Config{URL: closedURL}, 5*time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait on nothing: %v", err)
	}
	if _, err := WaitReady(context.Background(), srv.Client(), Config{URL: srv.URL}, time.Millisecond); !errors.Is(err, ErrNotTARS) {
		t.Fatalf("a 503 is not TARS answering: %v", err)
	}
	calls.Store(2)
	if h, err := WaitReady(context.Background(), srv.Client(), Config{URL: srv.URL}, time.Millisecond); err != nil || !h.OK {
		t.Fatalf("ready = %+v, %v", h, err)
	}
}

func TestPlanStart(t *testing.T) {
	// A machine where `tars init` never ran has no LaunchAgent: the shell
	// asks tars to install it (and a starter config) first. A tars from
	// before the flag gets the plain start as the fallback.
	want := StartPlan{
		Binary:   "/opt/tars",
		Args:     []string{"service", "start", "--install-if-missing"},
		Fallback: []string{"service", "start"},
	}
	if got := PlanStart("darwin", "/opt/tars", Config{URL: DefaultURL}); !reflect.DeepEqual(got, want) {
		t.Fatalf("darwin = %+v", got)
	}
	got := PlanStart("darwin", "/opt/tars", Config{URL: "http://127.0.0.1:43185"})
	if !reflect.DeepEqual(got.Args, []string{"service", "start", "--install-if-missing", "--api-addr", "127.0.0.1:43185"}) {
		t.Fatalf("darwin custom port = %+v", got)
	}
	if got := PlanStart("linux", "tars", Config{URL: DefaultURL}); !reflect.DeepEqual(got, StartPlan{Binary: "tars", Args: []string{"serve"}, Detached: true}) {
		t.Fatalf("linux default = %+v", got)
	}
	got = PlanStart("windows", "tars.exe", Config{URL: "http://127.0.0.1:9000"})
	if !reflect.DeepEqual(got.Args, []string{"serve", "--api-addr", "127.0.0.1:9000"}) || !got.Detached {
		t.Fatalf("windows custom port = %+v", got)
	}
}

func TestFindTARS(t *testing.T) {
	name := "tars"
	if runtime.GOOS == "windows" {
		name = "tars.exe"
	}
	noPath := func(string) (string, error) { return "", errors.New("not found") }

	dir := t.TempDir()
	shell := filepath.Join(dir, "tars-desktop")
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := FindTARS(shell, noPath, nil); err != nil || got != filepath.Join(dir, name) {
		t.Fatalf("next to the shell = %q, %v", got, err)
	}

	bundle := t.TempDir()
	macOS := filepath.Join(bundle, "TARS.app", "Contents", "MacOS")
	if err := os.MkdirAll(macOS, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, name), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := FindTARS(filepath.Join(macOS, "TARS"), noPath, nil); err != nil || got != filepath.Join(bundle, name) {
		t.Fatalf("beside the app bundle = %q, %v", got, err)
	}

	onPath := func(string) (string, error) { return "/usr/local/bin/tars", nil }
	if got, err := FindTARS(filepath.Join(t.TempDir(), "shell"), onPath, nil); err != nil || got != "/usr/local/bin/tars" {
		t.Fatalf("on PATH = %q, %v", got, err)
	}
	if _, err := FindTARS("", noPath, nil); err == nil {
		t.Fatal("missing tars must be an error")
	}

	// An app opened from Finder gets launchd's PATH (/usr/bin:/bin:...), which
	// misses Homebrew's bin, so the well-known install places come last.
	brewBin := t.TempDir()
	if err := os.WriteFile(filepath.Join(brewBin, name), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "nope")
	if got, err := FindTARS("", noPath, []string{missing, brewBin}); err != nil || got != filepath.Join(brewBin, name) {
		t.Fatalf("well-known dir = %q, %v", got, err)
	}
	if got, err := FindTARS("", onPath, []string{brewBin}); err != nil || got != "/usr/local/bin/tars" {
		t.Fatalf("PATH must win over well-known dirs = %q, %v", got, err)
	}
}

func TestInstallDirs(t *testing.T) {
	darwin := InstallDirs("darwin", "/Users/me")
	for _, want := range []string{"/opt/homebrew/bin", "/usr/local/bin", filepath.Join("/Users/me", ".local", "bin")} {
		found := false
		for _, d := range darwin {
			if d == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("darwin install dirs %v miss %q", darwin, want)
		}
	}
	if darwin[0] != "/opt/homebrew/bin" {
		t.Fatalf("Apple silicon Homebrew must come first: %v", darwin)
	}
	linux := InstallDirs("linux", "/home/me")
	if len(linux) == 0 || linux[0] != filepath.Join("/home/me", ".local", "bin") {
		t.Fatalf("linux install dirs = %v", linux)
	}
	if got := InstallDirs("darwin", ""); len(got) != 2 {
		t.Fatalf("no home must skip ~/.local/bin: %v", got)
	}
}

func TestResolvePrecedence(t *testing.T) {
	file := FileConfig{ServerURL: "http://127.0.0.1:1111", APIToken: "file-user", AdminAPIToken: "file-admin"}
	cfg, err := Resolve("", env(nil), file)
	if err != nil || cfg.URL != "http://127.0.0.1:1111" || cfg.Token != "file-user" || cfg.AdminToken != "file-admin" {
		t.Fatalf("file only = %+v, %v", cfg, err)
	}
	cfg, err = Resolve("", env(map[string]string{"TARS_SERVER_URL": "http://127.0.0.1:2222", "TARS_API_TOKEN": "env-user"}), file)
	if err != nil || cfg.URL != "http://127.0.0.1:2222" || cfg.Token != "env-user" || cfg.AdminToken != "file-admin" {
		t.Fatalf("env over file = %+v, %v", cfg, err)
	}
	cfg, err = Resolve("http://localhost:3333", env(map[string]string{"TARS_SERVER_URL": "http://127.0.0.1:2222"}), file)
	if err != nil || cfg.URL != "http://localhost:3333" {
		t.Fatalf("flag over env = %+v, %v", cfg, err)
	}
	if _, err := Resolve("", env(nil), FileConfig{ServerURL: "http://192.168.0.2:43180"}); err == nil {
		t.Fatal("a remote server in the file must be refused")
	}
}

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if fc, err := LoadFile(path); err != nil || fc != (FileConfig{}) {
		t.Fatalf("missing file = %+v, %v", fc, err)
	}
	if fc, err := LoadFile(""); err != nil || fc != (FileConfig{}) {
		t.Fatalf("no path = %+v, %v", fc, err)
	}
	if err := os.WriteFile(path, []byte(`{"server_url":"http://127.0.0.1:9","api_token":"u","admin_api_token":"a"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	fc, err := LoadFile(path)
	if err != nil || fc.APIToken != "u" || fc.AdminAPIToken != "a" || fc.ServerURL != "http://127.0.0.1:9" {
		t.Fatalf("file = %+v, %v", fc, err)
	}
	if err := os.WriteFile(path, []byte(`{`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path); err == nil {
		t.Fatal("bad JSON must fail")
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadFile(path); err == nil {
			t.Fatal("a file others can read must be refused")
		}
	}
	if _, err := DefaultConfigPath(); err != nil {
		t.Skipf("no user config dir: %v", err)
	}
}

func TestUnknownFlag(t *testing.T) {
	if !UnknownFlag("Error: unknown flag: --install-if-missing\nUsage:") {
		t.Fatal("cobra's unknown flag error must be recognised")
	}
	if UnknownFlag("launchctl bootstrap failed: 5: Input/output error") {
		t.Fatal("other failures are not an old tars")
	}
}

func TestOutdated(t *testing.T) {
	for _, c := range []struct {
		server, app string
		want        bool
	}{
		{"0.37.1", "0.42.2", true},
		{"0.42.1", "0.42.2", true},
		{"0.42.2", "0.42.2", false},
		{"0.43.0", "0.42.2", false},
		{"1.0.0", "0.42.2", false},
		{"v0.41.0", "v0.42.2", true},
		// A server that does not report its version predates the field.
		{"", "0.42.2", true},
		// Development builds on either side are not compared.
		{"dev", "0.42.2", false},
		{"0.37.1", "dev", false},
		{"0.37.1", "", false},
		{"garbage", "0.42.2", false},
	} {
		if got := Outdated(c.server, c.app); got != c.want {
			t.Errorf("Outdated(%q, %q) = %v, want %v", c.server, c.app, got, c.want)
		}
	}
}

func TestOutdatedMessage(t *testing.T) {
	msg := OutdatedMessage("0.37.1", "0.42.2")
	for _, want := range []string{"0.37.1", "0.42.2", "brew upgrade devlikebear/tap/tars", "tars service install"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message %q misses %q", msg, want)
		}
	}
	if !strings.Contains(OutdatedMessage("", "0.42.2"), "older") {
		t.Fatal("an unknown server version still reads as older")
	}
}
