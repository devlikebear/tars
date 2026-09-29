package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/jev"
)

const doctorSystemOneTimeout = 3 * time.Second

// checkDoctorInitiative reports whether the initiative loop can reach its
// System One server, and whether that server gets user text (loopback only).
func checkDoctorInitiative(report *doctorReport, cfg config.Config) {
	if !cfg.Initiative.Enabled {
		report.add("ok", "initiative", "disabled")
		return
	}
	base := strings.TrimSpace(cfg.Jev.BaseURL)
	if base == "" {
		report.add("warn", "initiative", "enabled without jev.base_url: only Go signals are used")
		report.addHint("start a local System One, e.g. `uv run --extra serve python -m kev.serve --run jaredpalmer/kev-0.8b --port 8009`, then set jev.base_url: http://127.0.0.1:8009")
		return
	}
	client := jev.New(jev.Options{BaseURL: base, APIKey: cfg.Jev.APIKey, Model: cfg.Jev.Model, Timeout: doctorSystemOneTimeout})
	ctx, cancel := context.WithTimeout(context.Background(), doctorSystemOneTimeout)
	defer cancel()
	_, err := client.Ask(ctx, "doctor probe", map[string]jev.Question{
		"ok": {Type: "noul", Instructions: "Is this a probe?"},
	})
	if err != nil {
		report.add("warn", "initiative", fmt.Sprintf("System One at %s unreachable: %v", base, err))
		return
	}
	where := "remote (metadata only)"
	if client.IsLoopback() {
		where = "loopback (reads user text)"
	}
	report.add("ok", "initiative", fmt.Sprintf("shadow mode, System One %s", where))
}
