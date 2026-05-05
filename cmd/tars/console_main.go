package main

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/onboarding"
	protocol "github.com/devlikebear/tars/pkg/tarsclient"
)

var (
	clientCommandRunner  = runClientCommand
	consoleCommandRunner = runConsoleCommand
	consoleURLOpener     = openConsoleURL
	consoleHealthChecker = realConsoleHealthChecker
)

// consoleHealthTimeout bounds how long the no-args `tars` invocation
// waits for the server to respond before giving up and printing the
// onboarding hint. Kept short so the user gets quick feedback when
// they expected a launched server.
const consoleHealthTimeout = 1500 * time.Millisecond

func runConsoleCommand(ctx context.Context, stdout, stderr io.Writer, opts clientOptions) error {
	target, err := buildConsoleURL(opts.serverURL)
	if err != nil {
		return err
	}

	// Probe the server before launching a browser. Opening to a dead
	// URL is the worst kind of failure mode — the user sees a blank
	// "connection refused" page with no idea what to do next.
	healthCtx, cancel := context.WithTimeout(ctx, consoleHealthTimeout)
	defer cancel()
	if err := consoleHealthChecker(healthCtx, opts.serverURL); err != nil {
		printConsoleNotRunningHint(stderr, opts.serverURL)
		return fmt.Errorf("server not reachable at %s", opts.serverURL)
	}

	if err := consoleURLOpener(ctx, target); err != nil {
		if _, writeErr := fmt.Fprintf(stderr, "browser open failed: %v\n", err); writeErr != nil {
			return writeErr
		}
	}
	_, err = fmt.Fprintf(stdout, "Open the console: %s\n", target)
	return err
}

func realConsoleHealthChecker(ctx context.Context, serverURL string) error {
	base := strings.TrimRight(strings.TrimSpace(serverURL), "/")
	if base == "" {
		base = protocol.DefaultServerURL
	}
	return onboarding.WaitForHealthz(ctx, base, 250*time.Millisecond)
}

func printConsoleNotRunningHint(stderr io.Writer, serverURL string) {
	target := strings.TrimSpace(serverURL)
	if target == "" {
		target = protocol.DefaultServerURL
	}
	_, _ = fmt.Fprintf(stderr, "TARS server not reachable at %s\n\n", target)
	_, _ = fmt.Fprintln(stderr, "If this is your first install:")
	_, _ = fmt.Fprintln(stderr, "  tars init                       # one-shot onboarding (picks port, starts server, opens wizard)")
	_, _ = fmt.Fprintln(stderr, "")
	_, _ = fmt.Fprintln(stderr, "If you've already initialized:")
	_, _ = fmt.Fprintln(stderr, "  tars service start              # macOS LaunchAgent")
	_, _ = fmt.Fprintln(stderr, "  tars serve                      # foreground")
	_, _ = fmt.Fprintln(stderr, "")
	_, _ = fmt.Fprintln(stderr, "If your server uses a non-default port, set:")
	_, _ = fmt.Fprintln(stderr, "  export TARS_SERVER_URL=http://127.0.0.1:<port>")
}

func buildConsoleURL(serverURL string) (string, error) {
	return protocol.ConsoleURL(serverURL)
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
