package config

import "testing"

func shippedCompaction() CompactionConfig {
	return Default().CompactionConfig
}

func TestDeriveCompactionBudget_UnknownWindowKeepsGlobalSettings(t *testing.T) {
	// A gateway-hosted model has no documented window. Guessing one would be
	// worse than leaving today's behavior in place.
	global := shippedCompaction()
	got := DeriveCompactionBudget(0, 16000, 0, global)
	if got.Derived {
		t.Fatal("derived a budget from an unknown window")
	}
	if got.TriggerTokens != global.CompactionTriggerTokens {
		t.Fatalf("TriggerTokens = %d, want the global %d", got.TriggerTokens, global.CompactionTriggerTokens)
	}
	if got.KeepRecentTokens != global.CompactionKeepRecentTokens {
		t.Fatalf("KeepRecentTokens = %d, want the global %d", got.KeepRecentTokens, global.CompactionKeepRecentTokens)
	}
}

func TestDeriveCompactionBudget_CustomizedSettingsWin(t *testing.T) {
	// The operator tuned compaction. That is a deliberate statement, and it
	// outranks anything this function would compute — which is what keeps an
	// existing deployment's behavior identical after upgrade.
	custom := CompactionConfig{
		CompactionTriggerTokens:    55000,
		CompactionKeepRecentTokens: 9000,
	}
	got := DeriveCompactionBudget(1000000, 16000, 0, custom)
	if got.Derived {
		t.Fatal("derived a budget over the operator's explicit settings")
	}
	if got.TriggerTokens != 55000 || got.KeepRecentTokens != 9000 {
		t.Fatalf("got %+v, want the operator's values untouched", got)
	}
}

func TestDeriveCompactionBudget_LargerWindowRaisesTheTrigger(t *testing.T) {
	// The whole point of the change: capacity that is paid for should be
	// used. A 1M model must budget more history than a 200k one.
	global := shippedCompaction()
	small := DeriveCompactionBudget(200000, 16000, 0, global)
	large := DeriveCompactionBudget(1000000, 16000, 0, global)

	if !small.Derived || !large.Derived {
		t.Fatalf("expected both to derive; small=%+v large=%+v", small, large)
	}
	if large.TriggerTokens <= small.TriggerTokens {
		t.Fatalf("1M trigger %d is not above the 200k trigger %d", large.TriggerTokens, small.TriggerTokens)
	}
	if large.TriggerTokens <= global.CompactionTriggerTokens {
		t.Fatalf("1M trigger %d did not exceed the global default %d — the extra window stays unused", large.TriggerTokens, global.CompactionTriggerTokens)
	}
}

func TestDeriveCompactionBudget_ReservesOutputAndThinkingHeadroom(t *testing.T) {
	// Input + reasoning + output must provably fit inside the window.
	const window = 200000
	global := shippedCompaction()
	cases := []struct {
		name           string
		maxTokens      int
		thinkingBudget int
	}{
		{"no reservations", 0, 0},
		{"output only", 64000, 0},
		{"output and thinking", 64000, 32000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveCompactionBudget(window, tc.maxTokens, tc.thinkingBudget, global)
			if !got.Derived {
				t.Fatalf("expected derivation, got %+v", got)
			}
			total := got.TriggerTokens + tc.maxTokens + tc.thinkingBudget
			if total > window {
				t.Fatalf("trigger %d + max_tokens %d + thinking %d = %d exceeds the window %d",
					got.TriggerTokens, tc.maxTokens, tc.thinkingBudget, total, window)
			}
		})
	}
}

func TestDeriveCompactionBudget_BiggerReservationsShrinkTheTrigger(t *testing.T) {
	global := shippedCompaction()
	lean := DeriveCompactionBudget(200000, 8000, 0, global)
	heavy := DeriveCompactionBudget(200000, 64000, 32000, global)
	if heavy.TriggerTokens >= lean.TriggerTokens {
		t.Fatalf("heavier reservations did not shrink the trigger: lean=%d heavy=%d", lean.TriggerTokens, heavy.TriggerTokens)
	}
}

func TestDeriveCompactionBudget_ReservationsExceedingWindowFallBack(t *testing.T) {
	// Nonsense sizing must not produce a nonsense trigger; leave the global
	// settings and let the pre-flight check report against real numbers.
	global := shippedCompaction()
	got := DeriveCompactionBudget(10000, 64000, 32000, global)
	if got.Derived {
		t.Fatalf("derived from a window smaller than its reservations: %+v", got)
	}
	if got.TriggerTokens != global.CompactionTriggerTokens {
		t.Fatalf("TriggerTokens = %d, want the global %d", got.TriggerTokens, global.CompactionTriggerTokens)
	}
}

func TestDeriveCompactionBudget_KeepRecentStaysBelowTheTrigger(t *testing.T) {
	global := shippedCompaction()
	for _, window := range []int{50000, 200000, 1000000} {
		got := DeriveCompactionBudget(window, 8000, 0, global)
		if got.KeepRecentTokens >= got.TriggerTokens {
			t.Errorf("window %d: keep-recent %d is not below the trigger %d", window, got.KeepRecentTokens, got.TriggerTokens)
		}
		if got.KeepRecentTokens <= 0 {
			t.Errorf("window %d: keep-recent is %d", window, got.KeepRecentTokens)
		}
	}
}

func TestContextWindowOverrun(t *testing.T) {
	cases := []struct {
		name        string
		window      int
		transcript  int
		prompt      int
		maxTokens   int
		thinking    int
		wantOverrun int
	}{
		{"unknown window never reports", 0, 999999, 999999, 128000, 0, 0},
		{"comfortable fit", 200000, 50000, 10000, 16000, 0, 0},
		{"exact fit is not an overrun", 100000, 50000, 20000, 30000, 0, 0},
		{"output pushes it over", 100000, 50000, 20000, 40000, 0, 10000},
		{"thinking pushes it over", 100000, 50000, 20000, 20000, 30000, 20000},
		{"transcript alone overruns", 100000, 150000, 0, 0, 0, 50000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ContextWindowOverrun(tc.window, tc.transcript, tc.prompt, tc.maxTokens, tc.thinking)
			if got != tc.wantOverrun {
				t.Fatalf("ContextWindowOverrun = %d, want %d", got, tc.wantOverrun)
			}
		})
	}
}

func TestResolveLLMTier_ContextWindowExplicitDefaultedAndUnknown(t *testing.T) {
	cases := []struct {
		name       string
		model      string
		configured int
		want       int
	}{
		{"explicit wins over the documented window", "claude-opus-5", 250000, 250000},
		{"explicit wins for an unknown model", "MiniMax-M2.7", 192000, 192000},
		{"unset takes the documented window", "claude-opus-5", 0, 1000000},
		{"unset takes the documented window for haiku", "claude-haiku-4-5", 0, 200000},
		{"unknown model stays unknown", "MiniMax-M2.7", 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := limitsTestConfig(LLMTierBinding{Provider: "pool", Model: tc.model, ContextWindow: tc.configured})
			resolved, err := ResolveLLMTier(&cfg, "heavy")
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if resolved.ContextWindow != tc.want {
				t.Fatalf("ContextWindow = %d, want %d", resolved.ContextWindow, tc.want)
			}
		})
	}
}

func TestResolveLLMTier_ChangingOnlyTheModelChangesTheBudget(t *testing.T) {
	// The headline acceptance criterion, end to end through the resolver:
	// swap the model and nothing else, and the history budget moves.
	global := shippedCompaction()
	budgetFor := func(model string) CompactionBudget {
		cfg := limitsTestConfig(LLMTierBinding{Provider: "pool", Model: model})
		resolved, err := ResolveLLMTier(&cfg, "heavy")
		if err != nil {
			t.Fatalf("resolve %s: %v", model, err)
		}
		return DeriveCompactionBudget(resolved.ContextWindow, resolved.MaxTokens, resolved.ThinkingBudget, global)
	}

	haiku := budgetFor("claude-haiku-4-5")
	opus := budgetFor("claude-opus-5")
	if haiku.TriggerTokens == opus.TriggerTokens {
		t.Fatalf("both models produced trigger %d — the model does not change the budget", haiku.TriggerTokens)
	}
	if opus.TriggerTokens <= haiku.TriggerTokens {
		t.Fatalf("the 1M model budgeted %d, not more than the 200k model's %d", opus.TriggerTokens, haiku.TriggerTokens)
	}
}

func TestDeriveCompactionBudget_UnchangedForAConfigThatSetsNeitherField(t *testing.T) {
	// Regression guard for the upgrade path: a tier on a model TARS does not
	// recognize, with untouched global settings, must behave exactly as it
	// did before this feature existed.
	cfg := limitsTestConfig(LLMTierBinding{Provider: "pool", Model: "MiniMax-M2.7"})
	resolved, err := ResolveLLMTier(&cfg, "heavy")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	global := shippedCompaction()
	got := DeriveCompactionBudget(resolved.ContextWindow, resolved.MaxTokens, resolved.ThinkingBudget, global)
	if got.Derived {
		t.Fatal("derived a budget for a config that opted into nothing")
	}
	if got.TriggerTokens != global.CompactionTriggerTokens || got.KeepRecentTokens != global.CompactionKeepRecentTokens {
		t.Fatalf("got %+v, want the shipped defaults unchanged", got)
	}
}
