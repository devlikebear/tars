package session

import (
	"os"
	"path/filepath"
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

// An index entry whose id is a path cannot make Delete remove a file
// outside the store's directory.
func TestDeleteDoesNotFollowAnIDOutOfTheStore(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "ws"))
	if _, err := store.Create("seed"); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "victim.tasks.json")
	if err := os.WriteFile(outside, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	// From the store's directory, "../../victim" + ".tasks.json" is that file.
	escaping, err := filepath.Rel(store.dir, filepath.Join(root, "victim"))
	if err != nil {
		t.Fatal(err)
	}
	index, err := store.loadIndex()
	if err != nil {
		t.Fatal(err)
	}
	index[escaping] = Session{ID: escaping, Title: "crafted"}
	if err := store.saveIndex(index); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(escaping); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("a file outside the store was removed: %v", err)
	}
}
