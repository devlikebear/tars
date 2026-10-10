package tarsserver

import (
	"context"
	"errors"
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

func TestInitiativeSpeakerBusyWhenClaimFails(t *testing.T) {
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
		logger:   zerolog.Nop(), claim: neverClaims,
	}
	outcome, err := speaker.Speak(context.Background(), initiative.SpeakRequest{Intent: initiative.IntentGreet})
	if err != nil {
		t.Fatalf("Speak: %v", err)
	}
	if outcome.Delivered || outcome.Composed || outcome.Reason != "busy" {
		t.Fatalf("outcome = %+v, want busy with no compose", outcome)
	}
	if len(client.messages) != 0 {
		t.Fatal("compose must not be called when the claim fails")
	}
	if called {
		t.Fatal("companion event must not fire when the claim fails")
	}
	if msgs, _ := session.ReadMessages(store.TranscriptPath(main.ID)); len(msgs) != 0 {
		t.Fatal("no message must be written when the claim fails")
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
