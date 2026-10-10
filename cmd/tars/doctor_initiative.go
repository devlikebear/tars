package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/initiative"
	"github.com/devlikebear/tars/internal/jev"
	"github.com/devlikebear/tars/pkg/llm"
)

const doctorSystemOneTimeout = 3 * time.Second

// checkDoctorInitiative reports whether the initiative loop's configured
// text-signal backend (initiative.backend: llm, default, or jev) can be
// called, and whether it gets the user's own words — and why (tars#1219).
// In live mode (tars#1220) it also reports the speak backend's
// availability/provider and, from the ledger, the last delivery result and
// how many initiatives have actually been spoken today.
func checkDoctorInitiative(report *doctorReport, cfg config.Config) {
	if !cfg.Initiative.Enabled {
		report.add("ok", "initiative", "disabled")
		return
	}
	mode := strings.ToLower(strings.TrimSpace(cfg.Initiative.Mode))
	if mode != initiative.ModeLive {
		mode = initiative.ModeShadow
	}
	if strings.ToLower(strings.TrimSpace(cfg.Initiative.Backend)) == "jev" {
		checkDoctorInitiativeJev(report, cfg, mode)
	} else {
		checkDoctorInitiativeLLM(report, cfg, mode)
	}
	if mode == initiative.ModeLive {
		checkDoctorInitiativeSpeak(report, cfg)
	}
}

func checkDoctorInitiativeJev(report *doctorReport, cfg config.Config, mode string) {
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
	report.add("ok", "initiative", fmt.Sprintf("%s mode, backend jev, System One %s", mode, where))
}

func checkDoctorInitiativeLLM(report *doctorReport, cfg config.Config, mode string) {
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
			"%s mode, backend llm, %s/%s (%s tier): reads user text (same provider as chat)", mode, resolved.Kind, resolved.Model, tier))
		return
	}
	report.add("ok", "initiative", fmt.Sprintf(
		"%s mode, backend llm, %s/%s (%s tier): metadata only (different provider than chat)", mode, resolved.Kind, resolved.Model, tier))
}

// checkDoctorInitiativeSpeak reports the live-mode speak composer
// (RoleInitiativeSpeak) — tier/provider/model and whether it is usable at
// all — plus, from the ledger, the last delivery outcome and how many
// initiatives have actually been spoken today. It never reads or reports
// composed text (tars#1003).
func checkDoctorInitiativeSpeak(report *doctorReport, cfg config.Config) {
	tier := strings.TrimSpace(cfg.LLMRoleDefaults[string(llm.RoleInitiativeSpeak)])
	if tier == "" {
		tier = strings.TrimSpace(cfg.LLMRoleDefaults[string(llm.RoleChatMain)])
	}
	if tier == "" {
		tier = strings.TrimSpace(cfg.LLMDefaultTier)
	}
	if tier == "" {
		report.add("warn", "initiative speak", "no tier available (set llm_default_tier or llm_role_defaults.initiative_speak)")
	} else if resolved, err := config.ResolveLLMTier(&cfg, tier); err != nil {
		report.add("warn", "initiative speak", "speak backend is not configured: "+err.Error())
	} else if !llm.SupportsDecisionOnly(resolved.Kind) {
		report.add("warn", "initiative speak", "antigravity-cli cannot disable native tools for decision-only calls; map initiative_speak to another tier")
	} else {
		report.add("ok", "initiative speak", fmt.Sprintf("%s/%s (%s tier)", resolved.Kind, resolved.Model, tier))
	}

	found, summary := initiativeLastDeliverySummary(cfg)
	if !found {
		report.add("ok", "initiative speak", "no delivery recorded yet")
		return
	}
	line := fmt.Sprintf("last delivery: %s (%s) at %s, spoken today: %d",
		summary.delivery, summary.intent, summary.at.Format(time.RFC3339), summary.spokenToday)
	if summary.reason != "" && summary.reason != summary.delivery {
		line += ", reason: " + summary.reason
	}
	report.add("ok", "initiative speak", line)
}

type initiativeDeliverySummary struct {
	at          time.Time
	intent      string
	delivery    string
	reason      string
	spokenToday int
}

// initiativeLastDeliverySummary reads the initiative ledger directly (the
// server process may not be running) and reports the last Speak-intent
// tick's delivery outcome and today's spoken count — the same two facts
// GET /v1/initiative/status exposes from the live Runtime. It never reads
// or returns composed text: the ledger does not store it (tars#1003).
func initiativeLastDeliverySummary(cfg config.Config) (bool, initiativeDeliverySummary) {
	if strings.TrimSpace(cfg.WorkspaceDir) == "" {
		return false, initiativeDeliverySummary{}
	}
	ledger := initiative.OpenLedger(filepath.Join(cfg.WorkspaceDir, "_shared", "initiative", "ledger.jsonl"), 0)
	entries, err := ledger.Recent(500)
	if err != nil || len(entries) == 0 {
		return false, initiativeDeliverySummary{}
	}
	loc := time.Local
	if tz := strings.TrimSpace(cfg.Initiative.Timezone); tz != "" {
		if l, lerr := time.LoadLocation(tz); lerr == nil {
			loc = l
		}
	}
	today := time.Now().In(loc).Format("2006-01-02")
	var out initiativeDeliverySummary
	var found bool
	for _, e := range entries {
		if e.Speak && (e.Mode != initiative.ModeLive || e.Delivery == initiative.DeliveryDelivered) &&
			e.At.In(loc).Format("2006-01-02") == today {
			out.spokenToday++
		}
		if e.Delivery != "" {
			found = true
			out.at, out.intent, out.delivery, out.reason = e.At, string(e.Intent), e.Delivery, e.DeliveryReason
		}
	}
	return found, out
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
