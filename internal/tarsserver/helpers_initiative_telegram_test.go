package tarsserver

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/initiative"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// fakeInitiativeTelegramSender is a scriptable telegramSender (tars#1221 P2c): it
// never performs real network I/O, records every Send call, and fails
// when err is set — exactly the kind of fake the plan requires in place
// of a live bot token.
type fakeInitiativeTelegramSender struct {
	err   error
	calls []telegramSendRequest
}

func (f *fakeInitiativeTelegramSender) Send(_ context.Context, req telegramSendRequest) (telegramSendResult, error) {
	f.calls = append(f.calls, req)
	if f.err != nil {
		return telegramSendResult{}, f.err
	}
	return telegramSendResult{ChatID: req.ChatID, Text: req.Text}, nil
}

func (f *fakeInitiativeTelegramSender) SendChatAction(context.Context, telegramChatActionRequest) error {
	return nil
}

// TestInitiativeSpeakerTelegramChannelSendsPlainText (requirement #3): a
// delivered speak over the telegram channel writes the main session once,
// sends the full composed text to the default chat with no parse mode, and
// never sends a companion event (console-only today).
func TestInitiativeSpeakerTelegramChannelSendsPlainText(t *testing.T) {
	store := session.NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatal(err)
	}
	client := &speakStubClient{resp: speakTextResponse("Welcome back!\nHope the trip went well.")}
	sender := &fakeInitiativeTelegramSender{}
	companionEvents := 0
	speaker := &initiativeSpeaker{
		mainSessionID: main.ID,
		store:         store,
		composer:      &initiative.SpeechComposer{Client: client},
		location:      time.UTC,
		notify:        func(context.Context, notificationEvent) { companionEvents++ },
		logger:        zerolog.Nop(),
		claim:         alwaysClaims,
		telegram:      sender,
		telegramDefaultChatID: func() (string, error) {
			return "chat-42", nil
		},
	}
	now := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	outcome, err := speaker.Speak(context.Background(), initiative.SpeakRequest{
		Intent: initiative.IntentGreet, EntryID: "entry-1", Now: now, Channel: initiative.ChannelTelegram,
	})
	if err != nil {
		t.Fatalf("Speak: %v", err)
	}
	if !outcome.Delivered || !outcome.Composed || outcome.ChannelError != "" {
		t.Fatalf("outcome = %+v, want delivered with no channel error", outcome)
	}
	if companionEvents != 0 {
		t.Fatal("telegram channel must never also send a companion event")
	}
	if len(sender.calls) != 1 {
		t.Fatalf("telegram sends = %d, want 1", len(sender.calls))
	}
	call := sender.calls[0]
	if call.ChatID != "chat-42" || call.Text != "Welcome back!\nHope the trip went well." || call.ParseMode != "" {
		t.Fatalf("telegram send = %+v, want full text, chat-42, no parse mode", call)
	}
	msgs, err := session.ReadMessages(store.TranscriptPath(main.ID))
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Content != "Welcome back!\nHope the trip went well." {
		t.Fatalf("main session messages = %+v, want one assistant message", msgs)
	}
}

// TestInitiativeSpeakerTelegramSendFailureStaysDelivered (requirement #3):
// the main-session write already succeeded, so a Telegram API failure
// must not change Delivered — only ChannelError carries a fixed code,
// never the underlying error text.
func TestInitiativeSpeakerTelegramSendFailureStaysDelivered(t *testing.T) {
	store := session.NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatal(err)
	}
	client := &speakStubClient{resp: speakTextResponse("hi")}
	sender := &fakeInitiativeTelegramSender{err: errors.New("telegram api status 400: secret-looking-detail")}
	speaker := &initiativeSpeaker{
		mainSessionID:         main.ID,
		store:                 store,
		composer:              &initiative.SpeechComposer{Client: client},
		location:              time.UTC,
		logger:                zerolog.Nop(),
		claim:                 alwaysClaims,
		telegram:              sender,
		telegramDefaultChatID: func() (string, error) { return "chat-1", nil },
	}
	outcome, err := speaker.Speak(context.Background(), initiative.SpeakRequest{
		Intent: initiative.IntentCheckIn, EntryID: "entry-2", Channel: initiative.ChannelTelegram,
	})
	if err != nil {
		t.Fatalf("Speak: %v", err)
	}
	if !outcome.Delivered {
		t.Fatal("a channel send failure must never un-deliver a speak already written to the main session")
	}
	if outcome.ChannelError != "send_failed" {
		t.Fatalf("channel error = %q, want the fixed code send_failed (never the error text)", outcome.ChannelError)
	}
	if msgs, _ := session.ReadMessages(store.TranscriptPath(main.ID)); len(msgs) != 1 {
		t.Fatal("main session must still have the written message despite the channel failure")
	}
}

// TestInitiativeSpeakerTelegramNoDefaultChatIsChannelError covers the
// send-time re-resolution failing (e.g. pairing changed between decide
// and send): still delivered, still a fixed channel-error code.
func TestInitiativeSpeakerTelegramNoDefaultChatIsChannelError(t *testing.T) {
	store := session.NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatal(err)
	}
	client := &speakStubClient{resp: speakTextResponse("hi")}
	sender := &fakeInitiativeTelegramSender{}
	speaker := &initiativeSpeaker{
		mainSessionID:         main.ID,
		store:                 store,
		composer:              &initiative.SpeechComposer{Client: client},
		location:              time.UTC,
		logger:                zerolog.Nop(),
		claim:                 alwaysClaims,
		telegram:              sender,
		telegramDefaultChatID: func() (string, error) { return "", nil },
	}
	outcome, err := speaker.Speak(context.Background(), initiative.SpeakRequest{
		Intent: initiative.IntentGreet, Channel: initiative.ChannelTelegram,
	})
	if err != nil {
		t.Fatalf("Speak: %v", err)
	}
	if !outcome.Delivered || outcome.ChannelError != "send_failed" {
		t.Fatalf("outcome = %+v, want delivered with channel_error=send_failed", outcome)
	}
	if len(sender.calls) != 0 {
		t.Fatal("no send must be attempted without a resolvable chat id")
	}
}

// TestInitiativeSpeakerTelegramPerUserDualWrite (requirement #5): under
// session_telegram_scope: per-user, the default chat's reply lands in a
// different session than the main one — the composed message must be
// written there too, with the same Initiative metadata, so the model
// answering the reply has context for what it is replying to.
func TestInitiativeSpeakerTelegramPerUserDualWrite(t *testing.T) {
	store := session.NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatal(err)
	}
	replySess, err := store.Create("telegram:alice")
	if err != nil {
		t.Fatal(err)
	}
	client := &speakStubClient{resp: speakTextResponse("Welcome back!")}
	sender := &fakeInitiativeTelegramSender{}
	speaker := &initiativeSpeaker{
		mainSessionID:         main.ID,
		store:                 store,
		composer:              &initiative.SpeechComposer{Client: client},
		location:              time.UTC,
		logger:                zerolog.Nop(),
		claim:                 alwaysClaims,
		telegram:              sender,
		telegramDefaultChatID: func() (string, error) { return "chat-1", nil },
		telegramReplySession:  func() (string, error) { return replySess.ID, nil },
	}
	now := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	outcome, err := speaker.Speak(context.Background(), initiative.SpeakRequest{
		Intent: initiative.IntentGreet, EntryID: "entry-3", Now: now, Channel: initiative.ChannelTelegram,
	})
	if err != nil {
		t.Fatalf("Speak: %v", err)
	}
	if !outcome.Delivered {
		t.Fatalf("outcome = %+v, want delivered", outcome)
	}
	mainMsgs, err := session.ReadMessages(store.TranscriptPath(main.ID))
	if err != nil {
		t.Fatal(err)
	}
	if len(mainMsgs) != 1 || mainMsgs[0].Content != "Welcome back!" {
		t.Fatalf("main session messages = %+v", mainMsgs)
	}
	replyMsgs, err := session.ReadMessages(store.TranscriptPath(replySess.ID))
	if err != nil {
		t.Fatal(err)
	}
	if len(replyMsgs) != 1 || replyMsgs[0].Content != "Welcome back!" {
		t.Fatalf("reply session messages = %+v, want the same composed message", replyMsgs)
	}
	if replyMsgs[0].Initiative == nil || replyMsgs[0].Initiative.EntryID != "entry-3" {
		t.Fatalf("reply session message initiative metadata = %+v", replyMsgs[0].Initiative)
	}
}

// TestInitiativeSpeakerTelegramMainScopeNoDualWrite: when
// telegramReplySession is nil (session_telegram_scope: main, the default)
// or resolves to the main session itself, the message is written exactly
// once — no duplicate.
func TestInitiativeSpeakerTelegramMainScopeNoDualWrite(t *testing.T) {
	store := session.NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatal(err)
	}
	client := &speakStubClient{resp: speakTextResponse("hi")}
	sender := &fakeInitiativeTelegramSender{}
	speaker := &initiativeSpeaker{
		mainSessionID:         main.ID,
		store:                 store,
		composer:              &initiative.SpeechComposer{Client: client},
		location:              time.UTC,
		logger:                zerolog.Nop(),
		claim:                 alwaysClaims,
		telegram:              sender,
		telegramDefaultChatID: func() (string, error) { return "chat-1", nil },
		// telegramReplySession left nil: main scope.
	}
	outcome, err := speaker.Speak(context.Background(), initiative.SpeakRequest{
		Intent: initiative.IntentGreet, Channel: initiative.ChannelTelegram,
	})
	if err != nil {
		t.Fatalf("Speak: %v", err)
	}
	if !outcome.Delivered {
		t.Fatalf("outcome = %+v", outcome)
	}
	msgs, err := session.ReadMessages(store.TranscriptPath(main.ID))
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("main session messages = %d, want exactly 1 (no duplicate)", len(msgs))
	}
}
