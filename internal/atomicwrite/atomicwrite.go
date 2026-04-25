// Package atomicwrite provides crash-safe file writes for TARS state
// files. Callers pass the full intended bytes; the helper writes a
// sibling temp file, fsyncs and closes it, then renames it over the
// destination. A crash before the rename leaves the previous file
// content untouched; a crash after means the new content is fully on
// disk. There is never a half-written file at the destination path.
//
// This is the canonical persistence helper for sessions.json,
// jobs.json, knowledge notes, the semantic index, and similar
// long-lived state. Callers that need extra behavior (chmod, Windows
// rename retry, JSON marshaling) should layer on top of Write rather
// than reinvent the temp+rename pattern.
package atomicwrite

import (
	"fmt"
	"os"
	"path/filepath"
)

// Write writes data atomically to path. The parent directory is
// created with 0o755 if missing. The created file has the default
// 0o600 permissions from os.CreateTemp; callers needing a specific
// mode should chmod the destination after Write returns, or use a
// helper that wraps this with the desired bits (see
// internal/tool/write_file.go for one such wrapper).
//
// Atomicity is provided by os.Rename, which is atomic on POSIX. On
// Windows os.Rename can fail when the destination already exists;
// callers targeting Windows should use the wrapper in
// internal/tool/write_file.go which carries a remove-then-rename
// fallback.
func Write(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("atomicwrite: mkdir %q: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("atomicwrite: create temp for %q: %w", path, err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("atomicwrite: write temp for %q: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("atomicwrite: sync temp for %q: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("atomicwrite: close temp for %q: %w", path, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("atomicwrite: rename %q -> %q: %w", tmpPath, path, err)
	}
	cleanup = false
	return nil
}
