package serverupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/devlikebear/tars/desktop/internal/server"
)

// Homebrew installs (macOS). `tars update` refuses a Homebrew install, and a
// server from before it existed cannot be asked at all, so here the shell
// drives Homebrew itself:
//
//	check: brew update, then brew outdated --json=v2 <formula>
//	apply: brew upgrade <formula>, then restart the launchd service
//
// The restart goes through `tars service`, and only when launchd runs the
// server: a server someone started by hand keeps running the old version
// until they restart it, which the shell says.

// Formula is the Homebrew formula of the tars server.
const Formula = "devlikebear/tap/tars"

// brewEnv keeps Homebrew quiet and stops `brew upgrade` from running its
// own update first: the check just did.
var brewEnv = []string{"HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_ENV_HINTS=1", "HOMEBREW_NO_ANALYTICS=1"}

// Source checks for and installs a newer server release. Updater (`tars
// update`) and Brew are the two.
type Source interface {
	Check(ctx context.Context) (Result, error)
	Apply(ctx context.Context) (Result, error)
}

// Brew updates a Homebrew-installed tars.
type Brew struct {
	// Brew and Tars are the brew and tars executables.
	Brew string
	Tars string
	// Start is how the shell starts the server (server.PlanStart).
	Start server.StartPlan
	Run   Run
}

// ManagedByHomebrew reports whether the tars at resolved (its path with
// symlinks followed) was installed by Homebrew.
func ManagedByHomebrew(resolved string) bool {
	p := filepath.ToSlash(resolved)
	return strings.Contains(p, "/Cellar/") || strings.Contains(p, "/homebrew/") || strings.Contains(p, "/linuxbrew/")
}

// BrewDirs are where Homebrew installs brew on goos: an app opened from
// Finder gets launchd's PATH, which has neither.
func BrewDirs(goos string) []string {
	if goos == "darwin" {
		return []string{"/opt/homebrew/bin", "/usr/local/bin"}
	}
	return nil
}

// ErrNoBrew is a Homebrew-installed tars whose brew cannot be found.
var ErrNoBrew = errors.New("tars was installed with Homebrew, but brew was not found; run: brew upgrade " + Formula)

// For picks how the server at tarsBin is updated on goos. resolved is
// tarsBin with symlinks followed; findBrew returns the brew executable or
// "". A Homebrew install is updated with brew; a winget install is not
// updated by the shell at all (ErrManagedByWinget); any other install with
// `tars update` (install.ps1 on Windows, install.sh on macOS).
func For(goos, tarsBin, resolved string, findBrew func() string, cfg server.Config, run Run) (Source, error) {
	if goos == "windows" && (ManagedByWinget(resolved) || ManagedByWinget(tarsBin)) {
		return nil, ErrManagedByWinget
	}
	if goos == "darwin" && ManagedByHomebrew(resolved) {
		brew := findBrew()
		if brew == "" {
			return nil, ErrNoBrew
		}
		return Brew{Brew: brew, Tars: tarsBin, Start: server.PlanStart(goos, tarsBin, cfg), Run: run}, nil
	}
	return Updater{Bin: tarsBin, Cfg: cfg, Run: run}, nil
}

func (b Brew) brew(ctx context.Context, args ...string) ([]byte, error) {
	stdout, stderr, err := b.Run(ctx, b.Brew, args, brewEnv)
	if err != nil {
		msg := strings.TrimSpace(string(stderr))
		if msg == "" {
			msg = strings.TrimSpace(string(stdout))
		}
		if msg != "" {
			return nil, fmt.Errorf("brew %s: %w: %s", args[0], err, lastLine(msg))
		}
		return nil, fmt.Errorf("brew %s: %w", args[0], err)
	}
	return stdout, nil
}

// brewOutdated is `brew outdated --json=v2`.
type brewOutdated struct {
	Formulae []struct {
		Name              string   `json:"name"`
		InstalledVersions []string `json:"installed_versions"`
		CurrentVersion    string   `json:"current_version"`
		Pinned            bool     `json:"pinned"`
	} `json:"formulae"`
}

// Check refreshes Homebrew's taps and asks whether the formula is outdated.
// A formula the user pinned is left alone.
func (b Brew) Check(ctx context.Context) (Result, error) {
	if _, err := b.brew(ctx, "update", "--quiet"); err != nil {
		return Result{}, err
	}
	// `brew outdated` exits 1 when something is outdated and still prints
	// the JSON, so the output decides, not the exit status.
	out, stderr, runErr := b.Run(ctx, b.Brew, []string{"outdated", "--json=v2", "--formula", Formula}, brewEnv)
	var parsed brewOutdated
	if err := json.Unmarshal(jsonObject(out), &parsed); err != nil {
		if runErr != nil {
			msg := strings.TrimSpace(string(stderr))
			if msg == "" {
				msg = strings.TrimSpace(string(out))
			}
			return Result{}, fmt.Errorf("brew outdated: %w: %s", runErr, lastLine(msg))
		}
		return Result{}, fmt.Errorf("brew outdated printed %q: %w", firstLine(strings.TrimSpace(string(out))), err)
	}
	for _, f := range parsed.Formulae {
		if f.Pinned {
			return Result{}, nil
		}
		res := Result{Latest: f.CurrentVersion, Available: true}
		if n := len(f.InstalledVersions); n > 0 {
			res.Current = f.InstalledVersions[n-1]
		}
		return res, nil
	}
	return Result{}, nil
}

// Apply upgrades the formula and, when launchd runs the server, restarts it
// on the new version.
func (b Brew) Apply(ctx context.Context) (Result, error) {
	check, err := b.Check(ctx)
	if err != nil {
		return Result{}, err
	}
	if !check.Available {
		return check, nil
	}
	if _, err := b.brew(ctx, "upgrade", "--formula", Formula); err != nil {
		return Result{}, err
	}
	res := Result{Current: check.Current, Latest: check.Latest, Updated: true}
	if !b.serviceLoaded(ctx) {
		res.Server = "not_running"
		return res, nil
	}
	if err := b.restart(ctx); err != nil {
		return res, fmt.Errorf("the server was updated to %s but did not restart: %w", check.Latest, err)
	}
	res.Restarted = true
	return res, nil
}

// serviceLoaded reports whether launchd has the server's service loaded:
// only then is restarting it the shell's to do.
func (b Brew) serviceLoaded(ctx context.Context) bool {
	stdout, _, err := b.Run(ctx, b.Tars, []string{"service", "status"}, nil)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(stdout), "\n") {
		if key, value, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(key) == "loaded" {
			return strings.TrimSpace(value) == "yes"
		}
	}
	return false
}

// restart stops the service and starts it again the way the shell's Start
// server does, falling back for a tars too old for its flags.
func (b Brew) restart(ctx context.Context) error {
	tars := func(args []string) (string, error) {
		stdout, stderr, err := b.Run(ctx, b.Tars, args, nil)
		return string(stdout) + string(stderr), err
	}
	if out, err := tars([]string{"service", "stop"}); err != nil {
		return fmt.Errorf("tars service stop: %w: %s", err, lastLine(strings.TrimSpace(out)))
	}
	out, err := tars(b.Start.Args)
	if err != nil && b.Start.Fallback != nil && server.UnknownFlag(out) {
		out, err = tars(b.Start.Fallback)
	}
	if err != nil {
		return fmt.Errorf("tars service start: %w: %s", err, lastLine(strings.TrimSpace(out)))
	}
	return nil
}

// jsonObject is out from its first "{": Homebrew may print a warning line
// before the JSON.
func jsonObject(out []byte) []byte {
	if i := strings.IndexByte(string(out), '{'); i >= 0 {
		return out[i:]
	}
	return out
}

func lastLine(s string) string {
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[i+1:])
	}
	return s
}
