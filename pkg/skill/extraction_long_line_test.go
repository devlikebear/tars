package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A long, blank or corrupt inbox line must not hide the other candidates
// (#1231).
func TestReadExtractionCandidatesReadsLongLinesAndSkipsBadOnes(t *testing.T) {
	root := t.TempDir()
	path := ExtractionInboxPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("s", 300*1024)
	raw := `{"id":"c1","status":"pending","name":"one","summary":"` + long + `"}` +
		"\n\n{not json\n" + `{"id":"c2","status":"pending","name":"two","summary":"small"}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readExtractionCandidates(root)
	if err != nil || len(got) != 2 || got[0].Summary != long || got[1].ID != "c2" {
		t.Fatalf("candidates = %d, err=%v", len(got), err)
	}
}
