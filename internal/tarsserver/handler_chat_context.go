package tarsserver

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/extensions"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/skill"
	"github.com/devlikebear/tars/internal/tool"
)

type chatRunState struct {
	requestWorkspaceDir  string
	workspaceID          string
	store                *session.Store
	sessionID            string
	sessionKind          string
	invokedSkill         *skill.Definition
	invokedSkillReason   string
	availableSkillNames  []string
	transcriptPath       string
	history              []session.Message
	registry             *tool.Registry
	toolChoice           *llm.ToolChoice
	llmMessages          []llm.ChatMessage
	injectedSchemas      []llm.ToolSchema
	blockedTools         map[string]tool.BlockedToolError
	compaction           chatCompactionInfo
	mentionedPaths       []string
	mentionedSubagents   []chatSubagentMention
	relevantMemoryCount  int
	relevantMemoryTokens int
	llmClient            llm.Client
	llmResolution        llm.TierResolution
	tierRecommendation   chatTierRecommendationState
	sessionStyle         sessionStyleValues
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
	resolvedSkill := resolveSkillSelection(userMessage, deps.tooling.Extensions, requestWorkspaceDir, sessionID, sessionToolConfigs...)
	invokedSkill := resolvedSkill.Definition
	contextDetails, err := prepareChatContextDetailsWithCache(requestWorkspaceDir, sessionID, userMessage, extSnapshot, invokedSkill, deps.tooling.MemoryCache, deps.tooling.MemorySemanticConfig, sessionWorkDirs, sessionCurrentDir, deps.tooling.PlanClarifyMode)
	if err != nil {
		return chatRunState{}, err
	}
	systemPrompt := contextDetails.SystemPrompt
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
		Msg("chat context assembled")

	if sessErr == nil && strings.TrimSpace(effectivePromptOverride) != "" {
		systemPrompt += "\n\n## Session Prompt Override\n" + strings.TrimSpace(effectivePromptOverride) + "\n"
	}
	sessionStyle := effectiveSessionStyle(deps.tooling.StyleDefaults, nil)
	if sessErr == nil {
		sessionStyle = effectiveSessionStyle(deps.tooling.StyleDefaults, sess.StyleControl)
		systemPrompt += formatSessionStylePrompt(sessionStyle, sess.AutomationConsent)
	}
	if hint := formatChatSubagentMentionHint(subagentMentions); hint != "" {
		systemPrompt += hint
	}

	llmMessages := buildLLMMessagesWithBlocks(systemPrompt, history, userMessage, contentBlocks)
	resolvedTools := resolveInjectedToolPolicy(registry, authRole, deps.tooling.ToolsAllowHighRiskUser, sessionToolConfigs...)
	injectedSchemas := resolvedTools.Schemas
	deps.logger.Debug().
		Str("session_id", sessionID).
		Int("tool_count_injected", len(injectedSchemas)).
		Strs("injected_tools", toolNamesFromSchemas(injectedSchemas)).
		Msg("tool injection result")

	return chatRunState{
		requestWorkspaceDir:  requestWorkspaceDir,
		workspaceID:          workspaceID,
		store:                reqStore,
		sessionID:            sessionID,
		sessionKind:          strings.TrimSpace(sess.Kind),
		invokedSkill:         invokedSkill,
		invokedSkillReason:   resolvedSkill.Reason,
		availableSkillNames:  skillNamesFromDefinitions(extSnapshot.Skills),
		transcriptPath:       transcriptPath,
		history:              history,
		registry:             registry,
		toolChoice:           toolChoice,
		llmMessages:          llmMessages,
		injectedSchemas:      injectedSchemas,
		blockedTools:         resolvedTools.Blocked,
		mentionedSubagents:   subagentMentions,
		relevantMemoryCount:  contextDetails.RelevantMemoryCount,
		relevantMemoryTokens: contextDetails.RelevantMemoryTokens,
		llmClient:            chatClient,
		llmResolution:        llmResolution,
		tierRecommendation:   tierRecommendation,
		sessionStyle:         sessionStyle,
	}, nil
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
	compactionInfo, err := maybeAutoCompactSession(requestWorkspaceDir, transcriptPath, sessionID, reqStore, deps.router, deps.logger, deps.tooling.Compaction, deps.tooling.MemorySemanticConfig)
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
