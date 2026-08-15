package onboarding

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// echoCommand returns a program that prints to stdout and exits.
//
// Windows deliberately avoids Git-for-Windows' sh.exe: SpawnDetached hands the
// child a log handle opened with O_APPEND, which on Windows means
// FILE_APPEND_DATA without FILE_WRITE_DATA, and the MSYS2 runtime backing
// sh.exe fails such writes and exits 1. Native programs — including tars
// itself, the only thing SpawnDetached actually launches — write to it fine, so
// the native shell is the representative fixture here.
func echoCommand(t *testing.T) (string, []string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		bin, err := exec.LookPath("cmd.exe")
		if err != nil {
			t.Skipf("cmd.exe not available: %v", err)
		}
		return bin, []string{"/c", "echo hello"}
	}
	bin, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("sh not available: %v", err)
	}
	return bin, []string{"-c", "echo hello && sleep 0.05"}
}

// exitCommand returns a program that exits immediately without output.
func exitCommand(t *testing.T) (string, []string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		bin, err := exec.LookPath("cmd.exe")
		if err != nil {
			t.Skipf("cmd.exe not available: %v", err)
		}
		return bin, []string{"/c", "exit"}
	}
	bin, err := exec.LookPath("true")
	if err != nil {
		t.Skipf("true not available: %v", err)
	}
	return bin, nil
}

// waitForContent polls until path is non-empty. Process startup latency varies
// widely across platforms, so poll instead of sleeping a fixed interval.
func waitForContent(t *testing.T, path string) []byte {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			return data
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected log content at %s, got %d bytes (last err: %v)", path, len(data), err)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func TestSpawnDetached_RunsAndReturnsPID(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "spawn.log")
	bin, args := echoCommand(t)
	res, err := SpawnDetached(SpawnOptions{
		Executable: bin,
		Args:       args,
		LogPath:    logFile,
	})
	if err != nil {
		t.Fatalf("spawn detached: %v", err)
	}
	if res.PID <= 0 {
		t.Fatalf("expected positive pid, got %d", res.PID)
	}
	waitForContent(t, logFile)
}

func TestSpawnDetached_RejectsMissingExecutable(t *testing.T) {
	_, err := SpawnDetached(SpawnOptions{Executable: "", Args: nil})
	if err == nil {
		t.Fatal("expected error for empty executable")
	}
}

func TestSpawnDetached_CreatesLogParentDir(t *testing.T) {
	bin, args := exitCommand(t)
	logFile := filepath.Join(t.TempDir(), "nested", "deeper", "spawn.log")
	if _, err := SpawnDetached(SpawnOptions{Executable: bin, Args: args, LogPath: logFile}); err != nil {
		t.Fatalf("spawn detached: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(logFile)); err != nil {
		t.Fatalf("expected log dir created: %v", err)
	}
}
