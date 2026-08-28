package memory

import (
	"testing"
	"time"
)

func TestListMemoryNotesByPrefixGroupsMultilineNotes(t *testing.T) {
	root := t.TempDir()
	first := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	second := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	if err := AppendMemoryNote(root, first, "[archived plan] session=sess-a\nPlan: Older plan\nTasks: 1 total"); err != nil {
		t.Fatalf("append first note: %v", err)
	}
	if err := AppendMemoryNote(root, second, "[archived plan] session=sess-b\nPlan: Newer plan\nTasks: 2 total"); err != nil {
		t.Fatalf("append second note: %v", err)
	}
	if err := AppendMemoryNote(root, second.Add(time.Minute), "ordinary note"); err != nil {
		t.Fatalf("append ordinary note: %v", err)
	}

	notes, err := ListMemoryNotesByPrefix(root, "[archived plan]", 10)
	if err != nil {
		t.Fatalf("list notes: %v", err)
	}
	if len(notes) != 2 {
		t.Fatalf("expected two archived plan notes, got %+v", notes)
	}
	if notes[0].Timestamp != second || notes[0].Text != "[archived plan] session=sess-b\nPlan: Newer plan\nTasks: 2 total" {
		t.Fatalf("expected newest multiline note first, got %+v", notes[0])
	}
	if notes[1].Timestamp != first {
		t.Fatalf("expected older note second, got %+v", notes[1])
	}
}
