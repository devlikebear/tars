package tarsserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/cli"
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
	// llmClient is the chat-main tier client, kept for backward compat
	// with call sites that have not yet been migrated to llmRouter. New
	// code should request a client from llmRouter via a Role.
	llmClient          llm.Client
	llmRouter          llm.Router
	usageTracker       *usage.Tracker
	runPrompt          func(ctx context.Context, runLabel string, prompt string) (string, error)
	runPromptWithTools gatewayPromptRunner
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

	router, err := buildLLMRouter(cfg, tracker)
	if err != nil {
		return runtimeDeps{}, &runtimeDepsError{stage: "init_llm", err: err}
	}
	semanticCfg := semanticMemoryConfigFromConfig(cfg)
	if err := memory.ValidateSemanticConfig(semanticCfg); err != nil {
		return runtimeDeps{}, &runtimeDepsError{stage: "init_semantic_memory", err: err}
	}
	// chatClient is the tier-resolved client for the main chat role. It is
	// stored on deps.llmClient for backward compatibility with call sites
	// that have not yet been migrated to the router. Non-chat call sites
	// (pulse, reflection, compaction) will migrate in follow-up PRs and
	// start requesting clients from deps.llmRouter directly.
	chatClient, chatResolution, err := router.ClientFor(llm.RoleChatMain)
	if err != nil {
		return runtimeDeps{}, &runtimeDepsError{stage: "init_llm", err: err}
	}
	deps.llmRouter = router
	deps.llmClient = chatClient
	logger.Debug().
		Str("tier", string(chatResolution.Tier)).
		Str("provider", chatResolution.Provider).
		Str("model", chatResolution.Model).
		Str("source", chatResolution.Source).
		Msg("llm router resolved chat_main tier")
	deps.runPrompt = newAgentPromptRunner(cfg.WorkspaceDir, deps.llmClient, cfg.AgentMaxIterations, logger, semanticCfg)
	deps.runPromptWithTools = newAgentPromptRunnerWithToolsAndMemory(cfg.WorkspaceDir, deps.llmClient, deps.llmRouter, cfg.AgentMaxIterations, logger, semanticCfg)

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

// runHeartbeatModes used to execute heartbeat one-shot and loop modes,
// but heartbeat has been replaced by the pulse surface. The CLI flags
// that triggered these modes are now deprecated and the server treats
// them as a request to run the HTTP server instead.
func runHeartbeatModes(
	_ context.Context,
	opts *options,
	_ runtimeDeps,
	_ func() time.Time,
	logger zerolog.Logger,
) error {
	if opts == nil {
		return &cli.ExitError{Code: 1, Err: fmt.Errorf("options are required")}
	}
	if opts.RunOnce || opts.RunLoop {
		logger.Warn().Msg("--run-once and --run-loop are deprecated; pulse runs automatically when the server is up")
	}
	return nil
}
