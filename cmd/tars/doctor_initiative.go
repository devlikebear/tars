package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/jev"
	"github.com/devlikebear/tars/pkg/llm"
)

const doctorSystemOneTimeout = 3 * time.Second

// checkDoctorInitiative reports whether the initiative loop's configured
// text-signal backend (initiative.backend: llm, default, or jev) can be
// called, and whether it gets the user's own words — and why (tars#1219).
func checkDoctorInitiative(report *doctorReport, cfg config.Config) {
	if !cfg.Initiative.Enabled {
		report.add("ok", "initiative", "disabled")
		return
	}
	if strings.ToLower(strings.TrimSpace(cfg.Initiative.Backend)) == "jev" {
		checkDoctorInitiativeJev(report, cfg)
		return
	}
	checkDoctorInitiativeLLM(report, cfg)
}

func checkDoctorInitiativeJev(report *doctorReport, cfg config.Config) {
	base := strings.TrimSpace(cfg.Jev.BaseURL)
	if base == "" {
		report.add("warn", "initiative", "backend jev but jev.base_url is empty: only Go signals are used")
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
	report.add("ok", "initiative", fmt.Sprintf("shadow mode, backend jev, System One %s", where))
}

func checkDoctorInitiativeLLM(report *doctorReport, cfg config.Config) {
	tier := strings.TrimSpace(cfg.LLMRoleDefaults[string(llm.RoleInitiative)])
	if tier == "" {
		tier = "light"
	}
	resolved, err := config.ResolveLLMTier(&cfg, tier)
	if err != nil {
		report.add("warn", "initiative", "LLM text-signal backend is not configured: "+err.Error())
		return
	}
	if !llm.SupportsDecisionOnly(resolved.Kind) {
		report.add("warn", "initiative", "antigravity-cli cannot disable native tools for decision-only calls; select another initiative role tier")
		return
	}
	chatAlias := initiativeChatProviderAlias(cfg)
	if resolved.ProviderAlias != "" && resolved.ProviderAlias == chatAlias {
		report.add("ok", "initiative", fmt.Sprintf(
			"shadow mode, backend llm, %s/%s (%s tier): reads user text (same provider as chat)", resolved.Kind, resolved.Model, tier))
		return
	}
	report.add("ok", "initiative", fmt.Sprintf(
		"shadow mode, backend llm, %s/%s (%s tier): metadata only (different provider than chat)", resolved.Kind, resolved.Model, tier))
}

// initiativeChatProviderAlias mirrors
// internal/tarsserver.chatProviderAlias — doctor runs standalone and does
// not import the server package, so this small resolution is duplicated
// rather than shared (the same boundary doctor_computer_use.go already
// crosses independently of internal/tarsserver).
func initiativeChatProviderAlias(cfg config.Config) string {
	tier := strings.TrimSpace(cfg.LLMRoleDefaults[string(llm.RoleChatMain)])
	if tier == "" {
		tier = strings.TrimSpace(cfg.LLMDefaultTier)
	}
	if tier == "" {
		return ""
	}
	resolved, err := config.ResolveLLMTier(&cfg, tier)
	if err != nil {
		return ""
	}
	return resolved.ProviderAlias
}
