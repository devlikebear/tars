package tarsserver

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// TestResolveInitiativeLLMBackendSameAndDifferentProvider checks the
// provider-alias comparison end to end through a real router built from
// config.ResolveLLMTier — not just the pure PlanText table — including the
// "same kind, different alias" case where a naive kind-only comparison
// would wrongly send text.
func TestResolveInitiativeLLMBackendSameAndDifferentProvider(t *testing.T) {
	cfg := config.Config{LLMConfig: config.LLMConfig{
		LLMDefaultTier:  "standard",
		LLMRoleDefaults: map[string]string{"initiative": "light"},
		LLMProviders: map[string]config.LLMProviderSettings{
			"shared": {Kind: "openai", BaseURL: "http://shared.example", APIKey: "k"},
			"other":  {Kind: "openai", BaseURL: "http://other.example", APIKey: "k2"},
		},
		LLMTiers: map[string]config.LLMTierBinding{
			"heavy":    {Provider: "shared", Model: "heavy-model"},
			"standard": {Provider: "shared", Model: "chat-model"},
			"light":    {Provider: "shared", Model: "light-model"},
		},
	}}
	router, err := buildLLMRouter(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, _, client, model := resolveInitiativeLLMBackend(cfg, router)
	if !plan.SendsText || plan.Reason != "llm_same_provider" || client == nil || model != "light-model" {
		t.Fatalf("same-provider plan = %+v client=%v model=%q", plan, client, model)
	}

	cfg.LLMTiers["light"] = config.LLMTierBinding{Provider: "other", Model: "light-model"}
	router2, err := buildLLMRouter(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan2, _, client2, _ := resolveInitiativeLLMBackend(cfg, router2)
	if plan2.SendsText || plan2.Reason != "llm_other_provider" || client2 == nil {
		t.Fatalf("different-provider (same kind) plan = %+v client=%v", plan2, client2)
	}
}

func TestResolveInitiativeLLMBackendMissingRoleTier(t *testing.T) {
	cfg := config.Config{LLMConfig: config.LLMConfig{
		LLMDefaultTier:  "standard",
		LLMRoleDefaults: map[string]string{"initiative": "missing-tier"},
		LLMProviders:    map[string]config.LLMProviderSettings{"shared": {Kind: "openai", BaseURL: "http://shared.example", APIKey: "k"}},
		LLMTiers: map[string]config.LLMTierBinding{
			"heavy":    {Provider: "shared", Model: "heavy-model"},
			"standard": {Provider: "shared", Model: "chat-model"},
			"light":    {Provider: "shared", Model: "light-model"},
		},
	}}
	router, err := buildLLMRouter(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, _, client, _ := resolveInitiativeLLMBackend(cfg, router)
	if plan.Usable || plan.Reason != "llm_unavailable" || client != nil {
		t.Fatalf("plan = %+v client=%v", plan, client)
	}
}

func TestSessionObserverToleratesMissingFiles(t *testing.T) {
	store := session.NewStore(t.TempDir())
	obs := newSessionInitiativeObserver(sessionObserverDeps{Store: store, WorkspaceDir: t.TempDir()})
	got, err := obs.Observe(context.Background(), time.Now())
	if err != nil || len(got.RecentUser) != 0 || got.Profile != "" || !got.ConsoleConnectedAt.IsZero() {
		t.Fatalf("observation = %+v err=%v", got, err)
	}
}

func TestSessionObserverReadsUserMessagesAndConsole(t *testing.T) {
	store := session.NewStore(t.TempDir())
	sess, err := store.Create("main")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 29, 14, 0, 0, 0, time.UTC)
	path := store.TranscriptPath(sess.ID)
	for _, m := range []session.Message{
		{Role: "user", Content: "too old", Timestamp: now.Add(-20 * time.Hour)},
		{Role: "user", Content: "오늘 너무 피곤해", Timestamp: now.Add(-30 * time.Minute)},
		{Role: "user", Content: "[PULSE AUTO-RESUME] continue", Timestamp: now.Add(-20 * time.Minute)},
		{Role: "assistant", Content: "쉬어가세요", Timestamp: now.Add(-29 * time.Minute)},
	} {
		if err := session.AppendMessage(path, m); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Touch(sess.ID, now.Add(-20*time.Minute)); err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "USER.md"), []byte("birthday: Sep 29"), 0o644); err != nil {
		t.Fatal(err)
	}
	subscribers := 1
	obs := newSessionInitiativeObserver(sessionObserverDeps{
		Store: store, WorkspaceDir: workspace,
		SubscriberCount: func() int { return subscribers },
		ChatBusy:        func() bool { return true },
		TelegramPaired:  func() bool { return true },
	})

	got, err := obs.Observe(context.Background(), now)
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if len(got.RecentUser) != 1 || got.RecentUser[0].Text != "오늘 너무 피곤해" {
		t.Fatalf("recent user = %+v", got.RecentUser)
	}
	if got.Profile != "birthday: Sep 29" || !got.ChatBusy || !got.TelegramPaired || !got.ConsoleConnectedAt.Equal(now) {
		t.Fatalf("observation = %+v", got)
	}

	later, _ := obs.Observe(context.Background(), now.Add(time.Minute))
	if !later.ConsoleConnectedAt.Equal(now) {
		t.Fatalf("console connection time must stick while connected, got %v", later.ConsoleConnectedAt)
	}
	subscribers = 0
	if gone, _ := obs.Observe(context.Background(), now.Add(2*time.Minute)); !gone.ConsoleConnectedAt.IsZero() {
		t.Fatal("console connection must reset when no subscriber is left")
	}
}

func TestBuildInitiativeRuntimeDisabled(t *testing.T) {
	setup := buildInitiativeRuntime(initiativeSetupInputs{Config: config.Config{}, Logger: zerolog.Nop()})
	if setup.Runtime != nil || setup.Handler == nil {
		t.Fatalf("setup = %+v", setup)
	}
}

// TestBuildInitiativeRuntimeLiveButDisabled checks that an explicit
// mode: live does nothing when enabled is false (tars#1220): no runtime at
// all, same as shadow, so Start never ticks and nothing can ever compose or
// deliver speech.
func TestBuildInitiativeRuntimeLiveButDisabled(t *testing.T) {
	cfg := config.Config{Initiative: config.InitiativeConfig{Enabled: false, Mode: "live"}}
	setup := buildInitiativeRuntime(initiativeSetupInputs{Config: cfg, Logger: zerolog.Nop()})
	if setup.Runtime != nil || setup.Handler == nil {
		t.Fatalf("setup = %+v", setup)
	}
}

func TestBuildInitiativeRuntimeGatesTextByLoopback(t *testing.T) {
	for base, loopback := range map[string]bool{"http://127.0.0.1:8009": true, "https://api.typesafe.ai": false} {
		cfg := config.Config{Initiative: config.InitiativeConfig{Enabled: true, Backend: "jev"}, Jev: config.JevConfig{BaseURL: base}}
		setup := buildInitiativeRuntime(initiativeSetupInputs{Config: cfg, WorkspaceDir: t.TempDir(), Logger: zerolog.Nop()})
		if setup.Runtime == nil {
			t.Fatal("expected a runtime when enabled")
		}
		if b := setup.Runtime.Snapshot().Backend; !b.Configured || b.Loopback != loopback || b.Backend != "jev" || b.SendsText != loopback {
			t.Fatalf("%s backend = %+v", base, b)
		}
	}
}

// TestBuildInitiativeRuntimeLLMBackendDefault checks the default backend
// ("llm", no explicit Backend) degrades to unusable without a router (e.g.
// setup-only mode) instead of panicking or silently falling back to jev.
func TestBuildInitiativeRuntimeLLMBackendDefault(t *testing.T) {
	cfg := config.Config{Initiative: config.InitiativeConfig{Enabled: true}}
	setup := buildInitiativeRuntime(initiativeSetupInputs{Config: cfg, WorkspaceDir: t.TempDir(), Logger: zerolog.Nop()})
	if setup.Runtime == nil {
		t.Fatal("expected a runtime when enabled")
	}
	b := setup.Runtime.Snapshot().Backend
	if b.Backend != "llm" || b.Configured || b.SendsText || b.TextReason != "llm_unavailable" {
		t.Fatalf("backend = %+v", b)
	}
}

func appendAndTouch(t *testing.T, store *session.Store, id string, msgs ...session.Message) {
	t.Helper()
	var last time.Time
	for _, m := range msgs {
		if err := session.AppendMessage(store.TranscriptPath(id), m); err != nil {
			t.Fatal(err)
		}
		last = m.Timestamp
	}
	if err := store.Touch(id, last); err != nil {
		t.Fatal(err)
	}
}

func TestSessionObserverReadsEveryVisibleSession(t *testing.T) {
	store := session.NewStore(t.TempDir())
	now := time.Date(2026, 9, 29, 14, 0, 0, 0, time.UTC)
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatal(err)
	}
	project, err := store.Create("tars workbench")
	if err != nil {
		t.Fatal(err)
	}
	worker, err := store.CreateWithOptions("worker run", "worker", true)
	if err != nil {
		t.Fatal(err)
	}
	appendAndTouch(t, store, main.ID, session.Message{Role: "user", Content: "main earlier", Timestamp: now.Add(-40 * time.Minute)})
	appendAndTouch(t, store, project.ID, session.Message{Role: "user", Content: "project now", Timestamp: now.Add(-2 * time.Minute)})
	appendAndTouch(t, store, worker.ID, session.Message{Role: "user", Content: "worker prompt", Timestamp: now.Add(-time.Minute)})

	got, err := newSessionInitiativeObserver(sessionObserverDeps{Store: store}).Observe(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.RecentUser) != 2 || got.RecentUser[1].Text != "project now" || got.RecentUser[0].Text != "main earlier" {
		t.Fatalf("recent user = %+v", got.RecentUser)
	}
	if !got.LastUserAt.Equal(now.Add(-2 * time.Minute)) {
		t.Fatalf("last user at = %v", got.LastUserAt)
	}
}

// TestSessionObserverIgnoresInitiativeMessagesAsUserActivity is the
// regression test for tars#1220's observer bug: an initiative-authored
// assistant message (and the session Touch that comes with delivering it)
// must not look like the user being active on the next tick — neither as
// a (nonexistent) user message, nor through the no-user-message fallback
// that otherwise uses the newest session activity as LastUserAt's upper
// bound.
func TestSessionObserverIgnoresInitiativeMessagesAsUserActivity(t *testing.T) {
	store := session.NewStore(t.TempDir())
	now := time.Date(2026, 9, 29, 14, 0, 0, 0, time.UTC)
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatal(err)
	}
	// No user ever spoke in this session (or anywhere else); TARS spoke
	// first a moment ago.
	appendAndTouch(t, store, main.ID, session.Message{
		Role: "assistant", Content: "Welcome back!", Timestamp: now.Add(-time.Minute),
		Initiative: &session.MessageInitiative{Intent: "greet", EntryID: "e1"},
	})
	got, err := newSessionInitiativeObserver(sessionObserverDeps{Store: store}).Observe(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if !got.LastUserAt.IsZero() {
		t.Fatalf("LastUserAt = %v, want zero — an initiative message is not user activity", got.LastUserAt)
	}
	if len(got.RecentUser) != 0 {
		t.Fatalf("RecentUser = %+v, want none", got.RecentUser)
	}
}

func TestSessionObserverKnowsLastUserBeyondTheTextLookback(t *testing.T) {
	store := session.NewStore(t.TempDir())
	now := time.Date(2026, 9, 29, 14, 0, 0, 0, time.UTC)
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatal(err)
	}
	old := now.Add(-50 * time.Hour)
	appendAndTouch(t, store, main.ID, session.Message{Role: "user", Content: "see you", Timestamp: old})
	got, err := newSessionInitiativeObserver(sessionObserverDeps{Store: store}).Observe(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.RecentUser) != 0 || !got.LastUserAt.Equal(old) {
		t.Fatalf("observation = %+v", got)
	}
}
