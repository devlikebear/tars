package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/execsandbox"
)

func TestCheckDoctorExecSandbox(t *testing.T) {
	seatbelt := func() (execsandbox.Backend, error) {
		return execsandbox.Backend{Name: execsandbox.Seatbelt, Path: "/usr/bin/sandbox-exec"}, nil
	}
	bwrap := func() (execsandbox.Backend, error) {
		return execsandbox.Backend{Name: execsandbox.Bubblewrap, Path: "/usr/bin/bwrap"}, nil
	}
	none := func() (execsandbox.Backend, error) {
		return execsandbox.Backend{}, fmt.Errorf("%w: bwrap not found (install bubblewrap)", execsandbox.ErrUnavailable)
	}
	runs := func(context.Context, execsandbox.Backend) error { return nil }
	fails := func(context.Context, execsandbox.Backend) error {
		return errors.New("setting up uid map: Permission denied")
	}
	mustNotRun := func() (execsandbox.Backend, error) {
		t.Error("the sandbox was probed although it is off")
		return execsandbox.Backend{}, nil
	}
	on := func(network bool) config.Config {
		return config.Config{ToolConfig: config.ToolConfig{ToolsExecSandboxEnabled: true, ToolsExecSandboxNetwork: network}}
	}

	cases := []struct {
		name     string
		cfg      config.Config
		probe    doctorExecSandboxProbe
		status   string
		detail   string
		wantHint bool
	}{
		{"off", config.Config{}, doctorExecSandboxProbe{detect: mustNotRun, run: runs}, "ok", "off", false},
		{"seatbelt ready", on(false), doctorExecSandboxProbe{detect: seatbelt, run: runs}, "ok", "seatbelt (/usr/bin/sandbox-exec), network blocked except localhost", false},
		{"bwrap with network", on(true), doctorExecSandboxProbe{detect: bwrap, run: runs}, "ok", "bubblewrap (/usr/bin/bwrap), network allowed", false},
		{"no backend", on(false), doctorExecSandboxProbe{detect: none, run: runs}, "warn", "bwrap not found", true},
		{"backend cannot start", on(false), doctorExecSandboxProbe{detect: bwrap, run: fails}, "warn", "cannot start a sandbox: setting up uid map", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var report doctorReport
			checkDoctorExecSandboxWith(&report, tc.cfg, tc.probe)
			if len(report.checks) != 1 {
				t.Fatalf("checks = %+v", report.checks)
			}
			c := report.checks[0]
			if c.name != "exec sandbox" || c.status != tc.status || !strings.Contains(c.detail, tc.detail) {
				t.Fatalf("check = %+v, want %s containing %q", c, tc.status, tc.detail)
			}
			if got := len(report.hints) > 0 && strings.Contains(report.hints[0], "refused"); got != tc.wantHint {
				t.Fatalf("hints = %v", report.hints)
			}
		})
	}
}
