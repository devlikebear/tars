package main

import (
	"fmt"
	"io"
	"os"

	"github.com/devlikebear/tarsncase/internal/envloader"
	"github.com/spf13/cobra"
)

func main() {
	bootstrapEnv()
	if err := newRootCommand(os.Stdin, os.Stdout, os.Stderr).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func bootstrapEnv() {
	envloader.Load(".env", ".env.secret")
}

func newRootCommand(stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	clientOpts := defaultClientOptions()
	cmd := &cobra.Command{
		Use:   "tars",
		Short: "Go TUI client for tars",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runClientCommand(cmd.Context(), stdin, stdout, stderr, clientOpts)
		},
	}
	bindClientFlags(cmd, &clientOpts)
	cmd.AddCommand(newServeCommand(stdout, stderr))
	cmd.AddCommand(newAssistantCommand(stdout, stderr))
	return cmd
}
