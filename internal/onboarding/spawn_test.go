package onboarding

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestSpawnDetached_RunsAndReturnsPID(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("detached spawn helper currently targets unix-like OSes")
	}
	logFile := filepath.Join(t.TempDir(), "spawn.log")
	bin, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("sh not available: %v", err)
	}
	res, err := SpawnDetached(SpawnOptions{
		Executable: bin,
		Args:       []string{"-c", "echo hello && sleep 0.05"},
		LogPath:    logFile,
	})
	if err != nil {
		t.Fatalf("spawn detached: %v", err)
	}
	if res.PID <= 0 {
		t.Fatalf("expected positive pid, got %d", res.PID)
	}
	// give it a moment to write
	time.Sleep(150 * time.Millisecond)
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if string(data) == "" {
		t.Fatalf("expected log content, got empty file")
	}
}

func TestSpawnDetached_RejectsMissingExecutable(t *testing.T) {
	_, err := SpawnDetached(SpawnOptions{Executable: "", Args: nil})
	if err == nil {
		t.Fatal("expected error for empty executable")
	}
}

func TestSpawnDetached_CreatesLogParentDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("detached spawn helper currently targets unix-like OSes")
	}
	bin, err := exec.LookPath("true")
	if err != nil {
		t.Skipf("true not available: %v", err)
	}
	logFile := filepath.Join(t.TempDir(), "nested", "deeper", "spawn.log")
	if _, err := SpawnDetached(SpawnOptions{Executable: bin, LogPath: logFile}); err != nil {
		t.Fatalf("spawn detached: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(logFile)); err != nil {
		t.Fatalf("expected log dir created: %v", err)
	}
}
