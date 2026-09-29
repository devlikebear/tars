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

func TestBuildInitiativeRuntimeGatesTextByLoopback(t *testing.T) {
	for base, loopback := range map[string]bool{"http://127.0.0.1:8009": true, "https://api.typesafe.ai": false} {
		cfg := config.Config{Initiative: config.InitiativeConfig{Enabled: true}, Jev: config.JevConfig{BaseURL: base}}
		setup := buildInitiativeRuntime(initiativeSetupInputs{Config: cfg, WorkspaceDir: t.TempDir(), Logger: zerolog.Nop()})
		if setup.Runtime == nil {
			t.Fatal("expected a runtime when enabled")
		}
		if b := setup.Runtime.Snapshot().Backend; !b.Configured || b.Loopback != loopback {
			t.Fatalf("%s backend = %+v", base, b)
		}
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
