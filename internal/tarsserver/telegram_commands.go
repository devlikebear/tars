package tarsserver

import (
	"context"
	"strings"

	"github.com/devlikebear/tars/internal/gateway"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

const telegramMaxMessageLength = 4096

type telegramCommandExecutor interface {
	Execute(ctx context.Context, line, currentSessionID string) (handled bool, result string, nextSessionID string, err error)
}

type telegramCommandExecFunc func(ctx context.Context, line, currentSessionID string) (handled bool, result string, nextSessionID string, err error)

func (f telegramCommandExecFunc) Execute(ctx context.Context, line, currentSessionID string) (bool, string, string, error) {
	if f == nil {
		return false, "", "", nil
	}
	return f(ctx, line, currentSessionID)
}

type telegramCommandHandlerOptions struct {
	Store          *session.Store
	CronResolver   *workspaceCronStoreResolver
	Runtime        *gateway.Runtime
	MainSession    string
	SessionScope   string
	ProviderModels *providerModelsService
	Logger         zerolog.Logger
}

type telegramCommandHandler struct {
	store          *session.Store
	cronResolver   *workspaceCronStoreResolver
	runtime        *gateway.Runtime
	mainSession    string
	sessionScope   string
	providerModels *providerModelsService
	logger         zerolog.Logger
}

type telegramCommandResult struct {
	handled       bool
	result        string
	nextSessionID string
	err           error
}

func newTelegramCommandHandler(opts telegramCommandHandlerOptions) *telegramCommandHandler {
	return &telegramCommandHandler{
		store:          opts.Store,
		cronResolver:   opts.CronResolver,
		runtime:        opts.Runtime,
		mainSession:    strings.TrimSpace(opts.MainSession),
		sessionScope:   normalizeTelegramSessionScope(opts.SessionScope),
		providerModels: opts.ProviderModels,
		logger:         opts.Logger,
	}
}

func (h *telegramCommandHandler) Execute(ctx context.Context, line, currentSessionID string) (bool, string, string, error) {
	if h == nil {
		return false, "", "", nil
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || !strings.HasPrefix(trimmed, "/") {
		return false, "", "", nil
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return false, "", "", nil
	}
	result := h.dispatchTelegramCommand(ctx, trimmed, fields, currentSessionID)
	return result.handled, result.result, result.nextSessionID, result.err
}
