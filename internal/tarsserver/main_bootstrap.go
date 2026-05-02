package tarsserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/llm"
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

// buildRuntimeDeps composes buildBaseDeps + buildLLMDeps. It exists as
// the single entry point used by callers that want full runtime
// dependencies (production boot, helpers_llm_router_test). The
// onboarding setup-only path (Phase 2) calls the two pieces directly
// so it can downgrade on init_llm failure instead of bailing.
func buildRuntimeDeps(opts *options, cfg config.Config, nowFn func() time.Time, logger zerolog.Logger) (runtimeDeps, error) {
	base, err := buildBaseDeps(opts, cfg, nowFn, logger)
	if err != nil {
		return runtimeDeps{}, err
	}
	return buildLLMDeps(base, cfg, logger)
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
