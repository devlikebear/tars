package tarsserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/agent"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/tool"
)

type cronExecutionContext struct {
	SessionID string
}

type cronExecutionContextKey struct{}

func withCronExecutionContext(ctx context.Context, cfg cronExecutionContext) context.Context {
	if strings.TrimSpace(cfg.SessionID) == "" {
		return ctx
	}
	return context.WithValue(ctx, cronExecutionContextKey{}, cronExecutionContext{
		SessionID: strings.TrimSpace(cfg.SessionID),
	})
}

func cronExecutionContextFromContext(ctx context.Context) cronExecutionContext {
	if ctx == nil {
		return cronExecutionContext{}
	}
	cfg, _ := ctx.Value(cronExecutionContextKey{}).(cronExecutionContext)
	cfg.SessionID = strings.TrimSpace(cfg.SessionID)
	return cfg
}

func newCronPromptRunnerWithSessionContext(fallback gatewayPromptRunner, deps chatHandlerDeps) gatewayPromptRunner {
	if fallback == nil && deps.client == nil {
		return nil
	}
	return func(ctx context.Context, runLabel string, promptText string, allowedTools []string) (string, error) {
		cfg := cronExecutionContextFromContext(ctx)
		if cfg.SessionID == "" {
			if fallback == nil {
				return "", fmt.Errorf("cron runner is not configured")
			}
			return fallback(ctx, runLabel, promptText, allowedTools)
		}

		requestWorkspaceDir := strings.TrimSpace(deps.workspaceDir)
		if deps.store == nil || requestWorkspaceDir == "" {
			return "", fmt.Errorf("session-bound cron runner is not configured")
		}

		transcriptPath := deps.store.TranscriptPath(cfg.SessionID)
		if err := maybeAutoCompactSession(requestWorkspaceDir, transcriptPath, cfg.SessionID, deps.router, deps.logger, deps.tooling.MemorySemanticConfig); err != nil {
			return "", err
		}

		state, err := buildSessionChatRunState(
			requestWorkspaceDir,
			defaultWorkspaceID,
			deps.store,
			cfg.SessionID,
			promptText,
			nil,
			serverauth.RoleAdmin,
			deps,
		)
		if err != nil {
			return "", err
		}

		tools := state.injectedSchemas
		if len(allowedTools) > 0 {
			allowedSet := map[string]struct{}{}
			for _, name := range toolNamesFromSchemas(state.injectedSchemas) {
				allowedSet[strings.TrimSpace(name)] = struct{}{}
			}
			filtered := make([]string, 0, len(allowedTools))
			for _, name := range normalizeToolNames(allowedTools) {
				if _, ok := allowedSet[name]; ok {
					filtered = append(filtered, name)
				}
			}
			tools = state.registry.SchemasForNames(filtered)
		}

		runCtx := tool.WithCurrentSessionInfo(ctx, state.sessionID, state.sessionKind)
		loop, _ := setupAgentLoop(deps.client, state.registry, state.sessionID, len(state.history), deps.logger, func(string, string, string, string, string, string) {})
		resp, err := loop.Run(runCtx, state.llmMessages, agent.RunOptions{
			MaxIterations: deps.maxIters,
			Tools:         tools,
			ToolChoice:    state.toolChoice,
		})
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(resp.Message.Content), nil
	}
}
