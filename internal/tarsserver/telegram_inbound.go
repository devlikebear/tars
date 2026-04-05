package tarsserver

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/approval"
	"github.com/devlikebear/tars/internal/gateway"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/rs/zerolog"
)

type telegramInboundHandler struct {
	workspaceDir  string
	store         *session.Store
	llmClient     llm.Client
	llmRouter     llm.Router
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
	otpManager    *approval.OTPManager
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

	if strings.TrimSpace(text) != "" && h.otpManager != nil && h.otpManager.Consume(chatID, text) {
		ack := "otp code received."
		_ = h.sendMessageChunks(ctx, chatID, threadID, ack)
		h.recordInbound(update.UpdateID, userID, chatID, threadID, text, "", "otp", inboundPayload)
		h.recordOutbound(chatID, threadID, ack, "", inboundPayload)
		return
	}

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
	responseText, sessionID, err := h.processMessage(ctx, userID, username, chatID, threadID, inputText)
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
