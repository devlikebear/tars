package autofix

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestCleanupStaleTmp_RemovesOldFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	writeFileWithMtime(t, dir, "stale.dat", "x", now.Add(-10*24*time.Hour))
	writeFileWithMtime(t, dir, "fresh.dat", "x", now.Add(-1*time.Hour))

	f := &CleanupStaleTmp{Dirs: []string{dir}, Now: func() time.Time { return now }}
	result, err := f.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !result.Changed {
		t.Error("expected Changed=true")
	}
	if _, err := os.Stat(filepath.Join(dir, "stale.dat")); !os.IsNotExist(err) {
		t.Errorf("stale.dat should be removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "fresh.dat")); err != nil {
		t.Errorf("fresh.dat should remain: %v", err)
	}
}

func TestCleanupStaleTmp_SkipsDirectories(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	subdir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_ = os.Chtimes(subdir, now.Add(-30*24*time.Hour), now.Add(-30*24*time.Hour))

	f := &CleanupStaleTmp{Dirs: []string{dir}, Now: func() time.Time { return now }}
	result, _ := f.Run(context.Background())
	if result.Changed {
		t.Error("should not remove directories")
	}
	if _, err := os.Stat(subdir); err != nil {
		t.Errorf("subdir removed: %v", err)
	}
}

func TestCleanupStaleTmp_SkipsSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require privilege on Windows")
	}
	dir := t.TempDir()
	now := time.Now()
	target := writeFileWithMtime(t, dir, "target.dat", "x", now.Add(-30*24*time.Hour))
	link := filepath.Join(dir, "link.dat")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	// Make symlink itself appear old.
	_ = os.Chtimes(link, now.Add(-30*24*time.Hour), now.Add(-30*24*time.Hour))

	f := &CleanupStaleTmp{Dirs: []string{dir}, Now: func() time.Time { return now }}
	result, _ := f.Run(context.Background())
	// target.dat (regular) should be removed, but link should remain.
	if !result.Changed {
		t.Error("expected target removal")
	}
	if _, err := os.Lstat(link); err != nil {
		t.Errorf("symlink removed: %v", err)
	}
}

func TestCleanupStaleTmp_NonExistentDirSkipped(t *testing.T) {
	f := &CleanupStaleTmp{Dirs: []string{"/does/not/exist", "/also/no"}}
	result, err := f.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if result.Changed {
		t.Error("non-existent dirs should produce no change")
	}
}

func TestCleanupStaleTmp_MultipleDirs(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	now := time.Now()
	writeFileWithMtime(t, dir1, "a.dat", "x", now.Add(-30*24*time.Hour))
	writeFileWithMtime(t, dir2, "b.dat", "x", now.Add(-30*24*time.Hour))

	f := &CleanupStaleTmp{Dirs: []string{dir1, dir2}, Now: func() time.Time { return now }}
	result, _ := f.Run(context.Background())
	if !result.Changed {
		t.Error("expected Changed=true")
	}
	if got, ok := result.Details["deleted_files"].(int); !ok || got != 2 {
		t.Errorf("deleted_files = %v (%T), want 2", result.Details["deleted_files"], result.Details["deleted_files"])
	}
}

func TestCleanupStaleTmp_NilReceiver(t *testing.T) {
	var f *CleanupStaleTmp
	_, err := f.Run(context.Background())
	if err != nil {
		t.Errorf("nil receiver should be safe: %v", err)
	}
}

func TestCleanupStaleTmp_DefaultMaxAge(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	writeFileWithMtime(t, dir, "old.dat", "x", now.Add(-8*24*time.Hour))
	f := &CleanupStaleTmp{Dirs: []string{dir}, Now: func() time.Time { return now }}
	result, _ := f.Run(context.Background())
	if !result.Changed {
		t.Error("default 7d threshold should apply")
	}
}

func TestCleanupStaleTmp_ContextCancel(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	writeFileWithMtime(t, dir, "a.dat", "x", now.Add(-30*24*time.Hour))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before run
	f := &CleanupStaleTmp{Dirs: []string{dir}, Now: func() time.Time { return now }}
	_, err := f.Run(ctx)
	if err == nil {
		t.Error("expected context error")
	}
}
