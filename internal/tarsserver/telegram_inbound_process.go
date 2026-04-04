package tarsserver

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/agent"
	"github.com/devlikebear/tars/internal/extensions"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/usage"
)

func (h *telegramInboundHandler) processMessage(
	ctx context.Context,
	userID int64,
	username string,
	text string,
) (string, string, error) {
	_ = userID
	_ = username
	if h.store == nil {
		return "", "", fmt.Errorf("session store is not configured")
	}
	if h.llmClient == nil {
		return "", "", fmt.Errorf("llm client is not configured")
	}
	sessionID, err := h.resolveSession(userID, username)
	if err != nil {
		return "", "", err
	}
	transcriptPath := h.store.TranscriptPath(sessionID)
	if err := maybeAutoCompactSession(h.workspaceDir, transcriptPath, sessionID, h.llmClient, h.logger, h.tooling.MemorySemanticConfig); err != nil {
		h.logger.Debug().Err(err).Str("session_id", sessionID).Msg("telegram auto compact skipped")
	}
	history, err := loadSessionHistory(transcriptPath, chatHistoryMaxTokens)
	if err != nil {
		return "", sessionID, err
	}
	registry := buildChatToolRegistry(
		h.store,
		defaultWorkspaceID,
		sessionID,
		h.workspaceDir,
		history,
		chatHandlerDeps{
			workspaceDir: h.workspaceDir,
			store:        h.store,
			client:       h.llmClient,
			logger:       h.logger,
			tooling:      h.tooling,
			extraTools:   h.extraTools,
		},
	)
	extSnapshot := extensions.Snapshot{}
	if h.tooling.Extensions != nil {
		extSnapshot = h.tooling.Extensions.Snapshot()
	}
	resolvedSkill := resolveSkillSelection(text, h.tooling.Extensions, h.workspaceDir, sessionID)
	invokedSkill := resolvedSkill.Definition
	resolvedProjectID := resolveSessionProjectID(h.store, sessionID, "")
	systemPrompt, toolChoice, err := prepareChatContextWithExtensions(h.workspaceDir, resolvedProjectID, sessionID, text, extSnapshot, invokedSkill, h.tooling.MemorySemanticConfig)
	if err != nil {
		return "", sessionID, err
	}
	llmMessages := buildLLMMessages(systemPrompt, history, text)
	injectedSchemas := resolveInjectedToolSchemas(
		registry,
		h.tooling.ToolsDefaultSet,
		nil,
		"user",
		h.tooling.ToolsAllowHighRiskUser,
	)

	now := time.Now().UTC()
	if err := session.AppendMessage(transcriptPath, session.Message{
		Role:      "user",
		Content:   text,
		Timestamp: now,
	}); err != nil {
		return "", sessionID, err
	}
	sessionProjectID := ""
	if sess, err := h.store.Get(sessionID); err == nil {
		sessionProjectID = strings.TrimSpace(sess.ProjectID)
	}
	if strings.TrimSpace(resolvedProjectID) != "" {
		sessionProjectID = strings.TrimSpace(resolvedProjectID)
	}
	runCtx := usage.WithCallMeta(ctx, usage.CallMeta{
		Source:    "chat",
		SessionID: sessionID,
		ProjectID: sessionProjectID,
	})

	loop := agent.NewLoop(h.llmClient, registry)
	resp, err := loop.Run(runCtx, llmMessages, agent.RunOptions{
		MaxIterations: resolveAgentMaxIterations(h.maxIterations),
		Tools:         injectedSchemas,
		ToolChoice:    toolChoice,
	})
	if err != nil {
		return "", sessionID, err
	}
	answer := strings.TrimSpace(resp.Message.Content)
	if answer == "" {
		answer = "(empty response)"
	}
	if invokedSkill != nil {
		notice := "SYSTEM > using skill " + strings.TrimSpace(invokedSkill.Name)
		if strings.TrimSpace(resolvedSkill.Reason) != "" {
			notice += " reason=" + strings.TrimSpace(resolvedSkill.Reason)
		}
		answer = notice + "\n\n" + answer
	}
	assistantAt := time.Now().UTC()
	if err := session.AppendMessage(transcriptPath, session.Message{
		Role:      "assistant",
		Content:   answer,
		Timestamp: assistantAt,
	}); err != nil {
		return "", sessionID, err
	}
	if err := h.store.Touch(sessionID, assistantAt); err != nil {
		h.logger.Debug().Err(err).Str("session_id", sessionID).Msg("telegram touch session failed")
	}
	projectID := ""
	if h.store != nil {
		if sess, getErr := h.store.Get(sessionID); getErr == nil {
			projectID = strings.TrimSpace(sess.ProjectID)
		}
	}
	if err := applyPostChatMemoryHooks(chatMemoryHookInput{
		WorkspaceDir:     h.workspaceDir,
		SessionID:        sessionID,
		ProjectID:        projectID,
		UserMessage:      text,
		AssistantMessage: answer,
		AssistantTime:    assistantAt,
		LLMClient:        h.llmClient,
	}); err != nil {
		h.logger.Debug().Err(err).Str("session_id", sessionID).Msg("telegram write chat memory failed")
	}
	return answer, sessionID, nil
}

func extractTelegramInboundMedia(msg *telegramMessage) (telegramInboundMedia, bool) {
	if msg == nil {
		return telegramInboundMedia{}, false
	}
	if len(msg.Photo) > 0 {
		photo := msg.Photo[len(msg.Photo)-1]
		return telegramInboundMedia{
			Type:         "photo",
			FileID:       strings.TrimSpace(photo.FileID),
			OriginalName: "photo.jpg",
			MimeType:     "image/jpeg",
			FileSize:     photo.FileSize,
		}, strings.TrimSpace(photo.FileID) != ""
	}
	if msg.Document != nil {
		return telegramInboundMedia{
			Type:         "document",
			FileID:       strings.TrimSpace(msg.Document.FileID),
			OriginalName: strings.TrimSpace(msg.Document.FileName),
			MimeType:     strings.TrimSpace(msg.Document.MimeType),
			FileSize:     msg.Document.FileSize,
		}, strings.TrimSpace(msg.Document.FileID) != ""
	}
	if msg.Voice != nil {
		return telegramInboundMedia{
			Type:         "voice",
			FileID:       strings.TrimSpace(msg.Voice.FileID),
			OriginalName: "voice.ogg",
			MimeType:     strings.TrimSpace(msg.Voice.MimeType),
			FileSize:     msg.Voice.FileSize,
		}, strings.TrimSpace(msg.Voice.FileID) != ""
	}
	return telegramInboundMedia{}, false
}

func formatTelegramMediaPrompt(saved telegramSavedMedia, text string) string {
	var b strings.Builder
	b.WriteString("[Attached file]\n")
	b.WriteString("type: " + strings.TrimSpace(saved.Type) + "\n")
	b.WriteString("saved_path: " + strings.TrimSpace(saved.SavedPath) + "\n")
	b.WriteString("mime: " + strings.TrimSpace(saved.MimeType) + "\n")
	b.WriteString("size: " + strconv.FormatInt(saved.Size, 10) + "\n")
	b.WriteString("original_name: " + strings.TrimSpace(saved.OriginalName))
	if strings.TrimSpace(text) != "" {
		b.WriteString("\n\n")
		b.WriteString(strings.TrimSpace(text))
	}
	return b.String()
}
