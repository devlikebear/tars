package focusprobe

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"time"
)

// GH sign-in states, as the phone's readiness shows them
// (docs/decisions/mobile-remote.md §2): the PR stages need gh installed and
// signed in on the desktop that runs the pipeline.
const (
	GHReady     = "ok"
	GHMissing   = "missing"
	GHSignedOut = "signed_out"
)

// ghAuthTimeout bounds `gh auth status`, which may reach the network to
// check a token.
var ghAuthTimeout = 10 * time.Second

// GHAuthState runs a read-only `gh auth status` with the same gh the PR
// probe runs (TARS_FOCUS_GH_PATH, else gh): GHReady when it exits 0,
// GHMissing when there is no gh to run, GHSignedOut otherwise. gh's own
// output never leaves this function.
func GHAuthState(ctx context.Context) string {
	return ghAuthState(ctx, focusGHBinary())
}

func ghAuthState(ctx context.Context, bin string) string {
	ctx, cancel := context.WithTimeout(ctx, ghAuthTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "auth", "status")
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "GH_NO_UPDATE_NOTIFIER=1")
	err := cmd.Run()
	switch {
	case err == nil:
		return GHReady
	case errors.Is(err, exec.ErrNotFound), errors.Is(err, os.ErrNotExist):
		return GHMissing
	default:
		return GHSignedOut
	}
}
