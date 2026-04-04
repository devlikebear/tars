package tarsserver

import (
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/session"
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

	resolved, err := resolveChatSession(store, "", mainSession.ID, "hello", "")
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

	resolved, err := resolveChatSession(store, customSession.ID, mainSession.ID, "hello", "")
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

func TestResolveSession_StaleMainSession_CreatesNewSession(t *testing.T) {
	store := session.NewStore(t.TempDir())
	// Simulate a stale mainSessionID that no longer exists in the store.
	resolved, err := resolveChatSession(store, "", "deleted-stale-id", "hello", "")
	if err != nil {
		t.Fatalf("expected fallback to new session, got error: %v", err)
	}
	if strings.TrimSpace(resolved) == "" {
		t.Fatalf("expected non-empty session id")
	}
	if resolved == "deleted-stale-id" {
		t.Fatalf("should not return the stale id")
	}
	// Verify the new session exists in the store.
	if _, err := store.Get(resolved); err != nil {
		t.Fatalf("new session should exist in store: %v", err)
	}
}

func TestResolveSession_StaleExplicitSession_CreatesNewSession(t *testing.T) {
	store := session.NewStore(t.TempDir())
	mainSession, err := store.Create("main")
	if err != nil {
		t.Fatalf("create main session: %v", err)
	}
	// Explicit session ID is stale, should create a fresh session instead of
	// silently attaching to the main session.
	resolved, err := resolveChatSession(store, "stale-explicit-id", mainSession.ID, "hello", "")
	if err != nil {
		t.Fatalf("expected fallback to new session, got error: %v", err)
	}
	if strings.TrimSpace(resolved) == "" {
		t.Fatalf("expected non-empty session id")
	}
	if resolved == mainSession.ID {
		t.Fatalf("should not reuse main session %q for stale explicit session", resolved)
	}
	if _, err := store.Get(resolved); err != nil {
		t.Fatalf("new session should exist in store: %v", err)
	}
}

func TestResolveSession_StaleExplicitAndMainSession_CreatesNew(t *testing.T) {
	store := session.NewStore(t.TempDir())
	// Both explicit and main session IDs are stale.
	resolved, err := resolveChatSession(store, "stale-explicit", "stale-main", "hello", "")
	if err != nil {
		t.Fatalf("expected fallback to new session, got error: %v", err)
	}
	if strings.TrimSpace(resolved) == "" {
		t.Fatalf("expected non-empty session id")
	}
	if resolved == "stale-explicit" || resolved == "stale-main" {
		t.Fatalf("should not return stale ids, got %q", resolved)
	}
	if _, err := store.Get(resolved); err != nil {
		t.Fatalf("new session should exist in store: %v", err)
	}
}

func TestResolveSession_KickoffMessage_UsesMainSession(t *testing.T) {
	store := session.NewStore(t.TempDir())
	mainSession, err := store.Create("main")
	if err != nil {
		t.Fatalf("create main session: %v", err)
	}

	// After project removal, kickoff messages use the main session
	resolved, err := resolveChatSession(store, "", mainSession.ID, "todo 앱 만드는 프로젝트 시작해줘", "")
	if err != nil {
		t.Fatalf("resolveChatSession: %v", err)
	}
	if resolved != mainSession.ID {
		t.Fatalf("expected main session %q, got %q", mainSession.ID, resolved)
	}
}

func TestResolveSession_ProjectChat_UsesMainSession(t *testing.T) {
	store := session.NewStore(t.TempDir())
	mainSession, err := store.Create("main")
	if err != nil {
		t.Fatalf("create main session: %v", err)
	}

	// After project removal, project ID doesn't trigger dedicated session
	resolved, err := resolveChatSession(store, "", mainSession.ID, "hello", "project-1")
	if err != nil {
		t.Fatalf("resolveChatSession: %v", err)
	}
	if resolved != mainSession.ID {
		t.Fatalf("expected main session %q, got %q", mainSession.ID, resolved)
	}
	if _, err := store.Get(resolved); err != nil {
		t.Fatalf("expected created project session to exist: %v", err)
	}
}
