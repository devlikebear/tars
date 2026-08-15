package exepath

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// writeExecutable creates a file LookPath will accept as runnable. Windows
// decides by extension against PATHEXT rather than a permission bit, so the
// name differs per platform while the lookup name stays the same.
func writeExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		name += ".bat"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}
	return path
}

func TestResolvePrefersDefaultPath(t *testing.T) {
	dir := t.TempDir()
	preferred := writeExecutable(t, dir, "preferred")
	fallbackDir := t.TempDir()
	fallback := writeExecutable(t, fallbackDir, "fallback")

	got, err := Resolve(Candidates{
		DefaultPath: preferred,
		LookupNames: []string{"definitely-not-a-real-tool"},
		Fallbacks:   func() []string { return []string{fallback} },
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != preferred {
		t.Fatalf("expected the default path %q, got %q", preferred, got)
	}
}

func TestResolveSkipsDefaultPathThatIsADirectory(t *testing.T) {
	dir := t.TempDir()
	fallback := writeExecutable(t, dir, "fallback")

	got, err := Resolve(Candidates{
		DefaultPath: dir,
		Fallbacks:   func() []string { return []string{fallback} },
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != fallback {
		t.Fatalf("expected the fallback %q, got %q", fallback, got)
	}
}

func TestResolveFindsNameOnPath(t *testing.T) {
	dir := t.TempDir()
	want := writeExecutable(t, dir, "tars-probe")
	t.Setenv("PATH", dir)

	got, err := Resolve(Candidates{
		DefaultPath: filepath.Join(t.TempDir(), "absent"),
		LookupNames: []string{"missing-first", "tars-probe"},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != want {
		t.Fatalf("expected %q from PATH, got %q", want, got)
	}
}

// TestResolveRejectsRelativePathLookup guards the reason these paths were
// hardcoded to begin with: a PATH entry relative to the working directory must
// never yield the executable tars runs.
func TestResolveRejectsRelativePathLookup(t *testing.T) {
	dir := t.TempDir()
	relativeDir := "bin"
	if err := os.MkdirAll(filepath.Join(dir, relativeDir), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeExecutable(t, filepath.Join(dir, relativeDir), "tars-probe")
	t.Chdir(dir)
	t.Setenv("PATH", relativeDir)

	_, err := Resolve(Candidates{LookupNames: []string{"tars-probe"}})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for a relative PATH hit, got %v", err)
	}
}

func TestResolveUsesFallbacksInOrder(t *testing.T) {
	dir := t.TempDir()
	second := writeExecutable(t, dir, "second")
	t.Setenv("PATH", t.TempDir())

	got, err := Resolve(Candidates{
		LookupNames: []string{"definitely-not-a-real-tool"},
		Fallbacks: func() []string {
			return []string{filepath.Join(dir, "absent"), second}
		},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != second {
		t.Fatalf("expected the second fallback %q, got %q", second, got)
	}
}

func TestResolveReturnsErrNotFoundWhenEverythingMisses(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := Resolve(Candidates{
		DefaultPath: filepath.Join(t.TempDir(), "absent"),
		LookupNames: []string{"definitely-not-a-real-tool"},
		Fallbacks:   func() []string { return []string{filepath.Join(t.TempDir(), "also-absent")} },
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestResolveToleratesNilFallbacks(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	if _, err := Resolve(Candidates{LookupNames: []string{"definitely-not-a-real-tool"}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
