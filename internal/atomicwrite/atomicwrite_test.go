package atomicwrite

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWrite_CreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	if err := Write(path, []byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("content mismatch: got %q want %q", got, "hello")
	}
}

func TestWrite_CreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deeper", "state.json")

	if err := Write(path, []byte("x")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
}

func TestWrite_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := Write(path, []byte("new")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "new" {
		t.Fatalf("content mismatch: got %q want %q", got, "new")
	}
}

func TestWrite_LeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	if err := Write(path, []byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

func TestWrite_DoesNotCorruptOnReadOnlyDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	// Seed a known-good file.
	if err := os.WriteFile(path, []byte("good"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	defer os.Chmod(dir, 0o755)

	// Write should fail (cannot create temp), but original must remain.
	if err := Write(path, []byte("new")); err == nil {
		t.Fatalf("expected Write to fail on read-only dir")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "good" {
		t.Fatalf("original file should be preserved on failure: got %q", got)
	}
}
