package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/selfupdate"
)

func TestUpdateCommandTellsWhatHappenedInText(t *testing.T) {
	srv := &fakeUpdateServer{version: "1.2.3"}
	stubUpdate(t, availableStatus, srv)
	out, err := runUpdateCommand(t, "--yes", "--server-url", "http://127.0.0.1:43180")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	for _, want := range []string{
		"Installed tars 1.3.0 to /opt/tars/bin/tars.",
		"Restarting the server at http://127.0.0.1:43180 (1.2.3)...",
		"The server is running tars 1.3.0.",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output %q misses %q", out, want)
		}
	}

	stubUpdate(t, availableStatus, &fakeUpdateServer{version: "1.2.3", restartErr: selfupdate.ErrRestartUnauthorized})
	out, err = runUpdateCommand(t, "--yes")
	if err != nil {
		t.Fatalf("a failed restart must not fail the update: %v", err)
	}
	if !strings.Contains(out, "Could not restart the server") || !strings.Contains(out, "Restart it yourself to run 1.3.0.") {
		t.Fatalf("output = %q", out)
	}
}

func TestUpdateCommandSurfacesInstallAndExecutableErrors(t *testing.T) {
	stubUpdate(t, availableStatus, &fakeUpdateServer{})
	updateInstall = func(context.Context, selfupdate.Options, selfupdate.Release) error {
		return errors.New("checksum mismatch")
	}
	if _, err := runUpdateCommand(t, "--yes"); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("install error: %v", err)
	}

	stubUpdate(t, availableStatus, &fakeUpdateServer{})
	updateExePath = func() (string, error) { return "", errors.New("no such process image") }
	if _, err := runUpdateCommand(t, "--check"); err == nil || !strings.Contains(err.Error(), "find the tars executable") {
		t.Fatalf("executable error: %v", err)
	}
}

func TestUpdateDefaults(t *testing.T) {
	exe, err := currentExecutable()
	if err != nil || !filepath.IsAbs(exe) {
		t.Fatalf("currentExecutable = %q, %v", exe, err)
	}
	srv, ok := updateServer("http://127.0.0.1:1", "token").(selfupdate.Server)
	if !ok || srv.URL != "http://127.0.0.1:1" || srv.Token != "token" {
		t.Fatalf("updateServer = %#v", srv)
	}
	// CI runs without a terminal, which is the case the check exists for;
	// from a developer's terminal the prompt would really ask.
	if stdinIsTerminal() {
		return
	}
	if ok, err := confirmOnTerminal(strings.NewReader("y\n"), &strings.Builder{}, "Update?"); ok || !errors.Is(err, errNotInteractive) {
		t.Fatalf("confirm without a terminal = %v, %v", ok, err)
	}
}
