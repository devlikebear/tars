// Package serverupdate keeps a release install of the tars server current
// from the desktop shell, by running `tars update` (#1104's archives).
//
// The shell checks on a timer. An update restarts the server, which cuts off
// a chat turn in progress and an unattended run waiting on an approval, so
// it is applied only while the server is idle; otherwise it waits for the
// next check. Updating a Homebrew install is left to Homebrew: `tars update`
// refuses it, and on macOS the shell keeps telling the user to run
// `brew upgrade` instead (see server.OutdatedMessage).
package serverupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/desktop/internal/activity"
	"github.com/devlikebear/tars/desktop/internal/server"
)

const (
	// CheckEvery is how often the shell looks for a new release.
	CheckEvery = 6 * time.Hour
	// RetryBusyEvery is how soon it tries again when an update is waiting
	// for the server to go idle.
	RetryBusyEvery = 15 * time.Minute
	// FirstCheckAfter leaves the shell and the server time to start.
	FirstCheckAfter = 2 * time.Minute
)

// Enabled reports whether the shell updates the server on goos: Windows,
// where install.ps1 is the install path. Homebrew owns macOS installs.
func Enabled(goos string) bool {
	return goos == "windows"
}

// Run runs bin with args and extra environment, returning its stdout and
// stderr. The shell runs it with no console window.
type Run func(ctx context.Context, bin string, args, env []string) (stdout, stderr []byte, err error)

// Result is `tars update --json`'s output.
type Result struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"available"`
	Updated   bool   `json:"updated"`
	Restarted bool   `json:"restarted"`
	Server    string `json:"server,omitempty"`
}

// Updater runs the tars executable the shell starts the server with.
type Updater struct {
	Bin string
	Cfg server.Config
	Run Run
}

func (u Updater) args(extra ...string) []string {
	args := append([]string{"update", "--json"}, extra...)
	if url := strings.TrimSpace(u.Cfg.URL); url != "" {
		args = append(args, "--server-url", url)
	}
	return args
}

// env passes the admin token through the environment, not the command
// line, where other local users could read it.
func (u Updater) env() []string {
	if token := strings.TrimSpace(u.Cfg.AdminToken); token != "" {
		return []string{"TARS_ADMIN_API_TOKEN=" + token}
	}
	return nil
}

func (u Updater) run(ctx context.Context, extra ...string) (Result, error) {
	stdout, stderr, err := u.Run(ctx, u.Bin, u.args(extra...), u.env())
	if err != nil {
		msg := strings.TrimSpace(string(stderr))
		if msg == "" {
			msg = strings.TrimSpace(string(stdout))
		}
		if msg != "" {
			return Result{}, fmt.Errorf("tars update: %w: %s", err, firstLine(msg))
		}
		return Result{}, fmt.Errorf("tars update: %w", err)
	}
	return parseResult(stdout)
}

// Check asks whether a newer release exists.
func (u Updater) Check(ctx context.Context) (Result, error) {
	return u.run(ctx, "--check")
}

// Apply installs the newer release and restarts the running server.
func (u Updater) Apply(ctx context.Context) (Result, error) {
	return u.run(ctx, "--yes")
}

// parseResult reads the last JSON object tars printed; anything logged
// before it is ignored.
func parseResult(stdout []byte) (Result, error) {
	lines := bytes.Split(bytes.TrimSpace(stdout), []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if !bytes.HasPrefix(line, []byte("{")) {
			continue
		}
		var r Result
		if err := json.Unmarshal(line, &r); err != nil {
			return Result{}, fmt.Errorf("tars update printed %q: %w", line, err)
		}
		return r, nil
	}
	return Result{}, errors.New("tars update printed no result; is this tars older than `tars update`?")
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

// Idle reports whether restarting the server would interrupt nothing: no
// chat turn running and no tool call waiting on an answer.
func Idle(snap activity.Snapshot) bool {
	return len(snap.Running) == 0 && len(snap.Pending) == 0 && len(snap.Queued) == 0
}

// Step is what a check decided.
type Step int

const (
	// UpToDate: nothing to do until the next check.
	UpToDate Step = iota
	// ApplyNow: install and restart now.
	ApplyNow
	// WaitForIdle: an update is ready but the server is busy; retry soon.
	WaitForIdle
)

// Decide picks the next step from a check and the server's activity.
// activityErr is the error reading activity; when the shell cannot tell
// whether the server is busy it waits rather than risk cutting off a turn.
func Decide(check Result, snap activity.Snapshot, activityErr error) Step {
	switch {
	case !check.Available:
		return UpToDate
	case activityErr != nil || !Idle(snap):
		return WaitForIdle
	default:
		return ApplyNow
	}
}

// Next is how long to wait before checking again after step.
func Next(step Step) time.Duration {
	if step == WaitForIdle {
		return RetryBusyEvery
	}
	return CheckEvery
}
