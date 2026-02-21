package tarsserver

import (
	"context"
	"strings"
	"testing"
	"time"

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
