package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/initiative"
	"github.com/devlikebear/tars/internal/jev"
	"github.com/devlikebear/tars/internal/memory"
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
	checkDoctorInitiativePresence(report, cfg)
	if mode == initiative.ModeLive {
		checkDoctorInitiativeSpeak(report, cfg)
	}
	checkDoctorInitiativeSubjects(report, cfg, mode, time.Now())
}

// checkDoctorInitiativeSubjects reports the two tars#1001 options, one line
// each when turned on (off is the default and says nothing): the counts
// behind them — queued reminders, pending candidates, today's spoken count
// from the ledger. Never a reminder's text, a candidate's summary or an id.
func checkDoctorInitiativeSubjects(report *doctorReport, cfg config.Config, mode string, now time.Time) {
	c := cfg.Initiative
	if !c.Remind && !c.ConfirmMemory && !c.ShareResult && !c.FollowUp && !c.MorningBrief && !c.EveningRecap {
		return
	}
	loc := time.Local
	if tz := strings.TrimSpace(c.Timezone); tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	var entries []initiative.Entry
	shared := filepath.Join(cfg.WorkspaceDir, "_shared", "initiative")
	if strings.TrimSpace(cfg.WorkspaceDir) != "" {
		entries, _ = initiative.OpenLedger(filepath.Join(shared, "ledger.jsonl"), 0).Recent(500)
	}
	switch {
	case !c.Remind:
	case mode != initiative.ModeLive:
		report.add("ok", "initiative remind", "on, but mode is shadow: cron delivers reminders")
	default:
		report.add("ok", "initiative remind", fmt.Sprintf("on: %d said today, %d queued",
			initiative.CountSpokenToday(entries, initiative.IntentRemind, now, loc), doctorQueuedReminders(filepath.Join(shared, "reminders.json"))))
	}
	checkDoctorInitiativeCoworker(report, c, mode, entries, filepath.Join(shared, "results.json"), now, loc)
	checkDoctorInitiativeDay(report, c, mode, entries, now, loc)
	if !c.ConfirmMemory {
		return
	}
	pending := 0
	if strings.TrimSpace(cfg.WorkspaceDir) != "" {
		if candidates, err := memory.ListMemoryCandidates(cfg.WorkspaceDir, memory.MemoryCandidateListOptions{Status: memory.MemoryCandidateStatusPending}); err == nil {
			for _, cand := range candidates {
				if strings.EqualFold(strings.TrimSpace(cand.Category), "preference") {
					pending++
				}
			}
		}
	}
	report.add("ok", "initiative confirm_memory", fmt.Sprintf("on: %d pending preference candidates, asked today: %d",
		pending, initiative.CountSpokenToday(entries, initiative.IntentConfirmMemory, now, loc)))
}

// checkDoctorInitiativeCoworker reports the two tars#1002 options, one line
// each when on: how many reports or follow-ups were said today, and for
// share_result how many results wait in the queue. Never a result's text, a
// goal, a title or an id.
func checkDoctorInitiativeCoworker(report *doctorReport, c config.InitiativeConfig, mode string, entries []initiative.Entry, resultsPath string, now time.Time, loc *time.Location) {
	switch {
	case !c.ShareResult:
	case mode != initiative.ModeLive:
		report.add("ok", "initiative share_result", "on, but mode is shadow: results keep their notes and notifications")
	default:
		report.add("ok", "initiative share_result", fmt.Sprintf("on: %d reported today, %d queued",
			initiative.CountSpokenToday(entries, initiative.IntentShareResult, now, loc), doctorQueuedReminders(resultsPath)))
	}
	if c.FollowUp {
		report.add("ok", "initiative follow_up", fmt.Sprintf("on: suggested today: %d",
			initiative.CountSpokenToday(entries, initiative.IntentFollowUp, now, loc)))
	}
}

// checkDoctorInitiativeDay reports the tars#1001 morning brief and evening
// recap, one line each when on: whether the brief can go out at all, and
// the recap's window and whether it was said today. Never a line of either.
func checkDoctorInitiativeDay(report *doctorReport, c config.InitiativeConfig, mode string, entries []initiative.Entry, now time.Time, loc *time.Location) {
	switch {
	case !c.MorningBrief:
	case mode != initiative.ModeLive:
		report.add("ok", "initiative morning_brief", "on, but mode is shadow: nothing is said")
	default:
		report.add("ok", "initiative morning_brief", "on: added to the first greeting of the day in CASE's bubble")
	}
	if !c.EveningRecap {
		return
	}
	window := initiative.Config{QuietHours: c.QuietHours, RecapWindow: c.EveningRecapWindow, Location: loc}.WithDefaults().RecapWindow
	report.add("ok", "initiative evening_recap", fmt.Sprintf("on: window %s, said today: %d",
		window, initiative.CountSpokenToday(entries, initiative.IntentEveningRecap, now, loc)))
}

// doctorQueuedReminders counts entries waiting in an initiative queue file
// — reminders.json or results.json (reading only its length, never the
// text).
func doctorQueuedReminders(path string) int {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var entries []json.RawMessage
	if json.Unmarshal(raw, &entries) != nil {
		return 0
	}
	return len(entries)
}

// checkDoctorInitiativePresence reports the console presence signal's last
// timestamp and whether the console reads as present right now (tars#1249)
// — read directly from presence.json (the server process may not be
// running), the same two facts GET /v1/initiative/status exposes. Never a
// session id or any message text: presence.json holds neither.
func checkDoctorInitiativePresence(report *doctorReport, cfg config.Config) {
	if strings.TrimSpace(cfg.WorkspaceDir) == "" {
		return
	}
	path := filepath.Join(cfg.WorkspaceDir, "_shared", "initiative", "presence.json")
	state, err := initiative.LoadPresenceState(path)
	if err != nil {
		report.add("warn", "initiative presence", "presence.json unreadable: "+err.Error())
		return
	}
	if state.LastAt.IsZero() {
		report.add("ok", "initiative presence", "no presence signal recorded yet")
		return
	}
	now := time.Now()
	age := now.Sub(state.LastAt).Round(time.Second)
	if state.Present(now) {
		report.add("ok", "initiative presence", fmt.Sprintf("present now (last signal %s ago)", age))
		return
	}
	report.add("ok", "initiative presence", fmt.Sprintf("not present (last signal %s ago)", age))
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

	found, summary := initiativeLastDeliverySummary(cfg, time.Now())
	if summary.greetedToday {
		report.add("ok", "initiative speak", "greeted today: yes (no more greets until local midnight)")
	} else {
		report.add("ok", "initiative speak", "greeted today: no")
	}
	if !found {
		report.add("ok", "initiative speak", "no delivery recorded yet")
	} else {
		line := fmt.Sprintf("last delivery: %s (%s) at %s, spoken today: %d",
			summary.delivery, summary.intent, summary.at.Format(time.RFC3339), summary.spokenToday)
		if summary.reason != "" && summary.reason != summary.delivery {
			line += ", reason: " + summary.reason
		}
		if summary.channel != "" {
			line += ", channel: " + summary.channel
			if summary.channelError != "" {
				line += " (channel_error: " + summary.channelError + ")"
			}
		}
		report.add("ok", "initiative speak", line)
	}

	checkDoctorInitiativeTelegram(report, cfg)

	o := summary.outcomes
	report.add("ok", "initiative replies", fmt.Sprintf(
		"today: %d replied, %d ignored, %d stopped (recent: %d/%d/%d); cooldown now %s",
		o.Today.Replied, o.Today.Ignored, o.Today.Stopped,
		o.Recent.Replied, o.Recent.Ignored, o.Recent.Stopped,
		time.Duration(o.EffectiveCooldownSeconds*float64(time.Second)).Round(time.Second)))
	if o.StoppedToday {
		report.add("ok", "initiative replies", "a reply asked for quiet today: no more speaking until tomorrow")
	}
}

type initiativeDeliverySummary struct {
	at          time.Time
	intent      string
	delivery    string
	reason      string
	spokenToday int
	// channel/channelError mirror Entry.Channel/ChannelError (tars#1221
	// P2c) — which transport the last delivery used, and a fixed code
	// for a channel send failure after the main-session write already
	// succeeded. Never the composed words, a chat id, or a token.
	channel      string
	channelError string
	// outcomes is reply tracking's aggregate (tars#1221), computed by the
	// same initiative.SummarizeOutcomes GET /v1/initiative/status uses —
	// one function, two readers.
	outcomes initiative.OutcomeSummary
	// greetedToday mirrors GoSignals.GreetedToday (tars#1269): whether a
	// greet already counted as spoken (live mode only when actually
	// delivered, shadow mode per the same Entry.spoken() rule
	// historyFrom and applyLocked use) earlier the same local day.
	// doctor cannot call historyFrom itself (unexported, and doctor runs
	// standalone without a live Runtime), so this loop mirrors its
	// per-entry logic rather than sharing it — same boundary this file's
	// spokenToday count already crosses.
	greetedToday bool
}

// initiativeReplyCooldown parses initiative.Cooldown the same way the
// running server does (helpers_initiative.go's parsePulseDuration), so
// doctor's reported cooldown-in-force matches live behavior even though
// doctor never imports internal/tarsserver.
func initiativeReplyCooldown(raw string) time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || d <= 0 {
		return 45 * time.Minute
	}
	return d
}

// initiativeLastDeliverySummary reads the initiative ledger directly (the
// server process may not be running) and reports the last Speak-intent
// tick's delivery outcome, today's spoken count, and reply tracking's
// aggregate (tars#1221) — the same facts GET /v1/initiative/status exposes
// from the live Runtime. It never reads or returns composed text: the
// ledger does not store it (tars#1003). now decides which local day counts
// as "today"; doctor passes time.Now().
func initiativeLastDeliverySummary(cfg config.Config, now time.Time) (bool, initiativeDeliverySummary) {
	loc := time.Local
	if tz := strings.TrimSpace(cfg.Initiative.Timezone); tz != "" {
		if l, lerr := time.LoadLocation(tz); lerr == nil {
			loc = l
		}
	}
	now = now.In(loc)
	baseCooldown := initiativeReplyCooldown(cfg.Initiative.Cooldown)
	// emptyOutcomes is what every early-return path below reports: no
	// entries read yet, so no streak and no stopped timestamp — but still
	// the real configured cooldown (tars#1221 review), not a zero value
	// that would otherwise make a brand-new ledger report "cooldown now
	// 0s" instead of the base cooldown actually in force.
	emptyOutcomes := initiative.SummarizeOutcomes(nil, now, loc, baseCooldown, 0, time.Time{})
	if strings.TrimSpace(cfg.WorkspaceDir) == "" {
		return false, initiativeDeliverySummary{outcomes: emptyOutcomes}
	}
	ledger := initiative.OpenLedger(filepath.Join(cfg.WorkspaceDir, "_shared", "initiative", "ledger.jsonl"), 0)
	entries, err := ledger.Recent(500)
	if err != nil || len(entries) == 0 {
		return false, initiativeDeliverySummary{outcomes: emptyOutcomes}
	}
	today := now.Format("2006-01-02")
	var out initiativeDeliverySummary
	var found bool
	for _, e := range entries {
		spoken := e.Speak && (e.Mode != initiative.ModeLive || e.Delivery == initiative.DeliveryDelivered)
		if spoken && e.At.In(loc).Format("2006-01-02") == today {
			out.spokenToday++
			if e.Intent == initiative.IntentGreet {
				out.greetedToday = true
			}
		}
		if e.Delivery != "" {
			found = true
			out.at, out.intent, out.delivery, out.reason = e.At, string(e.Intent), e.Delivery, e.DeliveryReason
			out.channel, out.channelError = e.Channel, e.ChannelError
		}
	}
	consecutiveIgnored, lastStoppedAt, _ := initiative.ReplyState(entries)
	out.outcomes = initiative.SummarizeOutcomes(entries, now, loc, baseCooldown, consecutiveIgnored, lastStoppedAt)
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

// telegramPairingStoreFileName mirrors
// internal/tarsserver.telegramPairingStoreFile — doctor runs standalone
// and does not import the server package (see initiativeChatProviderAlias
// above), so this file name and its minimal on-disk shape are duplicated
// rather than shared.
const telegramPairingStoreFileName = "telegram_pairings.json"

// checkDoctorInitiativeTelegram reports one line on tars#1221 P2c's
// Telegram delivery opt-in: whether it is on, and — only when it is,
// since the setting means nothing otherwise — how many Telegram chats are
// currently paired (none/one/several), which is exactly what deciding a
// default chat to deliver to needs (the same "exactly one" rule
// internal/tarsserver's resolveDefaultChatID applies). It reads the
// pairing store file directly rather than resolving a true default chat
// id, since doctor never imports internal/tarsserver and a chat id is not
// something to print anyway (tars#1003). File-read errors are reported as
// unknown, never fatal to the rest of doctor.
func checkDoctorInitiativeTelegram(report *doctorReport, cfg config.Config) {
	if !cfg.Initiative.Telegram {
		report.add("ok", "initiative telegram", "opted out: console only when away")
		return
	}
	// channels.telegram.enabled gates this independently of the opt-in
	// (review f2): a bot_token and a prior pairing can both still be on
	// disk after the channel itself was turned off, and neither makes
	// delivery ready while it stays off (the matching runtime gate is
	// helpers_initiative.go's TelegramReady).
	if !cfg.ChannelsTelegramEnabled {
		report.add("warn", "initiative telegram", "opted in but channels.telegram.enabled is false: unreachable when away")
		return
	}
	if strings.TrimSpace(cfg.TelegramBotToken) == "" {
		report.add("warn", "initiative telegram", "opted in but channels.telegram.bot_token is empty: unreachable when away")
		return
	}
	path := filepath.Join(strings.TrimSpace(cfg.AgentRuntimePersistenceDir), telegramPairingStoreFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			report.add("warn", "initiative telegram", "opted in but no chat is paired yet: unreachable when away (pair with /pair in a Telegram DM)")
			return
		}
		report.add("warn", "initiative telegram", "opted in but the pairing store could not be read: "+err.Error())
		return
	}
	var payload struct {
		Allowed []struct {
			ChatID string `json:"chat_id"`
		} `json:"allowed"`
	}
	if strings.TrimSpace(string(data)) != "" {
		if err := json.Unmarshal(data, &payload); err != nil {
			report.add("warn", "initiative telegram", "opted in but the pairing store is unreadable: "+err.Error())
			return
		}
	}
	paired := 0
	for _, a := range payload.Allowed {
		if strings.TrimSpace(a.ChatID) != "" {
			paired++
		}
	}
	switch paired {
	case 0:
		report.add("warn", "initiative telegram", "opted in but no chat is paired yet: unreachable when away (pair with /pair in a Telegram DM)")
	case 1:
		report.add("ok", "initiative telegram", "opted in, one paired chat: ready to deliver when away")
	default:
		report.add("warn", "initiative telegram", fmt.Sprintf(
			"opted in but %d chats are paired with no single default: unreachable when away until only one remains", paired))
	}
}
