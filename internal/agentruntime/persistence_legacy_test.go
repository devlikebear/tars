package agentruntime

import (
	"os"
	"testing"
)

func TestReadRunsPreservesLegacyExecutionRoot(t *testing.T) {
	store := newSnapshotStore(t.TempDir())
	// A removed task worktree may no longer exist. Legacy snapshots must still load.
	payload := `{"runs":[{"run_id":"legacy-run","session_id":"legacy-session","prompt":"work","status":"completed","execution_root":"/removed/task/worktree"}]}`
	if err := os.WriteFile(store.runsPath, []byte(payload), 0600); err != nil {
		t.Fatal(err)
	}
	runs, err := store.readRuns()
	if err != nil {
		t.Fatalf("read legacy snapshot: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != "legacy-run" || runs[0].SessionID != "legacy-session" || runs[0].Prompt != "work" || runs[0].ExecutionRoot != "/removed/task/worktree" {
		t.Fatalf("legacy run not preserved: %+v", runs)
	}
	if err := store.writeRuns(runs); err != nil {
		t.Fatal(err)
	}
	restored, err := store.readRuns()
	if err != nil || len(restored) != 1 || restored[0].ID != runs[0].ID {
		t.Fatalf("round trip: %+v, %v", restored, err)
	}
}
