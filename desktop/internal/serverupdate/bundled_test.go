package serverupdate

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/devlikebear/tars/desktop/internal/server"
)

const bundledBin = "/Applications/TARS.app/Contents/Helpers/tars"

const serviceLoadedStatus = "service status\nlabel: io.tars.server\ninstalled: yes\nloaded: yes\nstate: running\n"

func bundledFor(running string, runningErr error, run Run) Bundled {
	app := App{
		Version:       "0.62.0",
		ServerVersion: func(context.Context) (string, error) { return running, runningErr },
	}
	return Bundled{Tars: bundledBin, App: app, Start: server.PlanStart("darwin", bundledBin, server.Config{URL: server.DefaultURL}), Run: run}
}

func TestForPicksTheServerTheAppShips(t *testing.T) {
	cfg := server.Config{URL: server.DefaultURL}
	app := App{Version: "0.62.0"}
	// Found inside the bundle, or through the cask's bin/tars link to it:
	// Homebrew's prefix in the link's path must not make it a brew install.
	for _, tc := range []struct{ bin, resolved string }{
		{bundledBin, bundledBin},
		{"/opt/homebrew/bin/tars", bundledBin},
	} {
		src, err := For("darwin", tc.bin, tc.resolved, func() string { return brewBin }, cfg, app, nil)
		b, ok := src.(Bundled)
		if err != nil || !ok || b.Tars != tc.bin || b.App.Version != "0.62.0" {
			t.Fatalf("%s = %#v, %v", tc.bin, src, err)
		}
	}
}

func TestBundledCheck(t *testing.T) {
	for name, tc := range map[string]struct {
		running string
		err     error
		want    Result
	}{
		"older server":      {running: "0.61.0", want: Result{Current: "0.61.0", Latest: "0.62.0", Available: true}},
		"same release":      {running: "0.62.0", want: Result{Current: "0.62.0", Latest: "0.62.0"}},
		"newer server":      {running: "0.63.0", want: Result{Current: "0.63.0", Latest: "0.62.0"}},
		"no version field":  {running: "", want: Result{Latest: "0.62.0", Available: true}},
		"no server running": {err: errors.New("connection refused"), want: Result{}},
	} {
		got, err := bundledFor(tc.running, tc.err, nil).Check(context.Background())
		if err != nil || got != tc.want {
			t.Fatalf("%s: %+v, %v", name, got, err)
		}
	}
}

func TestBundledApplyRestartsTheServiceOnTheBundledServer(t *testing.T) {
	run, calls := script(map[string]reply{bundledBin + " service status": {stdout: serviceLoadedStatus}})
	got, err := bundledFor("0.61.0", nil, run).Apply(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := (Result{Current: "0.61.0", Latest: "0.62.0", Available: true, Updated: true, Restarted: true}); got != want {
		t.Fatalf("apply = %+v", got)
	}
	want := []string{
		bundledBin + " service status",
		bundledBin + " service stop",
		bundledBin + " service start --install-if-missing --adopt",
	}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("calls = %q", *calls)
	}
}

func TestBundledApplyLeavesWhatItDoesNotRun(t *testing.T) {
	// Up to date: nothing runs.
	run, calls := script(nil)
	got, err := bundledFor("0.62.0", nil, run).Apply(context.Background())
	if err != nil || got.Updated || got.Restarted || len(*calls) != 0 {
		t.Fatalf("up to date: %+v, %v, %q", got, err, *calls)
	}

	// A server started by hand is not launchd's to restart.
	run, calls = script(map[string]reply{bundledBin + " service status": {stdout: "installed: yes\nloaded: no\n"}})
	got, err = bundledFor("0.61.0", nil, run).Apply(context.Background())
	if err != nil || got.Updated || got.Restarted || got.Server != "not_running" {
		t.Fatalf("started by hand: %+v, %v", got, err)
	}
	if len(*calls) != 1 {
		t.Fatalf("restarted a server launchd does not run: %q", *calls)
	}

	run, _ = script(map[string]reply{
		bundledBin + " service status": {stdout: serviceLoadedStatus},
		bundledBin + " service start":  {stderr: "port in use", err: errors.New("exit status 1")},
	})
	if _, err = bundledFor("0.61.0", nil, run).Apply(context.Background()); err == nil || !strings.Contains(err.Error(), "port in use") {
		t.Fatalf("failed restart: %v", err)
	}
}
