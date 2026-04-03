package tarsserver

import (
	"net/http"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/extensions"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/project"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/skill"
	"github.com/devlikebear/tars/internal/tool"
)

type chatRunState struct {
	requestWorkspaceDir string
	workspaceID         string
	store               *session.Store
	sessionID           string
	projectID           string
	invokedSkill        *skill.Definition
	invokedSkillReason  string
	transcriptPath      string
	history             []session.Message
	registry            *tool.Registry
	toolChoice          string
	llmMessages         []llm.ChatMessage
	injectedSchemas     []llm.ToolSchema
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

func prepareChatRunState(r *http.Request, req chatRequestPayload, deps chatHandlerDeps) (chatRunState, int, string, error) {
	reqStore, requestWorkspaceDir, workspaceID, err := resolveSessionStoreForRequest(deps.workspaceDir, deps.store, r)
	if err != nil {
		deps.logger.Error().Err(err).Msg("resolve workspace session store failed")
		return chatRunState{}, http.StatusInternalServerError, "resolve workspace failed", err
	}

	sessionID, err := resolveChatSession(reqStore, req.SessionID, deps.mainSessionID, req.Message, req.ProjectID)
	if err != nil {
		if strings.TrimSpace(req.SessionID) == "" {
			deps.logger.Error().Err(err).Msg("create session failed")
			return chatRunState{}, http.StatusInternalServerError, "create session failed", err
		}
		return chatRunState{}, http.StatusNotFound, "session not found", err
	}

	transcriptPath := reqStore.TranscriptPath(sessionID)
	deps.logger.Debug().Str("session_id", sessionID).Str("transcript_path", transcriptPath).Msg("chat session resolved")
	if err := maybeAutoCompactSession(requestWorkspaceDir, transcriptPath, sessionID, deps.client, deps.logger, deps.tooling.MemorySemanticConfig); err != nil {
		deps.logger.Error().Err(err).Str("session_id", sessionID).Msg("auto compaction failed")
		return chatRunState{}, http.StatusInternalServerError, "auto compaction failed", err
	}

	historySnapshot, err := loadSessionHistorySnapshot(transcriptPath, chatHistoryMaxTokens)
	if err != nil {
		deps.logger.Error().Err(err).Msg("load history failed")
		return chatRunState{}, http.StatusInternalServerError, "load history failed", err
	}
	history := historySnapshot.Messages

	registry := buildChatToolRegistry(
		reqStore,
		workspaceID,
		sessionID,
		requestWorkspaceDir,
		history,
		deps,
	)
	extSnapshot := extensions.Snapshot{}
	if deps.tooling.Extensions != nil {
		extSnapshot = deps.tooling.Extensions.Snapshot()
	}
	resolvedSkill := resolveSkillSelection(req.Message, deps.tooling.Extensions, requestWorkspaceDir, sessionID)
	invokedSkill := resolvedSkill.Definition
	resolvedProjectID, activeProject, projectPrompt, err := resolveChatProjectContext(requestWorkspaceDir, reqStore, sessionID, strings.TrimSpace(req.ProjectID))
	if err != nil {
		return chatRunState{}, http.StatusNotFound, err.Error(), err
	}
	contextDetails, err := prepareChatContextDetailsWithExtensions(requestWorkspaceDir, resolvedProjectID, sessionID, req.Message, extSnapshot, invokedSkill, deps.tooling.MemorySemanticConfig)
	if err != nil {
		return chatRunState{}, http.StatusInternalServerError, "prepare chat context failed", err
	}
	systemPrompt := contextDetails.SystemPrompt
	toolChoice := contextDetails.ToolChoice
	if strings.TrimSpace(projectPrompt) != "" {
		systemPrompt += "\n" + strings.TrimSpace(projectPrompt) + "\n"
	}
	deps.logger.Debug().
		Str("session_id", sessionID).
		Str("project_id", resolvedProjectID).
		Int("history_messages", len(history)).
		Int("history_tokens", historySnapshot.Tokens).
		Bool("compaction_used", historySnapshot.CompactionUsed).
		Int("relevant_memory_count", contextDetails.RelevantMemoryCount).
		Int("relevant_memory_tokens", contextDetails.RelevantMemoryTokens).
		Int("system_prompt_len", len(systemPrompt)).
		Int("system_prompt_tokens", promptTokenEstimate(systemPrompt)).
		Str("tool_choice", toolChoice).
		Msg("chat context assembled")

	userMsg := session.Message{Role: "user", Content: req.Message, Timestamp: time.Now().UTC()}
	if err := session.AppendMessage(transcriptPath, userMsg); err != nil {
		deps.logger.Error().Err(err).Msg("append user message failed")
		return chatRunState{}, http.StatusInternalServerError, "save message failed", err
	}

	contentBlocks := attachmentsToContentBlocks(req.Attachments)
	llmMessages := buildLLMMessagesWithBlocks(systemPrompt, history, req.Message, contentBlocks)
	authRole := strings.TrimSpace(serverauth.RoleFromRequest(r))
	injectedSchemas := resolveInjectedToolSchemas(
		registry,
		deps.tooling.ToolsDefaultSet,
		activeProject,
		authRole,
		deps.tooling.ToolsAllowHighRiskUser,
	)
	deps.logger.Debug().
		Str("session_id", sessionID).
		Int("tool_count_injected", len(injectedSchemas)).
		Strs("injected_tools", toolNamesFromSchemas(injectedSchemas)).
		Msg("tool injection result")

	return chatRunState{
		requestWorkspaceDir: requestWorkspaceDir,
		workspaceID:         workspaceID,
		store:               reqStore,
		sessionID:           sessionID,
		projectID:           resolvedProjectID,
		invokedSkill:        invokedSkill,
		invokedSkillReason:  resolvedSkill.Reason,
		transcriptPath:      transcriptPath,
		history:             history,
		registry:            registry,
		toolChoice:          toolChoice,
		llmMessages:         llmMessages,
		injectedSchemas:     injectedSchemas,
	}, 0, "", nil
}

func resolveChatProjectContext(
	workspaceDir string,
	store *session.Store,
	sessionID string,
	requestProjectID string,
) (string, *project.Project, string, error) {
	var sessionProjectID string
	if store != nil && strings.TrimSpace(sessionID) != "" {
		sess, err := store.Get(strings.TrimSpace(sessionID))
		if err == nil {
			sessionProjectID = strings.TrimSpace(sess.ProjectID)
		}
	}
	resolvedID := strings.TrimSpace(requestProjectID)
	if resolvedID == "" {
		resolvedID = sessionProjectID
	}
	if resolvedID == "" {
		return "", nil, "", nil
	}

	projectStore := project.NewStore(workspaceDir, nil)
	item, err := projectStore.Get(resolvedID)
	if err != nil {
		// Project deleted or not found — continue chat without project context
		return "", nil, "", nil
	}
	var state *project.ProjectState
	if s, err := projectStore.GetState(resolvedID); err == nil {
		state = &s
	}
	if store != nil && strings.TrimSpace(sessionID) != "" && strings.TrimSpace(requestProjectID) != "" {
		_ = store.SetProjectID(strings.TrimSpace(sessionID), item.ID)
	}
	return item.ID, &item, project.PhaseAwareProjectPromptContext(item, state), nil
}
