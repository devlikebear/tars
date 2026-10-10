package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A session id names the session's files. One that is not a plain name must
// not reach a file outside the store's directory, whatever the caller did or
// did not check first.
func TestSessionIDThatIsNotAPlainNameGetsNoFile(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	if err := os.MkdirAll(filepath.Join(root, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Files a traversing id would reach: <root>/outside.tasks.json and
	// <root>/outside.jsonl, one level above the store's directory.
	outsideTasks := filepath.Join(root, "outside.tasks.json")
	if err := os.WriteFile(outsideTasks, []byte(`{"plan":{"goal":"leaked"},"tasks":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	outsideTranscript := filepath.Join(root, "outside.jsonl")
	if err := os.WriteFile(outsideTranscript, []byte(`{"role":"user","content":"leaked"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{"../outside", `..\outside`, "a/b", `a\b`, "..", "x/../../outside"} {
		if path := store.TranscriptPath(id); path != "" {
			t.Errorf("TranscriptPath(%q) = %q, want no path", id, path)
		}
		if path := store.tasksPath(id); path != "" {
			t.Errorf("tasksPath(%q) = %q, want no path", id, path)
		}
	}

	got, err := store.GetTasks("../outside")
	if err != nil {
		t.Fatalf("GetTasks: %v", err)
	}
	if got.Plan != nil {
		t.Fatalf("GetTasks read a file outside the store: %+v", got.Plan)
	}
	if msgs, err := ReadMessages(store.TranscriptPath("../outside")); err == nil && len(msgs) > 0 {
		t.Fatalf("ReadMessages read a file outside the store: %+v", msgs)
	}
	if err := store.SaveTasks("../outside", SessionTasks{}); err == nil {
		t.Fatal("SaveTasks accepted an id that leaves the store")
	}
	raw, err := os.ReadFile(outsideTasks)
	if err != nil || !strings.Contains(string(raw), "leaked") {
		t.Fatalf("the outside tasks file was changed: %q, %v", raw, err)
	}
	if err := AppendMessage(store.TranscriptPath("../outside"), Message{Role: "user", Content: "x"}); err == nil {
		t.Fatal("AppendMessage wrote for an id that leaves the store")
	}

	// An ordinary id is unaffected.
	if want := filepath.Join(root, "sessions", "abc123.jsonl"); store.TranscriptPath("abc123") != want {
		t.Errorf("TranscriptPath(abc123) = %q, want %q", store.TranscriptPath("abc123"), want)
	}
}
