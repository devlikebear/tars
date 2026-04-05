package autofix

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CleanupStaleTmpName is the canonical name used in configs and logs.
const CleanupStaleTmpName = "cleanup_stale_tmp"

// CleanupStaleTmp removes plain files older than MaxAge from the
// workspace's temporary directories. It intentionally DOES NOT:
//
//   - follow symlinks (they are skipped; pulse must never chase links
//     out of workspace-controlled paths);
//   - delete directories (even empty ones — directory cleanup is a
//     separate concern and can remove user state accidentally);
//   - descend into subdirectories (recursion widens the blast radius
//     for a background agent; tmp layouts are expected to be flat);
//
// The fixer is idempotent: a second run in quick succession finds no
// candidates and reports Changed=false.
type CleanupStaleTmp struct {
	// Dirs is the list of absolute tmp paths to scan. Non-existent
	// entries are silently skipped. Typical values:
	//   <workspace>/tmp
	//   <workspace>/_shared/tmp
	Dirs []string
	// MaxAge is the minimum file age required before deletion. Zero
	// falls back to 7 days, matching the Phase 1 default.
	MaxAge time.Duration
	// Now is injectable for tests; real code uses time.Now.
	Now func() time.Time
}

func (c *CleanupStaleTmp) Name() string { return CleanupStaleTmpName }

func (c *CleanupStaleTmp) Run(ctx context.Context) (Result, error) {
	if c == nil {
		return Result{Name: CleanupStaleTmpName}, nil
	}
	now := c.Now
	if now == nil {
		now = time.Now
	}
	maxAge := c.MaxAge
	if maxAge <= 0 {
		maxAge = 7 * 24 * time.Hour
	}
	cutoff := now().Add(-maxAge)

	deleted := 0
	var deletedBytes int64
	scannedDirs := 0
	var skipped []string

	for _, dir := range c.Dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		scannedDirs++

		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return Result{
					Name:    CleanupStaleTmpName,
					Changed: deleted > 0,
					Details: map[string]any{
						"deleted_files": deleted,
						"deleted_bytes": deletedBytes,
						"scanned_dirs":  scannedDirs,
					},
				}, err
			}
			if entry.IsDir() {
				continue
			}
			fi, err := entry.Info()
			if err != nil {
				skipped = append(skipped, entry.Name())
				continue
			}
			if fi.Mode()&os.ModeSymlink != 0 {
				continue
			}
			if !fi.Mode().IsRegular() {
				continue
			}
			if fi.ModTime().After(cutoff) {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			if err := os.Remove(path); err != nil {
				skipped = append(skipped, entry.Name())
				continue
			}
			deleted++
			deletedBytes += fi.Size()
		}
	}

	result := Result{
		Name:    CleanupStaleTmpName,
		Changed: deleted > 0,
		Details: map[string]any{
			"deleted_files":   deleted,
			"deleted_bytes":   deletedBytes,
			"scanned_dirs":    scannedDirs,
			"max_age_seconds": int64(maxAge.Seconds()),
		},
	}
	if deleted == 0 {
		result.Summary = "no stale tmp files found"
	} else {
		result.Summary = "removed stale tmp files"
	}
	if len(skipped) > 0 {
		result.Details["skipped"] = skipped
	}
	return result, nil
}
