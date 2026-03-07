package tarsserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/extensions"
	"github.com/devlikebear/tarsncase/internal/llm"
	"github.com/devlikebear/tarsncase/internal/project"
	"github.com/devlikebear/tarsncase/internal/serverauth"
	"github.com/devlikebear/tarsncase/internal/session"
	"github.com/devlikebear/tarsncase/internal/tool"
)

type chatRunState struct {
	requestWorkspaceDir string
	workspaceID         string
	store               *session.Store
	sessionID           string
	projectID           string
	transcriptPath      string
	history             []session.Message
	registry            *tool.Registry
	toolChoice          string
	llmMessages         []llm.ChatMessage
	injectedSchemas     []llm.ToolSchema
}

func decodeChatRequestPayload(r *http.Request) (chatRequestPayload, int, string) {
	var req chatRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return chatRequestPayload{}, http.StatusBadRequest, "invalid request body"
	}
	if strings.TrimSpace(req.Message) == "" {
		return chatRequestPayload{}, http.StatusBadRequest, "message is required"
	}
	return req, 0, ""
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
	if err := maybeAutoCompactSession(requestWorkspaceDir, transcriptPath, sessionID, deps.client, deps.logger); err != nil {
		deps.logger.Error().Err(err).Str("session_id", sessionID).Msg("auto compaction failed")
		return chatRunState{}, http.StatusInternalServerError, "auto compaction failed", err
	}

	history, err := loadSessionHistory(transcriptPath, chatHistoryMaxTokens)
	if err != nil {
		deps.logger.Error().Err(err).Msg("load history failed")
		return chatRunState{}, http.StatusInternalServerError, "load history failed", err
	}

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
	invokedSkill := resolveInvokedSkill(req.Message, deps.tooling.Extensions)
	resolvedProjectID, activeProject, projectPrompt, err := resolveChatProjectContext(requestWorkspaceDir, reqStore, sessionID, strings.TrimSpace(req.ProjectID))
	if err != nil {
		return chatRunState{}, http.StatusNotFound, err.Error(), err
	}
	systemPrompt, toolChoice, _ := prepareChatContextWithExtensions(requestWorkspaceDir, resolvedProjectID, sessionID, req.Message, extSnapshot, invokedSkill)
	if strings.TrimSpace(projectPrompt) != "" {
		systemPrompt += "\n" + strings.TrimSpace(projectPrompt) + "\n"
	}
	deps.logger.Debug().
		Str("session_id", sessionID).
		Str("project_id", resolvedProjectID).
		Int("history_messages", len(history)).
		Int("system_prompt_len", len(systemPrompt)).
		Str("tool_choice", toolChoice).
		Msg("chat context assembled")

	userMsg := session.Message{Role: "user", Content: req.Message, Timestamp: time.Now().UTC()}
	if err := session.AppendMessage(transcriptPath, userMsg); err != nil {
		deps.logger.Error().Err(err).Msg("append user message failed")
		return chatRunState{}, http.StatusInternalServerError, "save message failed", err
	}

	llmMessages := buildLLMMessages(systemPrompt, history, req.Message)
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
		return "", nil, "", fmt.Errorf("project not found: %s", resolvedID)
	}
	if store != nil && strings.TrimSpace(sessionID) != "" && strings.TrimSpace(requestProjectID) != "" {
		_ = store.SetProjectID(strings.TrimSpace(sessionID), item.ID)
	}
	return item.ID, &item, formatProjectPromptSection(item), nil
}

func formatProjectPromptSection(item project.Project) string {
	return project.ProjectPromptContext(item)
}
