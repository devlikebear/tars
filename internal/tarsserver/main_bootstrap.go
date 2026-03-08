package tarsserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/cli"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/heartbeat"
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
	llmClient            llm.Client
	usageTracker         *usage.Tracker
	ask                  heartbeat.AskFunc
	runPrompt            func(ctx context.Context, runLabel string, prompt string) (string, error)
	runPromptWithTools   gatewayPromptRunner
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

func buildRuntimeDeps(opts *options, nowFn func() time.Time, logger zerolog.Logger) (runtimeDeps, error) {
	if opts == nil {
		return runtimeDeps{}, fmt.Errorf("options are required")
	}

	resolvedConfigPath := config.ResolveConfigPath(opts.ConfigPath)
	cfg, err := config.Load(resolvedConfigPath)
	if err != nil {
		return runtimeDeps{}, &runtimeDepsError{stage: "load_config", err: err}
	}
	if strings.TrimSpace(resolvedConfigPath) != "" {
		logger.Debug().Str("config_path", resolvedConfigPath).Msg("resolved config file")
	}

	if strings.TrimSpace(opts.Mode) != "" {
		cfg.Mode = strings.TrimSpace(opts.Mode)
	}
	if strings.TrimSpace(opts.WorkspaceDir) != "" {
		cfg.WorkspaceDir = strings.TrimSpace(opts.WorkspaceDir)
	}
	if err := validateAPIAuthSecurity(cfg, opts.ServeAPI); err != nil {
		return runtimeDeps{}, &runtimeDepsError{stage: "validate_config", err: err}
	}

	if err := memory.EnsureWorkspace(cfg.WorkspaceDir); err != nil {
		return runtimeDeps{}, &runtimeDepsError{stage: "ensure_workspace", err: err}
	}
	if err := memory.AppendDailyLog(cfg.WorkspaceDir, nowFn(), "tars startup complete"); err != nil {
		return runtimeDeps{}, &runtimeDepsError{stage: "daily_log", err: err}
	}

	deps := runtimeDeps{
		cfg:                  cfg,
		sessionStore:         session.NewStore(cfg.WorkspaceDir),
		sessionStoreResolver: newWorkspaceSessionStoreResolver(cfg.WorkspaceDir, nil),
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

	needLLM := opts.RunOnce || opts.RunLoop || opts.ServeAPI
	if !needLLM {
		return deps, nil
	}

	client, err := llm.NewProvider(llm.ProviderOptions{
		Provider:        cfg.LLMProvider,
		AuthMode:        cfg.LLMAuthMode,
		OAuthProvider:   cfg.LLMOAuthProvider,
		BaseURL:         cfg.LLMBaseURL,
		Model:           cfg.LLMModel,
		APIKey:          cfg.LLMAPIKey,
		ReasoningEffort: cfg.LLMReasoningEffort,
		ThinkingBudget:  cfg.LLMThinkingBudget,
		ServiceTier:     cfg.LLMServiceTier,
	})
	if err != nil {
		return runtimeDeps{}, &runtimeDepsError{stage: "init_llm", err: err}
	}
	deps.llmClient = usage.NewTrackedClient(client, tracker, cfg.LLMProvider, cfg.LLMModel)
	deps.runPrompt = newAgentPromptRunner(cfg.WorkspaceDir, deps.llmClient, cfg.AgentMaxIterations, logger)
	deps.runPromptWithTools = newAgentPromptRunnerWithTools(cfg.WorkspaceDir, deps.llmClient, cfg.AgentMaxIterations, logger)
	deps.ask = newAgentAskFunc(cfg.WorkspaceDir, deps.llmClient, cfg.AgentMaxIterations, logger)

	logger.Debug().
		Str("provider", cfg.LLMProvider).
		Str("auth_mode", cfg.LLMAuthMode).
		Str("model", cfg.LLMModel).
		Str("base_url", cfg.LLMBaseURL).
		Msg("llm provider initialized")
	return deps, nil
}

func validateAPIAuthSecurity(cfg config.Config, serveAPI bool) error {
	if !serveAPI {
		return nil
	}
	mode := strings.TrimSpace(strings.ToLower(cfg.APIAuthMode))
	switch mode {
	case "off", "external-required":
		if !cfg.APIAllowInsecureLocalAuth {
			return fmt.Errorf("api_auth_mode=%s requires api_allow_insecure_local_auth=true for explicit insecure local auth opt-in", mode)
		}
	}
	return nil
}

func runHeartbeatModes(
	parentCtx context.Context,
	opts *options,
	deps runtimeDeps,
	nowFn func() time.Time,
	logger zerolog.Logger,
) error {
	if opts == nil {
		return &cli.ExitError{Code: 1, Err: fmt.Errorf("options are required")}
	}

	if opts.RunOnce {
		ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
		defer cancel()
		runPolicy := buildHeartbeatPolicy(deps.sessionStore, deps.cfg.HeartbeatActiveHours, deps.cfg.HeartbeatTimezone, nil)
		if _, err := heartbeat.RunOnceWithLLMResultWithPolicy(ctx, deps.cfg.WorkspaceDir, nowFn(), deps.ask, runPolicy); err != nil {
			logger.Error().Err(err).Msg("failed to run heartbeat once")
			return &cli.ExitError{Code: 1, Err: err}
		}
		logger.Info().Msg("heartbeat run-once complete")
	}

	if opts.RunLoop {
		runPolicy := buildHeartbeatPolicy(deps.sessionStore, deps.cfg.HeartbeatActiveHours, deps.cfg.HeartbeatTimezone, nil)
		count, err := heartbeat.RunLoopWithLLMWithPolicy(
			parentCtx,
			deps.cfg.WorkspaceDir,
			opts.HeartbeatInterval,
			opts.MaxHeartbeats,
			nowFn,
			deps.ask,
			runPolicy,
		)
		if err != nil {
			logger.Error().Err(err).Msg("failed to run heartbeat loop")
			return &cli.ExitError{Code: 1, Err: err}
		}
		logger.Info().Int("heartbeat_count", count).Msg("heartbeat run-loop complete")
	}
	return nil
}
