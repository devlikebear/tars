package serverupdate

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/devlikebear/tars/desktop/internal/server"
)

const (
	brewBin = "/opt/homebrew/bin/brew"
	tarsBin = "/opt/homebrew/bin/tars"
)

const outdatedJSON = `{"formulae":[{"name":"devlikebear/tap/tars","installed_versions":["0.49.1"],"current_version":"0.52.0","pinned":false}],"casks":[]}`

type reply struct {
	stdout, stderr string
	err            error
}

// script answers each command by its "bin arg arg" prefix; an unknown
// command succeeds with no output. It records every call.
func script(replies map[string]reply) (Run, *[]string) {
	calls := &[]string{}
	return func(_ context.Context, bin string, args, env []string) ([]byte, []byte, error) {
		line := bin + " " + strings.Join(args, " ")
		*calls = append(*calls, line)
		if bin == brewBin && !reflect.DeepEqual(env, brewEnv) {
			return nil, nil, errors.New("brew ran without its quiet environment")
		}
		for prefix, r := range replies {
			if strings.HasPrefix(line, prefix) {
				return []byte(r.stdout), []byte(r.stderr), r.err
			}
		}
		return nil, nil, nil
	}, calls
}

func brewFor(run Run) Brew {
	return Brew{Brew: brewBin, Tars: tarsBin, Start: server.PlanStart("darwin", tarsBin, server.Config{URL: server.DefaultURL}), Run: run}
}

func TestForPicksHomebrewOnlyForAHomebrewInstallOnMacOS(t *testing.T) {
	cfg := server.Config{URL: server.DefaultURL}
	found := func() string { return brewBin }
	none := func() string { return "" }

	src, err := For("darwin", tarsBin, "/opt/homebrew/Cellar/tars/0.49.1/bin/tars", found, cfg, nil)
	b, ok := src.(Brew)
	if err != nil || !ok || b.Brew != brewBin || b.Tars != tarsBin || b.Start.Args[0] != "service" {
		t.Fatalf("homebrew install = %#v, %v", src, err)
	}
	if _, err := For("darwin", "/usr/local/bin/tars", "/usr/local/Cellar/tars/0.49.1/bin/tars", none, cfg, nil); !errors.Is(err, ErrNoBrew) {
		t.Fatalf("homebrew install without brew: %v", err)
	}
	// install.sh on macOS and install.ps1 on Windows update with `tars update`.
	for _, tc := range []struct{ goos, bin string }{
		{"darwin", "/opt/tars/bin/tars"},
		{"windows", `C:\TARS\tars.exe`},
		{"windows", `C:\homebrew\tars.exe`},
	} {
		src, err := For(tc.goos, tc.bin, tc.bin, none, cfg, nil)
		if u, ok := src.(Updater); err != nil || !ok || u.Bin != tc.bin {
			t.Fatalf("%s %s = %#v, %v", tc.goos, tc.bin, src, err)
		}
	}
	if !ManagedByHomebrew("/opt/linuxbrew/Cellar/tars/1/bin/tars") || ManagedByHomebrew("/opt/tars/bin/tars") {
		t.Fatal("ManagedByHomebrew")
	}
	if got := BrewDirs("darwin"); len(got) != 2 || got[0] != "/opt/homebrew/bin" {
		t.Fatalf("BrewDirs(darwin) = %v", got)
	}
	if got := BrewDirs("windows"); got != nil {
		t.Fatalf("BrewDirs(windows) = %v", got)
	}
}

func TestBrewCheck(t *testing.T) {
	// brew outdated exits 1 when the formula is outdated, JSON and all.
	run, calls := script(map[string]reply{brewBin + " outdated": {stdout: "Warning: something\n" + outdatedJSON, err: errors.New("exit status 1")}})
	got, err := brewFor(run).Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := (Result{Current: "0.49.1", Latest: "0.52.0", Available: true}); got != want {
		t.Fatalf("check = %+v", got)
	}
	want := []string{
		brewBin + " update --quiet",
		brewBin + " outdated --json=v2 --formula " + Formula,
	}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("calls = %q", *calls)
	}

	for name, stdout := range map[string]string{
		"up to date": `{"formulae":[],"casks":[]}`,
		"pinned":     strings.Replace(outdatedJSON, `"pinned":false`, `"pinned":true`, 1),
	} {
		run, _ := script(map[string]reply{brewBin + " outdated": {stdout: stdout}})
		got, err := brewFor(run).Check(context.Background())
		if err != nil || got.Available {
			t.Fatalf("%s: %+v, %v", name, got, err)
		}
	}
}

func TestBrewCheckErrors(t *testing.T) {
	run, calls := script(map[string]reply{brewBin + " update": {stderr: "fatal: no network\nError: Fetching failed", err: errors.New("exit status 1")}})
	_, err := brewFor(run).Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "brew update") || !strings.Contains(err.Error(), "Fetching failed") {
		t.Fatalf("err = %v", err)
	}
	if len(*calls) != 1 {
		t.Fatalf("a failed update still asked for outdated: %q", *calls)
	}

	run, _ = script(map[string]reply{brewBin + " outdated": {stderr: "Error: No available formula", err: errors.New("exit status 1")}})
	if _, err := brewFor(run).Check(context.Background()); err == nil || !strings.Contains(err.Error(), "brew outdated") {
		t.Fatalf("err = %v", err)
	}
	run, _ = script(map[string]reply{brewBin + " outdated": {stdout: "not json"}})
	if _, err := brewFor(run).Check(context.Background()); err == nil || !strings.Contains(err.Error(), "not json") {
		t.Fatalf("err = %v", err)
	}
}

func TestBrewApplyUpgradesAndRestartsTheService(t *testing.T) {
	run, calls := script(map[string]reply{
		brewBin + " outdated":       {stdout: outdatedJSON},
		tarsBin + " service status": {stdout: "service status\nlabel: io.tars.server\ninstalled: yes\nloaded: yes\nstate: running\n"},
	})
	got, err := brewFor(run).Apply(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := (Result{Current: "0.49.1", Latest: "0.52.0", Updated: true, Restarted: true}); got != want {
		t.Fatalf("apply = %+v", got)
	}
	want := []string{
		brewBin + " update --quiet",
		brewBin + " outdated --json=v2 --formula " + Formula,
		brewBin + " upgrade --formula " + Formula,
		tarsBin + " service status",
		tarsBin + " service stop",
		tarsBin + " service start --install-if-missing",
	}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("calls = %q", *calls)
	}
}

func TestBrewApplyLeavesAServerLaunchdDoesNotRun(t *testing.T) {
	for name, status := range map[string]reply{
		"not loaded":    {stdout: "installed: yes\nloaded: no\n"},
		"status failed": {err: errors.New("exit status 1")},
		"no such line":  {stdout: "installed: no\n"},
	} {
		run, calls := script(map[string]reply{brewBin + " outdated": {stdout: outdatedJSON}, tarsBin + " service status": status})
		got, err := brewFor(run).Apply(context.Background())
		if err != nil || !got.Updated || got.Restarted || got.Server != "not_running" {
			t.Fatalf("%s: %+v, %v", name, got, err)
		}
		if last := (*calls)[len(*calls)-1]; last != tarsBin+" service status" {
			t.Fatalf("%s: restarted a server launchd does not run: %q", name, *calls)
		}
	}
}

func TestBrewApplyDoesNothingWhenUpToDate(t *testing.T) {
	run, calls := script(map[string]reply{brewBin + " outdated": {stdout: `{"formulae":[]}`}})
	got, err := brewFor(run).Apply(context.Background())
	if err != nil || got.Updated || len(*calls) != 2 {
		t.Fatalf("apply = %+v, %v, calls %q", got, err, *calls)
	}
}

func TestBrewApplyErrors(t *testing.T) {
	loaded := reply{stdout: "loaded: yes\n"}
	outdated := reply{stdout: outdatedJSON}

	run, calls := script(map[string]reply{brewBin + " outdated": outdated, brewBin + " upgrade": {stdout: "Error: download failed", err: errors.New("exit status 1")}})
	if _, err := brewFor(run).Apply(context.Background()); err == nil || !strings.Contains(err.Error(), "download failed") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(strings.Join(*calls, "\n"), "service") {
		t.Fatalf("a failed upgrade touched the service: %q", *calls)
	}

	run, _ = script(map[string]reply{brewBin + " outdated": outdated, tarsBin + " service status": loaded, tarsBin + " service stop": {stderr: "Error: boom", err: errors.New("exit status 1")}})
	got, err := brewFor(run).Apply(context.Background())
	if err == nil || !strings.Contains(err.Error(), "did not restart") || !strings.Contains(err.Error(), "boom") || !got.Updated || got.Restarted {
		t.Fatalf("stop failure: %+v, %v", got, err)
	}

	// A tars too old for --install-if-missing gets the plain start.
	run, calls = script(map[string]reply{
		brewBin + " outdated": outdated, tarsBin + " service status": loaded,
		tarsBin + " service start --install-if-missing": {stderr: "Error: unknown flag: --install-if-missing", err: errors.New("exit status 1")},
	})
	got, err = brewFor(run).Apply(context.Background())
	if err != nil || !got.Restarted || (*calls)[len(*calls)-1] != tarsBin+" service start" {
		t.Fatalf("fallback start: %+v, %v, calls %q", got, err, *calls)
	}

	run, _ = script(map[string]reply{
		brewBin + " outdated": outdated, tarsBin + " service status": loaded,
		tarsBin + " service start": {stdout: "Error: port in use", err: errors.New("exit status 1")},
	})
	if _, err = brewFor(run).Apply(context.Background()); err == nil || !strings.Contains(err.Error(), "port in use") {
		t.Fatalf("start failure: %v", err)
	}
}
