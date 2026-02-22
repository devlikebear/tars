package main

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/devlikebear/tarsncase/internal/tarsserver"
	"github.com/devlikebear/tarsncase/pkg/tarsclient"
)

func TestRootCommand_DoesNotAcceptWorkspaceIDFlag(t *testing.T) {
	cmd := newRootCommand(strings.NewReader(""), io.Discard, io.Discard)
	cmd.SetArgs([]string{"--workspace-id", "ws-main"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected unknown flag error for --workspace-id")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unknown flag") {
		t.Fatalf("expected unknown flag error, got %v", err)
	}
}

func TestRootCommand_ServeSubcommandInvokesRunner(t *testing.T) {
	original := serveRunner
	defer func() { serveRunner = original }()

	var got serveOptions
	serveRunner = func(_ context.Context, opts serveOptions, _ io.Writer, _ io.Writer) error {
		got = opts
		return nil
	}

	cmd := newRootCommand(strings.NewReader(""), io.Discard, io.Discard)
	cmd.SetArgs([]string{
		"serve",
		"--config", "config/standalone.yaml",
		"--workspace-dir", "./workspace",
		"--api-addr", tarsserver.DefaultAPIAddr,
		"--verbose",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("serve command: %v", err)
	}

	if !got.serveAPI {
		t.Fatalf("expected serveAPI=true, got %#v", got)
	}
	if got.configPath != "config/standalone.yaml" {
		t.Fatalf("unexpected configPath: %#v", got)
	}
	if got.workspaceDir != "./workspace" {
		t.Fatalf("unexpected workspaceDir: %#v", got)
	}
	if got.apiAddr != tarsserver.DefaultAPIAddr {
		t.Fatalf("unexpected apiAddr: %#v", got)
	}
	if got.logFile != ".logs/tars-debug.log" {
		t.Fatalf("unexpected logFile default: %#v", got)
	}
}

func TestRootCommand_ServeRunOnceDoesNotForceServeAPI(t *testing.T) {
	original := serveRunner
	defer func() { serveRunner = original }()

	var got serveOptions
	serveRunner = func(_ context.Context, opts serveOptions, _ io.Writer, _ io.Writer) error {
		got = opts
		return nil
	}

	cmd := newRootCommand(strings.NewReader(""), io.Discard, io.Discard)
	cmd.SetArgs([]string{"serve", "--run-once"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("serve command: %v", err)
	}
	if !got.runOnce {
		t.Fatalf("expected runOnce=true, got %#v", got)
	}
	if got.serveAPI {
		t.Fatalf("did not expect serveAPI=true when run-once is set, got %#v", got)
	}
}

func TestDefaultClientOptions_UsesPkgDefaultWhenEnvMissing(t *testing.T) {
	prevServerURL, hadServerURL := os.LookupEnv("TARS_SERVER_URL")
	defer func() {
		if hadServerURL {
			_ = os.Setenv("TARS_SERVER_URL", prevServerURL)
			return
		}
		_ = os.Unsetenv("TARS_SERVER_URL")
	}()
	_ = os.Unsetenv("TARS_SERVER_URL")

	opts := defaultClientOptions()
	if strings.TrimSpace(opts.serverURL) != tarsclient.DefaultServerURL {
		t.Fatalf("unexpected serverURL default: %q", opts.serverURL)
	}
}

func TestDefaultServeOptions_UsesDefaultLogFile(t *testing.T) {
	opts := defaultServeOptions()
	if opts.logFile != ".logs/tars-debug.log" {
		t.Fatalf("unexpected default log file: %q", opts.logFile)
	}
}
