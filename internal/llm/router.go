package llm

import (
	"errors"
	"fmt"
	"maps"
)

// TierResolution describes how a Router resolved a request for a client.
// It is returned alongside the Client so that callers can log or emit
// events that tell the user exactly which tier and model served the call.
type TierResolution struct {
	// Tier is the tier that was selected.
	Tier Tier
	// Role is the role the caller asked for; zero value when the caller
	// asked for a tier directly via ClientForTier.
	Role Role
	// Provider is the provider identifier backing the selected tier
	// (e.g. "anthropic", "openai", "gemini-native").
	Provider string
	// Model is the concrete model name backing the selected tier.
	Model string
	// Source describes why this tier was picked. One of:
	// "role" (role→tier map), "default" (fell back to DefaultTier),
	// "explicit" (ClientForTier was called).
	Source string
}

// Router resolves (Role | Tier) to a concrete llm.Client.
//
// The intended usage pattern is: construct exactly one Router at server
// startup with three pre-wrapped tier clients; hand it to every subsystem
// (chat, pulse, reflection, gateway, compaction); callers ask for a client
// by Role and remain oblivious to which tier or model actually served them.
type Router interface {
	// ClientFor returns the client for the given role, applying the
	// configured Role→Tier mapping. Unknown roles fall back to DefaultTier.
	ClientFor(role Role) (Client, TierResolution, error)

	// ClientForTier returns the client for an explicitly requested tier.
	// This is used by callers that already know the tier (e.g. a sub-agent
	// task has set an explicit tier override).
	ClientForTier(tier Tier) (Client, TierResolution, error)

	// DefaultTier reports the tier used when a role has no explicit mapping.
	DefaultTier() Tier

	// TierForRole reports which tier the given role resolves to without
	// fetching the client. Returns DefaultTier when the role is not mapped.
	TierForRole(role Role) Tier

	// Close releases any resources held by backing clients. Implementations
	// may be no-ops when the underlying clients have nothing to close.
	Close() error
}

// TierEntry is one tier's concrete binding: the client to use and the
// provider/model labels that identify it (for logs and usage tracking).
type TierEntry struct {
	Client   Client
	Provider string
	Model    string
}

// RouterConfig is the input to NewRouter. The caller is responsible for
// constructing and (optionally) wrapping the clients for each tier before
// handing them to the router; this keeps the llm package free of a
// dependency on internal/usage.
type RouterConfig struct {
	// Tiers maps tier → client binding. All three tiers (heavy/standard/light)
	// should be present for production use; NewRouter will error if any is
	// missing, although the backing clients may be identical when the caller
	// wants a single model everywhere (legacy mode).
	Tiers map[Tier]TierEntry

	// DefaultTier is used when a role has no explicit mapping. Must be one
	// of the tiers present in Tiers.
	DefaultTier Tier

	// RoleDefaults maps role → tier. Roles not present here fall back to
	// DefaultTier. An unknown role in the map is silently ignored — the
	// Router validates only the tier side.
	RoleDefaults map[Role]Tier
}

// NewRouter validates the config and returns a Router ready to serve
// clients. Returns an error if any required tier is missing or if
// RoleDefaults references an unknown tier.
func NewRouter(cfg RouterConfig) (Router, error) {
	if cfg.DefaultTier == "" {
		return nil, errors.New("router: DefaultTier is empty")
	}
	if !cfg.DefaultTier.Valid() {
		return nil, fmt.Errorf("router: DefaultTier %q is not a known tier", cfg.DefaultTier)
	}
	if len(cfg.Tiers) == 0 {
		return nil, errors.New("router: Tiers map is empty")
	}
	for _, tier := range AllTiers() {
		entry, ok := cfg.Tiers[tier]
		if !ok {
			return nil, fmt.Errorf("router: tier %q is missing from Tiers map", tier)
		}
		if entry.Client == nil {
			return nil, fmt.Errorf("router: tier %q has nil client", tier)
		}
	}
	if _, ok := cfg.Tiers[cfg.DefaultTier]; !ok {
		return nil, fmt.Errorf("router: DefaultTier %q is not present in Tiers", cfg.DefaultTier)
	}
	roles := make(map[Role]Tier, len(cfg.RoleDefaults))
	for role, tier := range cfg.RoleDefaults {
		if !tier.Valid() {
			return nil, fmt.Errorf("router: role %q maps to unknown tier %q", role, tier)
		}
		if _, ok := cfg.Tiers[tier]; !ok {
			return nil, fmt.Errorf("router: role %q maps to tier %q which is not in Tiers", role, tier)
		}
		roles[role] = tier
	}
	tiers := make(map[Tier]TierEntry, len(cfg.Tiers))
	maps.Copy(tiers, cfg.Tiers)
	return &multiTierRouter{
		tiers:        tiers,
		defaultTier:  cfg.DefaultTier,
		roleDefaults: roles,
	}, nil
}

type multiTierRouter struct {
	tiers        map[Tier]TierEntry
	defaultTier  Tier
	roleDefaults map[Role]Tier
}

func (r *multiTierRouter) DefaultTier() Tier { return r.defaultTier }

func (r *multiTierRouter) TierForRole(role Role) Tier {
	if tier, ok := r.roleDefaults[role]; ok {
		return tier
	}
	return r.defaultTier
}

func (r *multiTierRouter) ClientFor(role Role) (Client, TierResolution, error) {
	tier := r.defaultTier
	source := "default"
	if mapped, ok := r.roleDefaults[role]; ok {
		tier = mapped
		source = "role"
	}
	entry, ok := r.tiers[tier]
	if !ok {
		return nil, TierResolution{}, fmt.Errorf("router: tier %q is not configured", tier)
	}
	return entry.Client, TierResolution{
		Tier:     tier,
		Role:     role,
		Provider: entry.Provider,
		Model:    entry.Model,
		Source:   source,
	}, nil
}

func (r *multiTierRouter) ClientForTier(tier Tier) (Client, TierResolution, error) {
	if !tier.Valid() {
		return nil, TierResolution{}, fmt.Errorf("router: tier %q is not a known tier", tier)
	}
	entry, ok := r.tiers[tier]
	if !ok {
		return nil, TierResolution{}, fmt.Errorf("router: tier %q is not configured", tier)
	}
	return entry.Client, TierResolution{
		Tier:     tier,
		Provider: entry.Provider,
		Model:    entry.Model,
		Source:   "explicit",
	}, nil
}

func (r *multiTierRouter) Close() error {
	// llm.Client currently has no Close method; reserved for future use.
	return nil
}
