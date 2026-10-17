package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/config"
)

func TestCheckDoctorInitiative(t *testing.T) {
	only := func(t *testing.T, r doctorReport) doctorCheck {
		t.Helper()
		if len(r.checks) != 1 || r.checks[0].name != "initiative" {
			t.Fatalf("checks = %+v", r.checks)
		}
		return r.checks[0]
	}

	var disabled doctorReport
	checkDoctorInitiative(&disabled, config.Config{})
	if c := only(t, disabled); c.status != "ok" || c.detail != "disabled" {
		t.Fatalf("disabled = %+v", c)
	}

	var noBackend doctorReport
	checkDoctorInitiative(&noBackend, config.Config{Initiative: config.InitiativeConfig{Enabled: true, Backend: "jev"}})
	if c := only(t, noBackend); c.status != "warn" || !strings.Contains(c.detail, "jev.base_url") {
		t.Fatalf("no backend = %+v", c)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"answers":{"ok":{"noul":0.9}}}`))
	}))
	defer srv.Close()
	var ok doctorReport
	checkDoctorInitiative(&ok, config.Config{Initiative: config.InitiativeConfig{Enabled: true, Backend: "jev"}, Jev: config.JevConfig{BaseURL: srv.URL}})
	if c := only(t, ok); c.status != "ok" || !strings.Contains(c.detail, "shadow") || !strings.Contains(c.detail, "loopback") || !strings.Contains(c.detail, "backend jev") {
		t.Fatalf("reachable = %+v", c)
	}

	srv.Close()
	var down doctorReport
	checkDoctorInitiative(&down, config.Config{Initiative: config.InitiativeConfig{Enabled: true, Backend: "jev"}, Jev: config.JevConfig{BaseURL: srv.URL}})
	if c := only(t, down); c.status != "warn" || !strings.Contains(c.detail, "unreachable") {
		t.Fatalf("down = %+v", c)
	}
}

func TestCheckDoctorInitiativeLLMBackend(t *testing.T) {
	only := func(t *testing.T, r doctorReport) doctorCheck {
		t.Helper()
		if len(r.checks) != 1 || r.checks[0].name != "initiative" {
			t.Fatalf("checks = %+v", r.checks)
		}
		return r.checks[0]
	}

	llmConfig := func(lightAlias string) config.Config {
		return config.Config{
			Initiative: config.InitiativeConfig{Enabled: true, Backend: "llm"},
			LLMConfig: config.LLMConfig{
				LLMDefaultTier:  "standard",
				LLMRoleDefaults: map[string]string{"initiative": "light"},
				LLMProviders: map[string]config.LLMProviderSettings{
					"shared": {Kind: "openai", BaseURL: "http://shared.example", APIKey: "k"},
					"other":  {Kind: "openai", BaseURL: "http://other.example", APIKey: "k2"},
				},
				LLMTiers: map[string]config.LLMTierBinding{
					"heavy":    {Provider: "shared", Model: "heavy-model"},
					"standard": {Provider: "shared", Model: "chat-model"},
					"light":    {Provider: lightAlias, Model: "light-model"},
				},
			},
		}
	}

	var same doctorReport
	checkDoctorInitiative(&same, llmConfig("shared"))
	if c := only(t, same); c.status != "ok" || !strings.Contains(c.detail, "backend llm") ||
		!strings.Contains(c.detail, "reads user text") {
		t.Fatalf("same provider = %+v", c)
	}

	var different doctorReport
	checkDoctorInitiative(&different, llmConfig("other"))
	if c := only(t, different); c.status != "ok" || !strings.Contains(c.detail, "metadata only") {
		t.Fatalf("different provider = %+v", c)
	}

	var missingTier doctorReport
	cfg := llmConfig("shared")
	cfg.LLMRoleDefaults["initiative"] = "nonexistent"
	checkDoctorInitiative(&missingTier, cfg)
	if c := only(t, missingTier); c.status != "warn" || !strings.Contains(c.detail, "not configured") {
		t.Fatalf("missing tier = %+v", c)
	}

	var antigravity doctorReport
	cfg2 := llmConfig("shared")
	cfg2.LLMProviders["shared"] = config.LLMProviderSettings{Kind: "antigravity-cli"}
	checkDoctorInitiative(&antigravity, cfg2)
	if c := only(t, antigravity); c.status != "warn" || !strings.Contains(c.detail, "antigravity-cli") {
		t.Fatalf("antigravity-cli = %+v", c)
	}
}

func TestCheckDoctorInitiativeLiveSpeakBackend(t *testing.T) {
	cfg := config.Config{
		Initiative: config.InitiativeConfig{Enabled: true, Backend: "llm", Mode: "live"},
		LLMConfig: config.LLMConfig{
			LLMDefaultTier:  "standard",
			LLMRoleDefaults: map[string]string{"initiative": "light", "initiative_speak": "standard"},
			LLMProviders: map[string]config.LLMProviderSettings{
				"shared": {Kind: "openai", BaseURL: "http://shared.example", APIKey: "k"},
			},
			LLMTiers: map[string]config.LLMTierBinding{
				"heavy":    {Provider: "shared", Model: "heavy-model"},
				"standard": {Provider: "shared", Model: "chat-model"},
				"light":    {Provider: "shared", Model: "light-model"},
			},
		},
	}
	var report doctorReport
	checkDoctorInitiative(&report, cfg)
	names := make([]string, len(report.checks))
	for i, c := range report.checks {
		names[i] = c.name
	}
	if len(report.checks) != 5 {
		t.Fatalf("checks = %+v, want initiative + two initiative-speak checks + initiative-telegram + initiative-replies check", names)
	}
	if report.checks[0].name != "initiative" || !strings.Contains(report.checks[0].detail, "live mode") {
		t.Fatalf("first check = %+v, want live mode in the detail", report.checks[0])
	}
	foundBackend, foundDelivery, foundTelegram, foundReplies := false, false, false, false
	for _, c := range report.checks[1:] {
		switch c.name {
		case "initiative speak":
			if strings.Contains(c.detail, "chat-model") {
				foundBackend = true
			}
			if strings.Contains(c.detail, "no delivery recorded yet") {
				foundDelivery = true
			}
		case "initiative telegram":
			// Telegram is false by default — this leaves foundTelegram
			// true. tars#1221 P2c.
			if c.status == "ok" && strings.Contains(c.detail, "opted out") {
				foundTelegram = true
			}
		case "initiative replies":
			// Review fix: an empty ledger (no deliveries yet) must still
			// report the real configured cooldown (default 45m here,
			// Cooldown left unset), not "cooldown now 0s".
			if strings.Contains(c.detail, "0 replied") && strings.Contains(c.detail, "cooldown now 45m0s") {
				foundReplies = true
			}
		default:
			t.Fatalf("unexpected check name %q", c.name)
		}
	}
	if !foundBackend || !foundDelivery || !foundTelegram || !foundReplies {
		t.Fatalf("checks = %+v, want a speak backend line, a no-delivery-yet line, an opted-out telegram line, and a reply-outcomes line", report.checks[1:])
	}
}

func TestCheckDoctorInitiativeLiveSpeakUnavailable(t *testing.T) {
	cfg := config.Config{
		Initiative: config.InitiativeConfig{Enabled: true, Backend: "llm", Mode: "live"},
		LLMConfig: config.LLMConfig{
			LLMRoleDefaults: map[string]string{"initiative": "light"},
			LLMProviders: map[string]config.LLMProviderSettings{
				"shared": {Kind: "openai", BaseURL: "http://shared.example", APIKey: "k"},
			},
			LLMTiers: map[string]config.LLMTierBinding{
				"light": {Provider: "shared", Model: "light-model"},
			},
		},
	}
	var report doctorReport
	checkDoctorInitiative(&report, cfg)
	var speakCheck *doctorCheck
	for i := range report.checks {
		if report.checks[i].name == "initiative speak" && strings.Contains(report.checks[i].detail, "no tier") {
			speakCheck = &report.checks[i]
		}
	}
	if speakCheck == nil || speakCheck.status != "warn" {
		t.Fatalf("checks = %+v, want a warn initiative speak check with no tier", report.checks)
	}
}

// TestCheckDoctorInitiativePresence checks requirement #4: doctor reports
// the last console presence signal's age and whether it reads as present
// right now (tars#1249), reading presence.json directly — no running
// server needed — and never a session id or message text.
func TestCheckDoctorInitiativePresence(t *testing.T) {
	workspace := t.TempDir()
	cfg := config.Config{
		RuntimeConfig: config.RuntimeConfig{WorkspaceDir: workspace},
		Initiative:    config.InitiativeConfig{Enabled: true, Backend: "jev"},
		Jev:           config.JevConfig{BaseURL: ""},
	}
	findPresence := func(r doctorReport) *doctorCheck {
		for i := range r.checks {
			if r.checks[i].name == "initiative presence" {
				return &r.checks[i]
			}
		}
		return nil
	}

	// No presence.json at all yet.
	var none doctorReport
	checkDoctorInitiative(&none, cfg)
	if c := findPresence(none); c == nil || c.status != "ok" || !strings.Contains(c.detail, "no presence signal recorded") {
		t.Fatalf("no-file presence check = %+v", c)
	}

	// A fresh signal reads as present now.
	path := filepath.Join(workspace, "_shared", "initiative", "presence.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(fmt.Sprintf(`{"last_at":%q,"arrived_at":%q}`, time.Now().Format(time.RFC3339), time.Now().Format(time.RFC3339))), 0o644); err != nil {
		t.Fatal(err)
	}
	var present doctorReport
	checkDoctorInitiative(&present, cfg)
	if c := findPresence(present); c == nil || c.status != "ok" || !strings.Contains(c.detail, "present now") {
		t.Fatalf("fresh presence check = %+v", c)
	}
	if c := findPresence(present); c != nil && (strings.Contains(c.detail, "session") || strings.Contains(c.detail, "\"")) {
		t.Fatalf("presence detail leaks text/identifiers: %q", c.detail)
	}

	// A stale signal (past presenceTTL) reads as not present.
	stale := time.Now().Add(-time.Hour).Format(time.RFC3339)
	if err := os.WriteFile(path, []byte(fmt.Sprintf(`{"last_at":%q,"arrived_at":%q}`, stale, stale)), 0o644); err != nil {
		t.Fatal(err)
	}
	var notPresent doctorReport
	checkDoctorInitiative(&notPresent, cfg)
	if c := findPresence(notPresent); c == nil || c.status != "ok" || !strings.Contains(c.detail, "not present") {
		t.Fatalf("stale presence check = %+v", c)
	}
}

func TestInitiativeLastDeliverySummaryReadsLedgerNotCurrentProcess(t *testing.T) {
	workspace := t.TempDir()
	ledgerPath := workspace + "/_shared/initiative/ledger.jsonl"
	if err := os.MkdirAll(workspace+"/_shared/initiative", 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	line := fmt.Sprintf(`{"at":%q,"mode":"live","intent":"greet","speak":true,"delivery":"delivered","delivery_reason":"delivered"}`, now.Format(time.RFC3339))
	if err := os.WriteFile(ledgerPath, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	found, summary := initiativeLastDeliverySummary(config.Config{
		RuntimeConfig: config.RuntimeConfig{WorkspaceDir: workspace},
		Initiative:    config.InitiativeConfig{Timezone: "UTC"},
	}, now)
	if !found || summary.delivery != "delivered" || summary.intent != "greet" || summary.spokenToday != 1 {
		t.Fatalf("summary = %+v found=%v", summary, found)
	}
}

// TestInitiativeLastDeliverySummaryReadsReplyOutcomes checks that doctor's
// reply-tracking aggregate (tars#1221) is read straight from the ledger —
// the same facts, computed by the same initiative.SummarizeOutcomes GET
// /v1/initiative/status uses from the live Runtime. The clock is pinned so
// "today" does not depend on when the test runs: at 00:15 the outcomes
// written 20–50 minutes earlier belong to yesterday.
func TestInitiativeLastDeliverySummaryReadsReplyOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name      string
		now       time.Time
		wantToday int
	}{
		{"midday", time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC), 1},
		{"just before midnight", time.Date(2026, 10, 17, 23, 59, 0, 0, time.UTC), 1},
		{"just after midnight", time.Date(2026, 10, 17, 0, 15, 0, 0, time.UTC), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workspace := t.TempDir()
			dir := workspace + "/_shared/initiative"
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			now := tc.now
			lines := []string{
				fmt.Sprintf(`{"id":"e1","at":%q,"mode":"live","intent":"greet","speak":true,"delivery":"delivered","delivery_reason":"delivered"}`, now.Add(-time.Hour).Format(time.RFC3339)),
				fmt.Sprintf(`{"at":%q,"mode":"live","kind":"outcome","ref":"e1","outcome":"ignored"}`, now.Add(-50*time.Minute).Format(time.RFC3339)),
				fmt.Sprintf(`{"id":"e2","at":%q,"mode":"live","intent":"check_in","speak":true,"delivery":"delivered","delivery_reason":"delivered"}`, now.Add(-30*time.Minute).Format(time.RFC3339)),
				fmt.Sprintf(`{"at":%q,"mode":"live","kind":"outcome","ref":"e2","outcome":"stopped"}`, now.Add(-20*time.Minute).Format(time.RFC3339)),
			}
			if err := os.WriteFile(dir+"/ledger.jsonl", []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			_, summary := initiativeLastDeliverySummary(config.Config{
				RuntimeConfig: config.RuntimeConfig{WorkspaceDir: workspace},
				Initiative:    config.InitiativeConfig{Cooldown: "45m", Timezone: "UTC"},
			}, now)
			if summary.outcomes.Recent.Ignored != 1 || summary.outcomes.Recent.Stopped != 1 {
				t.Fatalf("outcomes = %+v, want 1 ignored and 1 stopped recently", summary.outcomes)
			}
			if summary.outcomes.Today.Ignored != tc.wantToday || summary.outcomes.Today.Stopped != tc.wantToday {
				t.Fatalf("outcomes = %+v, want %d ignored and %d stopped today", summary.outcomes, tc.wantToday, tc.wantToday)
			}
			if want := tc.wantToday == 1; summary.outcomes.StoppedToday != want {
				t.Fatalf("StoppedToday = %v, want %v", summary.outcomes.StoppedToday, want)
			}
			if want := (45 * time.Minute).Seconds(); summary.outcomes.EffectiveCooldownSeconds != want {
				t.Fatalf("EffectiveCooldownSeconds = %v, want %v (the later stopped outcome resets the streak the earlier ignored started)", summary.outcomes.EffectiveCooldownSeconds, want)
			}
		})
	}
}

// TestInitiativeLastDeliverySummaryReportsConfiguredCooldownWithEmptyLedger
// is the regression guard for the review finding: a brand-new live-mode
// setup with no deliveries yet (empty or missing ledger) must still report
// the real configured cooldown, not a zero value that would read as
// "cooldown disabled" in `tars doctor`'s output.
func TestInitiativeLastDeliverySummaryReportsConfiguredCooldownWithEmptyLedger(t *testing.T) {
	want := (2 * time.Hour).Seconds()

	// No workspace at all.
	if _, summary := initiativeLastDeliverySummary(config.Config{
		Initiative: config.InitiativeConfig{Cooldown: "2h"},
	}, time.Now()); summary.outcomes.EffectiveCooldownSeconds != want {
		t.Fatalf("no workspace: EffectiveCooldownSeconds = %v, want %v", summary.outcomes.EffectiveCooldownSeconds, want)
	}

	// A workspace with no ledger file yet.
	if _, summary := initiativeLastDeliverySummary(config.Config{
		RuntimeConfig: config.RuntimeConfig{WorkspaceDir: t.TempDir()},
		Initiative:    config.InitiativeConfig{Cooldown: "2h"},
	}, time.Now()); summary.outcomes.EffectiveCooldownSeconds != want {
		t.Fatalf("no ledger file: EffectiveCooldownSeconds = %v, want %v", summary.outcomes.EffectiveCooldownSeconds, want)
	}
}

// TestCheckDoctorInitiativeTelegram covers tars#1221 P2c's doctor line:
// opted out is the quiet default; opted in needs both a bot token and a
// resolvable single default chat to read "ready".
func TestCheckDoctorInitiativeTelegram(t *testing.T) {
	only := func(t *testing.T, r doctorReport) doctorCheck {
		t.Helper()
		if len(r.checks) != 1 || r.checks[0].name != "initiative telegram" {
			t.Fatalf("checks = %+v", r.checks)
		}
		return r.checks[0]
	}

	var off doctorReport
	checkDoctorInitiativeTelegram(&off, config.Config{})
	if c := only(t, off); c.status != "ok" || !strings.Contains(c.detail, "opted out") {
		t.Fatalf("opted out = %+v", c)
	}

	// Review finding f2: channels.telegram.enabled is checked before
	// bot_token/pairing — opted in with the channel itself still off
	// (the zero value here) must report that, not fall through to the
	// bot_token or pairing checks below.
	var channelDisabled doctorReport
	checkDoctorInitiativeTelegram(&channelDisabled, config.Config{Initiative: config.InitiativeConfig{Telegram: true}})
	if c := only(t, channelDisabled); c.status != "warn" || !strings.Contains(c.detail, "channels.telegram.enabled") {
		t.Fatalf("channel disabled = %+v", c)
	}

	var noToken doctorReport
	checkDoctorInitiativeTelegram(&noToken, config.Config{
		Initiative:    config.InitiativeConfig{Telegram: true},
		ChannelConfig: config.ChannelConfig{ChannelsTelegramEnabled: true},
	})
	if c := only(t, noToken); c.status != "warn" || !strings.Contains(c.detail, "bot_token") {
		t.Fatalf("no bot token = %+v", c)
	}

	persistenceDir := t.TempDir()
	cfgWithToken := config.Config{
		Initiative:    config.InitiativeConfig{Telegram: true},
		ChannelConfig: config.ChannelConfig{ChannelsTelegramEnabled: true},
	}
	cfgWithToken.TelegramBotToken = "token-123"
	cfgWithToken.AgentRuntimePersistenceDir = persistenceDir

	var noPairingFile doctorReport
	checkDoctorInitiativeTelegram(&noPairingFile, cfgWithToken)
	if c := only(t, noPairingFile); c.status != "warn" || !strings.Contains(c.detail, "no chat is paired") {
		t.Fatalf("no pairing file = %+v", c)
	}

	writePairings := func(t *testing.T, allowed string) {
		t.Helper()
		path := filepath.Join(persistenceDir, "telegram_pairings.json")
		if err := os.WriteFile(path, []byte(`{"allowed":[`+allowed+`]}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writePairings(t, `{"user_id":1,"chat_id":"101"}`)
	var onePaired doctorReport
	checkDoctorInitiativeTelegram(&onePaired, cfgWithToken)
	if c := only(t, onePaired); c.status != "ok" || !strings.Contains(c.detail, "ready") {
		t.Fatalf("one paired chat = %+v", c)
	}

	// Review finding f2's exact regression scenario: bot_token still set
	// and a chat already paired on disk, but channels.telegram.enabled is
	// false — must report the channel as the blocker, never "ready".
	cfgChannelDisabledWithPairing := cfgWithToken
	cfgChannelDisabledWithPairing.ChannelsTelegramEnabled = false
	var disabledWithPairing doctorReport
	checkDoctorInitiativeTelegram(&disabledWithPairing, cfgChannelDisabledWithPairing)
	if c := only(t, disabledWithPairing); c.status != "warn" || !strings.Contains(c.detail, "channels.telegram.enabled") || strings.Contains(c.detail, "ready") {
		t.Fatalf("channel disabled with a bot_token and a paired chat = %+v, want the channel-disabled warning, never ready", c)
	}

	writePairings(t, `{"user_id":1,"chat_id":"101"},{"user_id":2,"chat_id":"202"}`)
	var manyPaired doctorReport
	checkDoctorInitiativeTelegram(&manyPaired, cfgWithToken)
	if c := only(t, manyPaired); c.status != "warn" || !strings.Contains(c.detail, "2 chats") {
		t.Fatalf("multiple paired chats = %+v", c)
	}
}
