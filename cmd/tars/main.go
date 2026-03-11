package main

import (
	"fmt"
	"io"
	"os"

	"github.com/devlikebear/tars/internal/buildinfo"
	"github.com/devlikebear/tars/internal/envloader"
	"github.com/spf13/cobra"
)

func main() {
	bootstrapEnv()
	exitCode := 0
	runOnMainThread(func() {
		if err := newRootCommand(os.Stdin, os.Stdout, os.Stderr).Execute(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			exitCode = 1
		}
	})
	os.Exit(exitCode)
}

func bootstrapEnv() {
	envloader.Load(".env", ".env.secret")
}

func newRootCommand(stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	clientOpts := defaultClientOptions()
	showVersion := false
	cmd := &cobra.Command{
		Use:   "tars",
		Short: "Go TUI client for tars",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if showVersion {
				_, err := fmt.Fprintln(stdout, buildinfo.Summary())
				return err
			}
			return runClientCommand(cmd.Context(), stdin, stdout, stderr, clientOpts)
		},
	}
	bindClientFlags(cmd, &clientOpts)
	cmd.Flags().BoolVar(&showVersion, "version", false, "print version and exit")
	cmd.AddCommand(newInitCommand(stdout, stderr))
	cmd.AddCommand(newDoctorCommand(stdout, stderr))
	cmd.AddCommand(newServiceCommand(stdout, stderr))
	cmd.AddCommand(newServeCommand(stdout, stderr))
	cmd.AddCommand(newAssistantCommand(stdout, stderr))
	cmd.AddCommand(newVersionCommand(stdout))
	return cmd
}

func newVersionCommand(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build version",
		RunE: func(_ *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(stdout, buildinfo.Summary())
			return err
		},
	}
}
