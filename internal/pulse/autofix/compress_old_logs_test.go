package autofix

import (
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFileWithMtime(t *testing.T, dir, name, body string, mtime time.Time) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
	return path
}

func TestCompressOldLogs_SkipsRecentFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	writeFileWithMtime(t, dir, "fresh.log", "hello", now.Add(-1*time.Hour))

	f := &CompressOldLogs{LogsDir: dir, MaxAge: 24 * time.Hour, Now: func() time.Time { return now }}
	result, err := f.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if result.Changed {
		t.Error("should not change recent files")
	}
	if _, err := os.Stat(filepath.Join(dir, "fresh.log")); err != nil {
		t.Errorf("fresh.log removed: %v", err)
	}
}

func TestCompressOldLogs_CompressesOldFile(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	content := "old log data " + string(make([]byte, 200))
	writeFileWithMtime(t, dir, "old.log", content, now.Add(-10*24*time.Hour))

	f := &CompressOldLogs{LogsDir: dir, MaxAge: 7 * 24 * time.Hour, Now: func() time.Time { return now }}
	result, err := f.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !result.Changed {
		t.Error("expected Changed=true")
	}
	if _, err := os.Stat(filepath.Join(dir, "old.log")); !os.IsNotExist(err) {
		t.Errorf("old.log should be removed, err=%v", err)
	}
	// Verify gz exists and decompresses back to original.
	gzPath := filepath.Join(dir, "old.log.gz")
	in, err := os.Open(gzPath)
	if err != nil {
		t.Fatalf("open gz: %v", err)
	}
	defer in.Close()
	gr, err := gzip.NewReader(in)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gr.Close()
	data, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("read gz: %v", err)
	}
	if string(data) != content {
		t.Errorf("gz content mismatch")
	}
}

func TestCompressOldLogs_Idempotent(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	writeFileWithMtime(t, dir, "old.log", "data", now.Add(-10*24*time.Hour))

	f := &CompressOldLogs{LogsDir: dir, Now: func() time.Time { return now }}
	first, _ := f.Run(context.Background())
	if !first.Changed {
		t.Error("first run should change")
	}
	second, _ := f.Run(context.Background())
	if second.Changed {
		t.Error("second run should not change")
	}
}

func TestCompressOldLogs_IgnoresNonLogFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	writeFileWithMtime(t, dir, "old.txt", "x", now.Add(-10*24*time.Hour))
	writeFileWithMtime(t, dir, "old.log.gz", "x", now.Add(-10*24*time.Hour))

	f := &CompressOldLogs{LogsDir: dir, Now: func() time.Time { return now }}
	result, _ := f.Run(context.Background())
	if result.Changed {
		t.Error("should not touch .txt or .log.gz")
	}
	if _, err := os.Stat(filepath.Join(dir, "old.txt")); err != nil {
		t.Errorf("old.txt missing: %v", err)
	}
}

func TestCompressOldLogs_NonExistentDir(t *testing.T) {
	f := &CompressOldLogs{LogsDir: "/nonexistent/xyz"}
	result, err := f.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if result.Changed {
		t.Error("non-existent dir should not be changed")
	}
}

func TestCompressOldLogs_EmptyDirNoop(t *testing.T) {
	dir := t.TempDir()
	f := &CompressOldLogs{LogsDir: dir}
	result, err := f.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if result.Changed {
		t.Error("empty dir should produce no changes")
	}
}

func TestCompressOldLogs_NilReceiver(t *testing.T) {
	var f *CompressOldLogs
	_, err := f.Run(context.Background())
	if err != nil {
		t.Errorf("nil receiver should be safe: %v", err)
	}
}

func TestCompressOldLogs_DefaultMaxAge(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	writeFileWithMtime(t, dir, "old.log", "data", now.Add(-8*24*time.Hour))
	f := &CompressOldLogs{LogsDir: dir, Now: func() time.Time { return now }} // MaxAge zero → default 7d
	result, _ := f.Run(context.Background())
	if !result.Changed {
		t.Error("default 7d threshold should apply")
	}
}
