package tarsserver

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/agent"
	"github.com/devlikebear/tarsncase/internal/gateway"
	"github.com/devlikebear/tarsncase/internal/llm"
	"github.com/devlikebear/tarsncase/internal/prompt"
	"github.com/devlikebear/tarsncase/internal/session"
	"github.com/devlikebear/tarsncase/internal/tool"
	"github.com/rs/zerolog"
)

type telegramInboundHandler struct {
	workspaceDir string
	store        *session.Store
	llmClient    llm.Client
	sender       telegramSender
	runtime      *gateway.Runtime
	pairings     *telegramPairingStore
	dmPolicy     string
	logger       zerolog.Logger
}

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
		workspaceDir: strings.TrimSpace(workspaceDir),
		store:        store,
		llmClient:    llmClient,
		sender:       sender,
		runtime:      runtime,
		pairings:     pairings,
		dmPolicy:     normalizeTelegramDMPolicy(dmPolicy),
		logger:       logger,
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

	allowed, replyText, policyTag := h.applyPolicy(userID, chatID, username)
	if !allowed {
		if strings.TrimSpace(replyText) != "" {
			_ = h.sendMessage(ctx, chatID, threadID, replyText)
		}
		h.recordInbound(update.UpdateID, userID, chatID, threadID, text, "", policyTag)
		return
	}

	responseText, sessionID, err := h.processMessage(ctx, userID, username, text)
	if err != nil {
		h.logger.Error().Err(err).Int64("user_id", userID).Str("chat_id", chatID).Msg("telegram inbound processing failed")
		responseText = "tars failed to process your request. please try again."
	}
	if strings.TrimSpace(responseText) == "" {
		responseText = "done."
	}
	if sendErr := h.sendMessage(ctx, chatID, threadID, responseText); sendErr != nil {
		h.logger.Error().Err(sendErr).Int64("user_id", userID).Str("chat_id", chatID).Msg("telegram outbound send failed")
	}
	h.recordInbound(update.UpdateID, userID, chatID, threadID, text, sessionID, policyTag)
	h.recordOutbound(chatID, threadID, responseText, sessionID)
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
	systemPrompt := prompt.Build(prompt.BuildOptions{WorkspaceDir: h.workspaceDir})
	systemPrompt += "\n" + strings.TrimSpace(memoryToolSystemRule) + "\n"
	llmMessages := buildLLMMessages(systemPrompt, history, text)

	now := time.Now().UTC()
	if err := session.AppendMessage(transcriptPath, session.Message{
		Role:      "user",
		Content:   text,
		Timestamp: now,
	}); err != nil {
		return "", sessionID, err
	}

	// MVP constraint: inbound telegram uses one LLM turn without tools.
	loop := agent.NewLoop(h.llmClient, tool.NewRegistry())
	resp, err := loop.Run(ctx, llmMessages, agent.RunOptions{
		MaxIterations: 1,
		Tools:         nil,
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

func (h *telegramInboundHandler) recordInbound(updateID, userID int64, chatID, threadID, text, sessionID, policy string) {
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
	_, err := h.runtime.InboundTelegram("telegram", strings.TrimSpace(threadID), strings.TrimSpace(text), payload)
	if err != nil {
		h.logger.Debug().Err(err).Msg("telegram inbound gateway record failed")
	}
}

func (h *telegramInboundHandler) recordOutbound(chatID, threadID, text, sessionID string) {
	if h == nil || h.runtime == nil {
		return
	}
	payload := map[string]any{
		"provider": "telegram",
	}
	if strings.TrimSpace(sessionID) != "" {
		payload["session_id"] = strings.TrimSpace(sessionID)
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
