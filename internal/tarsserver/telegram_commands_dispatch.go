package tarsserver

import (
	"context"
	"strings"
)

func (h *telegramCommandHandler) dispatchTelegramCommand(
	ctx context.Context,
	trimmed string,
	fields []string,
	currentSessionID string,
) telegramCommandResult {
	command := strings.TrimSpace(fields[0])

	switch command {
	case "/help":
		return telegramCommandResult{handled: true, result: telegramHelpText()}
	case "/sessions":
		return telegramCommandResult{handled: true, result: h.cmdSessions()}
	case "/status":
		return telegramCommandResult{handled: true, result: h.cmdStatus()}
	case "/health":
		return telegramCommandResult{handled: true, result: "SYSTEM > ok=true component=tars"}
	case "/providers":
		return telegramCommandResult{handled: true, result: h.cmdProviders()}
	case "/models":
		result, err := h.cmdModels(ctx)
		return telegramCommandResult{handled: true, result: result, err: err}
	case "/model":
		result, err := h.cmdModel(ctx, fields)
		return telegramCommandResult{handled: true, result: result, err: err}
	case "/cron":
		result, err := h.cmdCron(ctx, fields)
		return telegramCommandResult{handled: true, result: result, err: err}
	case "/gateway":
		result, err := h.cmdGateway(fields)
		return telegramCommandResult{handled: true, result: result, err: err}
	case "/channels":
		result, err := h.cmdChannels()
		return telegramCommandResult{handled: true, result: result, err: err}
	case "/new":
		if h.sessionScope == "main" {
			return telegramCommandResult{handled: true, result: blockInMainSessionMessage()}
		}
		nextSessionID, result, err := h.cmdNew(trimmed)
		return telegramCommandResult{handled: true, result: result, nextSessionID: nextSessionID, err: err}
	case "/resume":
		nextSessionID, result, err := h.cmdResume(fields)
		return telegramCommandResult{handled: true, result: result, nextSessionID: nextSessionID, err: err}
	case "/reload":
		return telegramCommandResult{handled: true, result: blockedCommandMessage("admin-only command. use tars tui or admin api token.")}
	case "/notify", "/trace", "/quit":
		return telegramCommandResult{handled: true, result: blockedCommandMessage("tui-only command.")}
	case "/telegram":
		return telegramCommandResult{handled: true, result: blockedCommandMessage("admin-only command. use tars tui or admin api token.")}
	default:
		_ = currentSessionID
		return telegramCommandResult{}
	}
}
