package tarsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/devlikebear/tarsncase/internal/llm"
	"github.com/devlikebear/tarsncase/internal/session"
	"github.com/rs/zerolog"
)

func TestTelegramInbound_PairingThenApproveAndReply(t *testing.T) {
	workspace := t.TempDir()
	store := session.NewStore(workspace)
	pairings, err := newTelegramPairingStore(filepath.Join(t.TempDir(), "telegram_pairings.json"), nil)
	if err != nil {
		t.Fatalf("newTelegramPairingStore: %v", err)
	}
	mockLLM := &mockLLMClient{
		response: llm.ChatResponse{
			Message: llm.ChatMessage{
				Role:    "assistant",
				Content: "hello from tars",
			},
		},
	}
	var sent []telegramSendRequest
	sender := telegramSendFunc(func(_ context.Context, req telegramSendRequest) (telegramSendResult, error) {
		sent = append(sent, req)
		return telegramSendResult{
			MessageID: int64(len(sent)),
			ChatID:    req.ChatID,
			Text:      req.Text,
		}, nil
	})
	handler := newTelegramInboundHandler(
		workspace,
		store,
		mockLLM,
		sender,
		nil,
		pairings,
		"pairing",
		zerolog.New(io.Discard),
	)
	userMsg := telegramUpdate{
		UpdateID: 1,
		Message: &telegramMessage{
			Text: "hello",
			Chat: telegramChat{
				ID:   json.Number("101"),
				Type: "private",
			},
			From: telegramUser{
				ID:       json.Number("11"),
				Username: "alice",
			},
		},
	}
	handler.HandleUpdate(context.Background(), userMsg)
	if len(sent) != 1 {
		t.Fatalf("expected 1 pairing message, got %d", len(sent))
	}
	if !strings.Contains(sent[0].Text, "Pairing code:") {
		t.Fatalf("expected pairing message, got %q", sent[0].Text)
	}
	code := extractPairingCodeForTest(sent[0].Text)
	if code == "" {
		t.Fatalf("expected pairing code in %q", sent[0].Text)
	}
	if _, err := pairings.approve(code); err != nil {
		t.Fatalf("approve pairing: %v", err)
	}

	secondMsg := userMsg
	secondMsg.UpdateID = 2
	secondMsg.Message = &telegramMessage{
		Text: "run now",
		Chat: telegramChat{
			ID:   json.Number("101"),
			Type: "private",
		},
		From: telegramUser{
			ID:       json.Number("11"),
			Username: "alice",
		},
	}
	handler.HandleUpdate(context.Background(), secondMsg)
	if len(sent) != 2 {
		t.Fatalf("expected 2 total telegram sends, got %d", len(sent))
	}
	if strings.TrimSpace(sent[1].Text) != "hello from tars" {
		t.Fatalf("expected llm reply, got %q", sent[1].Text)
	}
	sessionID := pairings.sessionID(11)
	if strings.TrimSpace(sessionID) == "" {
		t.Fatalf("expected session binding for approved user")
	}
	messages, err := session.ReadMessages(store.TranscriptPath(sessionID))
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 transcript messages, got %d", len(messages))
	}
	if messages[0].Role != "user" || strings.TrimSpace(messages[0].Content) != "run now" {
		t.Fatalf("unexpected user transcript message: %+v", messages[0])
	}
	if messages[1].Role != "assistant" || strings.TrimSpace(messages[1].Content) != "hello from tars" {
		t.Fatalf("unexpected assistant transcript message: %+v", messages[1])
	}
}

func TestTelegramInbound_DropsNonPrivateChat(t *testing.T) {
	workspace := t.TempDir()
	store := session.NewStore(workspace)
	mockLLM := &mockLLMClient{
		response: llm.ChatResponse{
			Message: llm.ChatMessage{
				Role:    "assistant",
				Content: "hello",
			},
		},
	}
	sendCount := 0
	sender := telegramSendFunc(func(_ context.Context, req telegramSendRequest) (telegramSendResult, error) {
		sendCount++
		return telegramSendResult{ChatID: req.ChatID, Text: req.Text}, nil
	})
	handler := newTelegramInboundHandler(
		workspace,
		store,
		mockLLM,
		sender,
		nil,
		nil,
		"open",
		zerolog.New(io.Discard),
	)
	handler.HandleUpdate(context.Background(), telegramUpdate{
		UpdateID: 1,
		Message: &telegramMessage{
			Text: "hello",
			Chat: telegramChat{
				ID:   json.Number("101"),
				Type: "group",
			},
			From: telegramUser{
				ID:       json.Number("11"),
				Username: "alice",
			},
		},
	})
	if sendCount != 0 {
		t.Fatalf("expected non-private telegram update to be dropped")
	}
}

func TestTelegramInbound_MainSessionScope_UsesMainSession(t *testing.T) {
	workspace := t.TempDir()
	store := session.NewStore(workspace)
	mainSession, err := store.Create("main")
	if err != nil {
		t.Fatalf("create main session: %v", err)
	}
	mockLLM := &mockLLMClient{
		response: llm.ChatResponse{
			Message: llm.ChatMessage{
				Role:    "assistant",
				Content: "shared reply",
			},
		},
	}
	sender := telegramSendFunc(func(_ context.Context, req telegramSendRequest) (telegramSendResult, error) {
		return telegramSendResult{ChatID: req.ChatID, Text: req.Text}, nil
	})
	handler := newTelegramInboundHandler(
		workspace,
		store,
		mockLLM,
		sender,
		nil,
		nil,
		"open",
		zerolog.New(io.Discard),
	)
	handler.mainSessionID = mainSession.ID
	handler.sessionScope = "main"

	handler.HandleUpdate(context.Background(), telegramUpdate{
		UpdateID: 1,
		Message: &telegramMessage{
			Text: "hello from telegram",
			Chat: telegramChat{ID: json.Number("101"), Type: "private"},
			From: telegramUser{ID: json.Number("11"), Username: "alice"},
		},
	})

	sessions, err := store.List()
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected only main session, got %d", len(sessions))
	}
	messages, err := session.ReadMessages(store.TranscriptPath(mainSession.ID))
	if err != nil {
		t.Fatalf("read main transcript: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages in main session, got %d", len(messages))
	}
	if strings.TrimSpace(messages[0].Content) != "hello from telegram" {
		t.Fatalf("unexpected user message: %+v", messages[0])
	}
}

func TestTelegramInbound_MainSessionScope_PerUserCreatesDedicatedSession(t *testing.T) {
	workspace := t.TempDir()
	store := session.NewStore(workspace)
	mainSession, err := store.Create("main")
	if err != nil {
		t.Fatalf("create main session: %v", err)
	}
	mockLLM := &mockLLMClient{
		response: llm.ChatResponse{
			Message: llm.ChatMessage{
				Role:    "assistant",
				Content: "isolated reply",
			},
		},
	}
	sender := telegramSendFunc(func(_ context.Context, req telegramSendRequest) (telegramSendResult, error) {
		return telegramSendResult{ChatID: req.ChatID, Text: req.Text}, nil
	})
	handler := newTelegramInboundHandler(
		workspace,
		store,
		mockLLM,
		sender,
		nil,
		nil,
		"open",
		zerolog.New(io.Discard),
	)
	handler.mainSessionID = mainSession.ID
	handler.sessionScope = "per-user"

	handler.HandleUpdate(context.Background(), telegramUpdate{
		UpdateID: 1,
		Message: &telegramMessage{
			Text: "per-user hello",
			Chat: telegramChat{ID: json.Number("102"), Type: "private"},
			From: telegramUser{ID: json.Number("12"), Username: "bob"},
		},
	})

	sessions, err := store.List()
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected main + per-user session, got %d", len(sessions))
	}
	mainMessages, err := session.ReadMessages(store.TranscriptPath(mainSession.ID))
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "no such file") {
		t.Fatalf("read main transcript: %v", err)
	}
	if len(mainMessages) != 0 {
		t.Fatalf("expected main session to stay untouched, got %d messages", len(mainMessages))
	}
}

func extractPairingCodeForTest(text string) string {
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Pairing code:") {
			continue
		}
		code := strings.TrimSpace(strings.TrimPrefix(line, "Pairing code:"))
		return normalizeTelegramPairingCode(code)
	}
	return ""
}

type telegramTestSender struct {
	mu      sync.Mutex
	sent    []telegramSendRequest
	actions []telegramChatActionRequest
}

func (s *telegramTestSender) Send(_ context.Context, req telegramSendRequest) (telegramSendResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, req)
	return telegramSendResult{ChatID: req.ChatID, Text: req.Text}, nil
}

func (s *telegramTestSender) SendChatAction(_ context.Context, req telegramChatActionRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.actions = append(s.actions, req)
	return nil
}

func (s *telegramTestSender) actionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.actions)
}

func (s *telegramTestSender) lastText() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sent) == 0 {
		return ""
	}
	return strings.TrimSpace(s.sent[len(s.sent)-1].Text)
}

type telegramTestMediaDownloader struct {
	saved telegramSavedMedia
	err   error
	seen  []telegramInboundMedia
}

func (d *telegramTestMediaDownloader) DownloadAndSave(_ context.Context, _ string, media telegramInboundMedia) (telegramSavedMedia, error) {
	d.seen = append(d.seen, media)
	if d.err != nil {
		return telegramSavedMedia{}, d.err
	}
	return d.saved, nil
}

func TestTelegramInbound_TypingLoop_LLMOnly(t *testing.T) {
	workspace := t.TempDir()
	store := session.NewStore(workspace)
	mainSession, err := store.Create("main")
	if err != nil {
		t.Fatalf("create main session: %v", err)
	}
	mockLLM := &mockLLMClient{
		response: llm.ChatResponse{
			Message: llm.ChatMessage{
				Role:    "assistant",
				Content: "typing check",
			},
		},
	}
	sender := &telegramTestSender{}
	handler := newTelegramInboundHandler(
		workspace,
		store,
		mockLLM,
		sender,
		nil,
		nil,
		"open",
		zerolog.New(io.Discard),
	)
	handler.mainSessionID = mainSession.ID
	handler.sessionScope = "main"
	handler.commands = telegramCommandExecFunc(func(ctx context.Context, line, session string) (bool, string, string, error) {
		if strings.HasPrefix(strings.TrimSpace(line), "/help") {
			return true, "telegram help", "", nil
		}
		return false, "", "", nil
	})

	handler.HandleUpdate(context.Background(), telegramUpdate{
		UpdateID: 1,
		Message: &telegramMessage{
			Text: "hello",
			Chat: telegramChat{ID: json.Number("101"), Type: "private"},
			From: telegramUser{ID: json.Number("11"), Username: "alice"},
		},
	})
	llmActions := sender.actionCount()
	if llmActions == 0 {
		t.Fatalf("expected typing action during llm path")
	}

	handler.HandleUpdate(context.Background(), telegramUpdate{
		UpdateID: 2,
		Message: &telegramMessage{
			Text: "/help",
			Chat: telegramChat{ID: json.Number("101"), Type: "private"},
			From: telegramUser{ID: json.Number("11"), Username: "alice"},
		},
	})
	if sender.actionCount() != llmActions {
		t.Fatalf("expected command path to skip typing action")
	}
}

func TestTelegramInbound_MediaPhotoWithCaption_UsesLLM(t *testing.T) {
	workspace := t.TempDir()
	store := session.NewStore(workspace)
	mainSession, err := store.Create("main")
	if err != nil {
		t.Fatalf("create main session: %v", err)
	}
	mockLLM := &mockLLMClient{
		response: llm.ChatResponse{
			Message: llm.ChatMessage{
				Role:    "assistant",
				Content: "media handled",
			},
		},
	}
	sender := &telegramTestSender{}
	media := &telegramTestMediaDownloader{
		saved: telegramSavedMedia{
			Type:         "photo",
			SavedPath:    filepath.Join(workspace, "telegram", "media", "20260221", "chat_101", "photo.jpg"),
			MimeType:     "image/jpeg",
			Size:         245760,
			OriginalName: "photo.jpg",
		},
	}
	handler := newTelegramInboundHandler(
		workspace,
		store,
		mockLLM,
		sender,
		nil,
		nil,
		"open",
		zerolog.New(io.Discard),
	)
	handler.mainSessionID = mainSession.ID
	handler.sessionScope = "main"
	handler.media = media

	handler.HandleUpdate(context.Background(), telegramUpdate{
		UpdateID: 1,
		Message: &telegramMessage{
			Caption: "analyze this image",
			Photo: []telegramPhoto{
				{FileID: "photo-small", FileSize: 1000},
				{FileID: "photo-large", FileSize: 2000},
			},
			Chat: telegramChat{ID: json.Number("101"), Type: "private"},
			From: telegramUser{ID: json.Number("11"), Username: "alice"},
		},
	})

	if mockLLM.callCount == 0 {
		t.Fatalf("expected llm call for media with caption")
	}
	if len(media.seen) != 1 || media.seen[0].Type != "photo" || media.seen[0].FileID != "photo-large" {
		t.Fatalf("unexpected media selection: %+v", media.seen)
	}
	messages, err := session.ReadMessages(store.TranscriptPath(mainSession.ID))
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	if len(messages) == 0 || !strings.Contains(messages[0].Content, "[Attached file]") {
		t.Fatalf("expected attached file prompt in transcript, got %+v", messages)
	}
}

func TestTelegramInbound_MediaDocument_NoCaptionSkipsLLM(t *testing.T) {
	workspace := t.TempDir()
	store := session.NewStore(workspace)
	mainSession, err := store.Create("main")
	if err != nil {
		t.Fatalf("create main session: %v", err)
	}
	mockLLM := &mockLLMClient{
		response: llm.ChatResponse{
			Message: llm.ChatMessage{Role: "assistant", Content: "should not run"},
		},
	}
	sender := &telegramTestSender{}
	media := &telegramTestMediaDownloader{
		saved: telegramSavedMedia{
			Type:         "document",
			SavedPath:    filepath.Join(workspace, "telegram", "media", "20260221", "chat_101", "report.txt"),
			MimeType:     "text/plain",
			Size:         1024,
			OriginalName: "report.txt",
		},
	}
	handler := newTelegramInboundHandler(
		workspace,
		store,
		mockLLM,
		sender,
		nil,
		nil,
		"open",
		zerolog.New(io.Discard),
	)
	handler.mainSessionID = mainSession.ID
	handler.sessionScope = "main"
	handler.media = media

	handler.HandleUpdate(context.Background(), telegramUpdate{
		UpdateID: 1,
		Message: &telegramMessage{
			Document: &telegramDocument{FileID: "doc1", FileName: "report.txt", MimeType: "text/plain", FileSize: 1024},
			Chat:     telegramChat{ID: json.Number("101"), Type: "private"},
			From:     telegramUser{ID: json.Number("11"), Username: "alice"},
		},
	})

	if mockLLM.callCount != 0 {
		t.Fatalf("expected llm to be skipped for media without caption")
	}
	if !strings.Contains(strings.ToLower(sender.lastText()), "attachment saved:") {
		t.Fatalf("expected saved attachment notice, got %q", sender.lastText())
	}
}

func TestTelegramInbound_MediaVoice_TooLarge(t *testing.T) {
	workspace := t.TempDir()
	store := session.NewStore(workspace)
	mainSession, err := store.Create("main")
	if err != nil {
		t.Fatalf("create main session: %v", err)
	}
	mockLLM := &mockLLMClient{
		response: llm.ChatResponse{
			Message: llm.ChatMessage{Role: "assistant", Content: "should not run"},
		},
	}
	sender := &telegramTestSender{}
	media := &telegramTestMediaDownloader{
		err: fmt.Errorf("media file too large"),
	}
	handler := newTelegramInboundHandler(
		workspace,
		store,
		mockLLM,
		sender,
		nil,
		nil,
		"open",
		zerolog.New(io.Discard),
	)
	handler.mainSessionID = mainSession.ID
	handler.sessionScope = "main"
	handler.media = media

	handler.HandleUpdate(context.Background(), telegramUpdate{
		UpdateID: 1,
		Message: &telegramMessage{
			Text:  "please process",
			Voice: &telegramVoice{FileID: "voice1", MimeType: "audio/ogg", FileSize: telegramMediaMaxBytes + 1},
			Chat:  telegramChat{ID: json.Number("101"), Type: "private"},
			From:  telegramUser{ID: json.Number("11"), Username: "alice"},
		},
	})

	if mockLLM.callCount != 0 {
		t.Fatalf("expected llm to be skipped for oversized media")
	}
	if !strings.Contains(strings.ToLower(sender.lastText()), "too large") {
		t.Fatalf("expected too-large message, got %q", sender.lastText())
	}
}
