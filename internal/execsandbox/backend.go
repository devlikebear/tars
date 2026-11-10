package execsandbox

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Backend names.
const (
	Seatbelt   = "seatbelt"
	Bubblewrap = "bubblewrap"
)

// ErrUnavailable reports that this host has no sandbox backend. Callers with
// the sandbox enabled must refuse to run the command rather than run it
// unsandboxed.
var ErrUnavailable = errors.New("exec sandbox unavailable")

// Backend is a sandbox program found on this host.
type Backend struct {
	// Name is Seatbelt or Bubblewrap.
	Name string
	// Path is the absolute path of sandbox-exec or bwrap.
	Path string
}

// Detect finds the sandbox backend for this platform.
func Detect() (Backend, error) {
	return detect(runtime.GOOS, exec.LookPath)
}

func detect(goos string, lookPath func(string) (string, error)) (Backend, error) {
	switch goos {
	case "darwin":
		// sandbox-exec ships with macOS; PATH may not include /usr/bin under
		// launchd, so its fixed location is tried as well.
		for _, candidate := range []string{"sandbox-exec", "/usr/bin/sandbox-exec"} {
			if path, err := lookPath(candidate); err == nil {
				return Backend{Name: Seatbelt, Path: path}, nil
			}
		}
		return Backend{}, fmt.Errorf("%w: sandbox-exec not found", ErrUnavailable)
	case "linux":
		if path, err := lookPath("bwrap"); err == nil {
			return Backend{Name: Bubblewrap, Path: path}, nil
		}
		return Backend{}, fmt.Errorf("%w: bwrap not found (install bubblewrap)", ErrUnavailable)
	default:
		return Backend{}, fmt.Errorf("%w: no OS sandbox is supported on %s", ErrUnavailable, goos)
	}
}

// Command returns the program and arguments that run argv in dir under p.
func (b Backend) Command(p Policy, dir string, argv []string) (string, []string) {
	switch b.Name {
	case Bubblewrap:
		return b.Path, BubblewrapArgs(p, dir, argv)
	default:
		return b.Path, append([]string{"-p", SeatbeltProfile(p)}, argv...)
	}
}

// Probe runs a trivial command under the backend. Finding bwrap is not enough
// on Linux: distributions that restrict unprivileged user namespaces (Ubuntu
// 24.04's AppArmor default) make every bwrap call fail.
func (b Backend) Probe(ctx context.Context) error {
	var cmd *exec.Cmd
	switch b.Name {
	case Bubblewrap:
		cmd = exec.CommandContext(ctx, b.Path, "--unshare-pid", "--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc", "true")
	default:
		cmd = exec.CommandContext(ctx, b.Path, "-p", "(version 1)\n(allow default)\n", "true")
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("%v: %s", err, msg)
		}
		return err
	}
	return nil
}
