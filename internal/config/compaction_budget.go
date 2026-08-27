package config

// Derivation constants. They are deliberately conservative: overshooting the
// window costs a rejected request, while undershooting only means compacting
// a little earlier than strictly necessary.
const (
	// compactionWindowUsableFraction is how much of the window left after
	// reservations the transcript may occupy before compaction runs. The
	// remainder absorbs what this layer cannot measure — the system prompt,
	// tool schemas, retrieved memory, and the incoming turn itself, none of
	// which are in the transcript token count the trigger is compared to.
	compactionWindowUsableFraction = 0.60

	// compactionKeepRecentFractionOfTrigger sizes the recent-history floor
	// against the trigger, so a bigger window keeps proportionally more.
	compactionKeepRecentFractionOfTrigger = 0.12

	// compactionWindowSafetyMargin is held back from every window on top of
	// the output and thinking reservations, covering tokenizer drift between
	// TARS's estimate and the provider's count.
	compactionWindowSafetyMargin = 8000

	// compactionMinDerivedTrigger keeps a small or heavily reserved window
	// from deriving a trigger so low that every turn compacts.
	compactionMinDerivedTrigger = 8000
)

// CompactionBudget is the effective threshold set for one tier's history.
type CompactionBudget struct {
	TriggerTokens    int
	KeepRecentTokens int

	// Derived reports whether these came from the tier's context window
	// (true) or straight from the global compaction settings (false).
	// Callers log it so an operator can tell which regime is in force.
	Derived bool

	// Window is the context window the values were derived from; 0 when
	// Derived is false.
	Window int
}

// DeriveCompactionBudget sizes history budgeting against a tier's context
// window, reserving room for what the model will generate.
//
// The operator keeps the last word. Global compaction settings that differ
// from the shipped defaults are treated as a deliberate statement and are
// returned unchanged — that is what "existing settings become explicit
// overrides" means, and it is why a deployment that already tuned compaction
// sees no change from this function.
//
// Derivation applies only when the operator left the global settings at their
// defaults AND the tier's window is known. A gateway-hosted model with no
// documented window falls through to today's behavior; it is not guessed at.
//
// The window must hold input + reasoning + output, so the output ceiling and
// any thinking budget are subtracted before the transcript's share is taken.
func DeriveCompactionBudget(window, maxTokens, thinkingBudget int, global CompactionConfig) CompactionBudget {
	fallback := CompactionBudget{
		TriggerTokens:    global.CompactionTriggerTokens,
		KeepRecentTokens: global.CompactionKeepRecentTokens,
	}
	if window <= 0 || compactionSettingsAreCustomized(global) {
		return fallback
	}

	reserved := compactionWindowSafetyMargin
	if maxTokens > 0 {
		reserved += maxTokens
	}
	if thinkingBudget > 0 {
		reserved += thinkingBudget
	}

	usable := window - reserved
	if usable <= 0 {
		// The reservations already exceed the window. Deriving a trigger
		// here would be meaningless; leave the global settings in place and
		// let the pre-flight check report the overrun against real numbers.
		return fallback
	}

	trigger := int(float64(usable) * compactionWindowUsableFraction)
	if trigger < compactionMinDerivedTrigger {
		trigger = compactionMinDerivedTrigger
	}
	keepRecent := int(float64(trigger) * compactionKeepRecentFractionOfTrigger)
	if keepRecent < global.CompactionKeepRecentTokens && trigger >= global.CompactionTriggerTokens {
		// A window at least as large as the default regime should never keep
		// less recent history than the default did.
		keepRecent = global.CompactionKeepRecentTokens
	}
	if keepRecent >= trigger {
		keepRecent = trigger / 2
	}

	return CompactionBudget{
		TriggerTokens:    trigger,
		KeepRecentTokens: keepRecent,
		Derived:          true,
		Window:           window,
	}
}

// compactionSettingsAreCustomized reports whether the operator moved the
// global compaction knobs off their shipped values.
//
// This compares against Default() rather than tracking whether the key was
// present in YAML, because the runtime config has already had defaults
// applied by the time anything can ask. The edge case — an operator who
// writes the default value explicitly and thereby opts into derivation — is
// harmless: they get the behavior the default was chosen to approximate.
func compactionSettingsAreCustomized(global CompactionConfig) bool {
	shipped := Default().CompactionConfig
	return global.CompactionTriggerTokens != shipped.CompactionTriggerTokens ||
		global.CompactionKeepRecentTokens != shipped.CompactionKeepRecentTokens
}

// ContextWindowOverrun reports by how much a projected request exceeds the
// window, or 0 when it fits. All arguments are token counts.
//
// It is a pure projection so the caller can log loudly before sending rather
// than discovering the overflow as a provider error.
func ContextWindowOverrun(window, transcriptTokens, promptTokens, maxTokens, thinkingBudget int) int {
	if window <= 0 {
		return 0
	}
	projected := transcriptTokens + promptTokens
	if maxTokens > 0 {
		projected += maxTokens
	}
	if thinkingBudget > 0 {
		projected += thinkingBudget
	}
	if projected <= window {
		return 0
	}
	return projected - window
}
