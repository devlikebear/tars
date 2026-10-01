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
}

// PlanStart picks how to start the server on goos: the launchd service on
// macOS, a detached `tars serve` elsewhere. apiAddr is host:port from cfg.
func PlanStart(goos, tarsBinary string, cfg Config) StartPlan {
	if goos == "darwin" {
		return StartPlan{Binary: tarsBinary, Args: []string{"service", "start"}}
	}
	args := []string{"serve"}
	if u, err := url.Parse(cfg.URL); err == nil && u.Host != "" && cfg.URL != DefaultURL {
		args = append(args, "--api-addr", u.Host)
	}
	return StartPlan{Binary: tarsBinary, Args: args, Detached: true}
}

// FindTARS locates the tars executable: next to the shell first (a release
// archive ships both), then on PATH, then in installDirs. An app opened from
// Finder or the Dock gets launchd's minimal PATH, which misses Homebrew's and
// install.sh's bin directories, so those are tried explicitly last.
func FindTARS(shellExe string, lookPath func(string) (string, error), installDirs []string) (string, error) {
	name := "tars"
	if runtime.GOOS == "windows" {
		name = "tars.exe"
	}
	if shellExe != "" {
		dir := filepath.Dir(shellExe)
		// Inside a macOS bundle the shell lives in TARS.app/Contents/MacOS.
		for _, candidate := range []string{filepath.Join(dir, name), filepath.Join(dir, "..", "..", "..", name)} {
			if isFile(candidate) {
				return filepath.Clean(candidate), nil
			}
		}
	}
	path, err := lookPath(name)
	if err == nil {
		return path, nil
	}
	for _, dir := range installDirs {
		if candidate := filepath.Join(dir, name); isFile(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("tars executable not found next to the desktop app, on PATH or in %s: %w", strings.Join(installDirs, ", "), err)
}

// InstallDirs are where tars lands when installed by Homebrew or install.sh
// on goos; home is the user's home directory ("" skips ~/.local/bin).
func InstallDirs(goos, home string) []string {
	var dirs []string
	switch goos {
	case "darwin":
		dirs = []string{"/opt/homebrew/bin", "/usr/local/bin"}
	case "linux":
		dirs = []string{"/home/linuxbrew/.linuxbrew/bin", "/usr/local/bin"}
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
