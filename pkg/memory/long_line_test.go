package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The memory files are appended to line by line; a long, blank or corrupt
// line must not hide the others (#1231).
func TestMemoryFilesReadLongLinesAndSkipBadOnes(t *testing.T) {
	root := t.TempDir()
	long := strings.Repeat("s", 300*1024)
	if err := os.MkdirAll(filepath.Join(root, "memory"), 0o755); err != nil {
		t.Fatal(err)
	}

	experiences := `{"timestamp":"2026-10-11T00:00:00Z","category":"fact","summary":"` + long + `"}` +
		"\n\n{not json\n" + `{"timestamp":"2026-10-11T00:01:00Z","category":"other","summary":"small"}`
	if err := os.WriteFile(filepath.Join(root, "memory", "experiences.jsonl"), []byte(experiences), 0o644); err != nil {
		t.Fatal(err)
	}
	found, err := SearchExperiences(root, SearchOptions{Category: "fact"})
	if err != nil || len(found) != 1 || found[0].Summary != long {
		t.Fatalf("experiences = %d, err=%v", len(found), err)
	}

	inbox := `{"id":"c1","status":"pending","category":"fact","summary":"` + long + `"}` +
		"\n\n{not json\n" + `{"id":"c2","status":"pending","category":"fact","summary":"small"}`
	if err := os.WriteFile(memoryInboxPath(root), []byte(inbox), 0o644); err != nil {
		t.Fatal(err)
	}
	candidates, err := readMemoryCandidates(root)
	if err != nil || len(candidates) != 2 || candidates[0].Summary != long {
		t.Fatalf("candidates = %d, err=%v", len(candidates), err)
	}
}
