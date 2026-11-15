package focuspipeline

import (
	"slices"
	"sync"
	"testing"
)

func TestWatchChangesHearsEveryStoreOverTheFolder(t *testing.T) {
	dir := t.TempDir()
	var mu sync.Mutex
	var heard []string
	stop := WatchChanges(dir, func(id string) {
		mu.Lock()
		heard = append(heard, id)
		mu.Unlock()
	})

	// Stores are made per request: a second one over the same folder is
	// heard too, through a path spelled differently.
	if err := NewStore(dir).Save(Pipeline{SessionID: "s1"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewStore(dir+"/.").Update("s1", func(p Pipeline) (Pipeline, error) {
		p.Goal = "changed"
		return p, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := NewStore(dir).Delete("s1"); err != nil {
		t.Fatal(err)
	}
	// Deleting what is not there changes nothing.
	if err := NewStore(dir).Delete("s1"); err != nil {
		t.Fatal(err)
	}
	// Another folder is not heard.
	if err := NewStore(t.TempDir()).Save(Pipeline{SessionID: "other"}); err != nil {
		t.Fatal(err)
	}

	stop()
	stop() // twice is fine
	if err := NewStore(dir).Save(Pipeline{SessionID: "after"}); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if want := []string{"s1", "s1", "s1"}; !slices.Equal(heard, want) {
		t.Fatalf("heard %v, want %v", heard, want)
	}
}

func TestWatchChangesNilAndFailedSave(t *testing.T) {
	WatchChanges(t.TempDir(), nil)() // no watcher, no panic

	dir := t.TempDir()
	calls := 0
	defer WatchChanges(dir, func(string) { calls++ })()
	if err := NewStore(dir).Save(Pipeline{SessionID: "../escape"}); err == nil {
		t.Fatal("an unsafe id must not save")
	}
	if calls != 0 {
		t.Fatalf("a failed save was announced %d times", calls)
	}
}
