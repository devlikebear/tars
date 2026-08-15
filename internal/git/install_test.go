package git

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestInstallRoots(t *testing.T) {
	roots := InstallRoots()

	if runtime.GOOS != "windows" {
		if roots != nil {
			t.Fatalf("expected no install roots off Windows, got %v", roots)
		}
		return
	}

	// On Windows the roots are derived from the resolved git binary, so an
	// environment without git legitimately has none.
	if _, err := Executable(); err != nil {
		t.Skipf("git not available: %v", err)
	}
	if len(roots) == 0 {
		t.Fatal("expected install roots to be derived from the resolved git binary")
	}
	for _, root := range roots {
		if !filepath.IsAbs(root) {
			t.Fatalf("install root %q is not absolute", root)
		}
	}
}

func TestExecutableIsAbsolute(t *testing.T) {
	path, err := Executable()
	if err != nil {
		t.Skipf("git not available: %v", err)
	}
	if !filepath.IsAbs(path) {
		t.Fatalf("expected an absolute git path, got %q", path)
	}
}
