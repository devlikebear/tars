package tarsserver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

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
