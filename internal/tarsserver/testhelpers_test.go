package tarsserver

import (
	"context"
	"fmt"
	"net/http"

	"github.com/devlikebear/tars/internal/agent"
	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/cron"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/sessionoverride"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/devlikebear/tars/internal/usage"
	"github.com/devlikebear/tars/internal/workstore"
	"github.com/rs/zerolog"
)

// running reports whether the session has a run.
func (d *focusDriver) running(sessionID string) bool {
	if d == nil {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.runs[sessionID] != nil
}

// appendFocusGuidance puts the current stage's instructions after the user's
// message as one <focus-stage> block when the session has an active
// pipeline, and returns the mark the turn carries to focusAfterTurn (nil
// when no guidance was added). Unlike <console-context> it has no size cap:
// the block format it quotes must arrive whole. Slash commands keep their
// arguments clean and do not feed the pipeline.
func appendFocusGuidance(message string, sessions *session.Store, sessionID string, logger zerolog.Logger) (string, *focusTurnMark) {
	return appendFocusGuidanceAt(message, sessions, sessionID, "", logger)
}

// polling reports whether the session has a PR poller.
func (d *focusDriver) polling(sessionID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.pollers[sessionID] != nil
}

func newAgentRunsAPIHandler(runtime *agentruntime.Runtime, logger zerolog.Logger) http.Handler {
	return newAgentRunsAPIHandlerWithWorkLedgerAndInflightLimit(runtime, nil, logger, 4)
}

func newAgentRunsAPIHandlerWithWorkLedger(runtime *agentruntime.Runtime, ledger *workstore.Store, logger zerolog.Logger) http.Handler {
	return newAgentRunsAPIHandlerWithWorkLedgerAndInflightLimit(runtime, ledger, logger, 4)
}

func newChatAPIHandler(workspaceDir string, store *session.Store, client llm.Client, logger zerolog.Logger) http.Handler {
	return newChatAPIHandlerWithRuntimeConfig(
		workspaceDir,
		store,
		client,
		nil,
		logger,
		agent.DefaultMaxLoopIters,
		nil,
		"",
		defaultChatToolingOptions(),
	)
}

func newCronAPIHandler(
	store *cron.Store,
	runPrompt func(ctx context.Context, prompt string) (string, error),
	logger zerolog.Logger,
) http.Handler {
	var runJob func(ctx context.Context, job cron.Job) (string, error)
	if runPrompt != nil {
		runJob = func(ctx context.Context, job cron.Job) (string, error) {
			return runPrompt(ctx, job.Prompt)
		}
	}
	return newCronAPIHandlerWithRunner(store, runJob, logger)
}

func newCronAPIHandlerWithRunner(
	store *cron.Store,
	runJob func(ctx context.Context, job cron.Job) (string, error),
	logger zerolog.Logger,
) http.Handler {
	baseWorkspaceDir := ""
	runHistoryLimit := 0
	if store != nil {
		baseWorkspaceDir = store.WorkspaceDir()
		runHistoryLimit = store.RunHistoryLimit()
	}
	resolver := newWorkspaceCronStoreResolver(baseWorkspaceDir, runHistoryLimit, store)
	return newCronAPIHandlerWithRunnerAndResolver(resolver, runJob, logger)
}

func newExtensionsAPIHandler(provider extensionsProvider, logger zerolog.Logger, afterReload func() (bool, int)) http.Handler {
	return newExtensionsAPIHandlerWithHealth(provider, logger, afterReload, nil, extensionHealthOptions{})
}

func newSessionAPIHandler(store *session.Store, logger zerolog.Logger) http.Handler {
	return newSessionAPIHandlerFullWithLocalSkillsAndWorkLedger(store, logger, nil, sessionStyleDefaultsFromConfig(config.Default()), nil, nil, nil, localSkillsHandlerDeps{}, nil)
}

func newSessionAPIHandlerWithNotifier(store *session.Store, logger zerolog.Logger, usageTracker *usage.Tracker, styleDefaults sessionStyleValues, notify sessionNotifier) http.Handler {
	return newSessionAPIHandlerFull(store, logger, usageTracker, styleDefaults, notify, nil)
}

func newSessionAPIHandlerFull(store *session.Store, logger zerolog.Logger, usageTracker *usage.Tracker, styleDefaults sessionStyleValues, notify sessionNotifier, overrideService *sessionoverride.Service) http.Handler {
	return newSessionAPIHandlerFullWithLLM(store, logger, usageTracker, styleDefaults, notify, overrideService, nil)
}

func newSessionAPIHandlerFullWithLLM(store *session.Store, logger zerolog.Logger, usageTracker *usage.Tracker, styleDefaults sessionStyleValues, notify sessionNotifier, overrideService *sessionoverride.Service, llmRouter llm.Router) http.Handler {
	return newSessionAPIHandlerFullWithLocalSkillsAndWorkLedger(store, logger, usageTracker, styleDefaults, notify, overrideService, llmRouter, localSkillsHandlerDeps{}, nil)
}

func newSessionAPIHandlerWithWorkLedger(store *session.Store, ledger *workstore.Store, logger zerolog.Logger) http.Handler {
	return newSessionAPIHandlerFullWithLocalSkillsAndWorkLedger(store, logger, nil, sessionStyleDefaultsFromConfig(config.Default()), nil, nil, nil, localSkillsHandlerDeps{}, ledger)
}

func newTerminalAPIHandlerWithOpener(workspaceDir string, store *session.Store, opener terminalOpenFunc, logger zerolog.Logger) http.Handler {
	return newTerminalAPIHandlerWithDeps(workspaceDir, store, terminalHandlerDeps{
		OpenExternal: opener,
		StartSession: startPTYTerminalSession,
	}, logger)
}

func newAgentPromptRunnerWithTools(
	cfg config.Config,
	workspaceDir string,
	client llm.Client,
	tracker *usage.Tracker,
	maxIterations int,
	logger zerolog.Logger,
	extraTools ...tool.Tool,
) agentRuntimePromptRunner {
	return newAgentPromptRunnerWithToolsAndMemory(cfg, workspaceDir, client, nil, tracker, maxIterations, logger, memory.SemanticConfig{}, extraTools...)
}

func (f telegramCommandExecFunc) Execute(ctx context.Context, line, currentSessionID string) (bool, string, string, error) {
	if f == nil {
		return false, "", "", nil
	}
	return f(ctx, line, currentSessionID)
}

func (f telegramSendFunc) Send(ctx context.Context, req telegramSendRequest) (telegramSendResult, error) {
	if f == nil {
		return telegramSendResult{}, fmt.Errorf("telegram sender is not configured")
	}
	return f(ctx, req)
}

func (f telegramSendFunc) SendChatAction(ctx context.Context, req telegramChatActionRequest) error {
	_ = ctx
	_ = req
	return nil
}

type telegramCommandExecFunc func(ctx context.Context, line, currentSessionID string) (handled bool, result string, nextSessionID string, err error)

type telegramSendFunc func(ctx context.Context, req telegramSendRequest) (telegramSendResult, error)
