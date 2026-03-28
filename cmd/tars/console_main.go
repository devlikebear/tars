package main

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"

	protocol "github.com/devlikebear/tars/pkg/tarsclient"
)

var (
	clientCommandRunner  = runClientCommand
	consoleCommandRunner = runConsoleCommand
	consoleURLOpener     = openConsoleURL
)

func runConsoleCommand(ctx context.Context, stdout, stderr io.Writer, opts clientOptions) error {
	target := buildConsoleURL(opts.serverURL)
	if err := consoleURLOpener(ctx, target); err != nil {
		if _, writeErr := fmt.Fprintf(stderr, "browser open failed: %v\n", err); writeErr != nil {
			return writeErr
		}
	}
	_, err := fmt.Fprintf(stdout, "Open the console: %s\n", target)
	return err
}

func buildConsoleURL(serverURL string) string {
	base := strings.TrimRight(strings.TrimSpace(serverURL), "/")
	if base == "" {
		base = protocol.DefaultServerURL
	}
	return base + "/console"
}

func openConsoleURL(ctx context.Context, target string) error {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name = "open"
		args = []string{target}
	case "windows":
		name = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", target}
	default:
		name = "xdg-open"
		args = []string{target}
	}
	return exec.CommandContext(ctx, name, args...).Start()
}
