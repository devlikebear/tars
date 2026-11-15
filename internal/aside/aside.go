// Package aside runs the Aside browser's command-line client (`aside`).
//
// Aside is a browser the developer is signed in to. Its CLI offers two
// entry points TARS uses:
//
//   - `aside repl <code>` runs Playwright-style JavaScript in that browser.
//     No model is involved, so a call costs nothing but the page load.
//     web_search's aside provider (Search) uses it to read a results page.
//   - `aside exec <prompt>` runs Aside's own browser agent on a task, on the
//     model the developer picked in Aside's settings. Focus mode's @Browser
//     end-to-end goals use it (Exec).
//
// Both act in the developer's real, logged-in browser profile. That is the
// point — a search engine answers it like a person, and a page behind a
// sign-in is reachable — and it is also the risk, so every caller here is
// opt-in and documents what it sends.
package aside

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/devlikebear/tars/internal/exepath"
)

// PathEnv overrides where the aside binary is looked up.
const PathEnv = "ASIDE_CLI_PATH"

// ErrNotFound reports that no aside binary could be found.
var ErrNotFound = errors.New("aside CLI not found")

// runFunc runs one aside command. It is a seam for tests.
type runFunc func(ctx context.Context, bin string, args []string) (stdout, stderr []byte, err error)

// Client runs the aside CLI. The zero value looks the binary up per call.
type Client struct {
	// Path is the configured binary (tools.aside.path). Empty means
	// ASIDE_CLI_PATH, then PATH, then the installer's known locations.
	Path string

	run runFunc
}

// Resolve returns the aside binary to run. It is looked up on every call,
// like computer_use's driver, so installing Aside needs no restart.
func (c Client) Resolve() (string, error) {
	if p := strings.TrimSpace(c.Path); p != "" {
		return resolveExplicit(p)
	}
	if p := strings.TrimSpace(os.Getenv(PathEnv)); p != "" {
		return resolveExplicit(p)
	}
	path, err := exepath.Resolve(exepath.Candidates{
		LookupNames: []string{"aside"},
		// A server started by launchd gets launchd's PATH, which has none
		// of these.
		Fallbacks: func() []string {
			out := []string{}
			if home, err := os.UserHomeDir(); err == nil {
				out = append(out, filepath.Join(home, ".local", "bin", "aside"))
			}
			return append(out, "/opt/homebrew/bin/aside", "/usr/local/bin/aside")
		},
	})
	if err != nil {
		return "", fmt.Errorf("%w: install Aside's CLI or set tools.aside.path", ErrNotFound)
	}
	return path, nil
}

func resolveExplicit(p string) (string, error) {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[2:])
		}
	}
	if info, err := os.Stat(p); err != nil || info.IsDir() {
		return "", fmt.Errorf("%w at %s", ErrNotFound, p)
	}
	return p, nil
}

// Repl runs JavaScript in the browser and returns its printed output with
// terminal colors removed.
func (c Client) Repl(ctx context.Context, code string) (string, error) {
	return c.call(ctx, []string{"repl", code})
}

// Exec runs Aside's browser agent on prompt in Guard mode (the agent asks
// before touching other folders) and returns its transcript with terminal
// colors removed; the agent's answer is at the end.
func (c Client) Exec(ctx context.Context, prompt string) (string, error) {
	return c.call(ctx, []string{"exec", "--permission", "guard", prompt})
}

func (c Client) call(ctx context.Context, args []string) (string, error) {
	bin, err := c.Resolve()
	if err != nil {
		return "", err
	}
	run := c.run
	if run == nil {
		run = runCommand
	}
	stdout, stderr, err := run(ctx, bin, args)
	out := StripANSI(string(stdout))
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return out, ctxErr
		}
		msg := strings.TrimSpace(StripANSI(string(stderr)))
		if msg == "" {
			msg = lastLines(out, 3)
		}
		return out, fmt.Errorf("aside %s failed: %w: %s", args[0], err, msg)
	}
	return out, nil
}

func runCommand(ctx context.Context, bin string, args []string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

// StripANSI removes terminal escape sequences.
func StripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

// lastLines returns up to n trailing non-empty lines of s.
func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
