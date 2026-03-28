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
	case "/session":
		return telegramCommandResult{handled: true, result: h.cmdSession()}
	case "/sessions":
		return telegramCommandResult{handled: true, result: blockInMainSessionMessage()}
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
		return telegramCommandResult{handled: true, result: "SYSTEM > unsupported command. use /models"}
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
		return telegramCommandResult{handled: true, result: blockInMainSessionMessage()}
	case "/resume":
		return telegramCommandResult{handled: true, result: blockInMainSessionMessage()}
	case "/reload":
		return telegramCommandResult{handled: true, result: blockedCommandMessage("admin-only command. use the web console or an admin api token.")}
	case "/notify", "/trace", "/quit":
		return telegramCommandResult{handled: true, result: blockedCommandMessage("console-only command. use the web console.")}
	case "/telegram":
		return telegramCommandResult{handled: true, result: blockedCommandMessage("admin-only command. use the web console or an admin api token.")}
	default:
		_ = currentSessionID
		return telegramCommandResult{}
	}
}
