package tarsserver

import (
	"context"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/agent"
	"github.com/devlikebear/tars/internal/apptool"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/devlikebear/tars/internal/usage"
	"github.com/devlikebear/tars/pkg/agentloop"
	"github.com/rs/zerolog"
)

func executeChatLoop(
	ctx context.Context,
	deps chatHandlerDeps,
	state chatRunState,
	stream *chatStreamWriter,
) (llm.ChatResponse, bool, []ToolCallRecord, error) {
	if state.llmResolution.Tier != "" {
		ctx = llm.WithSelectionMetadata(ctx, llm.SelectionMetadata{
			Role:      llm.RoleChatMain,
			Tier:      state.llmResolution.Tier,
			Provider:  state.llmResolution.Provider,
			Model:     state.llmResolution.Model,
			Source:    state.llmResolution.Source,
			SessionID: state.sessionID,
		})
	}
	streamingAnnounced := false
	deltaSent := false
	var accumulated strings.Builder
	chatClient := state.llmClient
	if chatClient == nil {
		var err error
		chatClient, _, err = deps.resolveChatClient()
		if err != nil {
			return llm.ChatResponse{}, false, nil, err
		}
	}
	ctx = usage.WithCallMeta(ctx, usage.CallMeta{
		Source: "chat", SessionID: state.sessionID, CapabilityVersionIDs: state.capabilityVersionIDs,
	})
	afterToolHook := func(_ context.Context, evt agent.Event) {
		for _, change := range evt.ToolFileChanges {
			stream.fileChange(evt.ToolCallID, change)
		}
		if evt.ToolName != "tasks" {
			return
		}
		// The console keeps the chat pulse-bar Tasks badge in sync via this
		// event; failing to read tasks is non-fatal — the panel falls back
		// to its own poll on toggle.
		tasks, err := state.store.GetTasks(state.sessionID)
		if err != nil {
			deps.logger.Debug().Err(err).Str("session_id", state.sessionID).Msg("tasks_changed: read failed")
			return
		}
		stream.tasksChanged(tasks)
	}
	loop, toolCallRecords := setupChatAgentLoop(chatClient, state.registry, state.sessionID, len(state.history), deps.tooling.UsageTracker, deps.logger, stream.status, afterToolHook, state.turnText)
	ctx = apptool.WithCurrentSessionInfo(ctx, state.sessionID, state.sessionKind)
	ctx = tool.WithLineEmitter(ctx, stream)

	deps.logger.Debug().Str("session_id", state.sessionID).Int("messages", len(state.llmMessages)).Msg("llm chat call start")
	onTurnEnd := buildChatTurnEndHook(deps, state, stream)

	// Resume the upstream provider session (claude-code-cli only today) when
	// one was captured on a previous turn so we don't replay history. Same
	// load also lets us read the session-scoped claude-code-cli permission
	// mode override from .tars/settings*.json (falls back to the global
	// config value otherwise).
	resumeID := ""
	permissionMode := strings.TrimSpace(deps.tooling.ClaudeCodeCLIPermissionMode)
	gateMode := chatPermissionModeManual
	var permissionDeny []string
	if state.store != nil {
		if priorSess, lookupErr := state.store.Get(state.sessionID); lookupErr == nil {
			resumeID = strings.TrimSpace(priorSess.UpstreamSessionID)
			// The session's own mode (#970) wins over the .tars override and
			// the configured default.
			permissionMode = chatPermissionModeResolver{overrides: deps.tooling.OverrideService, configFlag: permissionMode}.claudeCodeFlag(priorSess)
			gateMode = gateModeFor(priorSess)
			permissionDeny = effectiveClaudeCodePermissionDeny(deps.tooling.OverrideService, priorSess)
		}
	}

	chatResp, err := loop.Run(ctx, state.llmMessages, agent.RunOptions{
		MaxIterations:   deps.maxIters,
		Tools:           state.injectedSchemas,
		BlockedTools:    state.blockedTools,
		ToolChoice:      state.toolChoice,
		OnTurnEnd:       onTurnEnd,
		ResumeSessionID: resumeID,
		// The session resumes this upstream session next turn, so the first
		// call must save it; and the CLI works in the session's directory.
		PersistUpstreamSession:   true,
		WorkDir:                  state.cwd,
		ClaudeCodeMCPServers:     state.claudeCodeMCPServers,
		ClaudeCodePermissionMode: permissionMode,
		ClaudeCodeSkills:         state.claudeCodeSkills,
		ClaudeCodePermissionDeny: permissionDeny,
		OnDelta: func(text string) {
			if text == "" {
				return
			}
			accumulated.WriteString(text)
			state.turnText.write(text)
			if !streamingAnnounced {
				streamingAnnounced = true
				stream.status("llm_stream", "streaming response", "", "", "", "")
			}
			deltaSent = true
			deps.logger.Debug().Str("session_id", state.sessionID).Int("delta_len", len(text)).Msg("llm delta")
			stream.delta(text)
		},
		OnReasoningDelta: func(text string) {
			if text == "" {
				return
			}
			if !streamingAnnounced {
				streamingAnnounced = true
				stream.status("llm_stream", "streaming response", "", "", "", "")
			}
			deps.logger.Debug().Str("session_id", state.sessionID).Int("delta_len", len(text)).Msg("llm reasoning delta")
			stream.reasoning(text)
		},
		ClaudeCodePermissionHandler: chatPermissionHandlerFor(deps, state, stream),
		ClaudeCodePermissionAllow:   chatClaudeCodeAlwaysRules(deps, state),
		ToolAuthorizer:              chatToolGateFor(deps, state, stream, gateMode),
	})
	if err != nil {
		// A CLI provider can fail (timeout, cancel, crash) after saving the
		// upstream session it started; keep it so the next turn resumes
		// with the context this one built instead of starting over.
		rememberUpstreamSession(deps, state, llm.UpstreamSessionIDFromError(err), resumeID)
		if ctx.Err() == context.Canceled {
			// Return partial content on cancellation
			partial := accumulated.String()
			deps.logger.Debug().Str("session_id", state.sessionID).Int("partial_len", len(partial)).Msg("chat cancelled, returning partial")
			return llm.ChatResponse{Message: llm.ChatMessage{Content: partial}}, deltaSent, *toolCallRecords, err
		}
		deps.logger.Debug().Str("session_id", state.sessionID).Err(err).Msg("llm chat call failed")
		// The tools that ran before the failure (a CLI provider's timeout,
		// say) stay on record so the reopened session still shows them.
		return llm.ChatResponse{}, false, *toolCallRecords, err
	}
	rememberUpstreamSession(deps, state, chatResp.SessionID, resumeID)

	deps.logger.Debug().
		Str("session_id", state.sessionID).
		Int("assistant_len", len(chatResp.Message.Content)).
		Int("input_tokens", chatResp.Usage.InputTokens).
		Int("output_tokens", chatResp.Usage.OutputTokens).
		Str("stop_reason", chatResp.StopReason).
		Msg("llm chat call complete")

	return chatResp, deltaSent, *toolCallRecords, nil
}

// rememberUpstreamSession stores the upstream session the turn ended on so
// the next turn resumes it. Empty or unchanged IDs are left alone.
func rememberUpstreamSession(deps chatHandlerDeps, state chatRunState, upstream, resumeID string) {
	upstream = strings.TrimSpace(upstream)
	if state.store == nil || upstream == "" || upstream == resumeID {
		return
	}
	if err := state.store.SetUpstreamSessionID(state.sessionID, upstream); err != nil {
		// Non-fatal: the next turn will just start a fresh upstream
		// session instead of resuming. Log and continue.
		deps.logger.Debug().Str("session_id", state.sessionID).Str("upstream_session_id", upstream).Err(err).Msg("persist upstream session id failed")
	}
}

// persistInterruptedTurn saves what a failed or cancelled turn got done. A
// partial reply is saved with every tool call, as before. Without one, only
// the tools the upstream provider ran are saved: a CLI provider's turn can
// run for minutes of tool work before a timeout, and that work is on disk.
// Tool messages with no assistant reply after them never reach the model
// again (buildLLMMessageHistory drops them), so they only feed the console.
func persistInterruptedTurn(state chatRunState, userMessage string, chatResp llm.ChatResponse, toolCalls []ToolCallRecord, logger zerolog.Logger) {
	toolCalls = markInterruptedProviderTools(toolCalls)
	if chatResp.Message.Content != "" {
		persistChatResult(state, userMessage, chatResp, toolCalls, logger)
		return
	}
	persistToolCallRecords(state, upstreamToolCallRecords(toolCalls), time.Now().UTC(), logger)
}

func persistToolCallRecords(state chatRunState, toolCalls []ToolCallRecord, now time.Time, logger zerolog.Logger) {
	for _, tc := range toolCalls {
		if err := session.AppendMessage(state.transcriptPath, toolCallMessage(tc, now)); err != nil {
			logger.Error().Err(err).Str("tool", tc.ToolName).Msg("append tool message failed")
		}
	}
}

func persistChatResult(state chatRunState, userMessage string, chatResp llm.ChatResponse, toolCalls []ToolCallRecord, logger zerolog.Logger) {
	now := time.Now().UTC()
	// The turn's text and tools in the order they streamed; the reply last.
	messages := chatTurnMessages(chatResp, toolCalls, state.turnText.take(), now)
	assistantMsg := messages[len(messages)-1]
	for _, msg := range messages[:len(messages)-1] {
		if err := session.AppendMessage(state.transcriptPath, msg); err != nil {
			logger.Error().Err(err).Str("role", msg.Role).Str("tool", msg.ToolName).Msg("append turn message failed")
		}
	}
	if err := session.AppendMessage(state.transcriptPath, assistantMsg); err != nil {
		logger.Error().Err(err).Msg("append assistant message failed")
	} else if err := state.store.Touch(state.sessionID, assistantMsg.Timestamp); err != nil {
		logger.Error().Err(err).Str("session_id", state.sessionID).Msg("touch session updated_at failed")
	}
	if err := applyPostChatMemoryHooks(chatMemoryHookInput{
		WorkspaceDir:     state.requestWorkspaceDir,
		SessionID:        state.sessionID,
		UserMessage:      userMessage,
		AssistantMessage: chatResp.Message.Content,
		AssistantTime:    assistantMsg.Timestamp,
		LLMClient:        state.llmClient,
	}); err != nil {
		logger.Error().Err(err).Str("session_id", state.sessionID).Msg("write chat memory failed")
	}
}

// chatPermissionHandlerFor returns the handler that routes Claude Code's
// permission prompts to the console, or nil when the client cannot answer
// them: then the provider keeps its one-shot path, where the CLI settles
// prompts by its own permission mode.
func chatPermissionHandlerFor(deps chatHandlerDeps, state chatRunState, stream *chatStreamWriter) llm.ClaudeCodePermissionHandler {
	if !state.interactivePermissions || deps.permissions == nil {
		return nil
	}
	return newChatPermissionHandler(deps.permissions, state.sessionID, state.cwd, stream, sessionModeSwitch(state.store, state.sessionID))
}

// chatToolGateFor returns the gate that asks the console before a native
// provider's turn runs a high-risk tool, or nil when the client cannot
// answer. CLI providers run their own tools and never reach it.
func chatToolGateFor(deps chatHandlerDeps, state chatRunState, stream *chatStreamWriter, mode string) agentloop.ToolAuthorizer {
	if !state.interactivePermissions || deps.permissions == nil {
		return nil
	}
	return newChatToolGate(deps.permissions, state.sessionID, state.cwd, stream, mode)
}

// chatClaudeCodeAlwaysRules are the Claude Code rules the person chose to
// always allow in this turn's folder. They apply to every turn there, asked
// interactively or not; they come only from TARS' own store.
func chatClaudeCodeAlwaysRules(deps chatHandlerDeps, state chatRunState) []string {
	if deps.permissions == nil {
		return nil
	}
	return claudeCodeAlwaysRules(deps.permissions.always, state.cwd)
}

// claudeCodeAlwaysRules lists the Claude Code rules always allowed in cwd.
func claudeCodeAlwaysRules(store *chatAlwaysRuleStore, cwd string) []string {
	if store == nil {
		return nil
	}
	var rules []string
	for _, rule := range store.list(cwd) {
		if rule.Provider == chatRuleProviderClaudeCode {
			rules = append(rules, rule.Display())
		}
	}
	return rules
}
