package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/devlikebear/tarsncase/internal/cli"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	_ = godotenv.Load(".env")
	logger := zerolog.New(stderr).With().Timestamp().Str("component", "tars").Logger()

	cmd := newRootCmd(stdout, stderr)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		var ex *cli.ExitError
		if errors.As(err, &ex) {
			return ex.Code
		}
		logger.Error().Err(err).Msg("failed to parse command")
		if cli.IsFlagError(err) {
			return 2
		}
		return 1
	}
	return 0
}

func newRootCmd(stdout, stderr io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:           "tars",
		Short:         "CLI client for TARS",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.AddCommand(newHeartbeatCmd(stdout))

	return root
}

func newHeartbeatCmd(stdout io.Writer) *cobra.Command {
	heartbeatCmd := &cobra.Command{
		Use:   "heartbeat",
		Short: "Heartbeat commands",
	}

	var serverURL string
	runOnceCmd := &cobra.Command{
		Use:   "run-once",
		Short: "Request one heartbeat run from tarsd",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runHeartbeatOnce(serverURL, stdout)
		},
	}
	runOnceCmd.Flags().StringVar(&serverURL, "server-url", "http://127.0.0.1:8080", "tarsd API server URL")

	heartbeatCmd.AddCommand(runOnceCmd)
	return heartbeatCmd
}

func runHeartbeatOnce(serverURL string, stdout io.Writer) error {
	base := strings.TrimRight(serverURL, "/")
	client := &http.Client{Timeout: 60 * time.Second}

	req, err := http.NewRequest(http.MethodPost, base+"/v1/heartbeat/run-once", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request run-once endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read run-once response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("run-once endpoint status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("decode run-once response: %w", err)
	}
	_, _ = fmt.Fprintln(stdout, parsed.Response)
	return nil
}
