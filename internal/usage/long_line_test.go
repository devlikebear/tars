package usage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// One unreadable or oversized line must not hide the rest of a day's log
// (#1231): blank and corrupt lines are skipped, a long one is read.
func TestReadUsageAndSignalFilesSkipBadLinesAndReadLongOnes(t *testing.T) {
	long := strings.Repeat("m", 300*1024)
	dir := t.TempDir()

	usagePath := filepath.Join(dir, "usage.jsonl")
	usageRaw := `{"provider":"a","model":"` + long + `"}` + "\n\n{not json\n" + `{"provider":"b","model":"small"}`
	if err := os.WriteFile(usagePath, []byte(usageRaw), 0o600); err != nil {
		t.Fatal(err)
	}
	entries := readUsageFile(usagePath)
	if len(entries) != 2 || entries[0].Model != long || entries[1].Provider != "b" {
		t.Fatalf("read %d usage entries", len(entries))
	}

	signalPath := filepath.Join(dir, "signals.jsonl")
	signalRaw := `{"name":"tool_call","session_id":"` + long + `"}` + "\n\n{not json\n" + `{"name":"turn"}`
	if err := os.WriteFile(signalPath, []byte(signalRaw), 0o600); err != nil {
		t.Fatal(err)
	}
	signals := readSignalFile(signalPath)
	if len(signals) != 2 || signals[0].SessionID != long || signals[1].Name != "turn" {
		t.Fatalf("read %d signal entries", len(signals))
	}
}
