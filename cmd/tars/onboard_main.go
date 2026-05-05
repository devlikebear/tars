package main

import (
	"io"

	"github.com/spf13/cobra"
)

// newOnboardCommand returns a thin alias for `tars init`. It delegates
// to the same flag set and runner so behavior cannot drift; the
// command exists so users typing "tars onboard" land in the same
// orchestrator as "tars init".
func newOnboardCommand(stdout, stderr io.Writer) *cobra.Command {
	cmd := newInitCommand(stdout, stderr)
	cmd.Use = "onboard"
	cmd.Aliases = []string{"init"}
	cmd.Short = "Alias for `tars init` — run the onboarding orchestrator"
	cmd.Long = "Alias for `tars init`. Picks a free port, writes a skeleton " +
		"config, ensures the workspace, starts the server, waits for " +
		"healthz, and opens the setup wizard in your browser. See " +
		"`tars init --help` for full flag documentation."
	return cmd
}
