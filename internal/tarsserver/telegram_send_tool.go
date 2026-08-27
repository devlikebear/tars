package tarsserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/apptool"
	"github.com/rs/zerolog"
)

// telegramSendToolDeps is what the telegram_send tool needs from the server.
//
// The send and default-chat resolution used to be two anonymous closures
// inside buildAPIMux, which made a chunk of outbound-message logic reachable
// only by starting a server. Naming them here lets the behavior be tested
// directly and leaves the startup path to wiring.
type telegramSendToolDeps struct {
	// Sender performs the actual send. nil means Telegram is not configured.
	Sender telegramSendPerformer

	// AgentRuntime records outbound messages into channel history. It is
	// read through a pointer-to-pointer because the runtime is constructed
	// after this tool and assigned later during startup; nil at call time
	// simply skips the record.
	AgentRuntime **agentruntime.Runtime

	// ResolveDefaultChatID reports the chat to use when a caller omits one.
	// nil means there is no default.
	ResolveDefaultChatID func() (string, error)

	Logger zerolog.Logger
}

// telegramSendPerformer is the send half of the server's Telegram client,
// narrowed to what this tool uses so a test can substitute it.
type telegramSendPerformer interface {
	Send(ctx context.Context, req telegramSendRequest) (telegramSendResult, error)
}

// newTelegramSendTool builds the telegram_send tool from server dependencies.
func newTelegramSendTool(deps telegramSendToolDeps, enabled bool) apptool.Tool {
	return apptool.NewTelegramSendTool(
		apptool.TelegramSendFunc(func(ctx context.Context, req apptool.TelegramSendRequest) (apptool.TelegramSendResult, error) {
			return sendTelegramForTool(ctx, deps, req)
		}),
		enabled,
		apptool.TelegramDefaultChatIDResolveFunc(func(_ context.Context) (string, error) {
			if deps.ResolveDefaultChatID == nil {
				return "", nil
			}
			return deps.ResolveDefaultChatID()
		}),
	)
}

// sendTelegramForTool performs one outbound send and records it in agent
// runtime channel history when a runtime is available.
//
// A failure to record is logged and swallowed: the message has already gone
// out, and turning a history-write problem into a tool error would tell the
// model the send failed when it did not.
func sendTelegramForTool(ctx context.Context, deps telegramSendToolDeps, req apptool.TelegramSendRequest) (apptool.TelegramSendResult, error) {
	if deps.Sender == nil {
		return apptool.TelegramSendResult{}, fmt.Errorf("telegram sender is not configured")
	}
	sendResult, err := deps.Sender.Send(ctx, telegramSendRequest{
		BotID:     strings.TrimSpace(req.BotID),
		ChatID:    strings.TrimSpace(req.ChatID),
		Text:      strings.TrimSpace(req.Text),
		ThreadID:  strings.TrimSpace(req.ThreadID),
		ParseMode: strings.TrimSpace(req.ParseMode),
	})
	if err != nil {
		return apptool.TelegramSendResult{}, err
	}

	if runtime := currentTelegramAgentRuntime(deps); runtime != nil {
		if _, recordErr := runtime.OutboundTelegram(
			req.BotID, req.ChatID, req.ThreadID, req.Text,
			telegramOutboundRecordPayload(req, sendResult),
		); recordErr != nil {
			deps.Logger.Debug().
				Err(recordErr).
				Str("chat_id", strings.TrimSpace(req.ChatID)).
				Msg("telegram_send tool agent runtime record failed")
		}
	}

	return apptool.TelegramSendResult{
		MessageID: sendResult.MessageID,
		ChatID:    sendResult.ChatID,
		Text:      sendResult.Text,
	}, nil
}

func currentTelegramAgentRuntime(deps telegramSendToolDeps) *agentruntime.Runtime {
	if deps.AgentRuntime == nil {
		return nil
	}
	return *deps.AgentRuntime
}

// telegramOutboundRecordPayload builds the channel-history payload, omitting
// fields the send did not produce so the record carries no empty keys.
func telegramOutboundRecordPayload(req apptool.TelegramSendRequest, result telegramSendResult) map[string]any {
	payload := map[string]any{"provider": "telegram"}
	if botID := strings.TrimSpace(req.BotID); botID != "" {
		payload["bot_id"] = botID
	}
	if parseMode := strings.TrimSpace(req.ParseMode); parseMode != "" {
		payload["parse_mode"] = parseMode
	}
	if result.MessageID > 0 {
		payload["message_id"] = result.MessageID
	}
	if result.ChatID != "" {
		payload["provider_chat_id"] = result.ChatID
	}
	if result.Text != "" {
		payload["provider_text"] = result.Text
	}
	return payload
}
