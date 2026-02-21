package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestTelegramSendTool_Success(t *testing.T) {
	var seenReq TelegramSendRequest
	send := TelegramSendFunc(func(_ context.Context, req TelegramSendRequest) (TelegramSendResult, error) {
		seenReq = req
		return TelegramSendResult{
			MessageID: 17,
			ChatID:    req.ChatID,
			Text:      req.Text,
		}, nil
	})
	tg := NewTelegramSendTool(send, true, nil)
	res, err := tg.Execute(context.Background(), json.RawMessage(`{"chat_id":"12345","text":"hello","thread_id":"9","parse_mode":"Markdown"}`))
	if err != nil {
		t.Fatalf("execute telegram_send: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got error payload=%s", res.Text())
	}
	if seenReq.ChatID != "12345" || seenReq.Text != "hello" || seenReq.ThreadID != "9" || seenReq.ParseMode != "Markdown" {
		t.Fatalf("unexpected sender request: %+v", seenReq)
	}
	var payload struct {
		MessageID int64  `json:"message_id"`
		ChatID    string `json:"chat_id"`
		Text      string `json:"text"`
	}
	if err := json.Unmarshal([]byte(res.Text()), &payload); err != nil {
		t.Fatalf("decode result payload: %v", err)
	}
	if payload.MessageID != 17 || payload.ChatID != "12345" || payload.Text != "hello" {
		t.Fatalf("unexpected result payload: %+v", payload)
	}
}

func TestTelegramSendTool_InvalidAndFailure(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		tg := NewTelegramSendTool(TelegramSendFunc(func(_ context.Context, req TelegramSendRequest) (TelegramSendResult, error) {
			return TelegramSendResult{}, nil
		}), false, nil)
		res, err := tg.Execute(context.Background(), json.RawMessage(`{"chat_id":"1","text":"x"}`))
		if err != nil {
			t.Fatalf("execute disabled telegram_send: %v", err)
		}
		if !res.IsError || !strings.Contains(res.Text(), "telegram_send tool is disabled") {
			t.Fatalf("expected disabled error payload, got %q", res.Text())
		}
	})

	t.Run("sender missing", func(t *testing.T) {
		tg := NewTelegramSendTool(nil, true, nil)
		res, err := tg.Execute(context.Background(), json.RawMessage(`{"chat_id":"1","text":"x"}`))
		if err != nil {
			t.Fatalf("execute without sender: %v", err)
		}
		if !res.IsError || !strings.Contains(res.Text(), "telegram sender is not configured") {
			t.Fatalf("expected sender missing error payload, got %q", res.Text())
		}
	})

	t.Run("chat_id required", func(t *testing.T) {
		tg := NewTelegramSendTool(TelegramSendFunc(func(_ context.Context, req TelegramSendRequest) (TelegramSendResult, error) {
			return TelegramSendResult{}, nil
		}), true, nil)
		res, err := tg.Execute(context.Background(), json.RawMessage(`{"text":"x"}`))
		if err != nil {
			t.Fatalf("execute missing chat_id: %v", err)
		}
		if !res.IsError || !strings.Contains(res.Text(), "chat_id is required") {
			t.Fatalf("expected chat_id required error payload, got %q", res.Text())
		}
	})

	t.Run("chat_id fallback resolver", func(t *testing.T) {
		var seenReq TelegramSendRequest
		tg := NewTelegramSendTool(TelegramSendFunc(func(_ context.Context, req TelegramSendRequest) (TelegramSendResult, error) {
			seenReq = req
			return TelegramSendResult{MessageID: 1, ChatID: req.ChatID, Text: req.Text}, nil
		}), true, TelegramDefaultChatIDResolveFunc(func(_ context.Context) (string, error) {
			return "777", nil
		}))
		res, err := tg.Execute(context.Background(), json.RawMessage(`{"text":"fallback send"}`))
		if err != nil {
			t.Fatalf("execute fallback resolver: %v", err)
		}
		if res.IsError {
			t.Fatalf("expected success with fallback resolver, got %q", res.Text())
		}
		if seenReq.ChatID != "777" {
			t.Fatalf("expected fallback chat_id 777, got %+v", seenReq)
		}
	})

	t.Run("sender failure", func(t *testing.T) {
		tg := NewTelegramSendTool(TelegramSendFunc(func(_ context.Context, req TelegramSendRequest) (TelegramSendResult, error) {
			return TelegramSendResult{}, fmt.Errorf("upstream failed")
		}), true, nil)
		res, err := tg.Execute(context.Background(), json.RawMessage(`{"chat_id":"1","text":"x"}`))
		if err != nil {
			t.Fatalf("execute sender failure: %v", err)
		}
		if !res.IsError || !strings.Contains(res.Text(), "upstream failed") {
			t.Fatalf("expected sender failure payload, got %q", res.Text())
		}
	})
}
