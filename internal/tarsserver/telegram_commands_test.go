package tarsserver

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tarsncase/internal/config"
	"github.com/devlikebear/tarsncase/internal/cron"
	"github.com/devlikebear/tarsncase/internal/gateway"
	"github.com/devlikebear/tarsncase/internal/session"
	"github.com/rs/zerolog"
)

func TestTelegramCommand_Help_ReturnsCommands(t *testing.T) {
	workspace := t.TempDir()
	store := session.NewStore(workspace)
	handler := newTelegramCommandHandler(telegramCommandHandlerOptions{
		Store:        store,
		CronResolver: newWorkspaceCronStoreResolver(workspace, 0, cron.NewStore(workspace)),
		Runtime:      nil,
		MainSession:  "sess-main",
		SessionScope: "main",
		Logger:       zerolog.Nop(),
	})

	handled, result, nextSession, err := handler.Execute(context.Background(), "/help", "")
	if err != nil {
		t.Fatalf("Execute /help: %v", err)
	}
	if !handled {
		t.Fatalf("expected /help to be handled")
	}
	if strings.TrimSpace(nextSession) != "" {
		t.Fatalf("expected no next session for /help, got %q", nextSession)
	}
	if !strings.Contains(result, "/help") || !strings.Contains(result, "/status") {
		t.Fatalf("expected help output, got %q", result)
	}
	if !strings.Contains(result, "/providers") || !strings.Contains(result, "/models") {
		t.Fatalf("expected provider/model commands in help output, got %q", result)
	}
}

func TestTelegramCommand_Allowed_Providers(t *testing.T) {
	workspace := t.TempDir()
	store := session.NewStore(workspace)
	cache, err := newProviderModelsCache(filepath.Join(workspace, "provider_models_cache.json"), providerModelsCacheTTL, time.Now)
	if err != nil {
		t.Fatalf("newProviderModelsCache: %v", err)
	}
	service := newProviderModelsService(config.Config{
		LLMProvider: "openai-codex",
		LLMModel:    "gpt-5.3-codex",
		LLMAuthMode: "oauth",
	}, cache, &fakeModelFetcher{}, time.Now)
	handler := newTelegramCommandHandler(telegramCommandHandlerOptions{
		Store:          store,
		CronResolver:   newWorkspaceCronStoreResolver(workspace, 0, cron.NewStore(workspace)),
		Runtime:        nil,
		MainSession:    "sess-main",
		SessionScope:   "main",
		ProviderModels: service,
		Logger:         zerolog.New(io.Discard),
	})

	handled, result, nextSession, err := handler.Execute(context.Background(), "/providers", "")
	if err != nil {
		t.Fatalf("Execute /providers: %v", err)
	}
	if !handled || strings.TrimSpace(nextSession) != "" {
		t.Fatalf("expected handled without session switch, handled=%t next=%q", handled, nextSession)
	}
	if !strings.Contains(result, "provider=openai-codex") || !strings.Contains(result, "openai-codex") {
		t.Fatalf("unexpected /providers output: %q", result)
	}
}

func TestTelegramCommand_Allowed_ModelsUnsupportedForOpenAICodex(t *testing.T) {
	workspace := t.TempDir()
	store := session.NewStore(workspace)
	cache, err := newProviderModelsCache(filepath.Join(workspace, "provider_models_cache.json"), providerModelsCacheTTL, time.Now)
	if err != nil {
		t.Fatalf("newProviderModelsCache: %v", err)
	}
	service := newProviderModelsService(config.Config{
		LLMProvider: "openai-codex",
		LLMModel:    "gpt-5.3-codex",
		LLMAuthMode: "oauth",
		LLMBaseURL:  "https://chatgpt.com/backend-api",
	}, cache, &fakeModelFetcher{models: []string{"gpt-5.3-codex", "gpt-4.1-codex"}}, time.Now)
	handler := newTelegramCommandHandler(telegramCommandHandlerOptions{
		Store:          store,
		CronResolver:   newWorkspaceCronStoreResolver(workspace, 0, cron.NewStore(workspace)),
		Runtime:        nil,
		MainSession:    "sess-main",
		SessionScope:   "main",
		ProviderModels: service,
		Logger:         zerolog.New(io.Discard),
	})

	handled, result, nextSession, err := handler.Execute(context.Background(), "/models", "")
	if err == nil {
		t.Fatal("expected /models error for openai-codex")
	}
	if !handled || strings.TrimSpace(nextSession) != "" {
		t.Fatalf("expected handled without session switch, handled=%t next=%q", handled, nextSession)
	}
	if strings.TrimSpace(result) != "" {
		t.Fatalf("expected empty result on error, got %q", result)
	}
	if !strings.Contains(err.Error(), "unsupported for llm provider") {
		t.Fatalf("unexpected /models error: %v", err)
	}
}

func TestTelegramCommand_Allowed_ModelListIsUnsupported(t *testing.T) {
	workspace := t.TempDir()
	store := session.NewStore(workspace)
	handler := newTelegramCommandHandler(telegramCommandHandlerOptions{
		Store:        store,
		CronResolver: newWorkspaceCronStoreResolver(workspace, 0, cron.NewStore(workspace)),
		Runtime:      nil,
		MainSession:  "sess-main",
		SessionScope: "main",
		Logger:       zerolog.New(io.Discard),
	})

	handled, result, _, err := handler.Execute(context.Background(), "/model list", "")
	if err != nil {
		t.Fatalf("Execute /model list: %v", err)
	}
	if !handled {
		t.Fatalf("expected /model list to be handled")
	}
	if !strings.Contains(result, "unsupported command") || !strings.Contains(result, "/models") {
		t.Fatalf("unexpected /model list output: %q", result)
	}
}

func TestTelegramCommand_Allowed_Status(t *testing.T) {
	workspace := t.TempDir()
	store := session.NewStore(workspace)
	if _, err := store.Create("main"); err != nil {
		t.Fatalf("create session: %v", err)
	}
	handler := newTelegramCommandHandler(telegramCommandHandlerOptions{
		Store:        store,
		CronResolver: newWorkspaceCronStoreResolver(workspace, 0, cron.NewStore(workspace)),
		Runtime:      nil,
		MainSession:  "sess-main",
		SessionScope: "main",
		Logger:       zerolog.Nop(),
	})

	handled, result, _, err := handler.Execute(context.Background(), "/status", "")
	if err != nil {
		t.Fatalf("Execute /status: %v", err)
	}
	if !handled {
		t.Fatalf("expected /status to be handled")
	}
	if !strings.Contains(result, "sessions=") {
		t.Fatalf("expected status output, got %q", result)
	}
}

func TestTelegramCommand_Blocked_MainScopeSessionSwitch(t *testing.T) {
	workspace := t.TempDir()
	store := session.NewStore(workspace)
	handler := newTelegramCommandHandler(telegramCommandHandlerOptions{
		Store:        store,
		CronResolver: newWorkspaceCronStoreResolver(workspace, 0, cron.NewStore(workspace)),
		Runtime:      nil,
		MainSession:  "sess-main",
		SessionScope: "main",
		Logger:       zerolog.Nop(),
	})

	handled, result, nextSession, err := handler.Execute(context.Background(), "/new test", "")
	if err != nil {
		t.Fatalf("Execute /new: %v", err)
	}
	if !handled {
		t.Fatalf("expected /new to be handled")
	}
	if strings.TrimSpace(nextSession) != "" {
		t.Fatalf("expected no session switch in main scope, got %q", nextSession)
	}
	if !strings.Contains(strings.ToLower(result), "main session mode") {
		t.Fatalf("expected main scope block message, got %q", result)
	}
}

func TestTelegramCommand_New_PerUserCreatesAndReturnsNextSession(t *testing.T) {
	workspace := t.TempDir()
	store := session.NewStore(workspace)
	handler := newTelegramCommandHandler(telegramCommandHandlerOptions{
		Store:        store,
		CronResolver: newWorkspaceCronStoreResolver(workspace, 0, cron.NewStore(workspace)),
		Runtime:      nil,
		MainSession:  "sess-main",
		SessionScope: "per-user",
		Logger:       zerolog.Nop(),
	})

	handled, result, nextSession, err := handler.Execute(context.Background(), "/new research thread", "")
	if err != nil {
		t.Fatalf("Execute /new: %v", err)
	}
	if !handled {
		t.Fatalf("expected /new to be handled")
	}
	if strings.TrimSpace(nextSession) == "" {
		t.Fatalf("expected next session id")
	}
	if !strings.Contains(result, "created session") || !strings.Contains(result, "research thread") {
		t.Fatalf("unexpected /new output: %q", result)
	}
	sess, err := store.Get(nextSession)
	if err != nil {
		t.Fatalf("get created session: %v", err)
	}
	if sess.Title != "research thread" {
		t.Fatalf("unexpected created title: %+v", sess)
	}
}

func TestTelegramCommand_ResumeMain_AllowedInMainScope(t *testing.T) {
	workspace := t.TempDir()
	store := session.NewStore(workspace)
	mainSession, err := store.Create("main")
	if err != nil {
		t.Fatalf("create main session: %v", err)
	}
	handler := newTelegramCommandHandler(telegramCommandHandlerOptions{
		Store:        store,
		CronResolver: newWorkspaceCronStoreResolver(workspace, 0, cron.NewStore(workspace)),
		Runtime:      nil,
		MainSession:  mainSession.ID,
		SessionScope: "main",
		Logger:       zerolog.Nop(),
	})

	handled, result, nextSession, err := handler.Execute(context.Background(), "/resume main", "")
	if err != nil {
		t.Fatalf("Execute /resume main: %v", err)
	}
	if !handled {
		t.Fatalf("expected /resume main to be handled")
	}
	if strings.TrimSpace(nextSession) != "" {
		t.Fatalf("expected no session switch token in main scope, got %q", nextSession)
	}
	if !strings.Contains(result, mainSession.ID) {
		t.Fatalf("expected main session id in result, got %q", result)
	}
}

func TestTelegramCommand_ResumeLatest_BlockedInMainScope(t *testing.T) {
	workspace := t.TempDir()
	store := session.NewStore(workspace)
	mainSession, err := store.Create("main")
	if err != nil {
		t.Fatalf("create main session: %v", err)
	}
	handler := newTelegramCommandHandler(telegramCommandHandlerOptions{
		Store:        store,
		CronResolver: newWorkspaceCronStoreResolver(workspace, 0, cron.NewStore(workspace)),
		Runtime:      nil,
		MainSession:  mainSession.ID,
		SessionScope: "main",
		Logger:       zerolog.Nop(),
	})

	handled, result, nextSession, err := handler.Execute(context.Background(), "/resume latest", "")
	if err != nil {
		t.Fatalf("Execute /resume latest: %v", err)
	}
	if !handled {
		t.Fatalf("expected /resume latest to be handled")
	}
	if strings.TrimSpace(nextSession) != "" {
		t.Fatalf("expected no next session in main scope, got %q", nextSession)
	}
	if !strings.Contains(strings.ToLower(result), "main session mode") {
		t.Fatalf("expected block message, got %q", result)
	}
}

func TestTelegramCommand_ResumeByIndex_UsesUpdatedAtOrder(t *testing.T) {
	workspace := t.TempDir()
	store := session.NewStore(workspace)
	first, err := store.Create("first")
	if err != nil {
		t.Fatalf("create first session: %v", err)
	}
	second, err := store.Create("second")
	if err != nil {
		t.Fatalf("create second session: %v", err)
	}
	if err := store.Touch(first.ID, time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("touch first: %v", err)
	}
	if err := store.Touch(second.ID, time.Date(2026, 3, 7, 11, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("touch second: %v", err)
	}

	handler := newTelegramCommandHandler(telegramCommandHandlerOptions{
		Store:        store,
		CronResolver: newWorkspaceCronStoreResolver(workspace, 0, cron.NewStore(workspace)),
		Runtime:      nil,
		MainSession:  "sess-main",
		SessionScope: "per-user",
		Logger:       zerolog.Nop(),
	})

	handled, result, nextSession, err := handler.Execute(context.Background(), "/resume 2", "")
	if err != nil {
		t.Fatalf("Execute /resume 2: %v", err)
	}
	if !handled {
		t.Fatalf("expected /resume 2 to be handled")
	}
	if nextSession != first.ID {
		t.Fatalf("expected second ordered entry to be first session %q, got %q", first.ID, nextSession)
	}
	if !strings.Contains(result, first.ID) {
		t.Fatalf("expected resume output to mention %q, got %q", first.ID, result)
	}
}

func TestTelegramCommand_FallbackToChat_Unknown(t *testing.T) {
	handler := newTelegramCommandHandler(telegramCommandHandlerOptions{
		Store:        session.NewStore(t.TempDir()),
		CronResolver: newWorkspaceCronStoreResolver(t.TempDir(), 0, cron.NewStore(t.TempDir())),
		Runtime:      nil,
		MainSession:  "sess-main",
		SessionScope: "main",
		Logger:       zerolog.Nop(),
	})

	handled, result, nextSession, err := handler.Execute(context.Background(), "/unknown-skill arg", "")
	if err != nil {
		t.Fatalf("Execute unknown command: %v", err)
	}
	if handled {
		t.Fatalf("expected unknown command fallback, got handled=true result=%q next=%q", result, nextSession)
	}
}

func TestTelegramCommand_OutputChunking_SplitsLargeMessage(t *testing.T) {
	chunks := splitTelegramMessage(strings.Repeat("a", 5000), telegramMaxMessageLength)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for i, chunk := range chunks {
		if len(chunk) > telegramMaxMessageLength {
			t.Fatalf("chunk %d exceeds max length: %d", i, len(chunk))
		}
	}
	joined := strings.Join(chunks, "")
	if joined != strings.Repeat("a", 5000) {
		t.Fatalf("chunked output must preserve content")
	}
}

func TestTelegramCommand_Allowed_GatewayStatus(t *testing.T) {
	workspace := t.TempDir()
	store := session.NewStore(workspace)
	runtime := gateway.NewRuntime(gateway.RuntimeOptions{
		Enabled:                 true,
		WorkspaceDir:            workspace,
		SessionStore:            store,
		ChannelsLocalEnabled:    true,
		ChannelsWebhookEnabled:  true,
		ChannelsTelegramEnabled: true,
		Now:                     func() time.Time { return time.Now().UTC() },
	})
	t.Cleanup(func() {
		_ = runtime.Close(context.Background())
	})

	handler := newTelegramCommandHandler(telegramCommandHandlerOptions{
		Store:        store,
		CronResolver: newWorkspaceCronStoreResolver(workspace, 0, cron.NewStore(workspace)),
		Runtime:      runtime,
		MainSession:  "sess-main",
		SessionScope: "main",
		Logger:       zerolog.Nop(),
	})
	handled, result, _, err := handler.Execute(context.Background(), "/gateway status", "")
	if err != nil {
		t.Fatalf("Execute /gateway status: %v", err)
	}
	if !handled {
		t.Fatalf("expected handled")
	}
	if !strings.Contains(result, "gateway enabled=true") {
		t.Fatalf("unexpected gateway status output: %q", result)
	}
}
