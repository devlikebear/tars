package main

import (
	"fmt"
	"io"
	"os"

	_ "github.com/devlikebear/tars/internal/browserplugin" // register browser plugin
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
		Short: "Web console and automation CLI for tars",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if showVersion {
				_, err := fmt.Fprintln(stdout, buildinfo.Summary())
				return err
			}
			if clientOpts.message != "" {
				return clientCommandRunner(cmd.Context(), stdin, stdout, stderr, clientOpts)
			}
			return consoleCommandRunner(cmd.Context(), stdout, stderr, clientOpts)
		},
	}
	bindClientFlags(cmd, &clientOpts)
	cmd.Flags().BoolVar(&showVersion, "version", false, "print version and exit")
	cmd.AddCommand(newInitCommand(stdout, stderr))
	cmd.AddCommand(newDoctorCommand(stdout, stderr))
	cmd.AddCommand(newServiceCommand(stdout, stderr))
	cmd.AddCommand(newServeCommand(stdout, stderr))
	cmd.AddCommand(newStatusCommand(stdout, stderr))
	cmd.AddCommand(newHealthCommand(stdout, stderr))
	cmd.AddCommand(newCronCommand(stdout, stderr))
	cmd.AddCommand(newApproveCommand(stdout, stderr))
	cmd.AddCommand(newAssistantCommand(stdout, stderr))
	cmd.AddCommand(newSkillCommand(stdout, stderr))
	cmd.AddCommand(newPluginCommand(stdout, stderr))
	cmd.AddCommand(newMCPCommand(stdout, stderr))
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
