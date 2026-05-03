package tarsserver

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
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
	// LLMReady is true when buildLLMDeps populated the LLM-bound fields.
	// When false the server runs in setup-only mode (Phase 2 onboarding):
	// only the wizard endpoints + console + healthz are wired and chat /
	// agent / cron / pulse / reflection are inactive. The CLI's RunE is
	// the single setter; downstream code reads this to branch routes.
	LLMReady bool
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
//
// Missing config files are tolerated: a brand-new install has no
// ~/.tars/config/config.yaml yet, and the wizard's job (setup-only
// boot mode) is precisely to create it. When the resolved path does
// not exist, loadConfigForServe falls through with the default
// config; downstream buildLLMDeps then fails recoverably and the
// CLI's existing setup-only branch takes over.
//
// As a side effect, opts.ConfigPath is filled in with the path the
// wizard should write to (FixedConfigPath fallback when nothing was
// resolved) so handlers downstream — handler_setup, handler_config —
// can advertise / save to a concrete location.
func loadConfigForServe(opts *options) (config.Config, error) {
	if opts == nil {
		return config.Config{}, fmt.Errorf("options are required")
	}
	resolvedPath := config.ResolveConfigPath(opts.ConfigPath)
	cfg, err := config.Load(resolvedPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// First-run case: the operator (or `tars init`) hasn't
			// written a file yet. Boot with defaults but route through
			// config.Load("") so env-var overrides (TARS_API_AUTH_MODE
			// etc.) and the schema defaults still apply — otherwise
			// admin paths reject the wizard's first PATCH because the
			// loopback / off-mode posture only arrives via env vars.
			cfg, err = config.Load("")
			if err != nil {
				return config.Config{}, &runtimeDepsError{stage: "load_config", err: err}
			}
		} else {
			return config.Config{}, &runtimeDepsError{stage: "load_config", err: err}
		}
	}
	if strings.TrimSpace(opts.WorkspaceDir) != "" {
		cfg.WorkspaceDir = strings.TrimSpace(opts.WorkspaceDir)
	}
	// Decide where the wizard should save. Honor an explicit
	// --config / TARS_CONFIG override; otherwise fall back to the
	// fixed default so the rest of the runtime has a concrete path.
	if strings.TrimSpace(opts.ConfigPath) == "" {
		if resolvedPath != "" {
			opts.ConfigPath = resolvedPath
		} else {
			opts.ConfigPath = config.FixedConfigPath()
		}
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
