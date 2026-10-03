// Package server finds the TARS server the desktop shell attaches to, and
// starts it when it is not running.
//
// The shell never runs the server in its own process. The server keeps
// running cron, pulse, and reflection with the window closed or the shell
// quit, so it is either the launchd service (`tars service`, macOS) or a
// detached `tars serve` the shell started once.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// DefaultURL is where `tars serve` listens unless configured otherwise; it
// matches the CLI's default (pkg/tarsclient.DefaultServerURL).
const DefaultURL = "http://127.0.0.1:43180"

// Config says where the server is and how to authenticate.
type Config struct {
	// URL is the server's base URL, e.g. http://127.0.0.1:43180.
	URL string
	// Token is a user-tier API token, sent when the server requires one.
	Token string
	// AdminToken is sent on admin paths (creating a session in a folder).
	AdminToken string
}

// FileConfig is the shell's config file. An app started from Finder, the
// Start menu, or a desktop launcher gets no shell environment, so this is
// where its tokens live.
type FileConfig struct {
	ServerURL     string `json:"server_url"`
	APIToken      string `json:"api_token"`
	AdminAPIToken string `json:"admin_api_token"`
}

// DefaultConfigPath is <user config dir>/tars-desktop/config.json.
func DefaultConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tars-desktop", "config.json"), nil
}

// LoadFile reads the config file. A missing file is an empty config. On
// Unix a file others can read is refused, since it holds tokens.
func LoadFile(path string) (FileConfig, error) {
	var fc FileConfig
	if path == "" {
		return fc, nil
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return fc, nil
	}
	if err != nil {
		return fc, err
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return fc, fmt.Errorf("%s holds API tokens and must be readable only by you (chmod 600 %s)", path, path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fc, err
	}
	if err := json.Unmarshal(raw, &fc); err != nil {
		return fc, fmt.Errorf("%s: %w", path, err)
	}
	return fc, nil
}

// Resolve picks each setting from the flag, then the environment (the same
// variables the CLI reads: TARS_SERVER_URL, TARS_API_TOKEN,
// TARS_ADMIN_API_TOKEN), then the config file, then the default.
func Resolve(flagURL string, getenv func(string) string, file FileConfig) (Config, error) {
	first := func(values ...string) string {
		for _, v := range values {
			if v = strings.TrimSpace(v); v != "" {
				return v
			}
		}
		return ""
	}
	base, err := NormalizeURL(first(flagURL, getenv("TARS_SERVER_URL"), file.ServerURL, DefaultURL))
	if err != nil {
		return Config{}, err
	}
	return Config{
		URL:        base,
		Token:      first(getenv("TARS_API_TOKEN"), file.APIToken),
		AdminToken: first(getenv("TARS_ADMIN_API_TOKEN"), file.AdminAPIToken),
	}, nil
}

// NormalizeURL checks that raw is an http(s) URL on a loopback host and
// drops any path. The shell loads this origin's console in its webview and
// sends it tokens, so it refuses anything that is not this machine: a
// remote server is reached with a browser, not with the shell.
func NormalizeURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("server url %q: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("server url %q: scheme must be http or https", raw)
	}
	host := u.Hostname()
	if host == "" {
		return "", fmt.Errorf("server url %q: missing host", raw)
	}
	if !isLoopbackHost(host) {
		return "", fmt.Errorf("server url %q: the desktop shell attaches only to a server on this machine (127.0.0.1, ::1, or localhost)", raw)
	}
	return u.Scheme + "://" + u.Host, nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// ConsoleURL is the console page the window shows for path, which may carry
// a query ("/console/chat/abc"). Any path outside /console falls back to the
// console root.
func (c Config) ConsoleURL(path string) string {
	path = strings.TrimSpace(path)
	if path != "/console" && !strings.HasPrefix(path, "/console/") && !strings.HasPrefix(path, "/console?") {
		path = "/console"
	}
	return c.URL + path
}

// Health is what GET /v1/healthz reports.
type Health struct {
	OK         bool   `json:"ok"`
	Component  string `json:"component"`
	NeedsSetup bool   `json:"needs_setup"`
	// Version is the server's release, reported to loopback callers since
	// 0.42.2; "" from an older server.
	Version string `json:"version"`
}

// Outdated reports that the server is an older release than the app. A
// server that reports no version predates the field, so it is older too.
// Development builds on either side are never compared.
func Outdated(serverVersion, appVersion string) bool {
	app, ok := parseRelease(appVersion)
	if !ok {
		return false
	}
	if strings.TrimSpace(serverVersion) == "" {
		return true
	}
	srv, ok := parseRelease(serverVersion)
	if !ok {
		return false
	}
	for i := range app {
		if srv[i] != app[i] {
			return srv[i] < app[i]
		}
	}
	return false
}

// OutdatedMessage tells the user how to bring the server up to the app on
// goos: on Windows the shell updates it itself (see serverupdate), elsewhere
// Homebrew does.
func OutdatedMessage(goos, serverVersion, appVersion string) string {
	running := "an older release"
	if v := strings.TrimSpace(serverVersion); v != "" {
		running = v
	}
	if goos == "windows" {
		return fmt.Sprintf("The TARS server is %s, older than this app (%s), so some of the app may not work.\n\n"+
			"The app updates the server to the latest release once no chat is running. "+
			"To update it now, choose Check for updates… in the tray menu, or run:\n\n"+
			"  tars update", running, appVersion)
	}
	return fmt.Sprintf("The TARS server is %s, older than this app (%s), so some of the app may not work.\n\n"+
		"Installing or updating the app does not update the server. With Homebrew:\n\n"+
		"  brew upgrade devlikebear/tap/tars\n  tars service install && tars service start", running, appVersion)
}

// parseRelease reads "1.2.3" (with an optional "v" and pre-release or build
// suffix) into its three numbers.
func parseRelease(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+ "); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// ErrNotTARS reports that something answered on the port but it is not a
// TARS server.
var ErrNotTARS = errors.New("the server at this address is not TARS")

// Probe asks the server for its health. It returns an error when nothing
// answers, and ErrNotTARS when something else does.
func Probe(ctx context.Context, client *http.Client, cfg Config) (Health, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.URL+"/v1/healthz", nil)
	if err != nil {
		return Health{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return Health{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	var h Health
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&h) != nil || h.Component != "tars" {
		return Health{}, ErrNotTARS
	}
	return h, nil
}

// WaitReady probes until the server answers or ctx ends.
func WaitReady(ctx context.Context, client *http.Client, cfg Config, every time.Duration) (Health, error) {
	for {
		h, err := Probe(ctx, client, cfg)
		if err == nil || errors.Is(err, ErrNotTARS) {
			return h, err
		}
		select {
		case <-ctx.Done():
			return Health{}, ctx.Err()
		case <-time.After(every):
		}
	}
}

// StartPlan is how the shell starts a server that is not running.
type StartPlan struct {
	// Binary is the tars executable.
	Binary string
	// Args are its arguments.
	Args []string
	// Detached reports that the shell spawns and forgets the process;
	// otherwise the command returns once launchd has taken over.
	Detached bool
	// Fallback are the arguments to retry with when the tars found is too
	// old for Args (see UnknownFlag); nil when there is nothing to retry.
	Fallback []string
}

// PlanStart picks how to start the server on goos: the launchd service on
// macOS, a detached `tars serve` elsewhere. apiAddr is host:port from cfg.
//
// On macOS the service is installed first when it is missing, as on a
// machine where `tars init` never ran (a fresh Homebrew install); a tars
// from before --install-if-missing gets the plain start as a fallback.
func PlanStart(goos, tarsBinary string, cfg Config) StartPlan {
	var addr []string
	if u, err := url.Parse(cfg.URL); err == nil && u.Host != "" && cfg.URL != DefaultURL {
		addr = []string{"--api-addr", u.Host}
	}
	if goos == "darwin" {
		return StartPlan{
			Binary:   tarsBinary,
			Args:     append([]string{"service", "start", "--install-if-missing"}, addr...),
			Fallback: []string{"service", "start"},
		}
	}
	return StartPlan{Binary: tarsBinary, Args: append([]string{"serve"}, addr...), Detached: true}
}

// UnknownFlag reports that a tars command failed because the binary is too
// old to know one of its flags.
func UnknownFlag(output string) bool {
	return strings.Contains(output, "unknown flag")
}

// FindTARS locates the tars executable: next to the shell first (a release
// archive ships both), then on PATH, then in installDirs. An app opened from
// Finder or the Dock gets launchd's minimal PATH, which misses Homebrew's and
// install.sh's bin directories, so those are tried explicitly last.
func FindTARS(shellExe string, lookPath func(string) (string, error), installDirs []string) (string, error) {
	// Compare file identity, not spelling: on macOS, TARS and tars can
	// name the same executable. Stat also follows symlink aliases.
	shellInfo, _ := os.Stat(shellExe)
	isShell := func(path string) bool {
		info, err := os.Stat(path)
		return err == nil && shellInfo != nil && os.SameFile(shellInfo, info)
	}
	name := "tars"
	if runtime.GOOS == "windows" {
		name = "tars.exe"
	}
	if shellExe != "" {
		dir := filepath.Dir(shellExe)
		// Inside a macOS bundle the shell lives in TARS.app/Contents/MacOS.
		for _, candidate := range []string{filepath.Join(dir, name), filepath.Join(dir, "..", "..", "..", name)} {
			if isFile(candidate) && !isShell(candidate) {
				return filepath.Clean(candidate), nil
			}
		}
	}
	path, err := lookPath(name)
	if err == nil && !isShell(path) {
		return path, nil
	}
	for _, dir := range installDirs {
		if candidate := filepath.Join(dir, name); isFile(candidate) && !isShell(candidate) {
			return candidate, nil
		}
	}
	if err == nil {
		err = errors.New("PATH resolves to the desktop app itself")
	}
	return "", fmt.Errorf("tars executable not found next to the desktop app, on PATH or in %s: %w", strings.Join(installDirs, ", "), err)
}

// InstallDirs are where tars lands when installed by Homebrew, install.sh,
// or install.ps1 on goos; home is the user's home directory ("" skips the
// per-user folders).
func InstallDirs(goos, home string) []string {
	var dirs []string
	switch goos {
	case "darwin":
		dirs = []string{"/opt/homebrew/bin", "/usr/local/bin"}
	case "linux":
		dirs = []string{"/home/linuxbrew/.linuxbrew/bin", "/usr/local/bin"}
	case "windows":
		// install.ps1's default, %LOCALAPPDATA%\Programs\TARS. A shell it
		// installed finds tars.exe next to itself; this covers a shell
		// unpacked elsewhere before the new PATH reaches it.
		if home == "" {
			return nil
		}
		return []string{filepath.Join(home, "AppData", "Local", "Programs", "TARS")}
	default:
		return nil
	}
	if home != "" {
		local := filepath.Join(home, ".local", "bin")
		if goos == "linux" {
			return append([]string{local}, dirs...)
		}
		dirs = append(dirs, local)
	}
	return dirs
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
