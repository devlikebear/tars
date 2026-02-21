package tarsserver

import (
	"strings"
	"testing"

	"github.com/devlikebear/tarsncase/internal/session"
)

func TestResolveSession_MainSession_UsesMainWhenEmpty(t *testing.T) {
	store := session.NewStore(t.TempDir())
	mainSession, err := store.Create("main")
	if err != nil {
		t.Fatalf("create main session: %v", err)
	}
	otherSession, err := store.Create("other")
	if err != nil {
		t.Fatalf("create other session: %v", err)
	}
	_ = otherSession

	resolved, err := resolveChatSession(store, "", mainSession.ID)
	if err != nil {
		t.Fatalf("resolveChatSession: %v", err)
	}
	if resolved != mainSession.ID {
		t.Fatalf("expected main session %q, got %q", mainSession.ID, resolved)
	}
}

func TestResolveSession_MainSession_ExplicitSessionWins(t *testing.T) {
	store := session.NewStore(t.TempDir())
	mainSession, err := store.Create("main")
	if err != nil {
		t.Fatalf("create main session: %v", err)
	}
	customSession, err := store.Create("custom")
	if err != nil {
		t.Fatalf("create custom session: %v", err)
	}

	resolved, err := resolveChatSession(store, customSession.ID, mainSession.ID)
	if err != nil {
		t.Fatalf("resolveChatSession: %v", err)
	}
	if resolved != customSession.ID {
		t.Fatalf("expected explicit session %q, got %q", customSession.ID, resolved)
	}
}

func TestResolveSession_MainSession_DefaultIDValidation(t *testing.T) {
	store := session.NewStore(t.TempDir())
	if _, err := resolveMainSessionID(store, "missing"); err == nil {
		t.Fatalf("expected resolveMainSessionID to fail for missing configured id")
	}
}

func TestResolveSession_MainSession_CreatesWhenNoSessions(t *testing.T) {
	store := session.NewStore(t.TempDir())
	mainSessionID, err := resolveMainSessionID(store, "")
	if err != nil {
		t.Fatalf("resolveMainSessionID: %v", err)
	}
	if strings.TrimSpace(mainSessionID) == "" {
		t.Fatalf("expected created main session id")
	}
	if _, err := store.Get(mainSessionID); err != nil {
		t.Fatalf("expected main session to exist: %v", err)
	}
}
