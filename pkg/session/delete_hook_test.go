package session_test

import (
	"testing"
	"time"

	"github.com/devlikebear/tars/pkg/session"
)

func TestDeleteHookSeesDeletedSession(t *testing.T) {
	store := session.NewStore(t.TempDir())
	sess, err := store.Create("doomed")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	var deleted []string
	store.SetDeleteHook(func(id string) { deleted = append(deleted, id) })
	if err := store.Delete(sess.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(deleted) != 1 || deleted[0] != sess.ID {
		t.Fatalf("hook saw %v, want [%s]", deleted, sess.ID)
	}
	// Deleting a missing session is a no-op and has nothing to report.
	if err := store.Delete(sess.ID); err != nil {
		t.Fatalf("Delete again: %v", err)
	}
	if len(deleted) != 1 {
		t.Fatalf("a no-op delete ran the hook: %v", deleted)
	}

	store.SetDeleteHook(nil)
	other, err := store.Create("unobserved")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Delete(other.ID); err != nil {
		t.Fatalf("Delete without hook: %v", err)
	}
	var nilStore *session.Store
	nilStore.SetDeleteHook(func(string) {})
}

func TestNewMessageIDIsTimeOrdered(t *testing.T) {
	at := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	first, err := session.NewMessageID(at)
	if err != nil {
		t.Fatalf("NewMessageID: %v", err)
	}
	second, err := session.NewMessageID(at.Add(time.Second))
	if err != nil {
		t.Fatalf("NewMessageID: %v", err)
	}
	if first == "" || first == second || first > second {
		t.Fatalf("ids %q, %q: want distinct and ordered by time", first, second)
	}
}
