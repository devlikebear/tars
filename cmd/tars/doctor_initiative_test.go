package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
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
	if len(report.checks) != 4 {
		t.Fatalf("checks = %+v, want initiative + two initiative-speak checks + one initiative-replies check", names)
	}
	if report.checks[0].name != "initiative" || !strings.Contains(report.checks[0].detail, "live mode") {
		t.Fatalf("first check = %+v, want live mode in the detail", report.checks[0])
	}
	foundBackend, foundDelivery, foundReplies := false, false, false
	for _, c := range report.checks[1:] {
		switch c.name {
		case "initiative speak":
			if strings.Contains(c.detail, "chat-model") {
				foundBackend = true
			}
			if strings.Contains(c.detail, "no delivery recorded yet") {
				foundDelivery = true
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
	if !foundBackend || !foundDelivery || !foundReplies {
		t.Fatalf("checks = %+v, want a speak backend line, a no-delivery-yet line, and a reply-outcomes line", report.checks[1:])
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

func TestInitiativeLastDeliverySummaryReadsLedgerNotCurrentProcess(t *testing.T) {
	workspace := t.TempDir()
	ledgerPath := workspace + "/_shared/initiative/ledger.jsonl"
	if err := os.MkdirAll(workspace+"/_shared/initiative", 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	line := fmt.Sprintf(`{"at":%q,"mode":"live","intent":"greet","speak":true,"delivery":"delivered","delivery_reason":"delivered"}`, now.Format(time.RFC3339))
	if err := os.WriteFile(ledgerPath, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	found, summary := initiativeLastDeliverySummary(config.Config{RuntimeConfig: config.RuntimeConfig{WorkspaceDir: workspace}})
	if !found || summary.delivery != "delivered" || summary.intent != "greet" || summary.spokenToday != 1 {
		t.Fatalf("summary = %+v found=%v", summary, found)
	}
}

// TestInitiativeLastDeliverySummaryReadsReplyOutcomes checks that doctor's
// reply-tracking aggregate (tars#1221) is read straight from the ledger —
// the same facts, computed by the same initiative.SummarizeOutcomes GET
// /v1/initiative/status uses from the live Runtime.
func TestInitiativeLastDeliverySummaryReadsReplyOutcomes(t *testing.T) {
	workspace := t.TempDir()
	dir := workspace + "/_shared/initiative"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
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
		Initiative:    config.InitiativeConfig{Cooldown: "45m"},
	})
	if summary.outcomes.Today.Ignored != 1 || summary.outcomes.Today.Stopped != 1 {
		t.Fatalf("outcomes = %+v, want 1 ignored and 1 stopped today", summary.outcomes)
	}
	if !summary.outcomes.StoppedToday {
		t.Fatal("StoppedToday = false, want true")
	}
	if want := (45 * time.Minute).Seconds(); summary.outcomes.EffectiveCooldownSeconds != want {
		t.Fatalf("EffectiveCooldownSeconds = %v, want %v (the later stopped outcome resets the streak the earlier ignored started)", summary.outcomes.EffectiveCooldownSeconds, want)
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
	}); summary.outcomes.EffectiveCooldownSeconds != want {
		t.Fatalf("no workspace: EffectiveCooldownSeconds = %v, want %v", summary.outcomes.EffectiveCooldownSeconds, want)
	}

	// A workspace with no ledger file yet.
	if _, summary := initiativeLastDeliverySummary(config.Config{
		RuntimeConfig: config.RuntimeConfig{WorkspaceDir: t.TempDir()},
		Initiative:    config.InitiativeConfig{Cooldown: "2h"},
	}); summary.outcomes.EffectiveCooldownSeconds != want {
		t.Fatalf("no ledger file: EffectiveCooldownSeconds = %v, want %v", summary.outcomes.EffectiveCooldownSeconds, want)
	}
}
