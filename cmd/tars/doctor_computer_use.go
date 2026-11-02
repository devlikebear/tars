package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/computeruse"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/jev"
	"github.com/devlikebear/tars/pkg/llm"
)

const doctorCuaDriverTimeout = 5 * time.Second

// doctorComputerUseProbe is the host side of the check, swappable in tests.
type doctorComputerUseProbe struct {
	findDriver func(configured string) (string, error)
	pingDriver func(ctx context.Context, path string) error
	// findAgentDesktop and pingAgentDesktop are the same pair for
	// tools.computer_use.driver: agent-desktop.
	findAgentDesktop func(configured string) (string, error)
	pingAgentDesktop func(ctx context.Context, path string) error
}

func defaultDoctorComputerUseProbe() doctorComputerUseProbe {
	return doctorComputerUseProbe{
		findDriver: computeruse.FindCuaDriverPath,
		pingDriver: func(ctx context.Context, path string) error {
			return computeruse.NewCuaDriver(path, doctorCuaDriverTimeout).Ping(ctx)
		},
		findAgentDesktop: computeruse.FindAgentDesktopPath,
		pingAgentDesktop: func(ctx context.Context, path string) error {
			return computeruse.NewAgentDesktopDriver(path, doctorCuaDriverTimeout).Ping(ctx)
		},
	}
}

// doctorComputerUseProbeFor builds the probe checkDoctorComputerUse uses. It
// is a package variable, not a constant call, so cmd/tars tests can swap in a
// hermetic probe (TestMain in network_guard_test.go) that never execs a real
// cua-driver binary — the production default is untouched.
var doctorComputerUseProbeFor = defaultDoctorComputerUseProbe

// checkDoctorComputerUse reports whether the computer_use tool can run: the
// driver binary, its daemon, and the System One server that decides each
// step — and whether that server is remote, since it receives screen text.
func checkDoctorComputerUse(report *doctorReport, cfg config.Config) {
	checkDoctorComputerUseWith(report, cfg, doctorComputerUseProbeFor())
}

// checkDoctorDriver reports the selected driver and returns its name and path,
// with healthy false when a warning was added.
func checkDoctorDriver(report *doctorReport, cfg config.Config, probe doctorComputerUseProbe) (label, path string, healthy bool) {
	const name = "computer_use"
	if cfg.ToolsComputerUseDriver == computeruse.DriverAgentDesktop {
		label = computeruse.DriverAgentDesktop
		path, err := probe.findAgentDesktop(cfg.ToolsComputerUseAgentDesktopPath)
		if err != nil {
			report.add("warn", name, "agent-desktop binary not found")
			report.addHint("install agent-desktop (https://github.com/vercel-labs/agent-desktop) or set tools.computer_use.agent_desktop_path / AGENT_DESKTOP_PATH")
			return label, "", false
		}
		ctx, cancel := context.WithTimeout(context.Background(), doctorCuaDriverTimeout)
		defer cancel()
		if err := probe.pingAgentDesktop(ctx, path); err != nil {
			report.add("warn", name, fmt.Sprintf("agent-desktop at %s is not usable: %v", path, err))
			report.addHint("grant Accessibility to the app that runs tars (the terminal, or the service binary under launchd)")
			return label, path, false
		}
		return label, path, true
	}
	label = computeruse.DriverCua
	path, err := probe.findDriver(cfg.ToolsComputerUseCuaDriverPath)
	if err != nil {
		report.add("warn", name, "cua-driver binary not found")
		report.addHint("install cua-driver (https://github.com/trycua/cua) or set tools.computer_use.cua_driver_path / CUA_DRIVER_PATH")
		return label, "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), doctorCuaDriverTimeout)
	defer cancel()
	if err := probe.pingDriver(ctx, path); err != nil {
		report.add("warn", name, fmt.Sprintf("cua-driver at %s is not answering: %v", path, err))
		report.addHint("start the daemon with `cua-driver serve` and grant Accessibility with `cua-driver permissions grant`")
		return label, path, false
	}
	return label, path, true
}

func checkDoctorComputerUseWith(report *doctorReport, cfg config.Config, probe doctorComputerUseProbe) {
	const name = "computer_use"
	if !cfg.ToolsComputerUseEnabled {
		report.add("ok", name, "disabled")
		return
	}
	label, path, healthy := checkDoctorDriver(report, cfg, probe)

	if cfg.ToolsComputerUseBackend == "" || cfg.ToolsComputerUseBackend == "llm" {
		tier := cfg.LLMRoleDefaults["computer_use"]
		if tier == "" {
			tier = "light"
		}
		resolved, err := config.ResolveLLMTier(&cfg, tier)
		if err != nil {
			report.add("warn", name, "LLM decision backend is not configured: "+err.Error())
			return
		}
		if !llm.SupportsDecisionOnly(resolved.Kind) {
			report.add("warn", name, "antigravity-cli cannot disable native tools for decision-only calls; select another computer_use role tier")
			return
		}
		if healthy {
			report.add("ok", name, fmt.Sprintf("%s %s, LLM %s/%s (%s tier): screen text is sent to the configured provider; usage follows its billing or subscription limits", label, path, resolved.Kind, resolved.Model, tier))
		}
		return
	}
	if cfg.ToolsComputerUseBackend != "jev" {
		report.add("warn", name, "unknown computer use backend: select llm or jev")
		return
	}
	base := strings.TrimSpace(cfg.Jev.BaseURL)
	if base == "" {
		report.add("warn", name, "jev.base_url is empty: no System One server to decide steps")
		report.addHint("set jev.base_url to https://api.typesafe.ai with TYPESAFE_API_KEY (hosted Jev), or to a local System One server")
		return
	}
	client := jev.New(jev.Options{BaseURL: base, APIKey: cfg.Jev.APIKey, Model: cfg.Jev.Model, Timeout: doctorSystemOneTimeout})
	ctx, cancel := context.WithTimeout(context.Background(), doctorSystemOneTimeout)
	defer cancel()
	if _, err := client.Ask(ctx, "doctor probe", map[string]jev.Question{
		"ok": {Type: "noul", Instructions: "Is this a probe?"},
	}); err != nil {
		report.add("warn", name, fmt.Sprintf("System One at %s unreachable: %v", base, err))
		return
	}
	if !healthy {
		return
	}
	where := "remote: on-screen text of the driven window leaves this machine"
	if client.IsLoopback() {
		where = "loopback: screen text stays on this machine"
	}
	report.add("ok", name, fmt.Sprintf("%s %s, System One %s", label, path, where))
}
