package tarsserver

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
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
