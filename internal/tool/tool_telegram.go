package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type TelegramSendRequest struct {
	BotID     string
	ChatID    string
	Text      string
	ThreadID  string
	ParseMode string
}

type TelegramSendResult struct {
	MessageID int64  `json:"message_id"`
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
}

type TelegramSender interface {
	Send(ctx context.Context, req TelegramSendRequest) (TelegramSendResult, error)
}

type TelegramSendFunc func(ctx context.Context, req TelegramSendRequest) (TelegramSendResult, error)

func (f TelegramSendFunc) Send(ctx context.Context, req TelegramSendRequest) (TelegramSendResult, error) {
	if f == nil {
		return TelegramSendResult{}, fmt.Errorf("telegram sender is not configured")
	}
	return f(ctx, req)
}

type TelegramDefaultChatIDResolver interface {
	ResolveDefaultChatID(ctx context.Context) (string, error)
}

type TelegramDefaultChatIDResolveFunc func(ctx context.Context) (string, error)

func (f TelegramDefaultChatIDResolveFunc) ResolveDefaultChatID(ctx context.Context) (string, error) {
	if f == nil {
		return "", nil
	}
	return f(ctx)
}

func NewTelegramSendTool(sender TelegramSender, enabled bool, resolver TelegramDefaultChatIDResolver) Tool {
	return Tool{
		Name:        "telegram_send",
		Description: "Send outbound Telegram message using the configured bot.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "chat_id":{"type":"string"},
    "text":{"type":"string"},
    "thread_id":{"type":"string"},
    "parse_mode":{"type":"string"},
    "bot_id":{"type":"string"}
  },
  "required":["text"],
  "additionalProperties":false
}`),
		Execute: func(ctx context.Context, params json.RawMessage) (Result, error) {
			if !enabled {
				return jsonTextResult(map[string]any{"message": "telegram_send tool is disabled"}, true), nil
			}
			if sender == nil {
				return jsonTextResult(map[string]any{"message": "telegram sender is not configured"}, true), nil
			}
			var input struct {
				ChatID    string `json:"chat_id"`
				Text      string `json:"text"`
				ThreadID  string `json:"thread_id,omitempty"`
				ParseMode string `json:"parse_mode,omitempty"`
				BotID     string `json:"bot_id,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return jsonTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			chatID := strings.TrimSpace(input.ChatID)
			if chatID == "" && resolver != nil {
				defaultChatID, err := resolver.ResolveDefaultChatID(ctx)
				if err != nil {
					return jsonTextResult(map[string]any{"message": err.Error()}, true), nil
				}
				chatID = strings.TrimSpace(defaultChatID)
			}
			if chatID == "" {
				return jsonTextResult(map[string]any{"message": "chat_id is required (no default paired chat available)"}, true), nil
			}
			text := strings.TrimSpace(input.Text)
			if text == "" {
				return jsonTextResult(map[string]any{"message": "text is required"}, true), nil
			}
			result, err := sender.Send(ctx, TelegramSendRequest{
				BotID:     strings.TrimSpace(input.BotID),
				ChatID:    chatID,
				Text:      text,
				ThreadID:  strings.TrimSpace(input.ThreadID),
				ParseMode: strings.TrimSpace(input.ParseMode),
			})
			if err != nil {
				return jsonTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return jsonTextResult(result, false), nil
		},
	}
}
