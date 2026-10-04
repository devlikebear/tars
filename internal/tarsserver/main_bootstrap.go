package tarsserver

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/onboarding"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/usage"
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
	if path, ok := firstRunSkeletonPath(opts, resolvedPath); ok {
		// First `tars serve` without `tars init`: write the same skeleton
		// init writes. Without it the server boots with auth required and
		// no token, so the wizard's saves to /v1/admin/* are all 401s.
		workspace := strings.TrimSpace(opts.WorkspaceDir)
		if workspace == "" {
			workspace = config.DefaultWorkspaceDir()
		}
		if err := onboarding.WriteSkeletonConfig(path, workspace, opts.APIAddr); err != nil {
			return config.Config{}, &runtimeDepsError{stage: "load_config", err: err}
		}
		resolvedPath = path
	}
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
		config.SetWorkspaceDir(&cfg, strings.TrimSpace(opts.WorkspaceDir))
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

// firstRunSkeletonPath returns where to write the first-run skeleton: the
// fixed config path, when that is the file serve would use (no --config, or
// --config naming it, as the LaunchAgent does), it does not exist yet, and
// the server listens on loopback only. Any other missing file is left to
// the operator.
func firstRunSkeletonPath(opts *options, resolvedPath string) (string, bool) {
	if !isLoopbackListenAddr(opts.APIAddr) {
		return "", false
	}
	fixed := config.FixedConfigPath()
	if resolvedPath != "" && filepath.Clean(resolvedPath) != filepath.Clean(fixed) {
		return "", false
	}
	if _, err := os.Stat(fixed); !errors.Is(err, fs.ErrNotExist) {
		return "", false
	}
	return fixed, true
}

// isLoopbackListenAddr reports whether addr (host:port) only accepts
// connections from this machine.
func isLoopbackListenAddr(addr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
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
