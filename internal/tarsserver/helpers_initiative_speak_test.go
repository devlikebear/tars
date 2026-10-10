package tarsserver

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/initiative"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/pkg/llm"
	"github.com/rs/zerolog"
)

// speakStubClient answers Chat with a fixed response/error and records the
// messages it was called with.
type speakStubClient struct {
	resp     llm.ChatResponse
	err      error
	messages []llm.ChatMessage
}

func (c *speakStubClient) Ask(context.Context, string) (string, error) { return "", nil }

func (c *speakStubClient) Chat(_ context.Context, msgs []llm.ChatMessage, _ llm.ChatOptions) (llm.ChatResponse, error) {
	c.messages = msgs
	if c.err != nil {
		return llm.ChatResponse{}, c.err
	}
	return c.resp, nil
}

func speakTextResponse(text string) llm.ChatResponse {
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: text}}
}

func alwaysClaims(string) (*chatCancelEntry, func(), bool) { return nil, func() {}, true }
func neverClaims(string) (*chatCancelEntry, func(), bool)  { return nil, func() {}, false }

func TestResolveInitiativeSpeakTierFallbackChain(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.Config
		want string
	}{
		{"explicit role mapping wins", config.Config{
			LLMConfig: config.LLMConfig{LLMDefaultTier: "standard", LLMRoleDefaults: map[string]string{
				"initiative_speak": "heavy", "chat_main": "light",
			}},
		}, "heavy"},
		{"falls back to chat_main", config.Config{
			LLMConfig: config.LLMConfig{LLMDefaultTier: "standard", LLMRoleDefaults: map[string]string{
				"chat_main": "light",
			}},
		}, "light"},
		{"falls back to default tier", config.Config{
			LLMConfig: config.LLMConfig{LLMDefaultTier: "standard"},
		}, "standard"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveInitiativeSpeakTier(c.cfg); got != c.want {
				t.Fatalf("tier = %q, want %q", got, c.want)
			}
		})
	}
}

// TestResolveInitiativeSpeakBackendSameAndDifferentProvider mirrors
// TestResolveInitiativeLLMBackendSameAndDifferentProvider: a real router
// built from config.ResolveLLMTier (via buildLLMRouter), checking the
// speak role's provider alias against chat's.
func TestResolveInitiativeSpeakBackendSameAndDifferentProvider(t *testing.T) {
	baseCfg := config.Config{LLMConfig: config.LLMConfig{
		LLMDefaultTier:  "standard",
		LLMRoleDefaults: map[string]string{"initiative_speak": "light", "chat_main": "standard"},
		LLMProviders: map[string]config.LLMProviderSettings{
			"shared": {Kind: "openai", BaseURL: "http://shared.example", APIKey: "k"},
			"other":  {Kind: "openai", BaseURL: "http://other.example", APIKey: "k2"},
		},
		LLMTiers: map[string]config.LLMTierBinding{
			"standard": {Provider: "shared", Model: "chat-model"},
			"light":    {Provider: "shared", Model: "speak-model"},
			"heavy":    {Provider: "shared", Model: "chat-model"},
		},
	}}

	t.Run("same provider alias as chat sends text", func(t *testing.T) {
		router, err := buildLLMRouter(baseCfg, nil)
		if err != nil {
			t.Fatal(err)
		}
		plan, client, model := resolveInitiativeSpeakBackend(baseCfg, router)
		if !plan.Usable || !plan.SendsText || client == nil || model != "speak-model" {
			t.Fatalf("plan = %+v, client=%v model=%q", plan, client, model)
		}
	})

	t.Run("different provider alias than chat withholds text", func(t *testing.T) {
		cfg := baseCfg
		cfg.LLMTiers = map[string]config.LLMTierBinding{
			"standard": {Provider: "shared", Model: "chat-model"},
			"light":    {Provider: "other", Model: "speak-model"},
			"heavy":    {Provider: "shared", Model: "chat-model"},
		}
		router, err := buildLLMRouter(cfg, nil)
		if err != nil {
			t.Fatal(err)
		}
		plan, client, _ := resolveInitiativeSpeakBackend(cfg, router)
		if !plan.Usable || plan.SendsText || client == nil {
			t.Fatalf("plan = %+v, client=%v", plan, client)
		}
	})

	t.Run("no router is unusable", func(t *testing.T) {
		plan, client, _ := resolveInitiativeSpeakBackend(baseCfg, nil)
		if plan.Usable || client != nil {
			t.Fatalf("plan = %+v, client=%v", plan, client)
		}
	})
}

func TestInitiativeSpeakerDeliversAssistantMessageAndCompanionEvent(t *testing.T) {
	store := session.NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatal(err)
	}
	client := &speakStubClient{resp: speakTextResponse("Welcome back!\nHope the trip went well.")}
	var events []notificationEvent
	speaker := &initiativeSpeaker{
		mainSessionID: main.ID,
		store:         store,
		composer:      &initiative.SpeechComposer{Client: client},
		location:      time.UTC,
		notify: func(_ context.Context, evt notificationEvent) {
			events = append(events, evt)
		},
		logger: zerolog.Nop(),
		claim:  alwaysClaims,
	}
	now := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	outcome, err := speaker.Speak(context.Background(), initiative.SpeakRequest{
		Intent: initiative.IntentGreet, EntryID: "entry-1", Now: now,
	})
	if err != nil {
		t.Fatalf("Speak: %v", err)
	}
	if !outcome.Delivered || !outcome.Composed {
		t.Fatalf("outcome = %+v", outcome)
	}

	msgs, err := session.ReadMessages(store.TranscriptPath(main.ID))
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(msgs))
	}
	got := msgs[0]
	if got.Role != "assistant" || got.Content != "Welcome back!\nHope the trip went well." {
		t.Fatalf("message = %+v", got)
	}
	if got.Initiative == nil || got.Initiative.Intent != "greet" || got.Initiative.EntryID != "entry-1" {
		t.Fatalf("message initiative metadata = %+v", got.Initiative)
	}

	sess, err := store.Get(main.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !sess.UpdatedAt.Equal(now) {
		t.Fatalf("session UpdatedAt = %v, want %v (Touch after write)", sess.UpdatedAt, now)
	}

	if len(events) != 1 {
		t.Fatalf("companion events = %d, want 1", len(events))
	}
	evt := events[0]
	if evt.Category != "companion" || evt.Expression != "greeting" || evt.Message != "Welcome back!" || evt.SessionID != main.ID {
		t.Fatalf("companion event = %+v", evt)
	}
}

// TestInitiativeSpeakerNoClaimAtAllIsBusyWithNoCompose covers the one case
// where Compose genuinely never runs: no claim function at all (s.claim ==
// nil, e.g. before the chat handler binds it, or setup-only mode — see
// initiativeSpeaker.claim's doc comment). Speak must fail fast here,
// before ever calling the composer.
func TestInitiativeSpeakerNoClaimAtAllIsBusyWithNoCompose(t *testing.T) {
	store := session.NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatal(err)
	}
	client := &speakStubClient{resp: speakTextResponse("hi")}
	called := false
	speaker := &initiativeSpeaker{
		mainSessionID: main.ID, store: store,
		composer: &initiative.SpeechComposer{Client: client},
		notify:   func(context.Context, notificationEvent) { called = true },
		logger:   zerolog.Nop(),
	}
	outcome, err := speaker.Speak(context.Background(), initiative.SpeakRequest{Intent: initiative.IntentGreet})
	if err != nil {
		t.Fatalf("Speak: %v", err)
	}
	if outcome.Delivered || outcome.Composed || outcome.Superseded || outcome.Reason != "busy" {
		t.Fatalf("outcome = %+v, want busy with no compose", outcome)
	}
	if len(client.messages) != 0 {
		t.Fatal("compose must not be called with no claim function at all")
	}
	if called {
		t.Fatal("companion event must not fire")
	}
	if msgs, _ := session.ReadMessages(store.TranscriptPath(main.ID)); len(msgs) != 0 {
		t.Fatal("no message must be written")
	}
}

// TestInitiativeSpeakerComposesWithoutClaimThenDiscardsIfTurnStarted
// covers review finding f2's first case (tars#1220): a real chat turn
// starts in the main session while Compose is in flight. Compose must
// still run (and be counted — a real LLM call happened, so it counts
// toward DailySpeakCalls) with no claim held the whole time, and only
// then does Speak try to claim the session to write — by which point the
// turn holds it, so Speak discards the composed text instead of writing
// it or blocking the turn. Composed=true + Superseded=true + reason
// "busy" is how the runtime (deliverLocked/applyLocked) tells this apart
// from an actual compose failure: counted toward the call cap, never
// toward the backoff schedule.
func TestInitiativeSpeakerComposesWithoutClaimThenDiscardsIfTurnStarted(t *testing.T) {
	store := session.NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatal(err)
	}
	client := &speakStubClient{resp: speakTextResponse("Welcome back!")}
	called := false
	speaker := &initiativeSpeaker{
		mainSessionID: main.ID, store: store,
		composer: &initiative.SpeechComposer{Client: client},
		notify:   func(context.Context, notificationEvent) { called = true },
		logger:   zerolog.Nop(), claim: neverClaims,
	}
	outcome, err := speaker.Speak(context.Background(), initiative.SpeakRequest{Intent: initiative.IntentGreet})
	if err != nil {
		t.Fatalf("Speak: %v, want no error (discarding is not a failure)", err)
	}
	if outcome.Delivered || !outcome.Composed || !outcome.Superseded || outcome.Reason != "busy" {
		t.Fatalf("outcome = %+v, want Composed+Superseded with reason busy", outcome)
	}
	if len(client.messages) == 0 {
		t.Fatal("compose must run with no claim held, before Speak ever tries to claim")
	}
	if called {
		t.Fatal("companion event must not fire for a discarded speak")
	}
	if msgs, _ := session.ReadMessages(store.TranscriptPath(main.ID)); len(msgs) != 0 {
		t.Fatal("no message must be written when the turn beat the claim")
	}
}

// TestInitiativeSpeakerComposesThenDiscardsIfUserAlreadySpoke covers
// review finding f2's second case: the claim itself is free by the time
// Compose finishes, but a message already landed in the main session
// while composing (a turn that started and finished entirely during
// Compose, or any other writer) — the user spoke first. The message-count
// comparison (not a timestamp — this spec uses deterministic e2e clocks
// that Speak never controls) is what catches this once the claim is held:
// a higher count than the one read before Compose started means Speak
// must still discard the text instead of appending a now-stale greeting
// behind it.
func TestInitiativeSpeakerComposesThenDiscardsIfUserAlreadySpoke(t *testing.T) {
	store := session.NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatal(err)
	}
	client := &speakStubClient{resp: speakTextResponse("Welcome back!")}
	called := false
	// claimAfterUserMessage simulates a turn that both started and
	// finished writing to the main session while Compose was running: by
	// the time Speak tries to claim, the turn is done (the claim is free)
	// but it already appended a real message.
	claimAfterUserMessage := func(string) (*chatCancelEntry, func(), bool) {
		if err := session.AppendMessage(store.TranscriptPath(main.ID), session.Message{Role: "user", Content: "hi there"}); err != nil {
			t.Fatal(err)
		}
		return nil, func() {}, true
	}
	speaker := &initiativeSpeaker{
		mainSessionID: main.ID, store: store,
		composer: &initiative.SpeechComposer{Client: client},
		notify:   func(context.Context, notificationEvent) { called = true },
		logger:   zerolog.Nop(), claim: claimAfterUserMessage,
	}
	outcome, err := speaker.Speak(context.Background(), initiative.SpeakRequest{Intent: initiative.IntentGreet})
	if err != nil {
		t.Fatalf("Speak: %v, want no error (discarding is not a failure)", err)
	}
	if outcome.Delivered || !outcome.Composed || !outcome.Superseded || outcome.Reason != "user_spoke" {
		t.Fatalf("outcome = %+v, want Composed+Superseded with reason user_spoke", outcome)
	}
	if len(client.messages) == 0 {
		t.Fatal("compose must run before the message-count check can even happen")
	}
	if called {
		t.Fatal("companion event must not fire for a discarded speak")
	}
	msgs, err := session.ReadMessages(store.TranscriptPath(main.ID))
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Role != "user" {
		t.Fatalf("transcript = %+v, want only the user's own message (no initiative text appended)", msgs)
	}
}

func TestInitiativeSpeakerNilClaimIsBusy(t *testing.T) {
	speaker := &initiativeSpeaker{logger: zerolog.Nop()}
	outcome, err := speaker.Speak(context.Background(), initiative.SpeakRequest{Intent: initiative.IntentGreet})
	if err != nil {
		t.Fatalf("Speak: %v", err)
	}
	if outcome.Reason != "busy" || outcome.Composed {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestInitiativeSpeakerComposeFailureReasons(t *testing.T) {
	store := session.NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		resp     llm.ChatResponse
		err      error
		wantCode string
	}{
		{"empty response", speakTextResponse("   \n```\n```"), nil, "compose_empty"},
		{"tool attempt", llm.ChatResponse{Message: llm.ChatMessage{ToolCalls: []llm.ToolCall{{ID: "1", Name: "bash"}}}}, nil, "compose_tool_attempt"},
		{"provider error", llm.ChatResponse{}, errors.New("boom"), "compose_error"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			client := &speakStubClient{resp: c.resp, err: c.err}
			speaker := &initiativeSpeaker{
				mainSessionID: main.ID, store: store,
				composer: &initiative.SpeechComposer{Client: client},
				logger:   zerolog.Nop(), claim: alwaysClaims,
			}
			outcome, err := speaker.Speak(context.Background(), initiative.SpeakRequest{Intent: initiative.IntentGreet})
			if err == nil {
				t.Fatal("expected an error")
			}
			if !outcome.Composed || outcome.Delivered || outcome.Reason != c.wantCode {
				t.Fatalf("outcome = %+v, want Composed with reason %q", outcome, c.wantCode)
			}
		})
	}
}

// TestInitiativeSpeakerWriteErrorWhenAppendFails covers the write_error
// outcome (tars#1220 review finding f3, re-verified after the claim
// reorder in finding f2): Compose succeeds with no claim held, but the
// transcript itself cannot be written once Speak holds the claim and goes
// to check for new messages/append. The directory is created inside the
// claim callback, not before Speak runs, so the baseline read (taken
// before Compose, with no claim held) still succeeds against a normal
// empty transcript — the failure is specifically at the claimed write
// step, standing in for a disk/permission error there (a directory
// sitting at that exact path makes the open fail, without needing
// OS-specific permission tricks). Composed must stay true (an LLM call
// did happen and counts toward the daily cap/backoff) while Delivered
// stays false, and no companion event fires for words that were never
// actually saved anywhere.
func TestInitiativeSpeakerWriteErrorWhenAppendFails(t *testing.T) {
	store := session.NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatal(err)
	}
	claimThenBreakTranscript := func(string) (*chatCancelEntry, func(), bool) {
		if err := os.MkdirAll(store.TranscriptPath(main.ID), 0o755); err != nil {
			t.Fatal(err)
		}
		return nil, func() {}, true
	}
	client := &speakStubClient{resp: speakTextResponse("Welcome back!")}
	var events []notificationEvent
	speaker := &initiativeSpeaker{
		mainSessionID: main.ID, store: store,
		composer: &initiative.SpeechComposer{Client: client},
		notify:   func(_ context.Context, evt notificationEvent) { events = append(events, evt) },
		logger:   zerolog.Nop(), claim: claimThenBreakTranscript,
	}
	outcome, err := speaker.Speak(context.Background(), initiative.SpeakRequest{Intent: initiative.IntentGreet, Now: time.Now()})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !outcome.Composed || outcome.Delivered || outcome.Superseded || outcome.Reason != "write_error" {
		t.Fatalf("outcome = %+v, want Composed with reason write_error", outcome)
	}
	if len(events) != 0 {
		t.Fatalf("companion events = %d, want 0 (never actually written)", len(events))
	}
}

// TestInitiativeSpeakerTouchFailureStillDelivers covers the fix for
// review finding f1: AppendMessage succeeds but the follow-up Touch fails
// (here because mainSessionID was never registered in the store's index,
// so Touch's own session lookup errors — the same shape a session deleted
// concurrently, or a session-index I/O error, would take). The message is
// already real at that point, so the outcome must still be Delivered, the
// companion event must still fire, and the message must still be the one
// in the transcript — matching persistChatResult's own
// AppendMessage-succeeds/Touch-fails handling in handler_chat_execution.go
// (log and keep going), not a dropped delivery that would let the next
// eligible tick compose and write a second greeting on top of this one.
func TestInitiativeSpeakerTouchFailureStillDelivers(t *testing.T) {
	store := session.NewStore(t.TempDir())
	// Creates the store's sessions/ directory (lazily created by the
	// store, not by NewStore) without registering unregisteredSessionID
	// below — AppendMessage only needs the directory to exist; Touch is
	// what looks the id up in the index and is the one that must fail.
	if _, err := store.EnsureMain(); err != nil {
		t.Fatal(err)
	}
	const unregisteredSessionID = "not-in-the-index"
	client := &speakStubClient{resp: speakTextResponse("Welcome back!")}
	var events []notificationEvent
	speaker := &initiativeSpeaker{
		mainSessionID: unregisteredSessionID, store: store,
		composer: &initiative.SpeechComposer{Client: client},
		notify:   func(_ context.Context, evt notificationEvent) { events = append(events, evt) },
		logger:   zerolog.Nop(), claim: alwaysClaims,
	}
	now := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	outcome, err := speaker.Speak(context.Background(), initiative.SpeakRequest{Intent: initiative.IntentGreet, EntryID: "entry-1", Now: now})
	if err != nil {
		t.Fatalf("Speak: %v, want no error (the write itself succeeded)", err)
	}
	if !outcome.Delivered || !outcome.Composed || outcome.Reason != "" {
		t.Fatalf("outcome = %+v, want Delivered despite the Touch failure", outcome)
	}
	msgs, err := session.ReadMessages(store.TranscriptPath(unregisteredSessionID))
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Content != "Welcome back!" {
		t.Fatalf("transcript = %+v, want the message actually written", msgs)
	}
	if len(events) != 1 || events[0].SessionID != unregisteredSessionID {
		t.Fatalf("companion events = %+v, want exactly 1 for the delivered message", events)
	}
}

func TestInitiativeSpeakerNotifyBodyOnlySendsNoMessage(t *testing.T) {
	var got notificationEvent
	speaker := &initiativeSpeaker{
		notify: func(_ context.Context, evt notificationEvent) { got = evt },
		logger: zerolog.Nop(),
	}
	if err := speaker.NotifyBodyOnly(context.Background()); err != nil {
		t.Fatalf("NotifyBodyOnly: %v", err)
	}
	if got.Category != "companion" || got.Message != "" || got.Expression == "" {
		t.Fatalf("event = %+v, want a message-less companion event", got)
	}
}

func TestFirstLineHelper(t *testing.T) {
	if firstLine("one\ntwo") != "one" {
		t.Fatal("expected first line only")
	}
	if firstLine("solo") != "solo" {
		t.Fatal("expected the whole string when there is no newline")
	}
}
