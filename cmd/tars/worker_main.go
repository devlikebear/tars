package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/workerprotocol"
	"github.com/spf13/cobra"
)

type workerServeOptions struct {
	stdio      bool
	protocol   string
	configPath string
}

var workerServeRunner = realWorkerServeRunner

func newWorkerCommand(stdin io.Reader, stdout io.Writer) *cobra.Command {
	workerCommand := &cobra.Command{
		Use:   "worker",
		Short: "Run an isolated remote worker",
	}
	opts := workerServeOptions{
		protocol:   workerprotocol.ProtocolVersionV1,
		configPath: filepath.Join(config.TarsHomeDir(), "worker.json"),
	}
	serveCommand := &cobra.Command{
		Use:   "serve",
		Short: "Serve one versioned worker request over stdio",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if !opts.stdio {
				return fmt.Errorf("worker serve requires --stdio")
			}
			if strings.TrimSpace(opts.protocol) != workerprotocol.ProtocolVersionV1 {
				return fmt.Errorf("unsupported worker protocol %q", opts.protocol)
			}
			return workerServeRunner(command.Context(), opts, stdin, stdout)
		},
	}
	serveCommand.Flags().BoolVar(&opts.stdio, "stdio", false, "read one JSONL request from stdin and write one response")
	serveCommand.Flags().StringVar(&opts.protocol, "protocol", opts.protocol, "worker protocol version")
	serveCommand.Flags().StringVar(&opts.configPath, "config", opts.configPath, "absolute worker configuration path")
	workerCommand.AddCommand(serveCommand)
	return workerCommand
}

func realWorkerServeRunner(ctx context.Context, opts workerServeOptions, stdin io.Reader, stdout io.Writer) error {
	service, limits, err := workerprotocol.OpenConfiguredWorkerService(strings.TrimSpace(opts.configPath), nil)
	if err != nil {
		return err
	}
	return workerprotocol.ServeJSONL(ctx, stdin, stdout, service, limits)
}
