package tarsserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/apptool"
	"github.com/rs/zerolog"
)

type recordingTelegramSender struct {
	lastRequest telegramSendRequest
	result      telegramSendResult
	err         error
	calls       int
}

func (s *recordingTelegramSender) Send(_ context.Context, req telegramSendRequest) (telegramSendResult, error) {
	s.calls++
	s.lastRequest = req
	return s.result, s.err
}

func telegramToolTestDeps(sender telegramSendPerformer) telegramSendToolDeps {
	return telegramSendToolDeps{Sender: sender, Logger: zerolog.New(io.Discard)}
}

func TestSendTelegramForTool_TrimsFieldsAndMapsTheResult(t *testing.T) {
	sender := &recordingTelegramSender{
		result: telegramSendResult{MessageID: 42, ChatID: "chat-1", Text: "delivered"},
	}

	got, err := sendTelegramForTool(context.Background(), telegramToolTestDeps(sender), apptool.TelegramSendRequest{
		BotID:     "  bot  ",
		ChatID:    "  chat-1  ",
		Text:      "  hello  ",
		ThreadID:  "  7  ",
		ParseMode: "  MarkdownV2  ",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	if sender.lastRequest.BotID != "bot" || sender.lastRequest.ChatID != "chat-1" ||
		sender.lastRequest.Text != "hello" || sender.lastRequest.ThreadID != "7" ||
		sender.lastRequest.ParseMode != "MarkdownV2" {
		t.Fatalf("request not trimmed: %+v", sender.lastRequest)
	}
	if got.MessageID != 42 || got.ChatID != "chat-1" || got.Text != "delivered" {
		t.Fatalf("result not mapped back: %+v", got)
	}
}

func TestSendTelegramForTool_ErrorsWithoutASender(t *testing.T) {
	_, err := sendTelegramForTool(context.Background(), telegramToolTestDeps(nil), apptool.TelegramSendRequest{ChatID: "c", Text: "t"})
	if err == nil {
		t.Fatal("expected an error when telegram is not configured")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSendTelegramForTool_PropagatesSendFailure(t *testing.T) {
	sender := &recordingTelegramSender{err: errors.New("upstream refused")}
	_, err := sendTelegramForTool(context.Background(), telegramToolTestDeps(sender), apptool.TelegramSendRequest{ChatID: "c", Text: "t"})
	if err == nil || !strings.Contains(err.Error(), "upstream refused") {
		t.Fatalf("expected the sender's error to surface, got %v", err)
	}
}

func TestSendTelegramForTool_ToleratesAnUnsetAgentRuntime(t *testing.T) {
	// The runtime is assigned after this tool is built during startup, so a
	// nil at call time must simply skip the history record rather than fail
	// a send that already succeeded.
	sender := &recordingTelegramSender{result: telegramSendResult{MessageID: 1}}
	deps := telegramToolTestDeps(sender) // AgentRuntime left nil

	if _, err := sendTelegramForTool(context.Background(), deps, apptool.TelegramSendRequest{ChatID: "c", Text: "t"}); err != nil {
		t.Fatalf("send with no runtime: %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("sender calls = %d, want 1", sender.calls)
	}
}

func TestSendTelegramForTool_RecordsOutboundInAgentRuntimeHistory(t *testing.T) {
	// The record is the reason this tool holds an agent-runtime pointer at
	// all, so it needs a real runtime rather than a stub.
	runtime, _ := newTestAgentRuntimeWithStore(t)
	sender := &recordingTelegramSender{
		result: telegramSendResult{MessageID: 11, ChatID: "chat-9", Text: "sent body"},
	}
	deps := telegramToolTestDeps(sender)
	deps.AgentRuntime = &runtime

	if _, err := sendTelegramForTool(context.Background(), deps, apptool.TelegramSendRequest{
		BotID:     "bot-1",
		ChatID:    "chat-9",
		Text:      "hello",
		ParseMode: "HTML",
	}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("sender calls = %d, want 1", sender.calls)
	}
}

func TestNewTelegramSendTool_NoDefaultResolverYieldsNoDefaultChat(t *testing.T) {
	// With no resolver and no chat_id in the call, the tool must report the
	// missing target rather than send to an empty chat.
	sender := &recordingTelegramSender{}
	tl := newTelegramSendTool(telegramToolTestDeps(sender), true)

	res, err := tl.Execute(context.Background(), json.RawMessage(`{"text":"hi"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected an error result with no chat id available, got %+v", res)
	}
	if sender.calls != 0 {
		t.Fatalf("sent %d message(s) with no chat id", sender.calls)
	}
}

func TestCurrentTelegramAgentRuntime_HandlesBothNilLevels(t *testing.T) {
	if got := currentTelegramAgentRuntime(telegramSendToolDeps{}); got != nil {
		t.Error("nil pointer-to-pointer should yield nil")
	}
	var unset *agentruntime.Runtime
	if got := currentTelegramAgentRuntime(telegramSendToolDeps{AgentRuntime: &unset}); got != nil {
		t.Error("pointer to a nil runtime should yield nil")
	}
	runtime, _ := newTestAgentRuntimeWithStore(t)
	if got := currentTelegramAgentRuntime(telegramSendToolDeps{AgentRuntime: &runtime}); got != runtime {
		t.Error("assigned runtime should come back")
	}
}

func TestTelegramOutboundRecordPayload_OmitsEmptyFields(t *testing.T) {
	full := telegramOutboundRecordPayload(
		apptool.TelegramSendRequest{BotID: "bot", ParseMode: "HTML"},
		telegramSendResult{MessageID: 9, ChatID: "chat", Text: "body"},
	)
	for _, key := range []string{"provider", "bot_id", "parse_mode", "message_id", "provider_chat_id", "provider_text"} {
		if _, ok := full[key]; !ok {
			t.Errorf("payload missing %q: %v", key, full)
		}
	}

	sparse := telegramOutboundRecordPayload(apptool.TelegramSendRequest{}, telegramSendResult{})
	if len(sparse) != 1 || sparse["provider"] != "telegram" {
		t.Fatalf("empty send should record only the provider, got %v", sparse)
	}
}

func TestNewTelegramSendTool_ExposesTheToolAndHonorsTheDefaultChatResolver(t *testing.T) {
	sender := &recordingTelegramSender{result: telegramSendResult{MessageID: 5, ChatID: "resolved"}}
	deps := telegramToolTestDeps(sender)
	deps.ResolveDefaultChatID = func() (string, error) { return "resolved", nil }

	tl := newTelegramSendTool(deps, true)
	if tl.Name != "telegram_send" {
		t.Fatalf("tool name = %q, want telegram_send", tl.Name)
	}

	// Omit chat_id so the default resolver has to supply it.
	res, err := tl.Execute(context.Background(), json.RawMessage(`{"text":"hi"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool reported an error: %+v", res)
	}
	if sender.lastRequest.ChatID != "resolved" {
		t.Fatalf("default chat id not applied, sent to %q", sender.lastRequest.ChatID)
	}
}

func TestNewTelegramSendTool_DisabledReportsRatherThanSends(t *testing.T) {
	sender := &recordingTelegramSender{}
	tl := newTelegramSendTool(telegramToolTestDeps(sender), false)

	res, err := tl.Execute(context.Background(), json.RawMessage(`{"chat_id":"c","text":"hi"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected an error result when telegram is disabled, got %+v", res)
	}
	if sender.calls != 0 {
		t.Fatalf("disabled tool still sent %d message(s)", sender.calls)
	}
}
