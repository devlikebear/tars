package serverupdate

import (
	"context"
	"fmt"

	"github.com/devlikebear/tars/desktop/internal/server"
)

// App is the shell asking for a server update: its release, and how to ask
// the running server for its own.
type App struct {
	// Version is the shell's release, which is also the release of the
	// server its bundle ships.
	Version string
	// ServerVersion is the running server's release; an error when no TARS
	// server answers.
	ServerVersion func(ctx context.Context) (string, error)
}

// Bundled is the server TARS.app ships (macOS, server.Bundled). Updating the
// app replaces that binary, so there is nothing to download here: a server
// still running an older release is restarted on the bundled one.
//
// As with Brew, the restart goes through `tars service` and only when
// launchd runs the server; one started by hand keeps running the old
// release until it is restarted.
type Bundled struct {
	Tars string
	App  App
	// Start is how the shell starts the server (server.PlanStart).
	Start server.StartPlan
	Run   Run
}

// Check compares the running server with the one the app ships. No server
// running is nothing to do: the next start runs the bundled one.
func (b Bundled) Check(ctx context.Context) (Result, error) {
	running, err := b.App.ServerVersion(ctx)
	if err != nil {
		return Result{}, nil
	}
	return Result{
		Current:   running,
		Latest:    b.App.Version,
		Available: server.Outdated(running, b.App.Version),
	}, nil
}

// Apply restarts the launchd service on the bundled server. Updated stays
// false when launchd does not run the server: nothing changed.
func (b Bundled) Apply(ctx context.Context) (Result, error) {
	res, err := b.Check(ctx)
	if err != nil || !res.Available {
		return res, err
	}
	if !serviceLoaded(ctx, b.Run, b.Tars) {
		res.Server = "not_running"
		return res, nil
	}
	if err := restartService(ctx, b.Run, b.Tars, b.Start); err != nil {
		return res, fmt.Errorf("the server did not restart on %s: %w", res.Latest, err)
	}
	res.Updated, res.Restarted = true, true
	return res, nil
}
