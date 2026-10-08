package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/buildinfo"
	"github.com/devlikebear/tars/internal/selfupdate"
	"github.com/spf13/cobra"
)

type updateOptions struct {
	check      bool
	yes        bool
	noRestart  bool
	jsonOutput bool
	serverURL  string
	adminToken string
}

// Swapped in tests: the real ones reach GitHub and the running server.
var (
	updateCheck   = selfupdate.Check
	updateInstall = selfupdate.Install
	updateServer  = func(url, token string) updateServerAPI {
		return selfupdate.Server{URL: url, Token: token}
	}
	updateExePath  = currentExecutable
	updateConfirm  = confirmOnTerminal
	updateRestartT = 60 * time.Second
)

type updateServerAPI interface {
	Version(ctx context.Context) (string, error)
	Restart(ctx context.Context, want string) error
}

func newUpdateCommand(stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	client := defaultClientOptions()
	opts := updateOptions{serverURL: client.serverURL, adminToken: client.adminToken}
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update tars to the latest release and restart the running server",
		Long: "Replace this tars with the latest GitHub release, verified against the release's checksums.txt, " +
			"then restart a server running on --server-url so it runs the new version.\n\n" +
			"For installs made with install.ps1 or install.sh. Homebrew installs update with `brew upgrade`, winget installs with `winget upgrade devlikebear.TARS`.",
		Args: cobra.NoArgs,
		// A failed download or check is not a usage mistake.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runUpdate(cmd.Context(), stdin, stdout, stderr, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.check, "check", false, "only report whether an update is available")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false, "update without asking")
	cmd.Flags().BoolVar(&opts.noRestart, "no-restart", false, "leave a running server on the old version")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "print the result as JSON")
	cmd.Flags().StringVar(&opts.serverURL, "server-url", opts.serverURL, "server to restart after updating")
	cmd.Flags().StringVar(&opts.adminToken, "admin-api-token", opts.adminToken, "admin api token for the restart")
	return cmd
}

// updateResult is --json's output.
type updateResult struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"available"`
	Updated   bool   `json:"updated"`
	Restarted bool   `json:"restarted"`
	// Server is what happened to the running server: "restarted",
	// "not_running", "skipped", or the restart error.
	Server string `json:"server,omitempty"`
}

func runUpdate(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, opts updateOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}
	exe, err := updateExePath()
	if err != nil {
		return fmt.Errorf("find the tars executable: %w", err)
	}
	status, err := updateCheck(ctx, selfupdate.Options{
		CurrentVersion: buildinfo.Version,
		GOOS:           runtime.GOOS,
		GOARCH:         runtime.GOARCH,
		ExePath:        exe,
	})
	if err != nil {
		return err
	}
	result := updateResult{Current: status.Current, Latest: status.Latest, Available: status.Available}
	report := func() error {
		if opts.jsonOutput {
			return json.NewEncoder(stdout).Encode(result)
		}
		return nil
	}
	if !status.Available {
		if !opts.jsonOutput {
			fprintf(stdout, "tars %s is the latest release.\n", status.Current)
		}
		return report()
	}
	if opts.check {
		if !opts.jsonOutput {
			fprintf(stdout, "tars %s is available (this is %s). Run `tars update` to install it.\n", status.Latest, status.Current)
		}
		return report()
	}
	if !opts.yes {
		ok, err := updateConfirm(stdin, stdout, fmt.Sprintf("Update tars %s to %s?", status.Current, status.Latest))
		if err != nil {
			return err
		}
		if !ok {
			fprintln(stderr, "Update cancelled.")
			return nil
		}
	}
	if err := updateInstall(ctx, selfupdate.Options{
		CurrentVersion: buildinfo.Version,
		GOOS:           runtime.GOOS,
		GOARCH:         runtime.GOARCH,
		ExePath:        exe,
	}, status.Release); err != nil {
		return err
	}
	result.Updated = true
	if !opts.jsonOutput {
		fprintf(stdout, "Installed tars %s to %s.\n", status.Latest, exe)
	}

	result.Server = restartAfterUpdate(ctx, stdout, opts, status.Latest)
	result.Restarted = result.Server == "restarted"
	return report()
}

// restartAfterUpdate restarts a server running on opts.serverURL and says
// what happened. A failed restart does not fail the update: the binary is
// already replaced, and the next start runs it.
func restartAfterUpdate(ctx context.Context, stdout io.Writer, opts updateOptions, latest string) string {
	if opts.noRestart {
		return "skipped"
	}
	srv := updateServer(opts.serverURL, opts.adminToken)
	running, err := srv.Version(ctx)
	if err != nil {
		return "not_running"
	}
	if !opts.jsonOutput {
		fprintf(stdout, "Restarting the server at %s (%s)...\n", opts.serverURL, running)
	}
	restartCtx, cancel := context.WithTimeout(ctx, updateRestartT)
	defer cancel()
	if err := srv.Restart(restartCtx, latest); err != nil {
		if !opts.jsonOutput {
			fprintf(stdout, "Could not restart the server: %v\nRestart it yourself to run %s.\n", err, latest)
		}
		return err.Error()
	}
	if !opts.jsonOutput {
		fprintf(stdout, "The server is running tars %s.\n", latest)
	}
	return "restarted"
}

func currentExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}

var errNotInteractive = errors.New("cannot ask for confirmation without a terminal; re-run with --yes")

func confirmOnTerminal(stdin io.Reader, stdout io.Writer, question string) (bool, error) {
	if !stdinIsTerminal() {
		return false, errNotInteractive
	}
	fprintf(stdout, "%s [y/N] ", question)
	resp, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	resp = strings.ToLower(strings.TrimSpace(resp))
	return resp == "y" || resp == "yes", nil
}
