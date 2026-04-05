package autofix

import (
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CompressOldLogsName is the canonical name used in configs and logs.
const CompressOldLogsName = "compress_old_logs"

// CompressOldLogs gzips .log files older than MaxAge found under the
// workspace logs directory. It is designed to run under the pulse
// watchdog as a periodic housekeeping fix, so its touch is narrow:
//
//   - only files ending in ".log" (not ".log.gz", not other extensions)
//   - only files with modification time older than MaxAge
//   - compressed output is written alongside as "<file>.log.gz"
//   - original is removed only after successful compression and fsync
//
// The fixer is idempotent: a second run finds no candidates (they are
// already gzipped) and reports Changed=false.
type CompressOldLogs struct {
	// LogsDir is an absolute path to the directory being scanned. It is
	// usually <workspace>/logs. Non-existent dirs are treated as empty.
	LogsDir string
	// MaxAge is the minimum file age required before compression. Zero
	// falls back to 7 days, matching the Phase 1 default.
	MaxAge time.Duration
	// Now is injectable for tests; real code uses time.Now.
	Now func() time.Time
}

func (c *CompressOldLogs) Name() string { return CompressOldLogsName }

func (c *CompressOldLogs) Run(ctx context.Context) (Result, error) {
	if c == nil {
		return Result{Name: CompressOldLogsName}, nil
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

	dir := strings.TrimSpace(c.LogsDir)
	if dir == "" {
		return Result{Name: CompressOldLogsName}, nil
	}
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{Name: CompressOldLogsName}, nil
		}
		return Result{Name: CompressOldLogsName}, err
	}
	if !info.IsDir() {
		return Result{Name: CompressOldLogsName}, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return Result{Name: CompressOldLogsName}, err
	}

	compressed := 0
	var compressedBytes int64
	var skipped []string
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return Result{Name: CompressOldLogsName, Changed: compressed > 0}, err
		}
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".log") {
			continue
		}
		path := filepath.Join(dir, name)
		fi, err := entry.Info()
		if err != nil {
			skipped = append(skipped, name)
			continue
		}
		if fi.ModTime().After(cutoff) {
			continue
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			continue
		}
		bytes, err := compressLogFile(path)
		if err != nil {
			skipped = append(skipped, name)
			continue
		}
		compressed++
		compressedBytes += bytes
	}

	result := Result{
		Name:    CompressOldLogsName,
		Changed: compressed > 0,
		Details: map[string]any{
			"compressed_files": compressed,
			"compressed_bytes": compressedBytes,
			"logs_dir":         dir,
			"max_age_seconds":  int64(maxAge.Seconds()),
		},
	}
	if compressed == 0 {
		result.Summary = "no log files older than threshold"
	} else {
		result.Summary = "compressed old log files"
	}
	if len(skipped) > 0 {
		result.Details["skipped"] = skipped
	}
	return result, nil
}

// compressLogFile gzips src to src+".gz" and, on success, removes src.
// Returns the original (uncompressed) size. On any error the original
// is preserved and the partial .gz is removed.
func compressLogFile(src string) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer in.Close()

	srcInfo, err := in.Stat()
	if err != nil {
		return 0, err
	}

	dst := src + ".gz"
	out, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	gz := gzip.NewWriter(out)
	if _, err := io.Copy(gz, in); err != nil {
		_ = gz.Close()
		_ = out.Close()
		_ = os.Remove(dst)
		return 0, err
	}
	if err := gz.Close(); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return 0, err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return 0, err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dst)
		return 0, err
	}
	if err := in.Close(); err != nil {
		_ = os.Remove(dst)
		return 0, err
	}
	if err := os.Remove(src); err != nil {
		// Compressed file exists but removal failed. We leak a .gz
		// duplicate — the next run will skip the .log file (it now
		// doesn't exist) so this is self-healing.
		return 0, err
	}
	return srcInfo.Size(), nil
}
