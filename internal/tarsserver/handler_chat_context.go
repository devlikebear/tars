package tarsserver

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/extensions"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/skill"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/devlikebear/tars/internal/workstore"
)

type chatRunState struct {
	requestWorkspaceDir   string
	workspaceID           string
	store                 *session.Store
	sessionID             string
	sessionKind           string
	invokedSkill          *skill.Definition
	invokedSkillReason    string
	invokedCommand        *skill.Definition
	invokedCommandReason  string
	availableSkillNames   []string
	capabilityVersionIDs  []string
	availableCommandNames []string
	transcriptPath        string
	history               []session.Message
	registry              *tool.Registry
	toolChoice            *llm.ToolChoice
	llmMessages           []llm.ChatMessage
	injectedSchemas       []llm.ToolSchema
	blockedTools          map[string]tool.BlockedToolError
	compaction            chatCompactionInfo
	mentionedPaths        []string
	mentionedSubagents    []chatSubagentMention
	relevantMemoryCount   int
	relevantMemoryTokens  int
	llmClient             llm.Client
	llmResolution         llm.TierResolution
	tierRecommendation    chatTierRecommendationState
	sessionStyle          sessionStyleValues
	sessionGoal           *session.SessionGoal
	sessionCritic         *session.SessionCritic
	// claudeCodeMCPServers is the session-effective MCP server list (global
	// extensions ∪ session-scoped extras, filtered by session tool config)
	// converted into the provider-agnostic shape consumed by
	// claude-code-cli's --mcp-config injection. nil when no servers apply.
	claudeCodeMCPServers []llm.ClaudeCodeMCPServer
	// claudeCodeSkills is the session-effective skill catalog (same snapshot
	// pipeline as the chat prompt) converted for claude-code-cli's
	// --plugin-dir materialization. nil when no skills apply.
	claudeCodeSkills []llm.ClaudeCodeSkill
}

func decodeChatRequestPayload(w http.ResponseWriter, r *http.Request) (chatRequestPayload, bool) {
	var req chatRequestPayload
	if !decodeJSONBody(w, r, &req) {
		return chatRequestPayload{}, false
	}
	if strings.TrimSpace(req.Message) == "" {
		writeError(w, http.StatusBadRequest, "", "message is required")
		return chatRequestPayload{}, false
	}
	return req, true
}

func buildSessionChatRunState(
	requestWorkspaceDir string,
	workspaceID string,
	reqStore *session.Store,
	sessionID string,
	userMessage string,
	contentBlocks []llm.ContentBlock,
	subagentMentions []chatSubagentMention,
	tierInput *chatTierRecommendationPayload,
	autoRecommendTier bool,
	authRole string,
	deps chatHandlerDeps,
) (chatRunState, error) {
	transcriptPath := reqStore.TranscriptPath(sessionID)
	historySnapshot, err := loadSessionHistorySnapshot(transcriptPath, chatHistoryMaxTokens)
	if err != nil {
		return chatRunState{}, err
	}
	history := historySnapshot.Messages
	tierRecommendation, err := resolveChatTierRecommendation(tierInput, userMessage, len(history) == 0 && autoRecommendTier)
	if err != nil {
		return chatRunState{}, err
	}
	requestedTier := ""
	if tierRecommendation.enabled() {
		requestedTier = tierRecommendation.ChosenTier.String()
	}
	chatClient, llmResolution, err := deps.resolveChatClientForTier(requestedTier)
	if err != nil {
		return chatRunState{}, err
	}

	// Fetch session early for WorkDirs
	sess, sessErr := reqStore.Get(sessionID)

	// Session artifacts directory — always available, isolated per session
	artifactsDir := filepath.Join(requestWorkspaceDir, "artifacts", sessionID)
	_ = os.MkdirAll(artifactsDir, 0o755)

	var sessionWorkDirs []string
	var sessionCurrentDir string
	if sessErr == nil && len(sess.WorkDirs) > 0 {
		sessionWorkDirs = append(sessionWorkDirs, sess.WorkDirs...)
		sessionCurrentDir = sess.CurrentDir
	} else {
		sessionWorkDirs = []string{artifactsDir}
	}
	policy := tool.NewPathPolicy(requestWorkspaceDir, sessionWorkDirs, sessionCurrentDir)

	registry := buildChatToolRegistry(
		reqStore,
		workspaceID,
		sessionID,
		requestWorkspaceDir,
		policy,
		history,
		deps,
	)
	extSnapshot := extensions.Snapshot{}
	if deps.tooling.Extensions != nil {
		extSnapshot = deps.tooling.Extensions.Snapshot()
	}
	if sessErr == nil {
		extSnapshot = augmentSnapshotWithCwdSkills(extSnapshot, sess.CurrentDir)
	}
	var sessionCommands []skill.Definition
	if sessErr == nil {
		var commandDiags []string
		sessionCommands, commandDiags = loadSessionCwdCommands(sess.CurrentDir)
		extSnapshot.Diagnostics = append(extSnapshot.Diagnostics, commandDiags...)
	}
	var sessionToolConfigs []session.SessionToolConfig
	effectivePromptOverride := ""
	if sessErr == nil {
		effTC, effPrompt, present := effectiveSessionView(deps.tooling.OverrideService, sess)
		if present {
			sessionToolConfigs = append(sessionToolConfigs, effTC)
		}
		effectivePromptOverride = effPrompt
	}
	extSnapshot = filterExtensionsSnapshotForSession(extSnapshot, sessionToolConfigs...)
	if len(sessionToolConfigs) > 0 {
		sessionCommands = applySessionCommandConfig(sessionCommands, sessionToolConfigs[0])
	}
	resolvedCommand := resolveCommandSelectionFromDefinitions(userMessage, sessionCommands, sessionToolConfigs...)
	resolvedSkill := skillSelection{}
	if resolvedCommand.Definition == nil {
		resolvedSkill = resolveSkillSelectionFromSnapshot(userMessage, extSnapshot, requestWorkspaceDir, sessionID, sessionToolConfigs...)
	}
	invokedSkill := resolvedSkill.Definition
	contextDetails, err := prepareChatContextDetailsWithCache(requestWorkspaceDir, sessionID, userMessage, extSnapshot, invokedSkill, deps.tooling.MemoryCache, deps.tooling.MemorySemanticConfig, sessionWorkDirs, sessionCurrentDir, deps.tooling.PlanClarifyMode)
	if err != nil {
		return chatRunState{}, err
	}
	systemPrompt := contextDetails.SystemPrompt
	systemPrompt = appendInvokedCommandPrompt(systemPrompt, requestWorkspaceDir, resolvedCommand.Definition)
	toolChoice := contextDetails.ToolChoice
	deps.logger.Debug().
		Str("session_id", sessionID).
		Int("history_messages", len(history)).
		Int("history_tokens", historySnapshot.Tokens).
		Bool("compaction_used", historySnapshot.CompactionUsed).
		Int("relevant_memory_count", contextDetails.RelevantMemoryCount).
		Int("relevant_memory_tokens", contextDetails.RelevantMemoryTokens).
		Int("system_prompt_len", len(systemPrompt)).
		Int("system_prompt_tokens", promptTokenEstimate(systemPrompt)).
		Str("tool_choice", toolChoice.String()).
		Int("context_window", llmResolution.ContextWindow).
		Msg("chat context assembled")

	// Pre-flight: report a projected overrun here, where the numbers are
	// still ours, rather than letting it come back as a provider error with
	// no indication of which part of the assembly was too big.
	if overrun := config.ContextWindowOverrun(
		llmResolution.ContextWindow,
		historySnapshot.Tokens,
		promptTokenEstimate(systemPrompt)+contextDetails.RelevantMemoryTokens,
		llmResolution.MaxTokens,
		llmResolution.ThinkingBudget,
	); overrun > 0 {
		deps.logger.Warn().
			Str("session_id", sessionID).
			Str("model", llmResolution.Model).
			Int("context_window", llmResolution.ContextWindow).
			Int("history_tokens", historySnapshot.Tokens).
			Int("system_prompt_tokens", promptTokenEstimate(systemPrompt)).
			Int("relevant_memory_tokens", contextDetails.RelevantMemoryTokens).
			Int("max_tokens", llmResolution.MaxTokens).
			Int("overrun_tokens", overrun).
			Msg("assembled request is projected to exceed the model's context window")
	}

	if sessErr == nil && strings.TrimSpace(effectivePromptOverride) != "" {
		systemPrompt += "\n\n## Session Prompt Override\n" + strings.TrimSpace(effectivePromptOverride) + "\n"
	}
	sessionStyle := effectiveSessionStyle(deps.tooling.StyleDefaults, nil)
	if sessErr == nil {
		sessionStyle = effectiveSessionStyle(deps.tooling.StyleDefaults, sess.StyleControl)
		systemPrompt += formatSessionStylePrompt(sessionStyle, sess.AutomationConsent)
	}
	var sessionGoal *session.SessionGoal
	if sessErr == nil && sess.Goal.IsActive() {
		sessionGoal = sess.Goal
		systemPrompt += formatSessionGoalPrompt(sessionGoal)
	}
	var sessionCritic *session.SessionCritic
	if sessErr == nil && sess.Critic.IsEnabled() {
		sessionCritic = sess.Critic
		systemPrompt += formatSessionCriticPrompt(sessionCritic)
	}
	if hint := formatChatSubagentMentionHint(subagentMentions); hint != "" {
		systemPrompt += hint
	}

	// contextDetails.SystemPromptTail closes the assembled prompt: everything
	// appended above (skills, override, style, goal, critic, mention hints) is
	// stable for the session, so it belongs ahead of the per-turn recall and
	// clock. See the ordering invariant on prompt.BuildResultFor.
	llmMessages := buildLLMMessagesWithTail(systemPrompt, contextDetails.SystemPromptTail, history, userMessage, contentBlocks)
	// Drain any pending critic feedback queued by the async reviewer on a
	// previous turn. Injected as a system-role message right before the
	// current user message so the LLM treats it as authoritative direction.
	if sessionCritic.IsEnabled() {
		if pending, perr := reqStore.TakePendingCriticFeedback(sessionID); perr == nil && strings.TrimSpace(pending.Feedback) != "" {
			llmMessages = insertSystemMessageBeforeUser(llmMessages, pending.Feedback)
			deps.logger.Debug().
				Str("session_id", sessionID).
				Str("critic_trigger", pending.Trigger).
				Int("critic_round", pending.Round).
				Msg("drained pending critic feedback into next turn")
		} else if perr != nil {
			deps.logger.Debug().Err(perr).Str("session_id", sessionID).Msg("drain pending critic feedback failed")
		}
	}
	resolvedTools := resolveInjectedToolPolicy(registry, authRole, deps.tooling.ToolsAllowHighRiskUser, sessionToolConfigs...)
	injectedSchemas := resolvedTools.Schemas
	deps.logger.Debug().
		Str("session_id", sessionID).
		Int("tool_count_injected", len(injectedSchemas)).
		Strs("injected_tools", toolNamesFromSchemas(injectedSchemas)).
		Msg("tool injection result")

	return chatRunState{
		requestWorkspaceDir:   requestWorkspaceDir,
		workspaceID:           workspaceID,
		store:                 reqStore,
		sessionID:             sessionID,
		sessionKind:           strings.TrimSpace(sess.Kind),
		invokedSkill:          invokedSkill,
		invokedSkillReason:    resolvedSkill.Reason,
		invokedCommand:        resolvedCommand.Definition,
		invokedCommandReason:  resolvedCommand.Reason,
		availableSkillNames:   skillNamesFromDefinitions(extSnapshot.Skills),
		availableCommandNames: skillNamesFromDefinitions(sessionCommands),
		transcriptPath:        transcriptPath,
		history:               history,
		registry:              registry,
		toolChoice:            toolChoice,
		llmMessages:           llmMessages,
		injectedSchemas:       injectedSchemas,
		blockedTools:          resolvedTools.Blocked,
		mentionedSubagents:    subagentMentions,
		relevantMemoryCount:   contextDetails.RelevantMemoryCount,
		relevantMemoryTokens:  contextDetails.RelevantMemoryTokens,
		llmClient:             chatClient,
		llmResolution:         llmResolution,
		tierRecommendation:    tierRecommendation,
		sessionStyle:          sessionStyle,
		sessionGoal:           sessionGoal,
		sessionCritic:         sessionCritic,
		claudeCodeMCPServers:  toClaudeCodeMCPServers(extSnapshot.MCPServers),
		claudeCodeSkills:      toClaudeCodeSkills(extSnapshot.Skills),
	}, nil
}

// formatSessionGoalPrompt produces the system-prompt section that surfaces
// the active session goal to the LLM. Returns an empty string when goal is
// nil so callers can use it unconditionally.
func formatSessionGoalPrompt(goal *session.SessionGoal) string {
	if !goal.IsActive() {
		return ""
	}
	budget := goal.MaxAutoContinues - goal.AutoContinueCount
	if budget < 0 {
		budget = 0
	}
	var b strings.Builder
	b.WriteString("\n\n## Active Session Goal\n")
	b.WriteString(strings.TrimSpace(goal.Description))
	b.WriteString("\n\nKeep working toward this goal across turns without waiting for the user. ")
	b.WriteString("Once you are confident it is satisfied, say so plainly in your final reply — an independent judge will verify and clear the goal. ")
	b.WriteString("If the goal is not yet satisfied, continue making concrete progress on the next step rather than asking for confirmation.\n")
	b.WriteString("Auto-continue budget remaining: ")
	b.WriteString(strconv.Itoa(budget))
	b.WriteString("/")
	b.WriteString(strconv.Itoa(goal.MaxAutoContinues))
	b.WriteString("\n")
	return b.String()
}

func prepareChatRunState(r *http.Request, req chatRequestPayload, deps chatHandlerDeps) (chatRunState, int, string, error) {
	reqStore, requestWorkspaceDir, workspaceID, err := resolveSessionStoreForRequest(deps.workspaceDir, deps.store, r)
	if err != nil {
		deps.logger.Error().Err(err).Msg("resolve workspace session store failed")
		return chatRunState{}, http.StatusInternalServerError, "resolve workspace failed", err
	}

	sessionID, err := resolveChatSession(reqStore, req.SessionID, deps.mainSessionID)
	if err != nil {
		if strings.TrimSpace(req.SessionID) == "" {
			deps.logger.Error().Err(err).Msg("create session failed")
			return chatRunState{}, http.StatusInternalServerError, "create session failed", err
		}
		return chatRunState{}, http.StatusNotFound, "session not found", err
	}

	transcriptPath := reqStore.TranscriptPath(sessionID)
	deps.logger.Debug().Str("session_id", sessionID).Str("transcript_path", transcriptPath).Msg("chat session resolved")
	// Size history against the model that will serve this turn, not against
	// a global constant that cannot know whether the tier is on a 200k or a
	// 1M window.
	//
	// The tier is resolved again inside buildSessionChatRunState, which may
	// additionally apply an auto-recommendation. That path only engages on a
	// session's first message, where there is no history to compact, so the
	// explicit-or-default tier is the right one to size against here.
	compactionOpts := deps.tooling.Compaction
	if _, sizing, resolveErr := deps.resolveChatClientForTier(chatRequestedTier(req)); resolveErr == nil {
		compactionOpts = applyTierContextWindow(compactionOpts, sizing, deps.logger)
	}
	compactionInfo, err := maybeAutoCompactSession(requestWorkspaceDir, transcriptPath, sessionID, reqStore, deps.router, deps.logger, compactionOpts, deps.tooling.MemorySemanticConfig)
	if err != nil {
		deps.logger.Error().Err(err).Str("session_id", sessionID).Msg("auto compaction failed")
		return chatRunState{}, http.StatusInternalServerError, "auto compaction failed", err
	}
	contentBlocks := attachmentsToContentBlocks(req.Attachments)
	mentionBlocks, mentionedPaths, err := chatMentionsToContentBlocks(reqStore, requestWorkspaceDir, sessionID, req.Mentions)
	if err != nil {
		return chatRunState{}, http.StatusBadRequest, err.Error(), err
	}
	contentBlocks = append(contentBlocks, mentionBlocks...)
	subagentMentions, err := normalizeChatSubagentMentions(deps.tooling.AgentRuntime, req.SubagentMentions)
	if err != nil {
		return chatRunState{}, http.StatusBadRequest, err.Error(), err
	}
	authRole := strings.TrimSpace(serverauth.RoleFromRequest(r))
	state, err := buildSessionChatRunState(
		requestWorkspaceDir,
		workspaceID,
		reqStore,
		sessionID,
		req.Message,
		contentBlocks,
		subagentMentions,
		req.TierRecommendation,
		true,
		authRole,
		deps,
	)
	if err != nil {
		deps.logger.Error().Err(err).Msg("build chat run state failed")
		return chatRunState{}, http.StatusInternalServerError, "prepare chat context failed", err
	}
	state.compaction = compactionInfo
	state.mentionedPaths = mentionedPaths
	state.mentionedSubagents = subagentMentions
	state.capabilityVersionIDs, err = resolvePromotedCapabilityVersionIDs(r.Context(), deps.tooling.WorkLedger, workspaceID, state.invokedSkill)
	if err != nil {
		deps.logger.Warn().Err(err).Str("workspace_id", workspaceID).Str("skill", skillNameOrEmpty(state.invokedSkill)).Msg("resolve promoted capability attribution failed")
		state.capabilityVersionIDs = nil
	}
	if compactionInfo.Applied && strings.TrimSpace(compactionInfo.Mode) != "" {
		if setErr := reqStore.SetLastCompactionMode(sessionID, compactionInfo.Mode); setErr != nil {
			deps.logger.Warn().Err(setErr).Str("session_id", sessionID).Msg("persist last compaction mode failed")
		}
	}

	userMsg := session.Message{Role: "user", Content: req.Message, Timestamp: time.Now().UTC()}
	if err := session.AppendMessage(transcriptPath, userMsg); err != nil {
		deps.logger.Error().Err(err).Msg("append user message failed")
		return chatRunState{}, http.StatusInternalServerError, "save message failed", err
	}

	return state, 0, "", nil
}

func resolvePromotedCapabilityVersionIDs(ctx context.Context, ledger *workstore.Store, workspaceID string, invokedSkill *skill.Definition) ([]string, error) {
	if ledger == nil || invokedSkill == nil || strings.TrimSpace(invokedSkill.Name) == "" {
		return nil, nil
	}
	versions, err := ledger.ListCapabilityVersions(ctx, workstore.ListCapabilityVersionsFilter{
		WorkspaceID: workspaceID, CapabilityName: strings.TrimSpace(invokedSkill.Name),
		States: []workstore.CapabilityState{workstore.CapabilityStatePromoted}, Limit: 1000,
	})
	if err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, nil
	}
	latest := versions[0]
	for _, version := range versions[1:] {
		if version.Version > latest.Version {
			latest = version
		}
	}
	return []string{latest.ID}, nil
}
