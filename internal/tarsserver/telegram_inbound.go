package tarsserver

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/agent"
	"github.com/devlikebear/tarsncase/internal/extensions"
	"github.com/devlikebear/tarsncase/internal/gateway"
	"github.com/devlikebear/tarsncase/internal/llm"
	"github.com/devlikebear/tarsncase/internal/session"
	"github.com/devlikebear/tarsncase/internal/tool"
	"github.com/rs/zerolog"
)

type telegramInboundHandler struct {
	workspaceDir  string
	store         *session.Store
	llmClient     llm.Client
	sender        telegramSender
	commands      telegramCommandExecutor
	media         telegramMediaDownloader
	runtime       *gateway.Runtime
	pairings      *telegramPairingStore
	dmPolicy      string
	sessionScope  string
	mainSessionID string
	maxIterations int
	tooling       chatToolingOptions
	extraTools    []tool.Tool
	logger        zerolog.Logger
}

const telegramTypingInterval = 4 * time.Second

func newTelegramInboundHandler(
	workspaceDir string,
	store *session.Store,
	llmClient llm.Client,
	sender telegramSender,
	runtime *gateway.Runtime,
	pairings *telegramPairingStore,
	dmPolicy string,
	logger zerolog.Logger,
) *telegramInboundHandler {
	return &telegramInboundHandler{
		workspaceDir:  strings.TrimSpace(workspaceDir),
		store:         store,
		llmClient:     llmClient,
		sender:        sender,
		runtime:       runtime,
		pairings:      pairings,
		dmPolicy:      normalizeTelegramDMPolicy(dmPolicy),
		sessionScope:  "per-user",
		maxIterations: 1,
		logger:        logger,
	}
}

func (h *telegramInboundHandler) HandleUpdate(ctx context.Context, update telegramUpdate) {
	if h == nil || update.Message == nil {
		return
	}
	msg := update.Message
	if !strings.EqualFold(strings.TrimSpace(msg.Chat.Type), "private") {
		return
	}
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		text = strings.TrimSpace(msg.Caption)
	}
	mediaReq, hasMedia := extractTelegramInboundMedia(msg)
	if text == "" && !hasMedia {
		return
	}
	userID := msg.From.IDInt64()
	chatID := msg.Chat.IDString()
	if userID <= 0 || chatID == "" {
		return
	}
	threadID := ""
	if msg.MessageThreadID > 0 {
		threadID = strconv.FormatInt(msg.MessageThreadID, 10)
	}
	username := strings.TrimSpace(msg.From.DisplayName())
	inboundPayload := map[string]any{}

	allowed, replyText, policyTag := h.applyPolicy(userID, chatID, username)
	if !allowed {
		if strings.TrimSpace(replyText) != "" {
			_ = h.sendMessage(ctx, chatID, threadID, replyText)
		}
		h.recordInbound(update.UpdateID, userID, chatID, threadID, text, "", policyTag, inboundPayload)
		return
	}

	currentSessionID := h.currentSessionID(userID)
	inputText := text
	if hasMedia {
		if h.media == nil {
			responseText := "attachment support is not configured on this server."
			_ = h.sendMessageChunks(ctx, chatID, threadID, responseText)
			h.recordInbound(update.UpdateID, userID, chatID, threadID, text, currentSessionID, policyTag, inboundPayload)
			h.recordOutbound(chatID, threadID, responseText, currentSessionID, inboundPayload)
			return
		}
		saved, mediaErr := h.media.DownloadAndSave(ctx, chatID, mediaReq)
		if mediaErr != nil {
			responseText := "failed to download attachment."
			if strings.Contains(strings.ToLower(mediaErr.Error()), "too large") {
				responseText = "attachment is too large. max size is 20MB."
			}
			_ = h.sendMessageChunks(ctx, chatID, threadID, responseText)
			h.recordInbound(update.UpdateID, userID, chatID, threadID, text, currentSessionID, policyTag, inboundPayload)
			h.recordOutbound(chatID, threadID, responseText, currentSessionID, inboundPayload)
			return
		}
		inboundPayload["media_type"] = strings.TrimSpace(saved.Type)
		inboundPayload["media_saved_path"] = strings.TrimSpace(saved.SavedPath)
		inboundPayload["media_mime"] = strings.TrimSpace(saved.MimeType)
		inboundPayload["media_size"] = saved.Size
		if strings.TrimSpace(saved.OriginalName) != "" {
			inboundPayload["media_original_name"] = strings.TrimSpace(saved.OriginalName)
		}
		if strings.TrimSpace(text) == "" {
			responseText := fmt.Sprintf(
				"attachment saved: %s\nsend a caption or text instruction to continue.",
				strings.TrimSpace(saved.SavedPath),
			)
			_ = h.sendMessageChunks(ctx, chatID, threadID, responseText)
			h.recordInbound(update.UpdateID, userID, chatID, threadID, text, currentSessionID, policyTag, inboundPayload)
			h.recordOutbound(chatID, threadID, responseText, currentSessionID, inboundPayload)
			return
		}
		inputText = formatTelegramMediaPrompt(saved, text)
	}
	if strings.HasPrefix(text, "/") && h.commands != nil {
		handled, result, nextSessionID, cmdErr := h.commands.Execute(ctx, text, currentSessionID)
		if cmdErr != nil {
			result = "SYSTEM > " + strings.TrimSpace(cmdErr.Error())
			handled = true
		}
		if handled {
			sessionID := strings.TrimSpace(currentSessionID)
			if normalizeTelegramSessionScope(h.sessionScope) == "per-user" && strings.TrimSpace(nextSessionID) != "" {
				if h.pairings != nil {
					if err := h.pairings.bindSession(userID, nextSessionID); err != nil {
						h.logger.Debug().Err(err).Int64("user_id", userID).Msg("bind telegram command session failed")
					}
				}
				sessionID = strings.TrimSpace(nextSessionID)
			}
			if strings.TrimSpace(result) == "" {
				result = "done."
			}
			if sendErr := h.sendMessageChunks(ctx, chatID, threadID, result); sendErr != nil {
				h.logger.Error().Err(sendErr).Int64("user_id", userID).Str("chat_id", chatID).Msg("telegram command send failed")
			}
			h.recordInbound(update.UpdateID, userID, chatID, threadID, text, sessionID, policyTag, inboundPayload)
			h.recordOutbound(chatID, threadID, result, sessionID, inboundPayload)
			return
		}
	}

	typingCancel := h.startTypingLoop(ctx, chatID, threadID)
	responseText, sessionID, err := h.processMessage(ctx, userID, username, inputText)
	typingCancel()
	if err != nil {
		h.logger.Error().Err(err).Int64("user_id", userID).Str("chat_id", chatID).Msg("telegram inbound processing failed")
		responseText = "tars failed to process your request. please try again."
	}
	if strings.TrimSpace(responseText) == "" {
		responseText = "done."
	}
	if sendErr := h.sendMessageChunks(ctx, chatID, threadID, responseText); sendErr != nil {
		h.logger.Error().Err(sendErr).Int64("user_id", userID).Str("chat_id", chatID).Msg("telegram outbound send failed")
	}
	h.recordInbound(update.UpdateID, userID, chatID, threadID, text, sessionID, policyTag, inboundPayload)
	h.recordOutbound(chatID, threadID, responseText, sessionID, inboundPayload)
}

func (h *telegramInboundHandler) applyPolicy(userID int64, chatID, username string) (bool, string, string) {
	policy := normalizeTelegramDMPolicy(h.dmPolicy)
	switch policy {
	case "disabled":
		return false, "telegram direct messages are disabled.", policy
	case "open":
		return true, "", policy
	case "allowlist":
		if h.pairings == nil || !h.pairings.isAllowed(userID) {
			return false, "telegram access is restricted. ask the bot owner to allow your account.", policy
		}
		return true, "", policy
	default:
		if h.pairings == nil {
			return false, "telegram pairing is not configured.", policy
		}
		if h.pairings.isAllowed(userID) {
			return true, "", policy
		}
		entry, _, err := h.pairings.issue(telegramPairingIdentity{
			UserID:   userID,
			ChatID:   chatID,
			Username: username,
		}, telegramPairingTTL)
		if err != nil {
			h.logger.Error().Err(err).Int64("user_id", userID).Str("chat_id", chatID).Msg("telegram pairing issue failed")
			return false, "failed to issue pairing code. please retry.", policy
		}
		message := fmt.Sprintf(
			"Pairing code: %s\nAsk the bot owner to approve with: /telegram pairing approve %s",
			entry.Code,
			entry.Code,
		)
		return false, message, policy
	}
}

func (h *telegramInboundHandler) processMessage(
	ctx context.Context,
	userID int64,
	username string,
	text string,
) (string, string, error) {
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
	// Reuse the same compaction guard as HTTP chat path for transcript health.
	// If latency becomes an issue on telegram path, this can be skipped behind a flag.
	if err := maybeAutoCompactSession(h.workspaceDir, transcriptPath, sessionID, h.llmClient, h.logger); err != nil {
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
	invokedSkill := resolveInvokedSkill(text, h.tooling.Extensions)
	systemPrompt, toolChoice, err := prepareChatContextWithExtensions(h.workspaceDir, text, extSnapshot, invokedSkill)
	if err != nil {
		return "", sessionID, err
	}
	llmMessages := buildLLMMessages(systemPrompt, history, text)
	injectedSchemas := registry.Schemas()

	now := time.Now().UTC()
	if err := session.AppendMessage(transcriptPath, session.Message{
		Role:      "user",
		Content:   text,
		Timestamp: now,
	}); err != nil {
		return "", sessionID, err
	}

	loop := agent.NewLoop(h.llmClient, registry)
	resp, err := loop.Run(ctx, llmMessages, agent.RunOptions{
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
	if err := writeChatMemory(h.workspaceDir, sessionID, text, answer, assistantAt); err != nil {
		h.logger.Debug().Err(err).Str("session_id", sessionID).Msg("telegram write chat memory failed")
	}
	return answer, sessionID, nil
}

func (h *telegramInboundHandler) resolveSession(userID int64, username string) (string, error) {
	if userID <= 0 {
		return "", fmt.Errorf("user id is required")
	}
	if normalizeTelegramSessionScope(h.sessionScope) == "main" {
		mainSessionID := strings.TrimSpace(h.mainSessionID)
		if mainSessionID != "" {
			if _, err := h.store.Get(mainSessionID); err != nil {
				return "", err
			}
			return mainSessionID, nil
		}
	}
	if h.pairings != nil {
		if sessionID := strings.TrimSpace(h.pairings.sessionID(userID)); sessionID != "" {
			if _, err := h.store.Get(sessionID); err == nil {
				return sessionID, nil
			}
		}
	}

	title := strings.TrimSpace(username)
	if title == "" {
		title = strconv.FormatInt(userID, 10)
	}
	created, err := h.store.Create("telegram:" + title)
	if err != nil {
		return "", err
	}
	if h.pairings != nil {
		if err := h.pairings.bindSession(userID, created.ID); err != nil {
			h.logger.Debug().Err(err).Int64("user_id", userID).Msg("bind telegram session failed")
		}
	}
	return created.ID, nil
}

func (h *telegramInboundHandler) sendMessage(ctx context.Context, chatID, threadID, text string) error {
	if h == nil || h.sender == nil {
		return fmt.Errorf("telegram sender is not configured")
	}
	sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := h.sender.Send(sendCtx, telegramSendRequest{
		ChatID:   strings.TrimSpace(chatID),
		ThreadID: strings.TrimSpace(threadID),
		Text:     strings.TrimSpace(text),
	})
	return err
}

func (h *telegramInboundHandler) sendMessageChunks(ctx context.Context, chatID, threadID, text string) error {
	chunks := splitTelegramMessage(text, telegramMaxMessageLength)
	for _, chunk := range chunks {
		if err := h.sendMessage(ctx, chatID, threadID, chunk); err != nil {
			return err
		}
	}
	return nil
}

func (h *telegramInboundHandler) startTypingLoop(parent context.Context, chatID, threadID string) context.CancelFunc {
	if h == nil || h.sender == nil {
		return func() {}
	}
	ctx, cancel := context.WithCancel(parent)
	go func() {
		sendAction := func() {
			actionCtx, actionCancel := context.WithTimeout(ctx, 2*time.Second)
			defer actionCancel()
			err := h.sender.SendChatAction(actionCtx, telegramChatActionRequest{
				ChatID:   strings.TrimSpace(chatID),
				ThreadID: strings.TrimSpace(threadID),
				Action:   "typing",
			})
			if shouldLogTelegramTypingError(err) {
				h.logger.Debug().Err(err).Str("chat_id", chatID).Msg("telegram typing action failed")
			}
		}
		sendAction()
		ticker := time.NewTicker(telegramTypingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sendAction()
			}
		}
	}()
	return cancel
}

func shouldLogTelegramTypingError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return true
}

func (h *telegramInboundHandler) currentSessionID(userID int64) string {
	if normalizeTelegramSessionScope(h.sessionScope) == "main" {
		return strings.TrimSpace(h.mainSessionID)
	}
	if h.pairings == nil || userID <= 0 {
		return ""
	}
	return strings.TrimSpace(h.pairings.sessionID(userID))
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

func (h *telegramInboundHandler) recordInbound(updateID, userID int64, chatID, threadID, text, sessionID, policy string, extraPayload map[string]any) {
	if h == nil || h.runtime == nil {
		return
	}
	payload := map[string]any{
		"provider":  "telegram",
		"update_id": updateID,
		"user_id":   userID,
		"chat_id":   strings.TrimSpace(chatID),
	}
	if strings.TrimSpace(policy) != "" {
		payload["dm_policy"] = strings.TrimSpace(policy)
	}
	if strings.TrimSpace(sessionID) != "" {
		payload["session_id"] = strings.TrimSpace(sessionID)
	}
	for key, value := range extraPayload {
		payload[strings.TrimSpace(key)] = value
	}
	_, err := h.runtime.InboundTelegram("telegram", strings.TrimSpace(threadID), strings.TrimSpace(text), payload)
	if err != nil {
		h.logger.Debug().Err(err).Msg("telegram inbound gateway record failed")
	}
}

func (h *telegramInboundHandler) recordOutbound(chatID, threadID, text, sessionID string, extraPayload map[string]any) {
	if h == nil || h.runtime == nil {
		return
	}
	payload := map[string]any{
		"provider": "telegram",
	}
	if strings.TrimSpace(sessionID) != "" {
		payload["session_id"] = strings.TrimSpace(sessionID)
	}
	for key, value := range extraPayload {
		payload[strings.TrimSpace(key)] = value
	}
	_, err := h.runtime.OutboundTelegram("telegram", strings.TrimSpace(chatID), strings.TrimSpace(threadID), strings.TrimSpace(text), payload)
	if err != nil {
		h.logger.Debug().Err(err).Msg("telegram outbound gateway record failed")
	}
}

func normalizeTelegramDMPolicy(raw string) string {
	policy := strings.TrimSpace(strings.ToLower(raw))
	switch policy {
	case "pairing", "allowlist", "open", "disabled":
		return policy
	default:
		return "pairing"
	}
}

func normalizeTelegramSessionScope(raw string) string {
	scope := strings.TrimSpace(strings.ToLower(raw))
	switch scope {
	case "main", "per-user":
		return scope
	default:
		return "main"
	}
}
