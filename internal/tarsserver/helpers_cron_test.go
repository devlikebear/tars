package tarsserver

import (
	"testing"

	"github.com/devlikebear/tarsncase/internal/session"
)

func TestResolveCronTargetSessionID_MainUsesConfiguredMainSession(t *testing.T) {
	store := session.NewStore(t.TempDir())
	mainSession, err := store.Create("main")
	if err != nil {
		t.Fatalf("create main session: %v", err)
	}
	otherSession, err := store.Create("other")
	if err != nil {
		t.Fatalf("create other session: %v", err)
	}
	sessionID, explicit, err := resolveCronTargetSessionID(store, "main", mainSession.ID)
	if err != nil {
		t.Fatalf("resolve main target: %v", err)
	}
	if explicit {
		t.Fatalf("main target should not be treated as explicit session id")
	}
	if sessionID != mainSession.ID {
		t.Fatalf("expected main session %q, got %q (latest=%q)", mainSession.ID, sessionID, otherSession.ID)
	}
}
