package llm

import (
	"fmt"
	"strings"
)

// Tier identifies a named LLM configuration bundle (provider+model+knobs).
// Three tiers are supported so that callers can target a class of model
// (heavy reasoning / standard / fast-light) without knowing the concrete
// provider or model name at the call site.
type Tier string

const (
	// TierHeavy is for high-reasoning work: planning, complex code changes,
	// architectural decisions, long-context synthesis.
	TierHeavy Tier = "heavy"
	// TierStandard is the general-purpose default used for chat and most
	// gateway agent work.
	TierStandard Tier = "standard"
	// TierLight is for fast, cheap operations: summarization, classification,
	// memory hooks, pulse deciders, reflection compaction.
	TierLight Tier = "light"
)

// AllTiers returns the full set of supported tiers in canonical order.
func AllTiers() []Tier {
	return []Tier{TierHeavy, TierStandard, TierLight}
}

// Valid reports whether t is one of the known tier constants.
func (t Tier) Valid() bool {
	switch t {
	case TierHeavy, TierStandard, TierLight:
		return true
	default:
		return false
	}
}

// String returns the canonical string form of the tier.
func (t Tier) String() string { return string(t) }

// ParseTier parses a tier name, returning an error for unknown values.
// Empty input is treated as an error so that callers can distinguish
// "unset" (use default) from "explicit but invalid".
func ParseTier(raw string) (Tier, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return "", fmt.Errorf("tier is empty")
	}
	tier := Tier(normalized)
	if !tier.Valid() {
		return "", fmt.Errorf("unknown tier %q (want heavy, standard, or light)", raw)
	}
	return tier, nil
}
