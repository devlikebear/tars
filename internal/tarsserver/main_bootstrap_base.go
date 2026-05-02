package tarsserver

import (
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/usage"
	"github.com/rs/zerolog"
)

// buildBaseDeps initializes the runtime dependencies that do not need
// the LLM router: workspace, session store, usage tracker, and the
// API auth security gate. It is safe to call even when LLM config is
// missing — the result powers the setup-only boot path (Phase 2 of
// the onboarding plan).
//
// Stages and their failure semantics:
//   - validate_config        — refuses insecure auth modes without opt-in
//   - ensure_workspace       — workspace dir creation/validation
//   - init_usage             — usage tracker construction
//
// Returns a partially populated runtimeDeps with cfg / sessionStore /
// sessionStoreResolver / usageTracker set. LLM-bound fields stay nil
// for the caller to fill via buildLLMDeps.
func buildBaseDeps(opts *options, cfg config.Config, nowFn func() time.Time, _ zerolog.Logger) (runtimeDeps, error) {
	if opts == nil {
		return runtimeDeps{}, fmt.Errorf("options are required")
	}

	if err := validateAPIAuthSecurity(cfg); err != nil {
		return runtimeDeps{}, &runtimeDepsError{stage: "validate_config", err: err}
	}

	if err := memory.EnsureWorkspace(cfg.WorkspaceDir); err != nil {
		return runtimeDeps{}, &runtimeDepsError{stage: "ensure_workspace", err: err}
	}

	deps := runtimeDeps{
		cfg:          cfg,
		sessionStore: session.NewStore(cfg.WorkspaceDir),
	}
	deps.sessionStoreResolver = newWorkspaceSessionStoreResolver(cfg.WorkspaceDir, deps.sessionStore)

	priceOverrides := map[string]usage.ModelPrice{}
	for key, value := range cfg.UsagePriceOverrides {
		priceOverrides[strings.TrimSpace(strings.ToLower(key))] = usage.ModelPrice{
			InputPer1MUSD:      value.InputPer1MUSD,
			OutputPer1MUSD:     value.OutputPer1MUSD,
			CacheReadPer1MUSD:  value.CacheReadPer1MUSD,
			CacheWritePer1MUSD: value.CacheWritePer1MUSD,
		}
	}
	tracker, err := usage.NewTracker(cfg.WorkspaceDir, usage.TrackerOptions{
		Now: nowFn,
		InitialLimits: usage.Limits{
			DailyUSD:    cfg.UsageLimitDailyUSD,
			WeeklyUSD:   cfg.UsageLimitWeeklyUSD,
			MonthlyUSD:  cfg.UsageLimitMonthlyUSD,
			DailyTokens: cfg.UsageDailyTokenBudget,
			Mode:        cfg.UsageLimitMode,
		},
		PriceOverrides: priceOverrides,
	})
	if err != nil {
		return runtimeDeps{}, &runtimeDepsError{stage: "init_usage", err: err}
	}
	deps.usageTracker = tracker

	return deps, nil
}
