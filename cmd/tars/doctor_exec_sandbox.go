package main

import (
	"context"
	"fmt"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/execsandbox"
)

const doctorExecSandboxTimeout = 5 * time.Second

// doctorExecSandboxProbe is the host side of the check, swappable in tests.
type doctorExecSandboxProbe struct {
	detect func() (execsandbox.Backend, error)
	run    func(ctx context.Context, backend execsandbox.Backend) error
}

func defaultDoctorExecSandboxProbe() doctorExecSandboxProbe {
	return doctorExecSandboxProbe{
		detect: execsandbox.Detect,
		run: func(ctx context.Context, backend execsandbox.Backend) error {
			return backend.Probe(ctx)
		},
	}
}

// checkDoctorExecSandbox reports in one line whether exec commands run in
// the OS sandbox, and warns when it is on but cannot run — every exec
// command is then refused.
func checkDoctorExecSandbox(report *doctorReport, cfg config.Config) {
	checkDoctorExecSandboxWith(report, cfg, defaultDoctorExecSandboxProbe())
}

func checkDoctorExecSandboxWith(report *doctorReport, cfg config.Config, probe doctorExecSandboxProbe) {
	const name = "exec sandbox"
	if !cfg.ToolsExecSandboxEnabled {
		report.add("ok", name, "off (tools.exec.sandbox.enabled: false)")
		return
	}
	const refused = "tools.exec.sandbox is on, so exec commands are refused until it can run; install the sandbox or set tools.exec.sandbox.enabled: false"
	backend, err := probe.detect()
	if err != nil {
		report.add("warn", name, err.Error())
		report.addHint(refused)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), doctorExecSandboxTimeout)
	defer cancel()
	if err := probe.run(ctx, backend); err != nil {
		report.add("warn", name, fmt.Sprintf("%s at %s cannot start a sandbox: %v", backend.Name, backend.Path, err))
		report.addHint(refused)
		return
	}
	network := "network blocked except localhost"
	if cfg.ToolsExecSandboxNetwork {
		network = "network allowed"
	}
	report.add("ok", name, fmt.Sprintf("%s (%s), %s", backend.Name, backend.Path, network))
}
