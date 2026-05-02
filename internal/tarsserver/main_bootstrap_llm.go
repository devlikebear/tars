package tarsserver

import (
	"context"
	"fmt"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/rs/zerolog"
)

// buildLLMDeps takes a runtimeDeps already populated by buildBaseDeps
// and adds the LLM-bound fields: router, agent prompt runner, and the
// runPrompt closure. It is the recoverable failure boundary for the
// onboarding setup-only mode (Phase 2): when this returns an
// *runtimeDepsError with stage init_llm / init_memory_backend /
// init_semantic_memory, the caller may downgrade to setup-only
// instead of exiting.
//
// On success the returned runtimeDeps is the same struct passed in
// with llmRouter / runPromptWithTools / runPrompt populated.
func buildLLMDeps(base runtimeDeps, cfg config.Config, logger zerolog.Logger) (runtimeDeps, error) {
	if base.usageTracker == nil {
		return runtimeDeps{}, fmt.Errorf("buildLLMDeps: base deps missing usage tracker — call buildBaseDeps first")
	}

	router, err := buildLLMRouter(cfg, base.usageTracker)
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

	deps := base
	deps.llmRouter = router
	deps.LLMReady = true
	logger.Debug().Msg("llm router initialized")
	deps.runPromptWithTools = newAgentPromptRunnerWithToolsAndMemory(cfg, cfg.WorkspaceDir, nil, deps.llmRouter, deps.usageTracker, cfg.AgentMaxIterations, logger, semanticCfg)
	if deps.runPromptWithTools != nil {
		deps.runPrompt = func(ctx context.Context, runLabel string, prompt string) (string, error) {
			return deps.runPromptWithTools(ctx, runLabel, prompt, nil, "", nil)
		}
	}
	return deps, nil
}
