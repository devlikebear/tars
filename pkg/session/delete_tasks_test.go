package session

import (
	"os"
	"testing"
)

// A deleted session's task file goes with its transcript.
func TestDeleteRemovesTheTaskFile(t *testing.T) {
	store := NewStore(t.TempDir())
	sess, err := store.Create("with tasks")
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.Create("kept")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{sess.ID, other.ID} {
		if err := store.SaveTasks(id, SessionTasks{Tasks: []Task{{ID: "t1", Title: "do it", Status: "pending"}}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Delete(sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.tasksPath(sess.ID)); !os.IsNotExist(err) {
		t.Fatalf("the deleted session's task file is still there (err = %v)", err)
	}
	if _, err := os.Stat(store.tasksPath(other.ID)); err != nil {
		t.Fatalf("another session's task file was removed: %v", err)
	}
}
