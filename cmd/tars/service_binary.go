package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/buildinfo"
	"github.com/devlikebear/tars/internal/launchagent"
)

// serviceBinaryVersion asks a tars executable for its version. A var so
// tests do not run real binaries.
var serviceBinaryVersion = func(path string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return "", err
	}
	// `tars --version` prints "tars <version> (<commit>, <date>)".
	fields := strings.Fields(string(out))
	if len(fields) < 2 || fields[0] != "tars" {
		return "", fmt.Errorf("unexpected version output: %q", strings.TrimSpace(string(out)))
	}
	return fields[1], nil
}

// serviceBinaryState compares the executable an installed LaunchAgent runs
// with the tars running now. A new install (Homebrew, install.sh, a build)
// does not touch the plist, so the service can keep starting an old binary
// at another path, or one that is gone.
type serviceBinaryState struct {
	// Installed reports that the plist exists and names an executable.
	Installed      bool
	ServiceBinary  string
	Missing        bool
	SameBinary     bool
	ServiceVersion string // "" when unknown
	CurrentBinary  string
	CurrentVersion string
}

func inspectServiceBinary(plistPath string) (serviceBinaryState, error) {
	state := serviceBinaryState{CurrentVersion: strings.TrimSpace(buildinfo.Version)}
	data, err := os.ReadFile(plistPath)
	if errors.Is(err, fs.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	args, err := launchagent.ProgramArgumentsFromPlist(data)
	if err != nil || len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		// Not a plist this tars wrote; nothing to compare.
		return state, nil
	}
	state.Installed = true
	state.ServiceBinary = args[0]

	current, err := serviceExecutablePath()
	if err != nil {
		return state, fmt.Errorf("resolve executable: %w", err)
	}
	state.CurrentBinary = current

	info, statErr := os.Stat(state.ServiceBinary)
	if statErr != nil || info.IsDir() {
		state.Missing = true
		return state, nil
	}
	state.SameBinary = sameExecutable(state.ServiceBinary, current)
	if !state.SameBinary {
		if v, err := serviceBinaryVersion(state.ServiceBinary); err == nil {
			state.ServiceVersion = v
		}
	}
	return state, nil
}

// sameExecutable compares two paths after resolving symlinks, so Homebrew's
// /opt/homebrew/bin/tars and its Cellar target count as one.
func sameExecutable(a, b string) bool {
	resolve := func(p string) string {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return r
		}
		return filepath.Clean(p)
	}
	return resolve(a) == resolve(b)
}

// Warning is what to tell the user, or "" when the service runs this tars
// (or another copy of the same version).
func (s serviceBinaryState) Warning() string {
	switch {
	case !s.Installed || s.SameBinary:
		return ""
	case s.Missing:
		return fmt.Sprintf("the service runs %s, which no longer exists; run `tars service install && tars service start` to run this tars (%s %s)", s.ServiceBinary, s.CurrentBinary, s.CurrentVersion)
	case s.ServiceVersion != "" && s.ServiceVersion == s.CurrentVersion:
		return ""
	}
	running := s.ServiceBinary
	if s.ServiceVersion != "" {
		running += " " + s.ServiceVersion
	}
	return fmt.Sprintf("the service runs %s, not this tars (%s %s); run `tars service install && tars service start` to switch", running, s.CurrentBinary, s.CurrentVersion)
}

// checkDoctorServiceBinary warns when the LaunchAgent runs another tars.
func checkDoctorServiceBinary(report *doctorReport) {
	if serviceRuntimeGOOS != "darwin" {
		return
	}
	target, err := defaultServerServiceTarget()
	if err != nil {
		return
	}
	state, err := inspectServiceBinary(target.plistPath)
	if err != nil || !state.Installed {
		return
	}
	if warning := state.Warning(); warning != "" {
		report.add("warn", "service binary", warning)
		report.addHint("run `tars service install && tars service start` so the service runs this tars")
		return
	}
	report.add("ok", "service binary", state.ServiceBinary)
}
