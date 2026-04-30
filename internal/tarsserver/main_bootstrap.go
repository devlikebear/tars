package tarsserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/usage"
	"github.com/rs/zerolog"
)

type runtimeDeps struct {
	cfg                  config.Config
	sessionStore         *session.Store
	sessionStoreResolver func(workspaceID string) *session.Store
	llmRouter            llm.Router
	usageTracker         *usage.Tracker
	runPrompt            func(ctx context.Context, runLabel string, prompt string) (string, error)
	runPromptWithTools   agentRuntimePromptRunner
}

type runtimeDepsError struct {
	stage string
	err   error
}

func (e *runtimeDepsError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *runtimeDepsError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// loadConfigForServe resolves and loads the config file, applies CLI
// overrides for Mode and WorkspaceDir, and returns the merged config. It
// performs no further validation or workspace bootstrapping — those are
// the responsibility of buildRuntimeDeps. Calling this early lets Serve()
// construct the runtime logger from final config values without a second
// reconfigure pass.
func loadConfigForServe(opts *options) (config.Config, error) {
	if opts == nil {
		return config.Config{}, fmt.Errorf("options are required")
	}
	cfg, err := config.Load(config.ResolveConfigPath(opts.ConfigPath))
	if err != nil {
		return config.Config{}, &runtimeDepsError{stage: "load_config", err: err}
	}
	if strings.TrimSpace(opts.WorkspaceDir) != "" {
		cfg.WorkspaceDir = strings.TrimSpace(opts.WorkspaceDir)
	}
	return cfg, nil
}

func buildRuntimeDeps(opts *options, cfg config.Config, nowFn func() time.Time, logger zerolog.Logger) (runtimeDeps, error) {
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
			DailyUSD:   cfg.UsageLimitDailyUSD,
			WeeklyUSD:  cfg.UsageLimitWeeklyUSD,
			MonthlyUSD: cfg.UsageLimitMonthlyUSD,
			Mode:       cfg.UsageLimitMode,
		},
		PriceOverrides: priceOverrides,
	})
	if err != nil {
		return runtimeDeps{}, &runtimeDepsError{stage: "init_usage", err: err}
	}
	deps.usageTracker = tracker

	router, err := buildLLMRouter(cfg, tracker)
	if err != nil {
		return runtimeDeps{}, &runtimeDepsError{stage: "init_llm", err: err}
	}
	semanticCfg := semanticMemoryConfigFromConfig(cfg)
	if err := validateMemoryBackend(cfg.MemoryBackend); err != nil {
		return runtimeDeps{}, &runtimeDepsError{stage: "init_memory_backend", err: err}
	}
	if err := memory.ValidateSemanticConfig(semanticCfg); err != nil {
		return runtimeDeps{}, &runtimeDepsError{stage: "init_semantic_memory", err: err}
	}
	deps.llmRouter = router
	logger.Debug().Msg("llm router initialized")
	deps.runPromptWithTools = newAgentPromptRunnerWithToolsAndMemory(cfg, cfg.WorkspaceDir, nil, deps.llmRouter, deps.usageTracker, cfg.AgentMaxIterations, logger, semanticCfg)
	if deps.runPromptWithTools != nil {
		deps.runPrompt = func(ctx context.Context, runLabel string, prompt string) (string, error) {
			return deps.runPromptWithTools(ctx, runLabel, prompt, nil, "", nil)
		}
	}

	// Per-role provider/model context is logged by call sites when they
	// resolve the router; there is no single top-level provider anymore.
	return deps, nil
}

func validateAPIAuthSecurity(cfg config.Config) error {
	mode := strings.TrimSpace(strings.ToLower(cfg.APIAuthMode))
	switch mode {
	case "off", "external-required":
		if !cfg.APIAllowInsecureLocalAuth {
			return fmt.Errorf("api_auth_mode=%s requires api_allow_insecure_local_auth=true for explicit insecure local auth opt-in", mode)
		}
	}
	return nil
}
